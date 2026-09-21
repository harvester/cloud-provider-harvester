package fakeclients

import (
	"fmt"

	"github.com/rancher/wrangler/v3/pkg/generic"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
)

// ConfigMapCache is a minimal in-memory ConfigMapCache for use in unit tests.
// Only one configmap can be stored.
type ConfigMapCache struct {
	cm  *v1.ConfigMap
	err error
}

// NewConfigMapCache returns a ConfigMapCache that returns the given ConfigMap (or
// NotFound if cm is nil) and optionally a fixed error.
func NewConfigMapCache(cm *v1.ConfigMap, err error) *ConfigMapCache {
	return &ConfigMapCache{
		cm:  cm.DeepCopy(), // Note: ConfigMap object can run DeepCopy safely on nil pointer
		err: err,
	}
}

func (f *ConfigMapCache) Get(namespace, name string) (*v1.ConfigMap, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.cm == nil || f.cm.Namespace != namespace || f.cm.Name != name {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "configmaps"}, name)
	}
	return f.cm.DeepCopy(), nil
}

func (f *ConfigMapCache) List(namespace string, selector labels.Selector) ([]*v1.ConfigMap, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.cm == nil {
		return nil, nil
	}

	if f.cm.Namespace != namespace {
		return nil, nil
	}

	if selector != nil && !selector.Matches(labels.Set(f.cm.Labels)) {
		return nil, nil
	}

	return []*v1.ConfigMap{f.cm.DeepCopy()}, nil
}

func (f *ConfigMapCache) AddIndexer(_ string, _ generic.Indexer[*v1.ConfigMap]) {}

func (f *ConfigMapCache) GetByIndex(_, _ string) ([]*v1.ConfigMap, error) {
	return nil, errImplementMe
}

// ConfigMapClient implements a minimal fake ConfigMapClient for unit tests.
type ConfigMapClient struct {
	cache *ConfigMapCache
	err   error
}

// NewConfigMapClient returns a ConfigMapClient that shares state with the given cache.
func NewConfigMapClient(cache *ConfigMapCache, err error) *ConfigMapClient {
	return &ConfigMapClient{cache: cache, err: err}
}

func (f *ConfigMapClient) Get(namespace, name string, opts metav1.GetOptions) (*v1.ConfigMap, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.cache.Get(namespace, name)
}

func (f *ConfigMapClient) Create(cm *v1.ConfigMap) (*v1.ConfigMap, error) {
	if f.err != nil {
		return nil, f.err
	}
	if cm == nil {
		return nil, fmt.Errorf("the input configmap is nil")
	}
	f.cache.cm = cm.DeepCopy()
	return cm.DeepCopy(), nil
}

func (f *ConfigMapClient) Update(cm *v1.ConfigMap) (*v1.ConfigMap, error) {
	if f.err != nil {
		return nil, f.err
	}
	if cm == nil {
		return nil, fmt.Errorf("the input configmap is nil")
	}
	if f.cache.cm == nil || f.cache.cm.Namespace != cm.Namespace || f.cache.cm.Name != cm.Name {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "configmaps"}, cm.Name)
	}
	f.cache.cm = cm.DeepCopy()
	return cm.DeepCopy(), nil
}

func (f *ConfigMapClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	if f.err != nil {
		return f.err
	}
	if f.cache.cm == nil || f.cache.cm.Namespace != namespace || f.cache.cm.Name != name {
		return apierrors.NewNotFound(schema.GroupResource{Resource: "configmaps"}, name)
	}
	f.cache.cm = nil
	return nil
}

func (f *ConfigMapClient) List(namespace string, opts metav1.ListOptions) (*v1.ConfigMapList, error) {
	if f.err != nil {
		return nil, f.err
	}
	selector, err := labels.Parse(opts.LabelSelector)
	if err != nil {
		return nil, err
	}
	cms, err := f.cache.List(namespace, selector)
	if err != nil {
		return nil, err
	}
	items := make([]v1.ConfigMap, len(cms))
	for i, cm := range cms {
		items[i] = *cm
	}
	return &v1.ConfigMapList{Items: items}, nil
}

func (f *ConfigMapClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (*v1.ConfigMap, error) {
	return nil, errImplementMe
}

func (f *ConfigMapClient) UpdateStatus(*v1.ConfigMap) (*v1.ConfigMap, error) {
	return nil, errImplementMe
}

func (f *ConfigMapClient) Watch(_ string, opts metav1.ListOptions) (watch.Interface, error) {
	return nil, errImplementMe
}

func (f *ConfigMapClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.ClientInterface[*v1.ConfigMap, *v1.ConfigMapList], error) {
	return nil, errImplementMe
}
