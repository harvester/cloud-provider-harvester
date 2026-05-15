package util

import (
	"encoding/json"
	"fmt"

	lbv1beta1 "github.com/harvester/harvester-load-balancer/pkg/apis/loadbalancer.harvesterhci.io/v1beta1"
	ctllbv1beta1 "github.com/harvester/harvester-load-balancer/pkg/generated/controllers/loadbalancer.harvesterhci.io/v1beta1"
	wranglecorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	kubevirtv1 "kubevirt.io/api/core/v1"

	utils "github.com/harvester/harvester-cloud-provider/pkg/utils"
)

// BuildIPPoolNetworkMapping returns a map of IPPool name -> spec.selector.network
// for pools scoped to the given guest cluster (or with no scope restriction).
// Example: {"n123": "default/net123"}
func BuildIPPoolNetworkMapping(ipPoolClient ctllbv1beta1.IPPoolClient, clusterName string) (map[string]string, error) {
	poolList, err := ipPoolClient.List(metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list IPPools: %w", err)
	}

	result := make(map[string]string)
	for _, pool := range poolList.Items {
		if pool.Spec.Selector.Network == "" {
			continue
		}
		if IPPoolMatchesCluster(&pool, clusterName) {
			result[pool.Name] = pool.Spec.Selector.Network
		}
	}
	return result, nil
}

// IPPoolMatchesCluster reports whether an IPPool is available for the given guest cluster.
// A pool matches if its scope is empty (no restriction) or any scope entry covers the cluster.
func IPPoolMatchesCluster(pool *lbv1beta1.IPPool, clusterName string) bool {
	if len(pool.Spec.Selector.Scope) == 0 {
		return true
	}
	for _, t := range pool.Spec.Selector.Scope {
		if t.GuestCluster == "*" || t.GuestCluster == clusterName {
			return true
		}
	}
	return false
}

// AnnotateNodeWithIPPoolNetworks writes the IPPool name -> VM network mapping as a JSON
// annotation on the given Node, skipping the update if the value is already current.
func AnnotateNodeWithIPPoolNetworks(nodeClient wranglecorev1.NodeClient, nodeName string, mapping map[string]string) error {
	data, err := json.Marshal(mapping)
	if err != nil {
		return fmt.Errorf("marshal IPPool networks: %w", err)
	}
	value := string(data)

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		node, err := nodeClient.Get(nodeName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if node.Annotations[utils.KeyIPPoolNetworkMapping] == value {
			return nil
		}
		nodeCopy := node.DeepCopy()
		if nodeCopy.Annotations == nil {
			nodeCopy.Annotations = make(map[string]string)
		}
		nodeCopy.Annotations[utils.KeyIPPoolNetworkMapping] = value
		_, err = nodeClient.Update(nodeCopy)
		return err
	})
}

// FilterIPPoolMappingByVMINetworks returns a copy of mapping that only contains
// entries whose network value matches a Multus network present in the VMI spec.
// This prevents annotating a node with IPPools whose VM network does not exist
// in the guest cluster.
func FilterIPPoolMappingByVMINetworks(mapping map[string]string, vmi *kubevirtv1.VirtualMachineInstance) map[string]string {
	nadSet := make(map[string]struct{}, len(vmi.Spec.Networks))
	for _, net := range vmi.Spec.Networks {
		if net.Multus != nil {
			nadSet[net.Multus.NetworkName] = struct{}{}
		}
	}

	result := make(map[string]string)
	for poolName, network := range mapping {
		if _, ok := nadSet[network]; ok {
			result[poolName] = network
		}
	}

	return result
}
