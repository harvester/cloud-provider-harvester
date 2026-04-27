package ccm

import (
	"reflect"
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester-cloud-provider/pkg/config"
	utils "github.com/harvester/harvester-cloud-provider/pkg/utils"
)

const (
	testNamespace = "default"
	nodeName      = "test"

	networkDefault = "default"
	network120     = "vlan120"
	network130     = "vlan130"

	networkDefaultIP = "192.168.100.10"
	network120IP     = "192.168.120.10"
	network130IP     = "192.168.130.10"
)

func Test_getNodeAddresses(t *testing.T) {
	// Setup global config state
	cfg := config.GetConfig()
	originalExclude := cfg.NodeExcludeIPRanges
	defer func() { cfg.NodeExcludeIPRanges = originalExclude }()

	tests := []struct {
		name        string
		node        *v1.Node
		vmi         *kubevirtv1.VirtualMachineInstance
		excludeList []string // Global config simulation
		output      []v1.NodeAddress
		wantErr     string
	}{
		{
			name: "Only management NIC is processed (Others ignored)",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
					Annotations: map[string]string{
						"alpha.kubernetes.io/provided-node-ip": networkDefaultIP,
					},
				},
			},
			vmi: &kubevirtv1.VirtualMachineInstance{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Networks: []kubevirtv1.Network{
						{Name: networkDefault, NetworkSource: kubevirtv1.NetworkSource{Multus: &kubevirtv1.MultusNetwork{}}},
						{Name: network120, NetworkSource: kubevirtv1.NetworkSource{Multus: &kubevirtv1.MultusNetwork{}}},
					},
				},
				Status: kubevirtv1.VirtualMachineInstanceStatus{
					Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{
						{Name: networkDefault, IPs: []string{networkDefaultIP}},
						{Name: network120, IPs: []string{network120IP}},
					},
				},
			},
			output: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: networkDefaultIP},
				{Type: v1.NodeHostName, Address: nodeName},
			},
		},
		{
			name:        "Exclusion List (Global Config) hides specific IP",
			excludeList: []string{network120IP},
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: nodeName},
			},
			vmi: &kubevirtv1.VirtualMachineInstance{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Networks: []kubevirtv1.Network{
						{Name: networkDefault, NetworkSource: kubevirtv1.NetworkSource{Multus: &kubevirtv1.MultusNetwork{}}},
					},
				},
				Status: kubevirtv1.VirtualMachineInstanceStatus{
					Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{
						{Name: networkDefault, IPs: []string{networkDefaultIP, network120IP}},
					},
				},
			},
			output: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: networkDefaultIP}, // Fallback logic picks this
				{Type: v1.NodeHostName, Address: nodeName},
			},
		},
		{
			name: "Mix Internal and External via CIDR annotation",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
					Annotations: map[string]string{
						"alpha.kubernetes.io/provided-node-ip": networkDefaultIP,
						utils.KeyAdditionalInternalIPs:         "[\"172.20.0.0/24\"]",
					},
				},
			},
			vmi: &kubevirtv1.VirtualMachineInstance{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Networks: []kubevirtv1.Network{
						{Name: networkDefault, NetworkSource: kubevirtv1.NetworkSource{Multus: &kubevirtv1.MultusNetwork{}}},
					},
				},
				Status: kubevirtv1.VirtualMachineInstanceStatus{
					Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{
						{Name: networkDefault, IPs: []string{networkDefaultIP, "172.20.0.50", "8.8.8.8"}},
					},
				},
			},
			output: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: networkDefaultIP},
				{Type: v1.NodeInternalIP, Address: "172.20.0.50"},
				{Type: v1.NodeExternalIP, Address: "8.8.8.8"},
				{Type: v1.NodeHostName, Address: nodeName},
			},
		},
		{
			name: "Fallback to First-Win (Dual Stack) when no annotation exists",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: nodeName},
			},
			vmi: &kubevirtv1.VirtualMachineInstance{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Networks: []kubevirtv1.Network{
						{Name: networkDefault, NetworkSource: kubevirtv1.NetworkSource{Multus: &kubevirtv1.MultusNetwork{}}},
					},
				},
				Status: kubevirtv1.VirtualMachineInstanceStatus{
					Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{
						{
							Name: networkDefault,
							IPs:  []string{"192.168.1.10", "192.168.1.11", "2001:db8::1", "2001:db8::2"},
						},
					},
				},
			},
			output: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: "192.168.1.10"},
				{Type: v1.NodeExternalIP, Address: "192.168.1.11"}, // Correct: only first is Internal
				{Type: v1.NodeInternalIP, Address: "2001:db8::1"},
				{Type: v1.NodeExternalIP, Address: "2001:db8::2"}, // Correct: only first is Internal
				{Type: v1.NodeHostName, Address: nodeName},
			},
		},
		{
			name: "Malformed node IP annotation",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
					Annotations: map[string]string{
						"alpha.kubernetes.io/provided-node-ip": "broken", // This triggers the error
					},
				},
			},
			vmi:     nil,
			wantErr: "vmi is empty",
		},
		{
			name: "Malformed node IP annotation 2",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
					Annotations: map[string]string{
						"alpha.kubernetes.io/provided-node-ip": "broken",
					},
				},
			},
			// Provide just enough VMI info to pass the network gatekeeper
			vmi: &kubevirtv1.VirtualMachineInstance{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Networks: []kubevirtv1.Network{
						{Name: networkDefault, NetworkSource: kubevirtv1.NetworkSource{Multus: &kubevirtv1.MultusNetwork{}}},
					},
				},
				Status: kubevirtv1.VirtualMachineInstanceStatus{
					Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{
						{
							Name: networkDefault,
							IPs:  []string{networkDefaultIP}, // Need an IP to trigger the InternalIPRanges check
						},
					},
				},
			},
			output: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: networkDefaultIP}, // Fallback logic picks this
				{Type: v1.NodeHostName, Address: nodeName},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set global config for this test case
			cfg.NodeExcludeIPRanges = tt.excludeList

			ips, err := getNodeAddresses(tt.node, tt.vmi)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("[%s] expected error containing %q, but got nil", tt.name, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("[%s] expected error containing %q, but got: %v", tt.name, tt.wantErr, err)
				}
				// If we expected an error and got it, we are done with this test case
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !reflect.DeepEqual(ips, tt.output) {
				t.Errorf("Mismatch!\nExpected: %+v\nActual:   %+v", tt.output, ips)
			}
		})
	}
}

