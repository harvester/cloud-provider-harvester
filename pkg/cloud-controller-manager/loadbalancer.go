package ccm

import (
	"context"
	"fmt"
	"hash/crc32"
	"time"

	lbv1 "github.com/harvester/harvester-load-balancer/pkg/apis/loadbalancer.harvesterhci.io/v1beta1"
	pkgctllb "github.com/harvester/harvester-load-balancer/pkg/controller/loadbalancer"
	ctllb "github.com/harvester/harvester-load-balancer/pkg/generated/controllers/loadbalancer.harvesterhci.io"
	ctllbv1 "github.com/harvester/harvester-load-balancer/pkg/generated/controllers/loadbalancer.harvesterhci.io/v1beta1"
	ctlcore "github.com/rancher/wrangler/pkg/generated/controllers/core"
	wranglecorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/rest"
	cloudprovider "k8s.io/cloud-provider"
)

const (
	defaultWaitIPTimeout = time.Second * 5
	serviceNamespaceKey  = prefix + "serviceNamespace"
	serviceNameKey       = prefix + "serviceName"
	clusterNameKey       = prefix + "cluster"

	maxNameLength = 63
	lenOfSuffix   = 8
)

type LoadBalancerManager struct {
	lbClient       ctllbv1.LoadBalancerClient
	localSvcClient wranglecorev1.ServiceClient
	namespace      string
}

func newLoadBalancerManager(cfg, localCfg *rest.Config, namespace string) (cloudprovider.LoadBalancer, error) {
	lbFactory, err := ctllb.NewFactoryFromConfig(cfg)
	if err != nil {
		return nil, err
	}

	coreFactory, err := ctlcore.NewFactoryFromConfig(localCfg)
	if err != nil {
		return nil, err
	}

	return &LoadBalancerManager{
		lbClient:       lbFactory.Loadbalancer().V1beta1().LoadBalancer(),
		localSvcClient: coreFactory.Core().V1().Service(),
		namespace:      namespace,
	}, nil
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
			return nil, false, nil
		} else {
			return nil, false, err
		}
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
	return loadBalancerName(clusterName, service.Namespace, service.Name, string(service.UID))
}

// The name must be a valid [RFC 1035 label name](https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-label-names).
// If cluster name doesn't start with an alphabetic character, add "a" as prefix to make the name as compliant as possible with the RFC1035 standard.
// If the name doesn't meet the standard, the CURD actions with the name will fail and return an error.
func loadBalancerName(clusterName, serviceNamespace, serviceName, serviceUid string) string {
	if len(validation.IsDNS1035Label(clusterName)) > 0 {
		clusterName = "a" + clusterName
	}
	base := clusterName + "-" + serviceNamespace + "-" + serviceName + "-"
	digest := crc32.ChecksumIEEE([]byte(base + serviceUid))
	suffix := fmt.Sprintf("%08x", digest) // print in 8 width and pad with 0's

	// The name contains no more than 63 characters.
	if len(base) > maxNameLength-lenOfSuffix {
		base = base[:maxNameLength-lenOfSuffix]
	}

	return base + suffix
}

// EnsureLoadBalancer is to create/update a Harvester load balancer for the service
//  1. Create/update harvester load balancer.
//     If the service has an external IP set by kube-vip, update it into the load balancer.
//  2. Wait for the harvester load balancer to get the allocated IP address
//  3. Set the allocated IP address into the field spec.loadBalancerIP of the service.
//     The kube-vip will set the external IP according to the spec.loadBalancerIP.
func (l *LoadBalancerManager) EnsureLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) (*v1.LoadBalancerStatus, error) {
	name := l.GetLoadBalancerName(ctx, clusterName, service)

	if err := l.createOrUpdateLoadBalancer(name, clusterName, service); err != nil {
		return nil, fmt.Errorf("create or update lb %s/%s failed, error: %w", l.namespace, name, err)
	}

	if err := l.updateServiceLoadBalancerIP(name, service); err != nil {
		return nil, fmt.Errorf("update load balancer IP of service %s/%s failed, error: %w", service.Namespace, service.Name, err)
	}

	return &service.Status.LoadBalancer, nil
}

func (l *LoadBalancerManager) UpdateLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) error {
	name := l.GetLoadBalancerName(ctx, clusterName, service)
	lb, err := l.lbClient.Get(l.namespace, name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	newLB := l.constructLB(lb, service, name, clusterName)
	_, err = l.lbClient.Update(newLB)
	if err != nil {
		return fmt.Errorf("update lb %s/%s failed, error: %w", l.namespace, name, err)
	}

	return nil
}

func (l *LoadBalancerManager) EnsureLoadBalancerDeleted(ctx context.Context, clusterName string, service *v1.Service) error {
	name := l.GetLoadBalancerName(ctx, clusterName, service)
	_, err := l.lbClient.Get(l.namespace, name, metav1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	if err == nil {
		return l.lbClient.Delete(l.namespace, name, &metav1.DeleteOptions{})
	}

	return nil
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
