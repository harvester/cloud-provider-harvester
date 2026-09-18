package fakeclients

import (
	"github.com/rancher/wrangler/v3/pkg/generic"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

// VMICache is a minimal in-memory VirtualMachineInstanceCache for use in unit tests.
type VMICache struct {
	vmis []*kubevirtv1.VirtualMachineInstance
	err  error
}

// NewVMICache returns a VMICache with the given VMIs and optional fixed error.
func NewVMICache(vmis []*kubevirtv1.VirtualMachineInstance, err error) *VMICache {
	return &VMICache{vmis: vmis, err: err}
}

func (f *VMICache) Get(namespace, name string) (*kubevirtv1.VirtualMachineInstance, error) {
	if f.err != nil {
		return nil, f.err
	}
	for _, vmi := range f.vmis {
		if vmi == nil {
			continue
		}
		if vmi.Namespace == namespace && vmi.Name == name {
			return vmi.DeepCopy(), nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: "kubevirt.io", Resource: "virtualmachineinstances"}, name)
}

func (f *VMICache) List(namespace string, selector labels.Selector) ([]*kubevirtv1.VirtualMachineInstance, error) {
	if f.err != nil {
		return nil, f.err
	}
	var result []*kubevirtv1.VirtualMachineInstance
	for _, vmi := range f.vmis {
		if vmi == nil {
			continue
		}
		if namespace != "" && vmi.Namespace != namespace {
			continue
		}
		if selector != nil && !selector.Matches(labels.Set(vmi.Labels)) {
			continue
		}
		result = append(result, vmi.DeepCopy())
	}
	return result, nil
}

func (f *VMICache) AddIndexer(_ string, _ generic.Indexer[*kubevirtv1.VirtualMachineInstance]) {}

func (f *VMICache) GetByIndex(_, _ string) ([]*kubevirtv1.VirtualMachineInstance, error) {
	return nil, errImplementMe
}
