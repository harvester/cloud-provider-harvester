package ccm

import (
	"context"

	ctlvm "github.com/harvester/harvester/pkg/controller/master/virtualmachine"
	ctlkubevirt "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	cloudprovider "k8s.io/cloud-provider"
)

type instanceManager struct {
	vmiClient ctlkubevirtv1.VirtualMachineInstanceClient
	namespace string
}

func newInstanceManager(cfg *rest.Config, namespace string) (cloudprovider.InstancesV2, error) {
	kubevirtFactory, err := ctlkubevirt.NewFactoryFromConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &instanceManager{
		vmiClient: kubevirtFactory.Kubevirt().V1().VirtualMachineInstance(),
		namespace: namespace,
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
	vmi, err := i.vmiClient.Get(i.namespace, node.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	// Set node topology metadata from virtual machine annotations
	meta := cloudprovider.InstanceMetadata{
		ProviderID: ProviderName + "://" + string(vmi.UID),
	}
	annotations := vmi.GetAnnotations()
	if region, ok := annotations[ctlvm.AnnotationTopologyRegion]; ok {
		meta.Region = region
	}
	if zone, ok := annotations[ctlvm.AnnotationTopologyZone]; ok {
		meta.Zone = zone
	}

	return &meta, nil
}
