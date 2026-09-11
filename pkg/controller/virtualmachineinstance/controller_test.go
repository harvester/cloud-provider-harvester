package virtualmachineinstance

import (
	"context"
	"strings"
	"testing"

	cfg "github.com/harvester/harvester-cloud-provider/pkg/config"

	utils "github.com/harvester/harvester-cloud-provider/pkg/utils"
	"github.com/harvester/harvester-cloud-provider/pkg/utils/fakeclients"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"
)

func TestAnnotateNode_WithVMIName(t *testing.T) {
	nodeName := "example"

	tests := []struct {
		name        string
		initialVMI  string
		targetVMI   string
		expectError bool
	}{
		{
			name:        "error on conflict with different existing VMI",
			initialVMI:  "example",
			targetVMI:   "example-1",
			expectError: true,
		},
		{
			name:        "success when VMI annotation is already the same",
			initialVMI:  "example",
			targetVMI:   "example",
			expectError: false,
		},
		{
			name:        "success when no VMI annotation exists yet",
			initialVMI:  "",
			targetVMI:   "example",
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			annotations := map[string]string{}
			if tc.initialVMI != "" {
				annotations[utils.AnnotationVMNameOfGuestClusterNode] = tc.initialVMI
			}

			initialNodes := map[string]*corev1.Node{
				nodeName: {
					ObjectMeta: metav1.ObjectMeta{
						Name:        nodeName,
						Annotations: annotations,
					},
				},
			}

			fakeCache := fakeclients.NewNodeCache(initialNodes)
			fakeClient := fakeclients.NewNodeClient(fakeCache)
			h := &Handler{
				nodeCache:  fakeCache,
				nodeClient: fakeClient,
			}

			node, _ := fakeCache.Get(nodeName)
			err := h.annotateNodeWithVMIName(node, tc.targetVMI)

			if tc.expectError && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.expectError && err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
		})
	}
}

func TestFindNodeByVMIAnnotation(t *testing.T) {
	tests := []struct {
		name             string
		initialNodes     map[string]*corev1.Node
		searchVMI        string
		expectedNodeName string
		expectError      bool
	}{
		{
			name: "found when node has matching VMI annotation",
			initialNodes: map[string]*corev1.Node{
				"worker-node-1": {
					ObjectMeta: metav1.ObjectMeta{
						Name: "worker-node-1",
						Annotations: map[string]string{
							utils.AnnotationVMNameOfGuestClusterNode: "vm-target",
						},
					},
				},
			},
			searchVMI:        "vm-target",
			expectedNodeName: "worker-node-1",
			expectError:      false,
		},
		{
			name: "not found when annotations do not match",
			initialNodes: map[string]*corev1.Node{
				"worker-node-1": {
					ObjectMeta: metav1.ObjectMeta{
						Name: "worker-node-1",
						Annotations: map[string]string{
							utils.AnnotationVMNameOfGuestClusterNode: "vm-other",
						},
					},
				},
			},
			searchVMI:        "vm-target",
			expectedNodeName: "",
			expectError:      false,
		},
		{
			name:             "not found when cache is empty",
			initialNodes:     map[string]*corev1.Node{},
			searchVMI:        "vm-target",
			expectedNodeName: "",
			expectError:      false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fakeCache := fakeclients.NewNodeCache(tc.initialNodes)
			h := &Handler{
				nodeCache: fakeCache,
			}

			foundNode, err := h.findNodeByVMIAnnotation(tc.searchVMI)

			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.expectedNodeName == "" {
				if foundNode != nil {
					t.Fatalf("expected no node, got %s", foundNode.Name)
				}
			} else {
				if foundNode == nil || foundNode.Name != tc.expectedNodeName {
					t.Fatalf("expected node %s, got %v", tc.expectedNodeName, foundNode)
				}
			}
		})
	}
}

func TestTopologyChanged(t *testing.T) {
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

	if topologyChanged(a, b) {
		t.Fatalf("expected topologies no change")
	}
	if !topologyChanged(a, c) {
		t.Fatalf("expected topologies changed")
	}
}

