package ccm

import (
	"context"
	"fmt"
	"hash/crc32"
	"strings"
	"time"

	lbv1 "github.com/harvester/harvester-load-balancer/pkg/apis/loadbalancer.harvesterhci.io/v1beta1"
	pkgctllb "github.com/harvester/harvester-load-balancer/pkg/controller/loadbalancer"
	ctllbv1 "github.com/harvester/harvester-load-balancer/pkg/generated/controllers/loadbalancer.harvesterhci.io/v1beta1"
	wranglecorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
)

const (
	defaultWaitIPTimeout = time.Second * 5
	serviceNamespaceKey  = prefix + "serviceNamespace"
	serviceNameKey       = prefix + "serviceName"
	clusterNameKey       = prefix + "cluster"

	maxNameLength = 63
)

// Primary service is the load balancer service which will be used to create the load balancer.
// Secondary service is the load balancer service which will share the load balancer created by the primary service. It has the annotation "cloudprovider.harvesterhci.io/primary-service" to specify the primary service.

type LoadBalancerManager struct {
	lbClient       ctllbv1.LoadBalancerClient
	localSvcClient wranglecorev1.ServiceClient
	localSvcCache  wranglecorev1.ServiceCache
	namespace      string
}

func (l *LoadBalancerManager) GetLoadBalancer(ctx context.Context, clusterName string, service *v1.Service) (status *v1.LoadBalancerStatus, exists bool, err error) {
	name := l.GetLoadBalancerName(ctx, clusterName, service)
	// If using loadbalancer cache here, the cloud provider will need a serviceAccount binding with a clusterrole which
	// includes the privilege to list and watch the load balancers in all namespaces, whereas the client only needs to
	// be allowed to get the load balancer in the specified namespace. Following the principle of least privilege, we
	// choose the client instead of the cache to get the load balancer.
	lb, err := l.lbClient.Get(l.namespace, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Infof("name: %s", name)
			return nil, false, nil
		} else {
			return nil, false, err
		}
	}

	if lb.Status.Address == "" {
		return nil, false, nil
	}

	return &v1.LoadBalancerStatus{
		Ingress: []v1.LoadBalancerIngress{
			{
				IP: lb.Status.Address,
			},
		},
	}, true, nil
}

func (l *LoadBalancerManager) GetLoadBalancerName(ctx context.Context, clusterName string, service *v1.Service) string {
	svc := l.getPrimaryService(service)
	return l.getLoadBalancerName(clusterName, svc)
}

func (l *LoadBalancerManager) getLoadBalancerName(clusterName string, svc *v1.Service) string {
	name := clusterName + "-" + svc.Namespace + "-" + svc.Name + "-"

	digest := crc32.ChecksumIEEE([]byte(name + string(svc.UID)))
	suffix := fmt.Sprintf("%08x", digest) // print in 8 width and pad with 0's
	name += suffix

	// The name of a Service object must be a valid [RFC 1035 label name](https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-label-names)
	// The name contains no more than 63 characters
	if len(name) > maxNameLength {
		name = name[:maxNameLength]
	}

	return name
}

// if annotation "cloudprovider.harvesterhci.io/primary-service" is set, return the service that the annotation points to.
// if it's an invalid service, return the original service.
func (l *LoadBalancerManager) getPrimaryService(service *v1.Service) *v1.Service {
	primary, ok := service.Annotations[KeyPrimaryService]
	if !ok {
		return service
	}

	f := strings.SplitN(primary, "/", 2)
	if len(f) != 2 {
		klog.Errorf("invalid service name %s", primary)
		return service
	}

	primarySvc, err := l.localSvcCache.Get(f[0], f[1])
	if err != nil {
		klog.Errorf("get service %s/%s failed: %v", f[0], f[1], err)
		return service
	}

	if primarySvc.Annotations[KeyPrimaryService] != "" {
		klog.Errorf("service %s is not a secondary service", primary)
		return service
	}

	return primarySvc
}

