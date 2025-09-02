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
	wranglecorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/util/retry"
)

const (
	retryTimes    = 10
	retryInterval = time.Second

	serviceNamespaceKey = prefix + "serviceNamespace"
	serviceNameKey      = prefix + "serviceName"
	clusterNameKey      = prefix + "cluster"

	maxNameLength = 63
	lenOfSuffix   = 8
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
			return nil, false, nil
		}
		return nil, false, err
	}

	if lb.Status.Address == "" {
		return nil, false, nil
	}

	return &service.Status.LoadBalancer, true, nil
}

func (l *LoadBalancerManager) GetLoadBalancerName(ctx context.Context, clusterName string, service *v1.Service) string {
	svc, err := l.getPrimaryService(service)
	if err != nil || svc == nil {
		svc = service
	}
	return loadBalancerName(clusterName, svc.Namespace, svc.Name, string(svc.UID))
}

// The name must be a valid [RFC 1035 label name](https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-label-names).
// If cluster name doesn't start with an alphabetic character, add "a" as prefix to make the name as compliant as possible with the RFC1035 standard.
// If the name doesn't meet the standard, the CURD actions with the name will fail and return an error.
func loadBalancerName(clusterName, serviceNamespace, serviceName, serviceUID string) string {
	if len(validation.IsDNS1035Label(clusterName)) > 0 {
		clusterName = "a" + clusterName
	}
	base := clusterName + "-" + serviceNamespace + "-" + serviceName + "-"
	digest := crc32.ChecksumIEEE([]byte(base + serviceUID))
	suffix := fmt.Sprintf("%08x", digest) // print in 8 width and pad with 0's

	// The name contains no more than 63 characters.
	if len(base) > maxNameLength-lenOfSuffix {
		base = base[:maxNameLength-lenOfSuffix]
	}

	return base + suffix
}

// if annotation "cloudprovider.harvesterhci.io/primary-service" is set, return the service that the annotation points to.
// if it's an invalid service, return error.
// if it's not a secondary service, return nil.
func (l *LoadBalancerManager) getPrimaryService(service *v1.Service) (*v1.Service, error) {
	primary, ok := service.Annotations[KeyPrimaryService]
	if !ok {
		return nil, nil
	}

	f := strings.SplitN(primary, "/", 2)
	if len(f) != 2 {
		return nil, fmt.Errorf("invalid service name %s", primary)
	}

	primarySvc, err := l.localSvcCache.Get(f[0], f[1])
	if err != nil {
		return nil, fmt.Errorf("get service %s failed: %w", primary, err)
	}

	if primarySvc.Annotations[KeyPrimaryService] != "" {
		return nil, fmt.Errorf("service %s is not a primary service", primary)
	}

	return primarySvc, nil
}

// EnsureLoadBalancer is to create/update a Harvester load balancer for the service
func (l *LoadBalancerManager) EnsureLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) (*v1.LoadBalancerStatus, error) {
	primarySvc, err := l.getPrimaryService(service)
	if err != nil {
		return nil, err
	}
	if primarySvc != nil {
		return l.ensureSecondaryLoadBalancer(clusterName, primarySvc, service)
	}

	return l.ensurePrimaryLoadBalancer(clusterName, service)
}

// ensurePrimaryLoadBalancer is to create/update a Harvester load balancer for the primary service
//  1. Create/update harvester load balancer.
//     If the service has an external IP set by kube-vip, update it into the load balancer.
//  2. Wait for the harvester load balancer to get the allocated IP address
//  3. Set the allocated IP address into the field spec.loadBalancerIP of the service.
//     The kube-vip will set the external IP according to the spec.loadBalancerIP.
func (l *LoadBalancerManager) ensurePrimaryLoadBalancer(clusterName string, service *v1.Service) (*v1.LoadBalancerStatus, error) {
	name := loadBalancerName(clusterName, service.Namespace, service.Name, string(service.UID))

	if err := l.createOrUpdateLoadBalancer(name, clusterName, service); err != nil {
		return nil, fmt.Errorf("create or update lb %s/%s failed, error: %w", l.namespace, name, err)
	}

	if err := l.updatePrimaryServiceLoadBalancerIP(name, service); err != nil {
		// Delete the load balancer if the service is not updated successfully.
		// If ensure service failed, the load balancer could not be deleted even though the service is deleted.
		if err := l.deleteLoadBalancer(clusterName, service); err != nil {
			return nil, fmt.Errorf("delete lb %s/%s failed, error: %w", l.namespace, name, err)
		}
		return nil, fmt.Errorf("update load balancer IP of service %s/%s failed, error: %w", service.Namespace, service.Name, err)
	}

	return &service.Status.LoadBalancer, nil
}