func Test_getManagementNetworks(t *testing.T) {
	// Setup a reusable VMI with mixed network types
	vmi := &kubevirtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "test-vmi"},
		Spec: kubevirtv1.VirtualMachineInstanceSpec{
			Networks: []kubevirtv1.Network{
				{
					Name: "nic-0",
					NetworkSource: kubevirtv1.NetworkSource{
						// In KubeVirt API, the field is Multus, and the type is MultusNetwork
						Multus: &kubevirtv1.MultusNetwork{
							NetworkName: "default/management-vlan",
						},
					},
				},
				{
					Name: "nic-1",
					NetworkSource: kubevirtv1.NetworkSource{
						Multus: &kubevirtv1.MultusNetwork{
							NetworkName: "default/data-vlan",
						},
					},
				},
				{
					Name: "nic-pod",
					NetworkSource: kubevirtv1.NetworkSource{
						// In KubeVirt API, the field is Pod, and the type is PodNetwork
						Pod: &kubevirtv1.PodNetwork{},
					},
				},
			},
		},
	}

	t.Run("when ManagementNetwork is configured, return only the matching name", func(t *testing.T) {
		cfg := config.GetConfig()
		cfg.ManagementNetwork = "default/management-vlan"
		defer func() { cfg.ManagementNetwork = "" }()

		result := getManagementNetworks(vmi)

		// Check if we got exactly one result and it's the right one
		// This also helps catch the "ghost empty string" bug if you haven't fixed it in the main code yet
		foundCorrect := false
		for _, name := range result {
			if name == "nic-0" {
				foundCorrect = true
			}
			if name == "" {
				t.Error("found empty string in result; check make([]string, 1) in your function")
			}
		}

		if !foundCorrect || len(result) != 1 {
			t.Fatalf("expected only [nic-0], got %v", result)
		}
	})

	t.Run("when ManagementNetwork is empty, return all multus networks", func(t *testing.T) {
		cfg := config.GetConfig()
		cfg.ManagementNetwork = ""

		result := getManagementNetworks(vmi)

		// Expected nic-0 and nic-1 (nic-pod should be ignored by your code)
		count := 0
		for _, name := range result {
			if name == "nic-0" || name == "nic-1" {
				count++
			}
			if name == "nic-pod" {
				t.Error("result contains nic-pod, but it should only contain multus networks")
			}
		}

		if count != 2 {
			t.Errorf("expected 2 multus networks, got %d in result %v", count, result)
		}
	})

	t.Run("return empty list if no multus networks exist", func(t *testing.T) {
		vmiPodOnly := &kubevirtv1.VirtualMachineInstance{
			Spec: kubevirtv1.VirtualMachineInstanceSpec{
				Networks: []kubevirtv1.Network{
					{
						Name: "only-pod",
						NetworkSource: kubevirtv1.NetworkSource{
							Pod: &kubevirtv1.PodNetwork{},
						},
					},
				},
			},
		}

		result := getManagementNetworks(vmiPodOnly)
		if len(result) != 0 {
			t.Errorf("expected 0 networks, got %d: %v", len(result), result)
		}
	})
}