func TestOnVmiChanged_SkipScenarios(t *testing.T) {
	namespace := "default"
	vmiName := "vmi-example"
	clusterName := "test-cluster"

	origClusterName := cfg.GetConfig().ClusterName
	cfg.GetConfig().ClusterName = clusterName
	defer func() { cfg.GetConfig().ClusterName = origClusterName }()

	now := metav1.Now()
	baseVMI := &kubevirtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmiName,
			Namespace: namespace,
			Labels: map[string]string{
				utils.LabelKeyGuestClusterNameOnVM:           clusterName,
				utils.HarvesterLabelKeyVirtualMachineCreator: utils.HarvesterVirtualMachineCreatorNodeDriver,
			},
		},
		Status: kubevirtv1.VirtualMachineInstanceStatus{
			Phase: kubevirtv1.Running,
		},
	}

	tests := []struct {
		name string
		vmi  *kubevirtv1.VirtualMachineInstance
	}{
		{
			name: "nil VMI",
			vmi:  nil,
		},
		{
			name: "VMI with deletion timestamp",
			vmi: func() *kubevirtv1.VirtualMachineInstance {
				v := baseVMI.DeepCopy()
				v.DeletionTimestamp = &now
				return v
			}(),
		},
		{
			name: "VMI in different namespace",
			vmi: func() *kubevirtv1.VirtualMachineInstance {
				v := baseVMI.DeepCopy()
				v.Namespace = "other-namespace"
				return v
			}(),
		},
		{
			name: "VMI not running",
			vmi: func() *kubevirtv1.VirtualMachineInstance {
				v := baseVMI.DeepCopy()
				v.Status.Phase = kubevirtv1.Pending
				return v
			}(),
		},
		{
			name: "VMI migration not completed",
			vmi: func() *kubevirtv1.VirtualMachineInstance {
				v := baseVMI.DeepCopy()
				v.Status.MigrationState = &kubevirtv1.VirtualMachineInstanceMigrationState{
					Completed: false,
				}
				return v
			}(),
		},
		{
			name: "VMI not created by Harvester creator",
			vmi: func() *kubevirtv1.VirtualMachineInstance {
				v := baseVMI.DeepCopy()
				delete(v.Labels, utils.HarvesterLabelKeyVirtualMachineCreator)
				return v
			}(),
		},
		{
			name: "VMI missing guest cluster name label",
			vmi: func() *kubevirtv1.VirtualMachineInstance {
				v := baseVMI.DeepCopy()
				delete(v.Labels, utils.LabelKeyGuestClusterNameOnVM)
				return v
			}(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{
				namespace: namespace,
			}

			resVMI, err := h.OnVmiChanged("", tc.vmi)
			if err != nil {
				t.Fatalf("expected no error on skip scenario, got: %v", err)
			}
			if tc.vmi != nil && resVMI != tc.vmi {
				t.Fatalf("expected returned VMI to be identical pointer on skip")
			}
		})
	}
}