// ensureSecondaryLoadBalancer is to create/update a Harvester load balancer for the secondary service
func (l *LoadBalancerManager) ensureSecondaryLoadBalancer(clusterName string, primary, secondary *v1.Service) (*v1.LoadBalancerStatus, error) {
	if len(primary.Status.LoadBalancer.Ingress) == 0 {
		return nil, fmt.Errorf("primary service %s/%s has no ingress IP", primary.Namespace, primary.Name)
	}
	// check if the port of the secondary service overlaps with the primary service and other secondary services
	if err := l.checkPortOverlap(primary, secondary); err != nil {
		return nil, fmt.Errorf("check port overlap failed, primary service: %s/%s, secondary service: %s/%s, error: %w",
			primary.Namespace, primary.Name, secondary.Namespace, secondary.Name, err)
	}
	// delete the original load balancer if existing
	if err := l.deleteLoadBalancer(clusterName, secondary); err != nil {
		return nil, err
	}
	// update secondary service load balancer IP
	return &secondary.Status.LoadBalancer, l.updateSecondaryServiceLoadBalancerIP(primary.Status.LoadBalancer.Ingress[0].IP, primary, secondary)
}

func (l *LoadBalancerManager) UpdateLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) error {
	if _, err := l.EnsureLoadBalancer(ctx, clusterName, service, nodes); err != nil {
		return fmt.Errorf("update load balancer failed, error: %w", err)
	}
	return nil
}

