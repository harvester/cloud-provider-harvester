package virtualmachineinstance

import (
	"encoding/json"

	"testing"

	cfg "github.com/harvester/harvester-cloud-provider/pkg/config"

	utils "github.com/harvester/harvester-cloud-provider/pkg/utils"
	"github.com/harvester/harvester-cloud-provider/pkg/utils/fakeclients"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
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

func TestOnVmiChangedNetworkMapping1(t *testing.T) {
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
