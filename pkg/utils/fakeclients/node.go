package fakeclients

import (
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

// NdoeCache provides an in-memory thread-safe store for simulating NodeCache and NodeClient in unit tests.
type NdoeCache struct {
	lock  sync.Mutex
	nodes map[string]*v1.Node
	err   error
}

func NewNdoeCache(initialNodes map[string]*v1.Node) *NdoeCache {
	if initialNodes == nil {
		initialNodes = make(map[string]*v1.Node)
	}
	return &NdoeCache{nodes: initialNodes}
}

// --- NodeCache methods ---

func (f *NdoeCache) Get(name string) (*v1.Node, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	node, ok := f.nodes[name]
	if !ok {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "nodes"}, name)
	}
	return node.DeepCopy(), nil
}

func (f *NdoeCache) List(selector labels.Selector) ([]*v1.Node, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	var result []*v1.Node
	for _, node := range f.nodes {
		if selector.Matches(labels.Set(node.Labels)) {
			result = append(result, node.DeepCopy())
		}
	}
	return result, nil
}

func (f *NdoeCache) AddIndexer(_ string, _ generic.Indexer[*v1.Node]) {}
func (f *NdoeCache) GetByIndex(_, _ string) ([]*v1.Node, error)       { return nil, nil }

// --- NodeClient methods ---

func (f *NdoeCache) GetClient(name string, _ metav1.GetOptions) (*v1.Node, error) {
	return f.Get(name)
}

func (f *NdoeCache) Create(node *v1.Node) (*v1.Node, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	if _, ok := f.nodes[node.Name]; ok {
		return nil, apierrors.NewAlreadyExists(schema.GroupResource{Resource: "nodes"}, node.Name)
	}
	f.nodes[node.Name] = node.DeepCopy()
	return node.DeepCopy(), nil
}

func (f *NdoeCache) Update(node *v1.Node) (*v1.Node, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	if _, ok := f.nodes[node.Name]; !ok {
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "nodes"}, node.Name)
	}
	f.nodes[node.Name] = node.DeepCopy()
	return node.DeepCopy(), nil
}

func (f *NdoeCache) Delete(name string, _ *metav1.DeleteOptions) error {
	f.lock.Lock()
	defer f.lock.Unlock()
	if _, ok := f.nodes[name]; !ok {
		return nil
	}
	delete(f.nodes, name)
	return nil
}

func (f *NdoeCache) DeleteCollection(_ *metav1.DeleteOptions, _ metav1.ListOptions) error { return nil }
func (f *NdoeCache) Watch(_ metav1.ListOptions) (watch.Interface, error)                  { return nil, nil }
func (f *NdoeCache) Patch(name string, _ types.PatchType, _ []byte, _ ...string) (*v1.Node, error) {
	return f.Get(name)
}

// FakeNodeClient implements ctlcorev1.NodeClient by wrapping NdoeCache to satisfy both client and cache methods cleanly.
type FakeNodeClient struct {
	*NdoeCache
}

func NewFakeNodeClient(cache *NdoeCache) *FakeNodeClient {
	return &FakeNodeClient{NdoeCache: cache}
}

func (f *FakeNodeClient) Get(name string, _ metav1.GetOptions) (*v1.Node, error) {
	return f.NdoeCache.Get(name)
}

func (f *FakeNodeClient) List(_ metav1.ListOptions) (*v1.NodeList, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	var items []v1.Node
	for _, node := range f.nodes {
		items = append(items, *node.DeepCopy())
	}
	return &v1.NodeList{Items: items}, nil
}

func (f *FakeNodeClient) UpdateStatus(node *v1.Node) (*v1.Node, error) {
	return f.Update(node)
}

func (f *FakeNodeClient) WithImpersonation(_ rest.ImpersonationConfig) (generic.NonNamespacedClientInterface[*v1.Node, *v1.NodeList], error) {
	panic("implement me")
}
