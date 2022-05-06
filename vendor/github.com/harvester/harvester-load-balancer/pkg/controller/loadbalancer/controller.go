package loadbalancer

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	ctldiscoveryv1 "github.com/harvester/harvester-load-balancer/pkg/generated/controllers/discovery.k8s.io/v1"
	"github.com/harvester/harvester-load-balancer/pkg/lb/servicelb"
	"github.com/harvester/harvester-load-balancer/pkg/utils"
	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	ctlCorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/labels"

	lb "github.com/harvester/harvester-load-balancer/pkg/apis/loadbalancer.harvesterhci.io"
	lbv1 "github.com/harvester/harvester-load-balancer/pkg/apis/loadbalancer.harvesterhci.io/v1alpha1"
	"github.com/harvester/harvester-load-balancer/pkg/config"
	ctllbv1 "github.com/harvester/harvester-load-balancer/pkg/generated/controllers/loadbalancer.harvesterhci.io/v1alpha1"
	"github.com/harvester/harvester-load-balancer/pkg/ipam"
	lbpkg "github.com/harvester/harvester-load-balancer/pkg/lb"
)

const (
	controllerName = "harvester-lb-controller"

	AnnotationKeyNetwork   = lb.GroupName + "/network"
	AnnotationKeyProject   = lb.GroupName + "/project"
	AnnotationKeyNamespace = lb.GroupName + "/namespace"
	AnnotationKeyCluster   = lb.GroupName + "/cluster"

	defaultWaitIPTimeout = time.Second * 5
)

type Handler struct {
	lbClient            ctllbv1.LoadBalancerClient
	ipPoolCache         ctllbv1.IPPoolCache
	nadCache            ctlcniv1.NetworkAttachmentDefinitionCache
	serviceClient       ctlCorev1.ServiceClient
	serviceCache        ctlCorev1.ServiceCache
	endpointSliceClient ctldiscoveryv1.EndpointSliceClient
	endpointSliceCache  ctldiscoveryv1.EndpointSliceCache

	allocatorMap *ipam.SafeAllocatorMap

	lbManager lbpkg.Manager
}

func Register(ctx context.Context, management *config.Management) error {
	lbs := management.LbFactory.Loadbalancer().V1alpha1().LoadBalancer()
	pools := management.LbFactory.Loadbalancer().V1alpha1().IPPool()
	nads := management.CniFactory.K8s().V1().NetworkAttachmentDefinition()
	services := management.CoreFactory.Core().V1().Service()
	endpointSlices := management.DiscoveryFactory.Discovery().V1().EndpointSlice()

	handler := &Handler{
		lbClient:            lbs,
		ipPoolCache:         pools.Cache(),
		nadCache:            nads.Cache(),
		serviceClient:       services,
		serviceCache:        services.Cache(),
		endpointSliceClient: endpointSlices,
		endpointSliceCache:  endpointSlices.Cache(),

		allocatorMap: management.AllocatorMap,
	}

	handler.lbManager = servicelb.NewManager(ctx, handler.serviceClient, handler.serviceCache, handler.endpointSliceClient, handler.endpointSliceCache)

	lbs.OnChange(ctx, controllerName, handler.OnChange)
	lbs.OnRemove(ctx, controllerName, handler.OnRemove)

	return nil
}

func (h *Handler) OnChange(key string, lb *lbv1.LoadBalancer) (*lbv1.LoadBalancer, error) {
	if lb == nil || lb.DeletionTimestamp != nil {
		return nil, nil
	}
	logrus.Infof("load balancer configuration %s has been changed, spec: %+v", lb.Name, lb.Spec)

	lbCopy := lb.DeepCopy()
	allocatedAddress, err := h.allocateIP(lb)
	if err != nil {
		return nil, err
	}
	if allocatedAddress != nil {
		lbCopy.Status.AllocatedAddress = *allocatedAddress
	}

	if lb.Spec.WorkloadType == "" || lb.Spec.WorkloadType == lbv1.VM {
		if err := h.lbManager.EnsureLoadBalancer(lbCopy); err != nil {
			return nil, err
		}
		ip, err := h.waitServiceExternalIP(lb.Namespace, lb.Name)
		if err != nil {
			return nil, err
		}
		lbCopy.Status.Address = ip
	}

	if lbCopy != nil {
		lbv1.LoadBalancerReady.True(lbCopy)
		lbv1.LoadBalancerReady.Message(lbCopy, "")
		return h.lbClient.Update(lbCopy)
	}

	return lb, nil
}

