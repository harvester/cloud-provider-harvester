package virtualmachineinstance

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	cfg "github.com/harvester/harvester-cloud-provider/pkg/config"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	kubefake "k8s.io/client-go/kubernetes/fake"

	utils "github.com/harvester/harvester-cloud-provider/pkg/utils"
	"github.com/harvester/harvester-cloud-provider/pkg/utils/fakeclients"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"
)

func newNADMappingConfigMap(mapping map[string]string) *corev1.ConfigMap {
	data, _ := json.Marshal(mapping)
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.ConfigMapNADMapping,
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			utils.ConfigMapKeyNADMapping: string(data),
		},
	}
}

func TestOnVmiChangedNetworkMapping(t *testing.T) {
	namespace := "default"
	clusterName := "test-cluster"

	origClusterName := cfg.GetConfig().ClusterName
	cfg.GetConfig().ClusterName = clusterName
	defer func() { cfg.GetConfig().ClusterName = origClusterName }()

	// Define the expected NAD mapping derived from Spec.Networks + Status.Interfaces intersection
	expectedMapping := map[string]string{
		"default/vlan100": "enp1s0",
	}
	expectedDataBytes, _ := json.Marshal(expectedMapping)
	expectedDataStr := string(expectedDataBytes)

	harvesterVMIWithNet := &kubevirtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vmi-harvester-net",
			Namespace: namespace,
			Labels: map[string]string{
				utils.LabelKeyGuestClusterNameOnVM:           clusterName,
				utils.HarvesterLabelKeyVirtualMachineCreator: utils.HarvesterVirtualMachineCreatorNodeDriver,
			},
		},
		Spec: kubevirtv1.VirtualMachineInstanceSpec{
			Networks: []kubevirtv1.Network{
				{
					Name: "network1",
					NetworkSource: kubevirtv1.NetworkSource{
						Multus: &kubevirtv1.MultusNetwork{
							NetworkName: "default/vlan100",
						},
					},
				},
			},
		},
		Status: kubevirtv1.VirtualMachineInstanceStatus{
			Phase: kubevirtv1.Running,
			MigrationState: &kubevirtv1.VirtualMachineInstanceMigrationState{
				Completed: true,
			},
			Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{
				{
					Name:          "network1",
					InterfaceName: "enp1s0",
				},
			},
		},
	}

	tests := []struct {
		name             string
		vmi              *kubevirtv1.VirtualMachineInstance
		initialVMIs      []*kubevirtv1.VirtualMachineInstance
		expectError      bool
		checkConfigMap   bool
		expectedMapValue string
	}{
		{
			name:             "1. vmi is nil (deletion event), clears configmap data",
			vmi:              nil,
			initialVMIs:      []*kubevirtv1.VirtualMachineInstance{}, // No remaining VMIs means empty mapping
			expectError:      false,
			checkConfigMap:   true,
			expectedMapValue: "", // Expect empty string after deletion cleanup
		},
		{
			name: "2. vmi in different namespace is ignored",
			vmi: &kubevirtv1.VirtualMachineInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vmi-other",
					Namespace: "other-namespace",
				},
			},
			initialVMIs:    []*kubevirtv1.VirtualMachineInstance{},
			expectError:    false,
			checkConfigMap: false,
		},
		{
			name: "3. vmi not created by harvester creator is ignored",
			vmi: &kubevirtv1.VirtualMachineInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vmi-manual",
					Namespace: namespace,
					Labels: map[string]string{
						utils.LabelKeyGuestClusterNameOnVM: clusterName,
					},
				},
			},
			initialVMIs:    []*kubevirtv1.VirtualMachineInstance{},
			expectError:    false,
			checkConfigMap: false,
		},
		{
			name:             "4. valid harvester vmi with network/status triggers sync and updates configmap with data",
			vmi:              harvesterVMIWithNet,
			initialVMIs:      []*kubevirtv1.VirtualMachineInstance{harvesterVMIWithNet},
			expectError:      false,
			checkConfigMap:   true,
			expectedMapValue: expectedDataStr,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			vmiCache := fakeclients.NewVMICache(tc.initialVMIs, nil)

			initialCM := newNADMappingConfigMap(expectedMapping)
			cmCache := fakeclients.NewConfigMapCache(initialCM, nil)
			cmClient := fakeclients.NewConfigMapClient(cmCache, nil)

			h := &Handler{
				namespace:       namespace,
				vmiCache:        vmiCache,
				configMapClient: cmClient,
			}

			_, err := h.OnVmiChangedNetworkMapping("", tc.vmi)

			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}

				if tc.checkConfigMap {
					cm, findErr := cmClient.Get(metav1.NamespaceSystem, utils.ConfigMapNADMapping, metav1.GetOptions{})
					if findErr != nil {
						t.Fatalf("unexpected error getting configmap: %v", findErr)
					}
					if cm == nil {
						t.Fatalf("expected configmap to be created, got nil")
					}
					val, ok := cm.Data[utils.ConfigMapKeyNADMapping]
					if !ok {
						t.Fatalf("expected configmap data to contain key %s", utils.ConfigMapKeyNADMapping)
					}
					if val != tc.expectedMapValue {
						t.Fatalf("expected configmap mapping value %q, got %q", tc.expectedMapValue, val)
					}
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

	// expected result is `true`
	if compareTopology(a, b) != true {
		t.Fatalf("expected topologies are identical")
	}
	// expected result is `false`
	if compareTopology(a, c) != false {
		t.Fatalf("expected topologies are different")
	}
}

