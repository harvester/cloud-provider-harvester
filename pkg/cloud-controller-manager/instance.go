package ccm

import (
	"context"

	ctlkubevirt "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctlcore "github.com/rancher/wrangler/pkg/generated/controllers/core"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	cloudprovider "k8s.io/cloud-provider"
)

const (
	nodeLabelRegionKey       = "topology.harvesterhci.io/region"
	nodeLabelZoneKey         = "topology.harvesterhci.io/zone"
	nodeLabelHardwareTypeKey = "node.harvesterhci.io/hardware-type"
)

type instanceManager struct {
	vmiClient  ctlkubevirtv1.VirtualMachineInstanceClient
	nodeClient ctlcorev1.NodeController
	namespace  string
}

func newInstanceManager(cfg *rest.Config, namespace string) (cloudprovider.InstancesV2, error) {
	kubevirtFactory, err := ctlkubevirt.NewFactoryFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	coreFactory, err := ctlcore.NewFactoryFromConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &instanceManager{
		vmiClient:  kubevirtFactory.Kubevirt().V1().VirtualMachineInstance(),
		nodeClient: coreFactory.Core().V1().Node(),
		namespace:  namespace,
	}, nil
}

func (i *instanceManager) InstanceExists(ctx context.Context, node *v1.Node) (bool, error) {
	if _, err := i.vmiClient.Get(i.namespace, node.Name, metav1.GetOptions{}); err != nil && !errors.IsNotFound(err) {
		return false, err
	} else if errors.IsNotFound(err) {
		return false, nil
	} else {
		return true, nil
	}
}

func (i *instanceManager) InstanceShutdown(ctx context.Context, node *v1.Node) (bool, error) {
	vm, err := i.vmiClient.Get(i.namespace, node.Name, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	return !vm.IsRunning(), nil
}

func (i *instanceManager) InstanceMetadata(ctx context.Context, node *v1.Node) (*cloudprovider.InstanceMetadata, error) {
	vm, err := i.vmiClient.Get(i.namespace, node.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	hostName := vm.Status.NodeName
	host, err := i.nodeClient.Cache().Get(hostName)
	if err != nil {
		return nil, err
	}

	// Propagate zone, region and hardware type from the Harvester node on which the VM is running
	// By default the zone is set to the name of the physical server
	meta := cloudprovider.InstanceMetadata{
		ProviderID: ProviderName + "://" + string(vm.UID),
		Zone:       "hci-host-" + hostName,
	}
	labels := host.GetLabels()
	if region, ok := labels[nodeLabelRegionKey]; ok {
		meta.Region = region
	}
	if zone, ok := labels[nodeLabelZoneKey]; ok {
		meta.Zone = zone
	}
	if hwtype, ok := labels[nodeLabelHardwareTypeKey]; ok {
		meta.InstanceType = hwtype
	}

	return &meta, nil
}