func TestOnVmiChanged(t *testing.T) {
	namespace := "default"
	vmiName := "vmi-example"
	clusterName := "test-cluster"

	origClusterName := cfg.GetConfig().ClusterName
	cfg.GetConfig().ClusterName = clusterName
	defer func() { cfg.GetConfig().ClusterName = origClusterName }()

	baseVMI := &kubevirtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmiName,
			Namespace: namespace,
			Labels: map[string]string{
				utils.LabelKeyGuestClusterNameOnVM:           clusterName,
				utils.HarvesterLabelKeyVirtualMachineCreator: utils.HarvesterVirtualMachineCreatorNodeDriver,
			},
		},
		Status: kubevirtv1.VirtualMachineInstanceStatus{
			Phase: kubevirtv1.Running,
		},
	}

	tests := []struct {
		name                  string
		vmi                   *kubevirtv1.VirtualMachineInstance
		initialNodes          map[string]*corev1.Node
		expectError           bool
		errorContains         string
		expectedAnnotatedNode string
	}{
		{
			name: "1. vmi name match node name, annotate node successfully",
			vmi:  baseVMI,
			initialNodes: map[string]*corev1.Node{
				vmiName: {
					ObjectMeta: metav1.ObjectMeta{
						Name: vmiName,
					},
				},
			},
			expectError:           false,
			expectedAnnotatedNode: vmiName,
		},
		{
			name: "2. vmi name match node name, already annotated so return quickly",
			vmi:  baseVMI,
			initialNodes: map[string]*corev1.Node{
				vmiName: {
					ObjectMeta: metav1.ObjectMeta{
						Name: vmiName,
						Annotations: map[string]string{
							utils.AnnotationVMNameOfGuestClusterNode: vmiName,
						},
					},
				},
			},
			expectError:           false,
			expectedAnnotatedNode: vmiName,
		},
		{
			name: "3. vmi name match node name, annotation conflict falls through to agent check error",
			vmi:  baseVMI,
			initialNodes: map[string]*corev1.Node{
				vmiName: {
					ObjectMeta: metav1.ObjectMeta{
						Name: vmiName,
						Annotations: map[string]string{
							utils.AnnotationVMNameOfGuestClusterNode: "other-vmi",
						},
					},
				},
			},
			expectError:   true,
			errorContains: "guest agent is not connected",
		},
		{
			name: "4. vmi name does not match node name, agent not ready returns error",
			vmi:  baseVMI,
			initialNodes: map[string]*corev1.Node{
				"unrelated-node": {
					ObjectMeta: metav1.ObjectMeta{
						Name: "unrelated-node",
					},
				},
			},
			expectError:   true,
			errorContains: "guest agent is not connected",
		},
		{
			name: "5. vmi name does not match node name, found via node annotation",
			vmi:  baseVMI,
			initialNodes: map[string]*corev1.Node{
				"some-other-node": {
					ObjectMeta: metav1.ObjectMeta{
						Name: "some-other-node",
						Annotations: map[string]string{
							utils.AnnotationVMNameOfGuestClusterNode: vmiName,
						},
					},
				},
			},
			expectError:           false,
			expectedAnnotatedNode: "some-other-node",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fakeCache := fakeclients.NewNodeCache(tc.initialNodes)
			fakeClient := fakeclients.NewNodeClient(fakeCache)

			h := &Handler{
				namespace:  namespace,
				nodeCache:  fakeCache,
				nodeClient: fakeClient,
			}

			_, err := h.OnVmiChanged("", tc.vmi)

			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.errorContains != "" && !strings.Contains(err.Error(), tc.errorContains) {
					t.Fatalf("expected error containing %q, got %q", tc.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}

				// Verify node object annotation after successful execution
				foundNode, findErr := h.findNodeByVMIAnnotation(vmiName)
				if findErr != nil {
					t.Fatalf("unexpected error finding node by VMI annotation: %v", findErr)
				}
				if tc.expectedAnnotatedNode != "" {
					if foundNode == nil {
						t.Fatalf("expected node %s to be annotated with VMI %s, but found none", tc.expectedAnnotatedNode, vmiName)
					}
					if foundNode.Name != tc.expectedAnnotatedNode {
						t.Fatalf("expected annotated node to be %s, got %s", tc.expectedAnnotatedNode, foundNode.Name)
					}
					if foundNode.Annotations[utils.AnnotationVMNameOfGuestClusterNode] != vmiName {
						t.Fatalf("expected node annotation %s to be %s, got %s", utils.AnnotationVMNameOfGuestClusterNode, vmiName, foundNode.Annotations[utils.AnnotationVMNameOfGuestClusterNode])
					}
				}
			}
		})
	}
}

