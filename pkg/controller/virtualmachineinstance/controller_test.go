package virtualmachineinstance

import (
	"errors"
	"strings"
	"testing"

	cfg "github.com/harvester/harvester-cloud-provider/pkg/config"

	utils "github.com/harvester/harvester-cloud-provider/pkg/utils"
	"github.com/harvester/harvester-cloud-provider/pkg/utils/fakeclients"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

func TestAnnotateNodeWithVMIHostName_Conflict(t *testing.T) {
	nodeName := "my-node"
	existingVMI := "vm-old"
	newVMI := "vm-new"

	initialNodes := map[string]*corev1.Node{
		nodeName: {
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeName,
				Annotations: map[string]string{
					utils.AnnotationVMNameOfGuestClusterNode: existingVMI,
				},
			},
		},
	}

	fakeCache := fakeclients.NewNdoeCache(initialNodes)
	fakeClient := fakeclients.NewFakeNodeClient(fakeCache)
	h := &Handler{
		nodeCache:  fakeCache,
		nodeClient: fakeClient,
	}

	node, _ := fakeCache.Get(nodeName)
	err := h.annotateNodeWithVMIHostName(node, newVMI, "some-hostname")
	if err == nil {
		t.Fatalf("expected error due to annotation conflict, got nil")
	}
}

func TestFindNodeByVMIAnnotation(t *testing.T) {
	targetVMI := "vm-target"
	nodeName := "worker-node-1"

	initialNodes := map[string]*corev1.Node{
		nodeName: {
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeName,
				Annotations: map[string]string{
					utils.AnnotationVMNameOfGuestClusterNode: targetVMI,
				},
			},
		},
	}

	fakeCache := fakeclients.NewNdoeCache(initialNodes)
	h := &Handler{
		nodeCache: fakeCache,
	}

	foundNode, err := h.findNodeByVMIAnnotation(targetVMI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if foundNode == nil || foundNode.Name != nodeName {
		t.Fatalf("expected node %s, got %v", nodeName, foundNode)
	}
}

func TestCompareTopology(t *testing.T) {
	a := map[string]string{
		corev1.LabelTopologyRegion: "region-1",
		corev1.LabelTopologyZone:   "zone-1",
	}
	b := map[string]string{
		corev1.LabelTopologyRegion: "region-1",
		corev1.LabelTopologyZone:   "zone-1",
	}
	c := map[string]string{
		corev1.LabelTopologyRegion: "region-1",
		corev1.LabelTopologyZone:   "zone-2",
	}

	if !compareTopology(a, b) {
		t.Fatalf("expected topologies to match")
	}
	if compareTopology(a, c) {
		t.Fatalf("expected topologies not to match")
	}
}

type errorNodeClient struct {
	*fakeclients.FakeNodeClient
	injectErrorNodeName string
}

func (e *errorNodeClient) Update(node *corev1.Node) (*corev1.Node, error) {
	if node.Name == e.injectErrorNodeName {
		return nil, errors.New("injected update error")
	}
	return e.FakeNodeClient.Update(node)
}

func TestAnnotateNodeWithVMIName_Error(t *testing.T) {
	nodeName := "testtest"
	vmiName := "some-vmi"

	initialNodes := map[string]*corev1.Node{
		nodeName: {
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeName,
			},
		},
	}

	fakeStore := fakeclients.NewNdoeCache(initialNodes)
	fakeClient := &errorNodeClient{
		FakeNodeClient:      fakeclients.NewFakeNodeClient(fakeStore),
		injectErrorNodeName: nodeName,
	}

	h := &Handler{
		nodeClient: fakeClient,
		nodeCache:  fakeStore,
	}

	node, _ := fakeStore.Get(nodeName)
	err := h.annotateNodeWithVMIName(node, vmiName)
	if err == nil {
		t.Fatalf("expected error from annotateNodeWithVMIName, got nil")
	}
}

func TestOnVmiChanged_AnnotateError(t *testing.T) {
	namespace := "default"
	vmiName := "testtest"
	clusterName := "test-cluster"

	cfg.GetConfig().ClusterName = clusterName

	vmi := &kubevirtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmiName,
			Namespace: namespace,
			Labels: map[string]string{
				utils.LabelKeyGuestClusterNameOnVM:           clusterName,
				utils.HarvesterLabelKeyVirtualMachineCreator: utils.HarvesterVirtualMachineCreatorNodeDriver, // assuming creator label
			},
		},
		Status: kubevirtv1.VirtualMachineInstanceStatus{
			Phase: kubevirtv1.Running,
			Conditions: []kubevirtv1.VirtualMachineInstanceCondition{
				{
					Type:   kubevirtv1.VirtualMachineInstanceAgentConnected,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	initialNodes := map[string]*corev1.Node{
		vmiName: {
			ObjectMeta: metav1.ObjectMeta{
				Name: vmiName,
			},
		},
	}

	fakeStore := fakeclients.NewNdoeCache(initialNodes)
	fakeClient := &errorNodeClient{
		FakeNodeClient:      fakeclients.NewFakeNodeClient(fakeStore),
		injectErrorNodeName: vmiName,
	}

	h := &Handler{
		namespace:  namespace,
		nodeCache:  fakeStore,
		nodeClient: fakeClient,
	}

	// Note: We bypass/skip faking deeper syncNodeAndVMI execution here as more
	// complex controller dependencies (such as restClient/Lister mocks) are needed.
	_, err := h.OnVmiChanged("", vmi)
	if err == nil {
		t.Fatalf("expected error when annotation update fails, got nil")
	}
	expectedMsg := "injected update error"
	if err.Error() != expectedMsg && !strings.Contains(err.Error(), expectedMsg) {
		t.Fatalf("expected error containing %q, got %q", expectedMsg, err.Error())
	}
}
