package fakeclients

import (
	"github.com/rancher/wrangler/v3/pkg/generic"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ConfigMapCache is a minimal in-memory ConfigMapCache for use in unit tests.
type ConfigMapCache struct {
	cm  *v1.ConfigMap
	err error
}

// NewConfigMapCache returns a ConfigMapCache that returns the given ConfigMap (or
// NotFound if cm is nil) and optionally a fixed error.
func NewConfigMapCache(cm *v1.ConfigMap, err error) *ConfigMapCache {
	return &ConfigMapCache{cm: cm, err: err}
}

func (f *ConfigMapCache) Get(_, name string) (*v1.ConfigMap, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.cm == nil {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "configmaps"}, name)
	}
	return f.cm, nil
}

func (f *ConfigMapCache) List(_ string, _ labels.Selector) ([]*v1.ConfigMap, error) {
	return nil, nil
}

func (f *ConfigMapCache) AddIndexer(_ string, _ generic.Indexer[*v1.ConfigMap]) {}

func (f *ConfigMapCache) GetByIndex(_, _ string) ([]*v1.ConfigMap, error) {
	return nil, nil
}
