package ccm

import (
	"context"

	ctlkubevirt "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	cloudprovider "k8s.io/cloud-provider"
)

type instanceManager struct {
	vmClient  ctlkubevirtv1.VirtualMachineClient
	namespace string
}

func newInstanceManager(cfg *rest.Config, namespace string) (cloudprovider.InstancesV2, error) {
	kubevirtFactory, err := ctlkubevirt.NewFactoryFromConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &instanceManager{
		vmClient:  kubevirtFactory.Kubevirt().V1().VirtualMachine(),
		namespace: namespace,
	}, nil
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
	return !*vm.Spec.Running, nil
}

func (i *instanceManager) InstanceMetadata(ctx context.Context, node *v1.Node) (*cloudprovider.InstanceMetadata, error) {
	vm, err := i.vmClient.Get(i.namespace, node.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return &cloudprovider.InstanceMetadata{
		ProviderID: ProviderName + "://" + string(vm.UID),
	}, nil
}