func (l *LoadBalancerManager) EnsureLoadBalancerDeleted(ctx context.Context, clusterName string, service *v1.Service) error {
	primarySvc, err := l.getPrimaryService(service)
	if err != nil {
		return err
	}
	// do nothing for the secondary service
	if primarySvc != nil {
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

// only retry when conflict happens
func (l *LoadBalancerManager) retryUpdateService(service *v1.Service, serviceType, ip, primaryLabel string, updateServiceObject func(serviceCopy *v1.Service, ip, primaryLabel string)) error {
	retryFunc := func() error {
		newService, err := l.localSvcCache.Get(service.Namespace, service.Name)
		if err != nil {
			return fmt.Errorf("failed to get %s service %s/%s to update ip %s, error: %w", serviceType, service.Namespace, service.Name, ip, err)
		}
		// skip updating if UID has changed, it means service has been recreated by other controller
		if newService.UID != service.UID {
			return fmt.Errorf("failed to get %s service %s/%s to update ip %s, UID %v has changed to %v", serviceType, service.Namespace, service.Name, ip, service.UID, newService.UID)
		}
		serviceCopy := newService.DeepCopy()
		updateServiceObject(serviceCopy, ip, primaryLabel)
		_, err = l.localSvcClient.Update(serviceCopy)
		return err
	}

	err := retry.RetryOnConflict(retry.DefaultBackoff, retryFunc)
	if err != nil {
		return fmt.Errorf("failed to update %s service %s/%s with ip %s after retry, last error: %w", serviceType, service.Namespace, service.Name, ip, err)
	}
	return nil
}

func isPrimaryServiceUpdatedWithIP(service *v1.Service, lbAddress, ip string) bool {
	return service.Annotations != nil && service.Annotations[KeyKubevipLoadBalancerIP] == ip && lbAddress == ip && service.Labels != nil && service.Labels[KeyPrimaryService] == ""
}

func (l *LoadBalancerManager) updatePrimaryServiceLoadBalancerIP(lbName string, service *v1.Service) error {
	object, ip, err := waitForIP(func() (runtime.Object, string, error) {
		lb, err := l.lbClient.Get(l.namespace, lbName, metav1.GetOptions{})
		if err != nil {
			return nil, "", fmt.Errorf("fail to get lb %w", err)
		}
		if lb.Status.AllocatedAddress.IP == "" {
			// when Ready condition is false, the message has useful information
			return nil, "", fmt.Errorf("ip is not allocated, mode: %s, message: %s", string(lb.Spec.IPAM), lbv1.LoadBalancerReady.GetMessage(lb))
		}
		return lb, lb.Status.AllocatedAddress.IP, nil
	})
	if err != nil {
		return err
	}

	lb := object.(*lbv1.LoadBalancer)
	if isPrimaryServiceUpdatedWithIP(service, lb.Status.Address, ip) {
		return nil
	}

	updatePrimaryServiceObject := func(serviceCopy *v1.Service, ip, primaryLabel string) {
		if serviceCopy.Labels != nil && serviceCopy.Labels[KeyPrimaryService] != "" {
			serviceCopy.Labels[KeyPrimaryService] = ""
		}
		if serviceCopy.Annotations == nil {
			serviceCopy.Annotations = make(map[string]string)
		}
		serviceCopy.Annotations[KeyKubevipLoadBalancerIP] = ip
	}

	// the above waitForIP takes time, it has high chance to hit the `IsConflict` error like
	// "Operation cannot be fulfilled on services \"lb2\": the object has been modified; please apply your changes to the latest version and try again"
	return l.retryUpdateService(service, "primary", ip, "", updatePrimaryServiceObject)
}

func isSecondaryServiceUpdatedWithPrimary(secondary *v1.Service, ip, labelValue string) bool {
	return secondary.Annotations != nil && secondary.Annotations[KeyKubevipLoadBalancerIP] == ip && secondary.Annotations[KeyIPAM] == "" && secondary.Labels != nil && secondary.Labels[KeyPrimaryService] == labelValue
}

func (l *LoadBalancerManager) updateSecondaryServiceLoadBalancerIP(ip string, primary, secondary *v1.Service) error {
	labelValue := primaryServiceLabelValue(primary)
	if isSecondaryServiceUpdatedWithPrimary(secondary, ip, labelValue) {
		return nil
	}

	updateSecondaryServiceObject := func(secondaryCopy *v1.Service, ip, primaryLabel string) {
		if secondaryCopy.Labels == nil {
			secondaryCopy.Labels = make(map[string]string)
		}
		if secondaryCopy.Annotations == nil {
			secondaryCopy.Annotations = make(map[string]string)
		}
		// add a label for easy filtering
		secondaryCopy.Labels[KeyPrimaryService] = primaryLabel
		// update the annotations and kube-vip will update the service status load balancer
		secondaryCopy.Annotations[KeyKubevipLoadBalancerIP] = ip
		delete(secondaryCopy.Annotations, KeyIPAM)
	}

	return l.retryUpdateService(secondary, "secondary", ip, labelValue, updateSecondaryServiceObject)
}

func (l *LoadBalancerManager) deleteLoadBalancer(clusterName string, service *v1.Service) error {
	// check if there are other services using the same load balancer
	if err := l.checkSecondaryServicesBeforeDeleted(service); err != nil {
		return fmt.Errorf("could not delete load balancer for service %s/%s: %w", service.Namespace, service.Name, err)
	}

	name := loadBalancerName(clusterName, service.Namespace, service.Name, string(service.UID))
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
	var (
		err    error
		object runtime.Object
		ip     string
	)
	for i := 0; i < retryTimes; i++ {
		object, ip, err = callback()
		if err == nil {
			return object, ip, nil
		}
		time.Sleep(retryInterval)
	}

	return nil, "", fmt.Errorf("timeout waiting for IP address, last error:%w", err)
}

func (l *LoadBalancerManager) checkPortOverlap(primary, secondary *v1.Service) error {
	portMap := make(map[int32]bool)
	for _, port := range secondary.Spec.Ports {
		portMap[port.Port] = true
	}
	// TODO: Listing services filtered by primary service label could cause concurrency problem because the primary service
	// label is added after this function is called. Some eligible services may not be listed.
	svcs, err := l.localSvcCache.List(metav1.NamespaceAll, labels.Set(map[string]string{
		KeyPrimaryService: primaryServiceLabelValue(primary),
	}).AsSelector())
	if err != nil {
		return fmt.Errorf("list service failed: %w", err)
	}
	svcs = append(svcs, primary)

	for _, svc := range svcs {
		// ignore itself
		if svc.UID == secondary.UID {
			continue
		}
		for _, port := range svc.Spec.Ports {
			if portMap[port.Port] {
				return fmt.Errorf("port %d has been used in service %s/%s", port.Port, svc.Namespace, svc.Name)
			}
		}
	}

	return nil
}

func (l *LoadBalancerManager) checkSecondaryServicesBeforeDeleted(primary *v1.Service) error {
	name := primary.Namespace + "/" + primary.Name

	// Listing services filtered by primary service label could cause concurrency problem because there may be secondary
	// services added after this function is called and before the service is deleted.
	svcs, err := l.localSvcCache.List(metav1.NamespaceAll, labels.Set(map[string]string{
		KeyPrimaryService: primaryServiceLabelValue(primary),
	}).AsSelector())
	if err != nil {
		return err
	}

	if len(svcs) > 0 {
		svcNames := make([]string, 0, len(svcs))
		for _, svc := range svcs {
			svcNames = append(svcNames, svc.Namespace+"/"+svc.Name)
		}
		return fmt.Errorf("service %s is still used by other services %v", name, svcNames)
	}

	return nil
}

func primaryServiceLabelValue(svc *v1.Service) string {
	return svc.Namespace + "." + svc.Name
}