func TestOnVmiChanged_DisableHostnameLookup(t *testing.T) {
	namespace := "default"
	vmiName := "vmi-example"
	clusterName := "test-cluster"

	origClusterName := cfg.GetConfig().ClusterName
	cfg.GetConfig().ClusterName = clusterName
	defer func() { cfg.GetConfig().ClusterName = origClusterName }()

	origDisableLookup := cfg.GetConfig().DisableHostnameLookup
	cfg.GetConfig().DisableHostnameLookup = true
	defer func() { cfg.GetConfig().DisableHostnameLookup = origDisableLookup }()

	baseVMI := &kubevirtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmiName,
			Namespace: namespace,
			Labels: map[string]string{
				utils.LabelKeyGuestClusterNameOnVM:           clusterName,
				utils.HarvesterLabelKeyVirtualMachineCreator: utils.HarvesterVirtualMachineCreatorNodeDriver,
			},
		},
		Status: kubevirtv1.VirtualMachineInstanceStatus{
			Phase: kubevirtv1.Running,
		},
	}

	tests := []struct {
		name          string
		vmi           *kubevirtv1.VirtualMachineInstance
		initialNodes  map[string]*corev1.Node
		expectError   bool
		errorContains string
	}{
		{
			name: "1. strict mapping: node exists matching vmi name, succeeds",
			vmi:  baseVMI,
			initialNodes: map[string]*corev1.Node{
				vmiName: {
					ObjectMeta: metav1.ObjectMeta{
						Name: vmiName,
					},
				},
			},
			expectError: false,
		},
		{
			name:          "2. strict mapping: node missing for vmi name, returns error",
			vmi:           baseVMI,
			initialNodes:  map[string]*corev1.Node{},
			expectError:   true,
			errorContains: "failed to get node via vm name",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fakeCache := fakeclients.NewNodeCache(tc.initialNodes)
			fakeClient := fakeclients.NewNodeClient(fakeCache)

			h := &Handler{
				namespace:  namespace,
				nodeCache:  fakeCache,
				nodeClient: fakeClient,
			}

			_, err := h.OnVmiChanged("", tc.vmi)

			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.errorContains != "" && !strings.Contains(err.Error(), tc.errorContains) {
					t.Fatalf("expected error containing %q, got %q", tc.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
			}
		})
	}
}

// FakeKubevirtClient implements kubecli.KubevirtClient for testing the `GuestOsInfo` call.
type FakeKubevirtClient struct {
	kubecli.KubevirtClient
	VMIHandler func(namespace string) kubecli.VirtualMachineInstanceInterface
}

func (f *FakeKubevirtClient) VirtualMachineInstance(namespace string) kubecli.VirtualMachineInstanceInterface {
	if f.VMIHandler != nil {
		return f.VMIHandler(namespace)
	}
	return &FakeVMIInterface{}
}

// FakeVMIInterface implements kubecli.VirtualMachineInstanceInterface.
type FakeVMIInterface struct {
	kubecli.VirtualMachineInstanceInterface
	GuestOsInfoFunc func(ctx context.Context, name string) (kubevirtv1.VirtualMachineInstanceGuestAgentInfo, error)
}

func (m *FakeVMIInterface) GuestOsInfo(ctx context.Context, name string) (kubevirtv1.VirtualMachineInstanceGuestAgentInfo, error) {
	if m.GuestOsInfoFunc != nil {
		return m.GuestOsInfoFunc(ctx, name)
	}
	return kubevirtv1.VirtualMachineInstanceGuestAgentInfo{}, nil
}

