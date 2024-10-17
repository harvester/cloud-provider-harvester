package ccm

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"slices"
	"sync"

	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/cloud-provider/api"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

type instanceManager struct {
	vmClient     ctlkubevirtv1.VirtualMachineClient
	vmiClient    ctlkubevirtv1.VirtualMachineInstanceClient
	nodeToVMName *sync.Map
	namespace    string
}

func (i *instanceManager) InstanceExists(ctx context.Context, node *v1.Node) (bool, error) {
	if _, err := i.getVM(node); err != nil {
		if !errors.IsNotFound(err) {
			return false, err
		}
		return false, nil
	}
	return true, nil
}

func (i *instanceManager) InstanceShutdown(ctx context.Context, node *v1.Node) (bool, error) {
	vm, err := i.getVM(node)
	if err != nil {
		return false, err
	}
	return !vm.Status.Ready, nil
}

func (i *instanceManager) InstanceMetadata(ctx context.Context, node *v1.Node) (*cloudprovider.InstanceMetadata, error) {
	vm, err := i.getVM(node)
	if err != nil {
		return nil, err
	}

	// Set node topology metadata from virtual machine annotations
	meta := &cloudprovider.InstanceMetadata{
		ProviderID: ProviderName + "://" + string(vm.UID),
	}

	vmi, err := i.vmiClient.Get(i.namespace, vm.Name, metav1.GetOptions{})
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

func (i *instanceManager) getVM(node *v1.Node) (*kubevirtv1.VirtualMachine, error) {
	nodeName := node.Name
	if vmName, ok := i.nodeToVMName.Load(nodeName); ok {
		nodeName = vmName.(string)
	}
	return i.vmClient.Get(i.namespace, nodeName, metav1.GetOptions{})
}

// getNodeAddresses return nodeAddresses only when the value of annotation `alpha.kubernetes.io/provided-node-ip` is not empty
func getNodeAddresses(node *v1.Node, vmi *kubevirtv1.VirtualMachineInstance) []v1.NodeAddress {
	providedNodeIP, ok := node.Annotations[api.AnnotationAlphaProvidedIPAddr]
	if !ok {
		return nil
	}

	aiIPs, err := getAdditionalInternalIPs(node)
	if err != nil {
		// if additional IPs are not correctly marked, only log an error, do not return this error
		logrus.WithFields(logrus.Fields{
			"namespace": node.Namespace,
			"name":      node.Name,
		}).Debugf("%s, skip it", err.Error())
	}

	nodeAddresses := make([]v1.NodeAddress, 0, len(vmi.Spec.Networks)+1)

	for _, network := range vmi.Spec.Networks {
		for _, networkInterface := range vmi.Status.Interfaces {
			if network.Name == networkInterface.Name {
				if ip := net.ParseIP(networkInterface.IP); ip != nil && ip.To4() != nil {
					nodeAddr := v1.NodeAddress{
						Address: networkInterface.IP,
					}
					if networkInterface.IP == providedNodeIP || (aiIPs != nil && slices.Contains(aiIPs, networkInterface.IP)) {
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

// User may want to mark some IPs of the node also as internal
func getAdditionalInternalIPs(node *v1.Node) ([]string, error) {
	aiIPs, ok := node.Annotations[KeyAdditionalInternalIPs]
	if !ok {
		return nil, nil
	}
	var ips []string
	err := json.Unmarshal([]byte(aiIPs), &ips)
	if err != nil {
		return nil, fmt.Errorf("failed to decode additional external IPs from %v: %w", aiIPs, err)
	}
	return ips, nil
}
