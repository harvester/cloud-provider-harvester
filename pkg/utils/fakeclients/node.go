package fakeclients

import (
	"fmt"
	"sync"

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

// NodeCache provides an in-memory thread-safe store for simulating NodeCache and NodeClient in unit tests.
type NodeCache struct {
	lock  sync.Mutex
	nodes map[string]*v1.Node
}

func NewNodeCache(initialNodes map[string]*v1.Node) *NodeCache {
	if initialNodes == nil {
		initialNodes = make(map[string]*v1.Node)
	}
	// Deep copy initial nodes to prevent cross-test pollution
	copiedNodes := make(map[string]*v1.Node, len(initialNodes))
	for k, v := range initialNodes {
		if v != nil {
			copiedNodes[k] = v.DeepCopy()
		}
	}
	return &NodeCache{nodes: copiedNodes}
}

// --- NodeCache methods ---

func (f *NodeCache) Get(name string) (*v1.Node, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	node, ok := f.nodes[name]
	if !ok {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "nodes"}, name)
	}
	return node.DeepCopy(), nil
}

func (f *NodeCache) List(selector labels.Selector) ([]*v1.Node, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	var result []*v1.Node
	for _, node := range f.nodes {
		if selector.Matches(labels.Set(node.Labels)) {
			result = append(result, node.DeepCopy())
		}
	}
	return result, nil
}

func (f *NodeCache) AddIndexer(_ string, _ generic.Indexer[*v1.Node]) {}
func (f *NodeCache) GetByIndex(_, _ string) ([]*v1.Node, error)       { return nil, nil }

// --- NodeClient methods ---

func (f *NodeCache) GetClient(name string, _ metav1.GetOptions) (*v1.Node, error) {
	return f.Get(name)
}

func (f *NodeCache) Create(node *v1.Node) (*v1.Node, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	if _, ok := f.nodes[node.Name]; ok {
		return nil, apierrors.NewAlreadyExists(schema.GroupResource{Resource: "nodes"}, node.Name)
	}
	f.nodes[node.Name] = node.DeepCopy()
	return node.DeepCopy(), nil
}

func (f *NodeCache) Update(node *v1.Node) (*v1.Node, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	if _, ok := f.nodes[node.Name]; !ok {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "nodes"}, node.Name)
	}
	f.nodes[node.Name] = node.DeepCopy()
	return node.DeepCopy(), nil
}

func (f *NodeCache) Delete(name string, _ *metav1.DeleteOptions) error {
	f.lock.Lock()
	defer f.lock.Unlock()
	if _, ok := f.nodes[name]; !ok {
		return apierrors.NewNotFound(schema.GroupResource{Resource: "nodes"}, name)
	}
	delete(f.nodes, name)
	return nil
}

func (f *NodeCache) DeleteCollection(_ *metav1.DeleteOptions, _ metav1.ListOptions) error {
	return fmt.Errorf("implement me")
}
func (f *NodeCache) Watch(_ metav1.ListOptions) (watch.Interface, error) {
	return nil, fmt.Errorf("implement me")
}

func (f *NodeCache) Patch(name string, _ types.PatchType, _ []byte, _ ...string) (*v1.Node, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	if _, ok := f.nodes[name]; !ok {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "nodes"}, name)
	}
	return nil, fmt.Errorf("implement me")
}

// NodeClient implements ctlcorev1.NodeClient by wrapping NodeCache to satisfy both client and cache methods cleanly.
type NodeClient struct {
	cache *NodeCache
}

func NewNodeClient(cache *NodeCache) *NodeClient {
	return &NodeClient{cache: cache}
}

func (f *NodeClient) Get(name string, opts metav1.GetOptions) (*v1.Node, error) {
	return f.cache.Get(name)
}

func (f *NodeClient) List(opts metav1.ListOptions) (*v1.NodeList, error) {
	selector, err := labels.Parse(opts.LabelSelector)
	if err != nil {
		return nil, err
	}
	nodes, err := f.cache.List(selector)
	if err != nil {
		return nil, err
	}
	items := make([]v1.Node, len(nodes))
	for i, node := range nodes {
		items[i] = *node
	}
	return &v1.NodeList{Items: items}, nil
}

func (f *NodeClient) Create(node *v1.Node) (*v1.Node, error) {
	return f.cache.Create(node)
}

func (f *NodeClient) Update(node *v1.Node) (*v1.Node, error) {
	return f.cache.Update(node)
}

func (f *NodeClient) Delete(name string, options *metav1.DeleteOptions) error {
	return f.cache.Delete(name, options)
}

func (f *NodeClient) DeleteCollection(options *metav1.DeleteOptions, listOptions metav1.ListOptions) error {
	return f.cache.DeleteCollection(options, listOptions)
}

func (f *NodeClient) GetStatus(name string, opts metav1.GetOptions) (*v1.Node, error) {
	return f.cache.Get(name)
}

func (f *NodeClient) UpdateStatus(node *v1.Node) (*v1.Node, error) {
	return f.cache.Update(node)
}

func (f *NodeClient) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (*v1.Node, error) {
	return f.cache.Patch(name, pt, data, subresources...)
}

func (f *NodeClient) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	return f.cache.Watch(opts)
}

func (f *NodeClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.NonNamespacedClientInterface[*v1.Node, *v1.NodeList], error) {
	return nil, fmt.Errorf("implement me")
}