func TestOnVmiChanged_LegacyHostnameLookup(t *testing.T) {
	namespace := "default"
	vmiName := "vmi-example"
	targetNodeName := "target-node-name"
	clusterName := "test-cluster"

	origClusterName := cfg.GetConfig().ClusterName
	cfg.GetConfig().ClusterName = clusterName
	defer func() { cfg.GetConfig().ClusterName = origClusterName }()

	origDisableLookup := cfg.GetConfig().DisableHostnameLookup
	cfg.GetConfig().DisableHostnameLookup = false
	defer func() { cfg.GetConfig().DisableHostnameLookup = origDisableLookup }()

	baseVMI := &kubevirtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmiName,
			Namespace: namespace,
			Labels: map[string]string{
				utils.LabelKeyGuestClusterNameOnVM:           clusterName,
				utils.HarvesterLabelKeyVirtualMachineCreator: utils.HarvesterVirtualMachineCreatorNodeDriver,
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

	tests := []struct {
		name                  string
		guestOsInfoRet        kubevirtv1.VirtualMachineInstanceGuestAgentInfo
		guestOsInfoErr        error
		vmiModifier           func(v *kubevirtv1.VirtualMachineInstance)
		initialNodes          map[string]*corev1.Node
		expectError           bool
		errorContains         string
		expectedAnnotatedNode string
	}{
		{
			name:           "1. agent not ready (condition false)",
			guestOsInfoErr: nil,
			vmiModifier: func(v *kubevirtv1.VirtualMachineInstance) {
				v.Status.Conditions = []kubevirtv1.VirtualMachineInstanceCondition{
					{
						Type:   kubevirtv1.VirtualMachineInstanceAgentConnected,
						Status: corev1.ConditionFalse,
					},
				}
			},
			initialNodes: map[string]*corev1.Node{
				targetNodeName: {ObjectMeta: metav1.ObjectMeta{Name: targetNodeName}},
			},
			expectError:   true,
			errorContains: "guest agent is not connected",
		},
		{
			name:           "2. agent ready, but returns empty hostname",
			guestOsInfoRet: kubevirtv1.VirtualMachineInstanceGuestAgentInfo{Hostname: ""},
			initialNodes: map[string]*corev1.Node{
				targetNodeName: {ObjectMeta: metav1.ObjectMeta{Name: targetNodeName}},
			},
			expectError:   true,
			errorContains: "get an empty hostname",
		},
		{
			name:           "3. agent ready, returns non-matching hostname",
			guestOsInfoRet: kubevirtv1.VirtualMachineInstanceGuestAgentInfo{Hostname: "non-existent-node"},
			initialNodes: map[string]*corev1.Node{
				targetNodeName: {ObjectMeta: metav1.ObjectMeta{Name: targetNodeName}},
			},
			expectError:   true,
			errorContains: "did not find related node",
		},
		{
			name:           "4. agent ready, returns matching hostname and node is annotated successfully",
			guestOsInfoRet: kubevirtv1.VirtualMachineInstanceGuestAgentInfo{Hostname: targetNodeName},
			initialNodes: map[string]*corev1.Node{
				targetNodeName: {ObjectMeta: metav1.ObjectMeta{Name: targetNodeName}},
			},
			expectError:           false,
			expectedAnnotatedNode: targetNodeName,
		},
		{
			name:           "5. agent ready, hostname matches a node, but node is already annotated to another VMI, return conflict",
			guestOsInfoRet: kubevirtv1.VirtualMachineInstanceGuestAgentInfo{Hostname: targetNodeName},
			initialNodes: map[string]*corev1.Node{
				targetNodeName: {
					ObjectMeta: metav1.ObjectMeta{
						Name: targetNodeName,
						Annotations: map[string]string{
							utils.AnnotationVMNameOfGuestClusterNode: "other-vmi-name",
						},
					},
				},
			},
			expectError:   true,
			errorContains: "conflict",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			vmi := baseVMI.DeepCopy()
			if tc.vmiModifier != nil {
				tc.vmiModifier(vmi)
			}

			fakeCache := fakeclients.NewNodeCache(tc.initialNodes)
			fakeClient := fakeclients.NewNodeClient(fakeCache)

			fakeVMIClient := &FakeVMIInterface{
				GuestOsInfoFunc: func(ctx context.Context, name string) (kubevirtv1.VirtualMachineInstanceGuestAgentInfo, error) {
					return tc.guestOsInfoRet, tc.guestOsInfoErr
				},
			}

			fakeKubevirtClient := &FakeKubevirtClient{
				VMIHandler: func(ns string) kubecli.VirtualMachineInstanceInterface {
					return fakeVMIClient
				},
			}

			h := &Handler{
				namespace:      namespace,
				nodeCache:      fakeCache,
				nodeClient:     fakeClient,
				kubevirtClient: fakeKubevirtClient,
			}

			_, err := h.OnVmiChanged("", vmi)

			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.errorContains != "" && !strings.Contains(err.Error(), tc.errorContains) {
					t.Fatalf("expected error containing %q, got %q", tc.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error under legacy lookup fallback, got: %v", err)
				}

				foundNode, err := fakeCache.Get(tc.expectedAnnotatedNode)
				if err != nil {
					t.Fatalf("expected to find node %s in cache: %v", tc.expectedAnnotatedNode, err)
				}

				if foundNode.Annotations[utils.AnnotationVMNameOfGuestClusterNode] != vmiName {
					t.Fatalf("expected node annotation %s to be %s, got %s",
						utils.AnnotationVMNameOfGuestClusterNode,
						vmiName,
						foundNode.Annotations[utils.AnnotationVMNameOfGuestClusterNode])
				}
			}
		})
	}
}
