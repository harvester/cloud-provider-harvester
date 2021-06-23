/*
Copyright 2021 Rancher Labs, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by main. DO NOT EDIT.

package v1

import (
	"context"
	"time"

	"github.com/rancher/lasso/pkg/client"
	"github.com/rancher/lasso/pkg/controller"
	"github.com/rancher/wrangler/pkg/apply"
	"github.com/rancher/wrangler/pkg/condition"
	"github.com/rancher/wrangler/pkg/generic"
	"github.com/rancher/wrangler/pkg/kv"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	v1 "kubevirt.io/client-go/api/v1"
)

type VirtualMachineInstanceHandler func(string, *v1.VirtualMachineInstance) (*v1.VirtualMachineInstance, error)

type VirtualMachineInstanceController interface {
	generic.ControllerMeta
	VirtualMachineInstanceClient

	OnChange(ctx context.Context, name string, sync VirtualMachineInstanceHandler)
	OnRemove(ctx context.Context, name string, sync VirtualMachineInstanceHandler)
	Enqueue(namespace, name string)
	EnqueueAfter(namespace, name string, duration time.Duration)

	Cache() VirtualMachineInstanceCache
}

type VirtualMachineInstanceClient interface {
	Create(*v1.VirtualMachineInstance) (*v1.VirtualMachineInstance, error)
	Update(*v1.VirtualMachineInstance) (*v1.VirtualMachineInstance, error)
	UpdateStatus(*v1.VirtualMachineInstance) (*v1.VirtualMachineInstance, error)
	Delete(namespace, name string, options *metav1.DeleteOptions) error
	Get(namespace, name string, options metav1.GetOptions) (*v1.VirtualMachineInstance, error)
	List(namespace string, opts metav1.ListOptions) (*v1.VirtualMachineInstanceList, error)
	Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error)
	Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *v1.VirtualMachineInstance, err error)
}

type VirtualMachineInstanceCache interface {
	Get(namespace, name string) (*v1.VirtualMachineInstance, error)
	List(namespace string, selector labels.Selector) ([]*v1.VirtualMachineInstance, error)

	AddIndexer(indexName string, indexer VirtualMachineInstanceIndexer)
	GetByIndex(indexName, key string) ([]*v1.VirtualMachineInstance, error)
}

type VirtualMachineInstanceIndexer func(obj *v1.VirtualMachineInstance) ([]string, error)

type virtualMachineInstanceController struct {
	controller    controller.SharedController
	client        *client.Client
	gvk           schema.GroupVersionKind
	groupResource schema.GroupResource
}

func NewVirtualMachineInstanceController(gvk schema.GroupVersionKind, resource string, namespaced bool, controller controller.SharedControllerFactory) VirtualMachineInstanceController {
	c := controller.ForResourceKind(gvk.GroupVersion().WithResource(resource), gvk.Kind, namespaced)
	return &virtualMachineInstanceController{
		controller: c,
		client:     c.Client(),
		gvk:        gvk,
		groupResource: schema.GroupResource{
			Group:    gvk.Group,
			Resource: resource,
		},
	}
}

func FromVirtualMachineInstanceHandlerToHandler(sync VirtualMachineInstanceHandler) generic.Handler {
	return func(key string, obj runtime.Object) (ret runtime.Object, err error) {
		var v *v1.VirtualMachineInstance
		if obj == nil {
			v, err = sync(key, nil)
		} else {
			v, err = sync(key, obj.(*v1.VirtualMachineInstance))
		}
		if v == nil {
			return nil, err
		}
		return v, err
	}
}

func (c *virtualMachineInstanceController) Updater() generic.Updater {
	return func(obj runtime.Object) (runtime.Object, error) {
		newObj, err := c.Update(obj.(*v1.VirtualMachineInstance))
		if newObj == nil {
			return nil, err
		}
		return newObj, err
	}
}

func UpdateVirtualMachineInstanceDeepCopyOnChange(client VirtualMachineInstanceClient, obj *v1.VirtualMachineInstance, handler func(obj *v1.VirtualMachineInstance) (*v1.VirtualMachineInstance, error)) (*v1.VirtualMachineInstance, error) {
	if obj == nil {
		return obj, nil
	}

	copyObj := obj.DeepCopy()
	newObj, err := handler(copyObj)
	if newObj != nil {
		copyObj = newObj
	}
	if obj.ResourceVersion == copyObj.ResourceVersion && !equality.Semantic.DeepEqual(obj, copyObj) {
		return client.Update(copyObj)
	}

	return copyObj, err
}

func (c *virtualMachineInstanceController) AddGenericHandler(ctx context.Context, name string, handler generic.Handler) {
	c.controller.RegisterHandler(ctx, name, controller.SharedControllerHandlerFunc(handler))
}

func (c *virtualMachineInstanceController) AddGenericRemoveHandler(ctx context.Context, name string, handler generic.Handler) {
	c.AddGenericHandler(ctx, name, generic.NewRemoveHandler(name, c.Updater(), handler))
}

func (c *virtualMachineInstanceController) OnChange(ctx context.Context, name string, sync VirtualMachineInstanceHandler) {
	c.AddGenericHandler(ctx, name, FromVirtualMachineInstanceHandlerToHandler(sync))
}

func (c *virtualMachineInstanceController) OnRemove(ctx context.Context, name string, sync VirtualMachineInstanceHandler) {
	c.AddGenericHandler(ctx, name, generic.NewRemoveHandler(name, c.Updater(), FromVirtualMachineInstanceHandlerToHandler(sync)))
}

func (c *virtualMachineInstanceController) Enqueue(namespace, name string) {
	c.controller.Enqueue(namespace, name)
}

func (c *virtualMachineInstanceController) EnqueueAfter(namespace, name string, duration time.Duration) {
	c.controller.EnqueueAfter(namespace, name, duration)
}

func (c *virtualMachineInstanceController) Informer() cache.SharedIndexInformer {
	return c.controller.Informer()
}

func (c *virtualMachineInstanceController) GroupVersionKind() schema.GroupVersionKind {
	return c.gvk
}

func (c *virtualMachineInstanceController) Cache() VirtualMachineInstanceCache {
	return &virtualMachineInstanceCache{
		indexer:  c.Informer().GetIndexer(),
		resource: c.groupResource,
	}
}

func (c *virtualMachineInstanceController) Create(obj *v1.VirtualMachineInstance) (*v1.VirtualMachineInstance, error) {
	result := &v1.VirtualMachineInstance{}
	return result, c.client.Create(context.TODO(), obj.Namespace, obj, result, metav1.CreateOptions{})
}

func (c *virtualMachineInstanceController) Update(obj *v1.VirtualMachineInstance) (*v1.VirtualMachineInstance, error) {
	result := &v1.VirtualMachineInstance{}
	return result, c.client.Update(context.TODO(), obj.Namespace, obj, result, metav1.UpdateOptions{})
}

func (c *virtualMachineInstanceController) UpdateStatus(obj *v1.VirtualMachineInstance) (*v1.VirtualMachineInstance, error) {
	result := &v1.VirtualMachineInstance{}
	return result, c.client.UpdateStatus(context.TODO(), obj.Namespace, obj, result, metav1.UpdateOptions{})
}

func (c *virtualMachineInstanceController) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	if options == nil {
		options = &metav1.DeleteOptions{}
	}
	return c.client.Delete(context.TODO(), namespace, name, *options)
}

func (c *virtualMachineInstanceController) Get(namespace, name string, options metav1.GetOptions) (*v1.VirtualMachineInstance, error) {
	result := &v1.VirtualMachineInstance{}
	return result, c.client.Get(context.TODO(), namespace, name, result, options)
}

func (c *virtualMachineInstanceController) List(namespace string, opts metav1.ListOptions) (*v1.VirtualMachineInstanceList, error) {
	result := &v1.VirtualMachineInstanceList{}
	return result, c.client.List(context.TODO(), namespace, result, opts)
}

func (c *virtualMachineInstanceController) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c.client.Watch(context.TODO(), namespace, opts)
}

func (c *virtualMachineInstanceController) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (*v1.VirtualMachineInstance, error) {
	result := &v1.VirtualMachineInstance{}
	return result, c.client.Patch(context.TODO(), namespace, name, pt, data, result, metav1.PatchOptions{}, subresources...)
}

type virtualMachineInstanceCache struct {
	indexer  cache.Indexer
	resource schema.GroupResource
}

func (c *virtualMachineInstanceCache) Get(namespace, name string) (*v1.VirtualMachineInstance, error) {
	obj, exists, err := c.indexer.GetByKey(namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(c.resource, name)
	}
	return obj.(*v1.VirtualMachineInstance), nil
}

func (c *virtualMachineInstanceCache) List(namespace string, selector labels.Selector) (ret []*v1.VirtualMachineInstance, err error) {

	err = cache.ListAllByNamespace(c.indexer, namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.VirtualMachineInstance))
	})

	return ret, err
}

func (c *virtualMachineInstanceCache) AddIndexer(indexName string, indexer VirtualMachineInstanceIndexer) {
	utilruntime.Must(c.indexer.AddIndexers(map[string]cache.IndexFunc{
		indexName: func(obj interface{}) (strings []string, e error) {
			return indexer(obj.(*v1.VirtualMachineInstance))
		},
	}))
}

func (c *virtualMachineInstanceCache) GetByIndex(indexName, key string) (result []*v1.VirtualMachineInstance, err error) {
	objs, err := c.indexer.ByIndex(indexName, key)
	if err != nil {
		return nil, err
	}
	result = make([]*v1.VirtualMachineInstance, 0, len(objs))
	for _, obj := range objs {
		result = append(result, obj.(*v1.VirtualMachineInstance))
	}
	return result, nil
}

type VirtualMachineInstanceStatusHandler func(obj *v1.VirtualMachineInstance, status v1.VirtualMachineInstanceStatus) (v1.VirtualMachineInstanceStatus, error)

type VirtualMachineInstanceGeneratingHandler func(obj *v1.VirtualMachineInstance, status v1.VirtualMachineInstanceStatus) ([]runtime.Object, v1.VirtualMachineInstanceStatus, error)

func RegisterVirtualMachineInstanceStatusHandler(ctx context.Context, controller VirtualMachineInstanceController, condition condition.Cond, name string, handler VirtualMachineInstanceStatusHandler) {
	statusHandler := &virtualMachineInstanceStatusHandler{
		client:    controller,
		condition: condition,
		handler:   handler,
	}
	controller.AddGenericHandler(ctx, name, FromVirtualMachineInstanceHandlerToHandler(statusHandler.sync))
}

func RegisterVirtualMachineInstanceGeneratingHandler(ctx context.Context, controller VirtualMachineInstanceController, apply apply.Apply,
	condition condition.Cond, name string, handler VirtualMachineInstanceGeneratingHandler, opts *generic.GeneratingHandlerOptions) {
	statusHandler := &virtualMachineInstanceGeneratingHandler{
		VirtualMachineInstanceGeneratingHandler: handler,
		apply:                                   apply,
		name:                                    name,
		gvk:                                     controller.GroupVersionKind(),
	}
	if opts != nil {
		statusHandler.opts = *opts
	}
	controller.OnChange(ctx, name, statusHandler.Remove)
	RegisterVirtualMachineInstanceStatusHandler(ctx, controller, condition, name, statusHandler.Handle)
}

type virtualMachineInstanceStatusHandler struct {
	client    VirtualMachineInstanceClient
	condition condition.Cond
	handler   VirtualMachineInstanceStatusHandler
}

func (a *virtualMachineInstanceStatusHandler) sync(key string, obj *v1.VirtualMachineInstance) (*v1.VirtualMachineInstance, error) {
	if obj == nil {
		return obj, nil
	}

	origStatus := obj.Status.DeepCopy()
	obj = obj.DeepCopy()
	newStatus, err := a.handler(obj, obj.Status)
	if err != nil {
		// Revert to old status on error
		newStatus = *origStatus.DeepCopy()
	}

	if a.condition != "" {
		if errors.IsConflict(err) {
			a.condition.SetError(&newStatus, "", nil)
		} else {
			a.condition.SetError(&newStatus, "", err)
		}
	}
	if !equality.Semantic.DeepEqual(origStatus, &newStatus) {
		if a.condition != "" {
			// Since status has changed, update the lastUpdatedTime
			a.condition.LastUpdated(&newStatus, time.Now().UTC().Format(time.RFC3339))
		}

		var newErr error
		obj.Status = newStatus
		newObj, newErr := a.client.UpdateStatus(obj)
		if err == nil {
			err = newErr
		}
		if newErr == nil {
			obj = newObj
		}
	}
	return obj, err
}

type virtualMachineInstanceGeneratingHandler struct {
	VirtualMachineInstanceGeneratingHandler
	apply apply.Apply
	opts  generic.GeneratingHandlerOptions
	gvk   schema.GroupVersionKind
	name  string
}

func (a *virtualMachineInstanceGeneratingHandler) Remove(key string, obj *v1.VirtualMachineInstance) (*v1.VirtualMachineInstance, error) {
	if obj != nil {
		return obj, nil
	}

	obj = &v1.VirtualMachineInstance{}
	obj.Namespace, obj.Name = kv.RSplit(key, "/")
	obj.SetGroupVersionKind(a.gvk)

	return nil, generic.ConfigureApplyForObject(a.apply, obj, &a.opts).
		WithOwner(obj).
		WithSetID(a.name).
		ApplyObjects()
}

func (a *virtualMachineInstanceGeneratingHandler) Handle(obj *v1.VirtualMachineInstance, status v1.VirtualMachineInstanceStatus) (v1.VirtualMachineInstanceStatus, error) {
	objs, newStatus, err := a.VirtualMachineInstanceGeneratingHandler(obj, status)
	if err != nil {
		return newStatus, err
	}

	return newStatus, generic.ConfigureApplyForObject(a.apply, obj, &a.opts).
		WithOwner(obj).
		WithSetID(a.name).
		ApplyObjects(objs...)
}