// EnsureLoadBalancer is to create/update a Harvester load balancer for the service
func (l *LoadBalancerManager) EnsureLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) (*v1.LoadBalancerStatus, error) {
	primarySvc := l.getPrimaryService(service)
	if primarySvc != service {
		return l.ensureSecondaryLoadBalancer(ctx, clusterName, primarySvc, service)
	}

	return l.ensurePrimaryLoadBalancer(ctx, clusterName, service)
}

// ensurePrimaryLoadBalancer is to create/update a Harvester load balancer for the primary service
//  1. Create/update harvester load balancer.
//     If the service has an external IP set by kube-vip, update it into the load balancer.
//  2. Wait for the harvester load balancer to get the allocated IP address
//  3. Set the allocated IP address into the field spec.loadBalancerIP of the service.
//     The kube-vip will set the external IP according to the spec.loadBalancerIP.
func (l *LoadBalancerManager) ensurePrimaryLoadBalancer(ctx context.Context, clusterName string, service *v1.Service) (*v1.LoadBalancerStatus, error) {
	name := l.getLoadBalancerName(clusterName, service)

	if err := l.createOrUpdateLoadBalancer(name, clusterName, service); err != nil {
		return nil, fmt.Errorf("create or update lb %s/%s failed, error: %w", l.namespace, name, err)
	}

	if err := l.updateServiceLoadBalancerIP(name, service); err != nil {
		return nil, fmt.Errorf("update load balancer IP of service %s/%s failed, error: %w", service.Namespace, service.Name, err)
	}

	return &service.Status.LoadBalancer, nil
}

// ensureSecondaryLoadBalancer is to create/update a Harvester load balancer for the secondary service
func (l *LoadBalancerManager) ensureSecondaryLoadBalancer(ctx context.Context, clusterName string, primary, secondary *v1.Service) (*v1.LoadBalancerStatus, error) {
	// Delete the original load balancer if existing
	if err := l.deleteLoadBalancer(clusterName, secondary); err != nil {
		return nil, err
	}
	// Get the load balancer of the primary service
	status, existing, err := l.GetLoadBalancer(ctx, clusterName, primary)
	if err != nil {
		return nil, err
	}
	if !existing {
		return nil, fmt.Errorf("load balancer for service %s/%s not found", primary.Namespace, primary.Name)
	}
	// check if the port of the secondary service overlaps with the primary service and other secondary services
	if err := l.checkPortOverlap(primary, secondary); err != nil {
		return nil, fmt.Errorf("check port overlap failed, primary service: %s/%s, secondary service: %s/%s, error: %w",
			primary.Namespace, primary.Name, secondary.Namespace, secondary.Name, err)
	}

	return status, nil
}

func (l *LoadBalancerManager) UpdateLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) error {
	if _, err := l.EnsureLoadBalancer(ctx, clusterName, service, nodes); err != nil {
		return fmt.Errorf("update load balancer failed, error: %w", err)
	}
	return nil
}

func (l *LoadBalancerManager) EnsureLoadBalancerDeleted(ctx context.Context, clusterName string, service *v1.Service) error {
	primarySvc := l.getPrimaryService(service)
	// do nothing for the secondary service
	if primarySvc != service {
		return nil
	}

	return l.deleteLoadBalancer(clusterName, service)
}

