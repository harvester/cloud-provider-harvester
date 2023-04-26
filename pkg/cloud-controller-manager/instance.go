package ccm

import (
	"context"
	"net"

	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/cloud-provider/api"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

type instanceManager struct {
	vmClient  ctlkubevirtv1.VirtualMachineClient
	vmiClient ctlkubevirtv1.VirtualMachineInstanceClient
	namespace string
}

func (i *instanceManager) InstanceExists(ctx context.Context, node *v1.Node) (bool, error) {
	if _, err := i.vmClient.Get(i.namespace, node.Name, metav1.GetOptions{}); err != nil && !errors.IsNotFound(err) {
		return false, err
	} else if errors.IsNotFound(err) {
		return false, nil
	} else {
		return true, nil
	}
}

func (i *instanceManager) InstanceShutdown(ctx context.Context, node *v1.Node) (bool, error) {
	vm, err := i.vmClient.Get(i.namespace, node.Name, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	return !vm.Status.Ready, nil
}

func (i *instanceManager) InstanceMetadata(ctx context.Context, node *v1.Node) (*cloudprovider.InstanceMetadata, error) {
	vm, err := i.vmClient.Get(i.namespace, node.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	// Set node topology metadata from virtual machine annotations
	meta := &cloudprovider.InstanceMetadata{
		ProviderID: ProviderName + "://" + string(vm.UID),
	}

	vmi, err := i.vmiClient.Get(i.namespace, node.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return meta, nil
		}
		return nil, err
	}

	annotations := vmi.GetAnnotations()
	if region, ok := annotations[v1.LabelTopologyRegion]; ok {
		meta.Region = region
	}
	if zone, ok := annotations[v1.LabelTopologyZone]; ok {
		meta.Zone = zone
	}

	meta.NodeAddresses = getNodeAddresses(node, vmi)

	return meta, nil
}

// getNodeAddresses return nodeAddresses only when the value of annotation `alpha.kubernetes.io/provided-node-ip` is not empty
func getNodeAddresses(node *v1.Node, vmi *kubevirtv1.VirtualMachineInstance) []v1.NodeAddress {
	providedNodeIP, ok := node.Annotations[api.AnnotationAlphaProvidedIPAddr]
	if !ok {
		return nil
	}

	nodeAddresses := make([]v1.NodeAddress, 0, len(vmi.Spec.Networks)+1)

	for _, network := range vmi.Spec.Networks {
		for _, networkInterface := range vmi.Status.Interfaces {
			if network.Name == networkInterface.Name {
				if ip := net.ParseIP(networkInterface.IP); ip != nil && ip.To4() != nil {
					nodeAddr := v1.NodeAddress{
						Address: networkInterface.IP,
					}
					if networkInterface.IP == providedNodeIP {
						nodeAddr.Type = v1.NodeInternalIP
					} else {
						nodeAddr.Type = v1.NodeExternalIP
					}
					nodeAddresses = append(nodeAddresses, nodeAddr)
				}
			}
		}
	}
	nodeAddresses = append(nodeAddresses, v1.NodeAddress{
		Type:    v1.NodeHostName,
		Address: node.Name,
	})

	return nodeAddresses
}