func TestOnVmiChanged_SkipProcessing(t *testing.T) {
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
			UID:       types.UID("test-vmi-uid-12345"),
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
			name: "VMI has another guest cluster name label",
			vmi: func() *kubevirtv1.VirtualMachineInstance {
				v := baseVMI.DeepCopy()
				v.Labels[utils.LabelKeyGuestClusterNameOnVM] = "other-tenant"
				return v
			}(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{
				namespace:    namespace,
				nodeToVMName: &sync.Map{},
			}

			_, err := h.OnVmiChanged("", tc.vmi)
			if err != nil {
				t.Fatalf("expected no error on skip scenario, got: %v", err)
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
			UID:       types.UID("test-vmi-uid-12345"),
			Annotations: map[string]string{
				"test": "test",
			},
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

			h := &Handler{
				namespace:    namespace,
				nodeCache:    fakeCache,
				nodeToVMName: &sync.Map{},
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

func TestOnVmiChanged_HostnameLookup(t *testing.T) {
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

	initialNodes := map[string]*corev1.Node{
		targetNodeName: {ObjectMeta: metav1.ObjectMeta{Name: targetNodeName}},
	}

	baseVMI := &kubevirtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmiName,
			Namespace: namespace,
			UID:       types.UID("test-vmi-uid-12345"),
			Annotations: map[string]string{
				"test": "test",
			},
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
		initialNodeNameMaps   map[string]string
		expectError           bool
		errorContains         string
		targetNodeNameToCheck string
	}{
		{
			name:           "0. agent not ready (condition false)",
			guestOsInfoErr: nil,
			vmiModifier: func(v *kubevirtv1.VirtualMachineInstance) {
				v.Status.Conditions = []kubevirtv1.VirtualMachineInstanceCondition{
					{
						Type:   kubevirtv1.VirtualMachineInstanceAgentConnected,
						Status: corev1.ConditionFalse,
					},
				}
			},
			initialNodes:  initialNodes,
			expectError:   true,
			errorContains: "failed to get node",
		},
		{
			name:           "1. agent ready, but returns error, fallback",
			guestOsInfoRet: kubevirtv1.VirtualMachineInstanceGuestAgentInfo{},
			guestOsInfoErr: fmt.Errorf("a mocked GuestOsInfo error"),
			initialNodes:   initialNodes,
			expectError:    true,
			errorContains:  "failed to get node",
		},
		{
			name:           "2. agent ready, but returns empty hostname, fallback",
			guestOsInfoRet: kubevirtv1.VirtualMachineInstanceGuestAgentInfo{Hostname: ""},
			initialNodes:   initialNodes,
			expectError:    true,
			errorContains:  "failed to get node",
		},
		{
			name:                  "3. agent ready, returns matching hostname and node is stored successfully",
			guestOsInfoRet:        kubevirtv1.VirtualMachineInstanceGuestAgentInfo{Hostname: targetNodeName},
			initialNodes:          initialNodes,
			expectError:           false,
			targetNodeNameToCheck: targetNodeName,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			vmi := baseVMI.DeepCopy()
			if tc.vmiModifier != nil {
				tc.vmiModifier(vmi)
			}

			fakeCache := fakeclients.NewNodeCache(tc.initialNodes)

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
				kubevirtClient: fakeKubevirtClient,
				nodeToVMName:   &sync.Map{},
			}

			for k, v := range tc.initialNodeNameMaps {
				h.nodeToVMName.Store(k, v)
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

				if tc.targetNodeNameToCheck == "" {
					return
				}
				val, ok := h.nodeToVMName.Load(tc.targetNodeNameToCheck)
				if !ok || val.(string) != vmi.Name {
					t.Fatalf("expected to get value from map, but get ok %v val %v", ok, val)
				}
			}
		})
	}
}

func TestOnVmiChanged_TopologyReSyncPatch(t *testing.T) {
	vmiName := "vmi-custom-hostname"
	targetNodeName1 := "target-node-name-sync-successfully"
	targetNodeName2 := "target-node-name-sync-returns-error"
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
			Namespace: "default",
			Labels: map[string]string{
				utils.LabelKeyGuestClusterNameOnVM:           clusterName,
				utils.HarvesterLabelKeyVirtualMachineCreator: utils.HarvesterVirtualMachineCreatorNodeDriver,
			},
			Annotations: map[string]string{
				"topology.kubernetes.io/zone": "zone-b", // Mismatch triggers topologyChanged = true
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

	// vmi name does not match node name, but hostname match
	baseVMI2 := &kubevirtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "customized-with-host-name-match",
			Namespace: "default",
			Labels: map[string]string{
				utils.LabelKeyGuestClusterNameOnVM:           clusterName,
				utils.HarvesterLabelKeyVirtualMachineCreator: utils.HarvesterVirtualMachineCreatorNodeDriver,
			},
			Annotations: map[string]string{
				"topology.kubernetes.io/zone": "zone-b", // Mismatch triggers topologyChanged = true
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

	t.Run("1. vmi triggers node reSync successfully", func(t *testing.T) {
		initialNode := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: targetNodeName1,
				Labels: map[string]string{
					"topology.kubernetes.io/zone": "zone-a",
				},
			},
		}

		// Use standard client-go fake clientset which implements kubernetes.Interface completely
		fakeK8sClient := kubefake.NewSimpleClientset(initialNode)

		fakeCache := fakeclients.NewNodeCache(map[string]*corev1.Node{targetNodeName1: initialNode})

		fakeVMIClient := &FakeVMIInterface{
			GuestOsInfoFunc: func(ctx context.Context, name string) (kubevirtv1.VirtualMachineInstanceGuestAgentInfo, error) {
				return kubevirtv1.VirtualMachineInstanceGuestAgentInfo{
					Hostname: targetNodeName1,
				}, nil
			},
		}

		fakeKubevirtClient := &FakeKubevirtClient{
			VMIHandler: func(ns string) kubecli.VirtualMachineInstanceInterface {
				return fakeVMIClient
			},
		}

		h := &Handler{
			namespace:      "default",
			nodeCache:      fakeCache,
			restClient:     fakeK8sClient,
			kubevirtClient: fakeKubevirtClient,
			nodeToVMName:   &sync.Map{},
		}

		_, err := h.OnVmiChanged("", baseVMI)
		if err != nil {
			t.Fatalf("expected no error but got %v", err)
		}
	})

	t.Run("2. vmi triggers node reSync and hits patch error", func(t *testing.T) {
		initialNode := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: targetNodeName2,
				Labels: map[string]string{
					"topology.kubernetes.io/zone": "zone-a",
				},
			},
		}

		initialNode2 := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "not-the-target-node",
				Labels: map[string]string{
					"topology.kubernetes.io/zone": "zone-a",
				},
			},
		}

		// It is tedious to fully fake a k8s rest client, so we use a clever setup:
		// the local node cache is initialized with targetNodeName (resolved via guest agent),
		// but restClient is initialized with initialNode2 (with another name).
		//
		// Legacy code wrongly used vmi.Name to taint the node when a customized hostname
		// was used instead of the resolved node name. This test case simulates that scenario
		// to ensure the controller correctly targets the resolved node name rather than vmi.Name.
		fakeK8sClient := kubefake.NewSimpleClientset(initialNode2)
		fakeCache := fakeclients.NewNodeCache(map[string]*corev1.Node{targetNodeName2: initialNode})

		fakeVMIClient := &FakeVMIInterface{
			GuestOsInfoFunc: func(ctx context.Context, name string) (kubevirtv1.VirtualMachineInstanceGuestAgentInfo, error) {
				return kubevirtv1.VirtualMachineInstanceGuestAgentInfo{
					Hostname: targetNodeName2,
				}, nil
			},
		}

		fakeKubevirtClient := &FakeKubevirtClient{
			VMIHandler: func(ns string) kubecli.VirtualMachineInstanceInterface {
				return fakeVMIClient
			},
		}

		h := &Handler{
			namespace:      "default",
			nodeCache:      fakeCache,
			restClient:     fakeK8sClient,
			kubevirtClient: fakeKubevirtClient,
			nodeToVMName:   &sync.Map{},
		}

		_, err := h.OnVmiChanged("", baseVMI2)
		if err == nil {
			t.Fatalf("expected NotFound error when updating a removed node, got nil")
		}
		if !apierrors.IsNotFound(err) && !strings.Contains(err.Error(), "not found") {
			t.Fatalf("expected NotFound error, got: %v", err)
		}
	})

	t.Run("3. vmi has no annotation, the node reSync is skipped, it does not hit patch error", func(t *testing.T) {
		initialNode := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: targetNodeName2,
				Labels: map[string]string{
					"topology.kubernetes.io/zone": "zone-a",
				},
			},
		}

		initialNode2 := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "not-the-target-node",
				Labels: map[string]string{
					"topology.kubernetes.io/zone": "zone-a",
				},
			},
		}

		// It is tedious to fully fake a k8s rest client, so we use a clever setup:
		// the local node cache is initialized with targetNodeName (resolved via guest agent),
		// but restClient is initialized with initialNode2 (with another name).
		//
		// Legacy code wrongly used vmi.Name to taint the node when a customized hostname
		// was used instead of the resolved node name. This test case simulates that scenario
		// to ensure the controller correctly targets the resolved node name rather than vmi.Name.
		fakeK8sClient := kubefake.NewSimpleClientset(initialNode2)
		fakeCache := fakeclients.NewNodeCache(map[string]*corev1.Node{targetNodeName2: initialNode})

		fakeVMIClient := &FakeVMIInterface{
			GuestOsInfoFunc: func(ctx context.Context, name string) (kubevirtv1.VirtualMachineInstanceGuestAgentInfo, error) {
				return kubevirtv1.VirtualMachineInstanceGuestAgentInfo{
					Hostname: targetNodeName2,
				}, nil
			},
		}

		fakeKubevirtClient := &FakeKubevirtClient{
			VMIHandler: func(ns string) kubecli.VirtualMachineInstanceInterface {
				return fakeVMIClient
			},
		}

		h := &Handler{
			namespace:      "default",
			nodeCache:      fakeCache,
			restClient:     fakeK8sClient,
			kubevirtClient: fakeKubevirtClient,
			nodeToVMName:   &sync.Map{},
		}

		vmi := baseVMI2
		vmi.Annotations = nil // simulate the empty annotation, it skips the topology sync
		_, err := h.OnVmiChanged("", vmi)
		if err != nil {
			t.Fatalf("expected no error when VMI annotations are nil (topology sync skipped), got: %v", err)
		}
	})

}