func (l *LoadBalancerManager) createOrUpdateLoadBalancer(name, clusterName string, service *v1.Service) error {
	lb, err := l.lbClient.Get(l.namespace, name, metav1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	newLB := l.constructLB(lb, service, name, clusterName)
	if errors.IsNotFound(err) {
		_, err = l.lbClient.Create(newLB)
	} else {
		_, err = l.lbClient.Update(newLB)
	}

	return err
}

func (l *LoadBalancerManager) constructLB(oldLB *lbv1.LoadBalancer, service *v1.Service, name, clusterName string) *lbv1.LoadBalancer {
	var lb *lbv1.LoadBalancer

	// If the error returned by Get Interface is ErrNotFound, the returned lb would not be nil, but the name of the lb is empty.
	if oldLB == nil || oldLB.Name == "" {
		lb = &lbv1.LoadBalancer{
			TypeMeta: metav1.TypeMeta{
				APIVersion: lbv1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: l.namespace,
				Name:      name,
			},
		}
	} else {
		lb = oldLB.DeepCopy()
	}

	if lb.Annotations == nil {
		lb.Annotations = make(map[string]string)
	}
	lb.Annotations[pkgctllb.AnnotationKeyNetwork] = service.Annotations[KeyNetwork]
	lb.Annotations[pkgctllb.AnnotationKeyProject] = service.Annotations[KeyProject]
	lb.Annotations[pkgctllb.AnnotationKeyNamespace] = service.Annotations[KeyNamespace]
	lb.Annotations[pkgctllb.AnnotationKeyCluster] = clusterName

	if lb.Labels == nil {
		lb.Labels = make(map[string]string)
	}
	lb.Labels[clusterNameKey] = clusterName
	lb.Labels[serviceNamespaceKey] = service.Namespace
	lb.Labels[serviceNameKey] = service.Name

	ipam := lbv1.Pool
	if ipamStr, ok := service.Annotations[KeyIPAM]; ok {
		ipam = lbv1.IPAM(ipamStr)
	}
	lb.Spec.IPAM = ipam
	lb.Spec.WorkloadType = lbv1.Cluster

	if len(service.Status.LoadBalancer.Ingress) > 0 {
		lb.Status.Address = service.Status.LoadBalancer.Ingress[0].IP
	}

	return lb
}

func (l *LoadBalancerManager) updateServiceLoadBalancerIP(lbName string, service *v1.Service) error {
	object, ip, err := waitForIP(func() (runtime.Object, string, error) {
		lb, err := l.lbClient.Get(l.namespace, lbName, metav1.GetOptions{})
		if err != nil || lb.Status.AllocatedAddress.IP == "" {
			return nil, "", fmt.Errorf("could not get allocated IP address")
		}
		return lb, lb.Status.AllocatedAddress.IP, nil
	})
	if err != nil {
		return err
	}

	lb := object.(*lbv1.LoadBalancer)
	if service.Spec.LoadBalancerIP == ip && lb.Status.Address == ip {
		return nil
	}
	serviceCopy := service.DeepCopy()
	serviceCopy.Spec.LoadBalancerIP = ip
	if _, err := l.localSvcClient.Update(serviceCopy); err != nil {
		return err
	}

	return nil
}

func (l *LoadBalancerManager) deleteLoadBalancer(clusterName string, service *v1.Service) error {
	name := l.getLoadBalancerName(clusterName, service)
	_, err := l.lbClient.Get(l.namespace, name, metav1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	if err == nil {
		return l.lbClient.Delete(l.namespace, name, &metav1.DeleteOptions{})
	}

	return nil
}

func waitForIP(callback func() (runtime.Object, string, error)) (runtime.Object, string, error) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	tick := ticker.C

	for {
		select {
		case <-time.After(defaultWaitIPTimeout):
			return nil, "", fmt.Errorf("wait IP timeout")
		case <-tick:
			object, ip, err := callback()
			if err != nil {
				continue
			}
			return object, ip, nil
		}
	}
}

func (l *LoadBalancerManager) checkPortOverlap(primary, secondary *v1.Service) error {
	if len(primary.Status.LoadBalancer.Ingress) == 0 {
		return fmt.Errorf("primary service %s/%s has no ingress IP", primary.Namespace, primary.Name)
	}

	svcs, err := l.localSvcCache.List(metav1.NamespaceAll, labels.NewSelector())
	if err != nil {
		return fmt.Errorf("list service failed: %w", err)
	}
	// port map is used to check if the port is already used by the primary service and its secondary services
	portMap := make(map[int32]string)
	for _, svc := range svcs {
		if svc.UID == secondary.UID || len(svc.Status.LoadBalancer.Ingress) == 0 || svc.Status.LoadBalancer.Ingress[0].IP != primary.Status.LoadBalancer.Ingress[0].IP {
			continue
		}
		for _, port := range svc.Spec.Ports {
			portMap[port.Port] = svc.Namespace + "/" + svc.Name
		}
	}

	for _, port := range secondary.Spec.Ports {
		if name, ok := portMap[port.Port]; ok {
			return fmt.Errorf("port %d is already used by service %s", port.Port, name)
		}
	}

	return nil
}
