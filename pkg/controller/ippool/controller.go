package ippool

import (
	"context"
	"fmt"
	"sync"

	lbv1beta1 "github.com/harvester/harvester-load-balancer/pkg/apis/loadbalancer.harvesterhci.io/v1beta1"
	ctllbv1beta1 "github.com/harvester/harvester-load-balancer/pkg/generated/controllers/loadbalancer.harvesterhci.io/v1beta1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	wranglecorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ccmutil "github.com/harvester/harvester-cloud-provider/pkg/util"
)

const controllerName = "harvester-cloudprovider-ippool"

// Register starts watching IPPool resources on the Harvester cluster and keeps
// the cloudprovider.harvesterhci.io/ippool-network-mapping annotation on every
// guest-cluster Node up to date.
func Register(
	ctx context.Context,
	ipPools ctllbv1beta1.IPPoolController,
	nodes wranglecorev1.NodeController,
	vmiCache ctlkubevirtv1.VirtualMachineInstanceCache,
	nodeToVMName *sync.Map,
	namespace string,
	clusterName string,
) {
	h := &Handler{
		ipPools:      ipPools,
		nodeClient:   nodes,
		vmiCache:     vmiCache,
		nodeToVMName: nodeToVMName,
		namespace:    namespace,
		clusterName:  clusterName,
	}

	logrus.WithFields(logrus.Fields{
		"controller":  controllerName,
		"clusterName": clusterName,
	}).Info("start watching IPPool resources")

	ipPools.OnChange(ctx, controllerName, h.OnIPPoolChanged)
	ipPools.OnRemove(ctx, controllerName, h.OnIPPoolRemoved)
}

type Handler struct {
	ipPools      ctllbv1beta1.IPPoolController
	nodeClient   wranglecorev1.NodeController
	vmiCache     ctlkubevirtv1.VirtualMachineInstanceCache
	nodeToVMName *sync.Map
	namespace    string
	clusterName  string
}

func (h *Handler) OnIPPoolChanged(_ string, pool *lbv1beta1.IPPool) (*lbv1beta1.IPPool, error) {
	if pool == nil {
		return nil, nil
	}
	return pool, h.syncAllNodes()
}

func (h *Handler) OnIPPoolRemoved(_ string, pool *lbv1beta1.IPPool) (*lbv1beta1.IPPool, error) {
	if pool == nil {
		return nil, nil
	}
	return pool, h.syncAllNodes()
}

// syncAllNodes recalculates the pool→network mapping and writes it to every Node,
// filtering each node's mapping to only include IPPools whose network exists on the
// node's VMI.
func (h *Handler) syncAllNodes() error {
	mapping, err := h.buildMapping()
	if err != nil {
		return err
	}

	nodeList, err := h.nodeClient.List(metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list nodes: %w", err)
	}

	for i := range nodeList.Items {
		nodeName := nodeList.Items[i].Name
		filtered := h.filterMappingForNode(nodeName, mapping)
		if err := ccmutil.AnnotateNodeWithIPPoolNetworks(h.nodeClient, nodeName, filtered); err != nil {
			logrus.WithField("node", nodeName).Warnf("failed to update ippool-network-mapping: %v", err)
		}
	}
	return nil
}

// filterMappingForNode returns a copy of mapping restricted to IPPools whose network
// is present in the Multus spec of the VMI backing this node. If the VMI cannot be
// found, the full mapping is returned unchanged.
func (h *Handler) filterMappingForNode(nodeName string, mapping map[string]string) map[string]string {
	vmiName := nodeName
	if name, ok := h.nodeToVMName.Load(nodeName); ok {
		vmiName = name.(string)
	}

	vmi, err := h.vmiCache.Get(h.namespace, vmiName)
	if err != nil {
		logrus.WithField("node", nodeName).Warnf("failed to get VMI %s/%s for IPPool filtering, using full mapping: %v", h.namespace, vmiName, err)
		return mapping
	}

	return ccmutil.FilterIPPoolMappingByVMINetworks(mapping, vmi)
}

func (h *Handler) buildMapping() (map[string]string, error) {
	return ccmutil.BuildIPPoolNetworkMapping(h.ipPools, h.clusterName)
}