func (h *Handler) OnRemove(key string, lb *lbv1.LoadBalancer) (*lbv1.LoadBalancer, error) {
	if lb == nil {
		return nil, nil
	}

	logrus.Infof("load balancer configuration %s has been deleted", lb.Name)

	if lb.Spec.IPAM == lbv1.Pool && lb.Status.AllocatedAddress.IPPool != "" {
		if err := h.releaseIP(lb); err != nil {
			return nil, err
		}
	}

	if lb.Spec.WorkloadType == "" || lb.Spec.WorkloadType == lbv1.VM {
		h.lbManager.DeleteLoadBalancer(lb)
	}

	return lb, nil
}

func (h *Handler) allocateIP(lb *lbv1.LoadBalancer) (*lbv1.AllocatedAddress, error) {
	allocated := lb.Status.AllocatedAddress
	var err error

	if lb.Spec.IPAM == lbv1.DHCP {
		// release the IP if the lb has applied an IP
		if allocated.IPPool != "" {
			if err = h.releaseIP(lb); err != nil {
				return nil, err
			}
		}
		if allocated.IP != ipam.Address4AskDHCP {
			return &lbv1.AllocatedAddress{
				IP: ipam.Address4AskDHCP,
			}, nil
		}
		return nil, nil
	}

	// If lb.Spec.IPAM equals pool
	pool := lb.Spec.IPPool
	if pool == "" {
		pool, err = h.selectIPPool(lb)
		if err != nil {
			return nil, fmt.Errorf("fail to select the pool for lb %s/%s", lb.Namespace, lb.Name)
		}
	}
	// release the IP from other IP pool
	if allocated.IPPool != "" && allocated.IPPool != pool {
		if err := h.releaseIP(lb); err != nil {
			return nil, err
		}
	}
	if allocated.IPPool != pool {
		return h.requestIP(lb, pool)
	}

	return nil, nil
}

func (h *Handler) requestIP(lb *lbv1.LoadBalancer, pool string) (*lbv1.AllocatedAddress, error) {
	// get allocator
	allocator := h.allocatorMap.Get(pool)
	if allocator == nil {
		return nil, fmt.Errorf("could not get the allocator %s", pool)
	}
	// get IP
	ipConfig, err := allocator.Get(fmt.Sprintf("%s/%s", lb.Namespace, lb.Name))
	if err != nil {
		return nil, err
	}

	return &lbv1.AllocatedAddress{
		IPPool:  pool,
		IP:      ipConfig.Address.IP.String(),
		Mask:    net.IP(ipConfig.Address.Mask).String(),
		Gateway: ipConfig.Gateway.String(),
	}, err
}

func (h *Handler) selectIPPool(lb *lbv1.LoadBalancer) (string, error) {
	// get the conditions
	con := ipam.Conditions{}
	vid, err := utils.GetVid(lb.Annotations[AnnotationKeyNetwork], h.nadCache)
	if err != nil {
		return "", err
	}
	con.HardCond = ipam.HardConditions{VLAN: strconv.Itoa(vid)}
	con.ElasticCond = ipam.ElasticConditions{
		Project:   lb.Annotations[AnnotationKeyProject],
		Namespace: lb.Annotations[AnnotationKeyNamespace],
		Cluster:   lb.Annotations[AnnotationKeyCluster],
	}

	// list all ip pools
	pools, err := h.ipPoolCache.List(labels.Everything())
	if err != nil {
		return "", err
	}

	// select an ip pool
	pool, err := ipam.NewSelector(pools, con).Select()
	if err != nil {
		return "", err
	}

	return pool.Name, nil
}

func (h *Handler) releaseIP(lb *lbv1.LoadBalancer) error {
	a := h.allocatorMap.Get(lb.Status.AllocatedAddress.IPPool)
	if a == nil {
		return fmt.Errorf("could not get the allocator %s", lb.Status.AllocatedAddress.IPPool)
	}
	return a.Release(fmt.Sprintf("%s/%s", lb.Namespace, lb.Name), "")
}

func (h *Handler) waitServiceExternalIP(namespace, name string) (string, error) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	tick := ticker.C

	for {
		select {
		case <-time.After(defaultWaitIPTimeout):
			return "", fmt.Errorf("wait IP timeout")
		case <-tick:
			svc, err := h.serviceCache.Get(namespace, name)
			if err != nil {
				continue
			}
			if len(svc.Status.LoadBalancer.Ingress) > 0 {
				return svc.Status.LoadBalancer.Ingress[0].IP, nil
			}
		}
	}
}
