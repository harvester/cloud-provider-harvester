package ccm

import (
	"encoding/json"
	"net/netip"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cloud-provider/api"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester-cloud-provider/pkg/config"
	utils "github.com/harvester/harvester-cloud-provider/pkg/utils"
)

const (
	testNamespace = "default"
	nodeName      = "test"

	mgmtNetwork = "default/mgmt-vlan"

	nic0 = "nic-0"

	networkDefault100IP = "192.168.100.10"
	network120IP        = "192.168.120.10"
	network130IP        = "192.168.130.10"
	network130IPStorage = "192.168.130.12"

	subnetDefault100 = "192.168.100.0/24"
	subnet120        = "192.168.120.0/24"
	subnet130        = "192.168.130.0/24"
	subnet200        = "192.168.200.0/24"
)

func getCommandAndFlag(mgmtNetwork, cidrRanges string, excludeList []string) (*cobra.Command, *flag.FlagSet) {
	cmd := &cobra.Command{}
	f := cmd.Flags()
	f.String(utils.FlagClusterName, "test", "") // name, value, usage
	f.String(utils.FlagManagementNetwork, mgmtNetwork, "")
	f.String(utils.FlagNodeIPCIDR, cidrRanges, "")
	f.Bool(utils.FlagDisableVmiController, false, "")
	f.Bool(utils.FlagShowFullHelpOnError, false, "")
	f.StringSlice(utils.FlagCloudProviderControllers, []string{}, "")
	f.StringSlice(utils.FlagNodeExcludeIPRanges, excludeList, "")
	f.Bool(utils.FlagDisableAnnotationAlphaProvidedIPAddr, false, "")

	return cmd, f
}

func Test_getNodeAddresses(t *testing.T) {
	mustMarshal := func(v interface{}) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("failed to marshal JSON for test setup: %v", err)
		}
		return string(b)
	}

	stubMultusVMI := func(nicName, multusName string, ips []string) *kubevirtv1.VirtualMachineInstance {
		return &kubevirtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      nodeName,
				Namespace: testNamespace,
			},
			Spec: kubevirtv1.VirtualMachineInstanceSpec{
				Networks: []kubevirtv1.Network{
					{
						Name: nicName,
						NetworkSource: kubevirtv1.NetworkSource{
							Multus: &kubevirtv1.MultusNetwork{
								NetworkName: multusName,
							},
						},
					},
				},
			},
			Status: kubevirtv1.VirtualMachineInstanceStatus{
				Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{
					{
						Name: nicName,
						IPs:  ips,
					},
				},
			},
		}
	}

	tests := []struct {
		name              string
		node              *v1.Node
		vmi               *kubevirtv1.VirtualMachineInstance
		managementNetwork string
		cidrRanges        string
		excludeList       []string // config --node-exclude-ip-ranges
		output            []v1.NodeAddress
		wantErr           string
	}{
		{
			name:              "Priority 1: Provided IP Wins (Multus) 1",
			managementNetwork: mgmtNetwork,
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
					Annotations: map[string]string{
						// key: "alpha.kubernetes.io/provided-node-ip",
						api.AnnotationAlphaProvidedIPAddr: network120IP,
					},
				},
			},
			vmi: stubMultusVMI(nic0, mgmtNetwork, []string{networkDefault100IP, network120IP}),
			output: []v1.NodeAddress{
				{Type: v1.NodeExternalIP, Address: networkDefault100IP}, // result follows interface ip listed order
				{Type: v1.NodeInternalIP, Address: network120IP},
				{Type: v1.NodeHostName, Address: nodeName},
			},
		},
		{
			name:              "Priority 1: Provided IP Wins (Multus) 2",
			managementNetwork: mgmtNetwork,
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
					Annotations: map[string]string{
						// key: "alpha.kubernetes.io/provided-node-ip",
						api.AnnotationAlphaProvidedIPAddr: network120IP,
					},
				},
			},
			vmi: stubMultusVMI(nic0, mgmtNetwork, []string{network120IP, networkDefault100IP}),
			output: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: network120IP}, // result follows interface ip listed order
				{Type: v1.NodeExternalIP, Address: networkDefault100IP},
				{Type: v1.NodeHostName, Address: nodeName},
			},
		},
		{
			name:              "Priority 3: Logic: Exclusion via Node Annotation (Fallback Mode)",
			managementNetwork: mgmtNetwork,
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
					Annotations: map[string]string{
						// key: "cloudprovider.harvesterhci.io/additional-internal-ips"
						utils.KeyAdditionalInternalIPs: mustMarshal([]string{network120IP}),
					},
				},
			},
			vmi: stubMultusVMI(nic0, mgmtNetwork, []string{networkDefault100IP, network120IP}),
			output: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: networkDefault100IP},
				// `network120IP` is excluded from InternalIP and ExternalIP
				{Type: v1.NodeHostName, Address: nodeName},
			},
		},
		{
			name:       "Priority 2: CIDR Mode (Strict Filtering)",
			cidrRanges: subnet130,
			node:       &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName}},
			vmi:        stubMultusVMI(nic0, "any/net", []string{networkDefault100IP, network130IP}),
			output: []v1.NodeAddress{
				{Type: v1.NodeExternalIP, Address: networkDefault100IP},
				{Type: v1.NodeInternalIP, Address: network130IP},
				{Type: v1.NodeHostName, Address: nodeName},
			},
		},
		{
			name:        "Priority 2: CIDR Mode (Strict Filtering), storage ip is also in node ip subnet which is filtered",
			cidrRanges:  subnet130,
			excludeList: []string{network130IPStorage},
			node:        &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName}},
			vmi:         stubMultusVMI(nic0, "any/net", []string{network130IP, network130IPStorage}),
			output: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: network130IP},
				// {Type: v1.NodeInternalIP, Address: network130IPStorage}, // filtered from internal
				{Type: v1.NodeHostName, Address: nodeName},
			},
		},
		{
			name:        "Priority 2: CIDR Mode (Strict Filtering), exclude a sub group from cidr range",
			excludeList: []string{network120IP}, // a sub group of following cidr range
			cidrRanges:  "192.168.0.0/16",
			node:        &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName}},
			vmi:         stubMultusVMI(nic0, "mgmt", []string{networkDefault100IP, network120IP}),
			output: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: networkDefault100IP},
				// {Type: v1.NodeInternalIP, Address: network120IP}, // filtered from internal
				{Type: v1.NodeHostName, Address: nodeName},
			},
		},
		{
			name: "Priority 3: Fallback Discovery (Dual Stack)",
			node: &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName}},
			vmi:  stubMultusVMI(nic0, "default/none", []string{networkDefault100IP, "fd00::1"}),
			output: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: networkDefault100IP},
				{Type: v1.NodeInternalIP, Address: "fd00::1"},
				{Type: v1.NodeHostName, Address: nodeName},
			},
		},
		{
			name: "Failure: Invalid Annotation (Strict Mode - Mismatch results in all IPs are marked as ExternalIP, no InternalIP)",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
					Annotations: map[string]string{
						// key: "alpha.kubernetes.io/provided-node-ip",
						api.AnnotationAlphaProvidedIPAddr: "invalid-ip-string",
					},
				},
			},
			vmi: stubMultusVMI(nic0, "mgmt", []string{networkDefault100IP}),
			output: []v1.NodeAddress{
				{Type: v1.NodeExternalIP, Address: networkDefault100IP},
				{Type: v1.NodeHostName, Address: nodeName},
			},
		},
		{
			name:        "Offensive: node exclude list is so big that it excludes all available IPs",
			excludeList: []string{"192.168.0.0/16"},
			cidrRanges:  "192.168.0.0/16",
			node:        &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName}},
			vmi:         stubMultusVMI(nic0, "mgmt", []string{networkDefault100IP, network120IP}),
			output: []v1.NodeAddress{
				// {Type: v1.NodeExternalIP, Address: networkDefault100IP},
				// {Type: v1.NodeExternalIP, Address: network120IP},
				{Type: v1.NodeHostName, Address: nodeName},
			},
		},
		{
			name:       "Mismatch: node ip CIDR mismatch, all available IPs are marked as external",
			cidrRanges: subnet200,
			node:       &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName}},
			vmi:        stubMultusVMI(nic0, "mgmt", []string{networkDefault100IP, network120IP}),
			output: []v1.NodeAddress{
				{Type: v1.NodeExternalIP, Address: networkDefault100IP},
				{Type: v1.NodeExternalIP, Address: network120IP},
				{Type: v1.NodeHostName, Address: nodeName},
			},
		},
		{
			name: "Robustness: Ignore Pod Networks (Multus Required)",
			node: &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName}},
			vmi: &kubevirtv1.VirtualMachineInstance{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Networks: []kubevirtv1.Network{
						{Name: "pod", NetworkSource: kubevirtv1.NetworkSource{Pod: &kubevirtv1.PodNetwork{}}},
					},
				},
				Status: kubevirtv1.VirtualMachineInstanceStatus{
					Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{
						{Name: "pod", IPs: []string{networkDefault100IP}},
					},
				},
			},
			output: []v1.NodeAddress{
				{Type: v1.NodeHostName, Address: nodeName},
			},
		},
		{
			name: "Robustness: Multus network has no IPs (e.g., DHCP pending or qemu-agent down)",
			node: &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName}},
			// Passing an empty slice of IPs
			vmi: stubMultusVMI(nic0, "mgmt", []string{}),
			output: []v1.NodeAddress{
				{Type: v1.NodeHostName, Address: nodeName},
			},
		},
		{
			name: "Robustness: Multus network has only invalid/loopback IPs",
			node: &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName}},
			vmi:  stubMultusVMI("nic-1", "mgmt", []string{"127.0.0.1"}),
			output: []v1.NodeAddress{
				{Type: v1.NodeHostName, Address: nodeName},
			},
		},
		{
			name: "Robustness: VMI Status Not Reported Yet",
			node: &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName}},
			vmi: &kubevirtv1.VirtualMachineInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nodeName,
					Namespace: "no-vmi-status",
				},
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Networks: []kubevirtv1.Network{
						{
							Name: "mgmt",
							NetworkSource: kubevirtv1.NetworkSource{
								Multus: &kubevirtv1.MultusNetwork{NetworkName: "mgmt-vlan"},
							},
						},
					},
				},
				// Status exists but is empty or missing the interface entry
				Status: kubevirtv1.VirtualMachineInstanceStatus{
					Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{},
				},
			},
			output: []v1.NodeAddress{
				{Type: v1.NodeHostName, Address: nodeName},
			},
		},
		{
			name:    "Robustness: Nil VMI",
			node:    &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName}},
			vmi:     nil,
			wantErr: "VMI is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// simulate a command with input params
			cmd, _ := getCommandAndFlag(tt.managementNetwork, tt.cidrRanges, tt.excludeList)
			cfg := config.Config{}
			err := utils.SyncAndValidateHarvesterConfig(cmd, &cfg)
			if err != nil {
				t.Fatalf("[%s] unexpected error when init command and flag: %v", tt.name, err)
			}
			actual, err := getNodeAddresses(tt.node, tt.vmi, &cfg)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("[%s] expected err %q, got %v", tt.name, tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("[%s] unexpected error: %v", tt.name, err)
			}
			if !reflect.DeepEqual(actual, tt.output) {
				t.Errorf("[%s] Mismatch!\nExpected: %+v\nActual:   %+v", tt.name, tt.output, actual)
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
					Name: nic0,
					NetworkSource: kubevirtv1.NetworkSource{
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
						Pod: &kubevirtv1.PodNetwork{},
					},
				},
			},
		},
	}

	t.Run("when ManagementNetwork is configured, return only the matching name", func(t *testing.T) {
		newcfg := *config.GetConfig()
		newcfg.ManagementNetwork = "default/management-vlan"
		result := getManagementNetworks(vmi, &newcfg)

		foundCorrect := false
		for _, name := range result {
			if name == nic0 {
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
		newcfg := *config.GetConfig()
		newcfg.ManagementNetwork = ""
		result := getManagementNetworks(vmi, &newcfg)

		count := 0
		for _, name := range result {
			if name == nic0 || name == "nic-1" {
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
		newcfg := *config.GetConfig()
		newcfg.ManagementNetwork = ""

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

		result := getManagementNetworks(vmiPodOnly, &newcfg)
		if len(result) != 0 {
			t.Errorf("expected 0 networks, got %d: %v", len(result), result)
		}
	})
}

func Test_resolveNodeIPs(t *testing.T) {
	const (
		v4Str1  = "10.0.0.5"
		v4Str2  = "10.0.0.10"
		v6Str1  = "fd00::5"
		extStr1 = "1.1.1.1"
		extStr2 = "99.99.99.99"
	)

	var (
		v4Prefix = netip.MustParsePrefix("10.0.0.0/24")
		v6Prefix = netip.MustParsePrefix("fd00::/64")

		v4PrefixList  = []netip.Prefix{v4Prefix}
		dualStackList = []netip.Prefix{v4Prefix, v6Prefix}
	)

	v4addr1 := netip.MustParseAddr(v4Str1)
	v4addr2 := netip.MustParseAddr(v4Str2)
	v6addr := netip.MustParseAddr(v6Str1)
	extAddr := netip.MustParseAddr(extStr1)

	tests := []struct {
		name     string
		ips      []netip.Addr
		ctx      AddressContext
		expected []v1.NodeAddress
	}{
		{
			name: "Priority 1: ModeProvidedIP (Strict Match)",
			ips:  []netip.Addr{v4addr1, v4addr2},
			ctx: AddressContext{
				Mode:               ModeProvidedIP,
				ProvidedIP:         v4Str2,
				NodeIPCIDRPrefixes: v4PrefixList,
			},
			expected: []v1.NodeAddress{
				{Type: v1.NodeExternalIP, Address: v4Str1},
				{Type: v1.NodeInternalIP, Address: v4Str2},
			},
		},
		{
			name: "Priority 2: ModeNodeIPCIDR (Multi-IP Policy)",
			ips:  []netip.Addr{v4addr1, v4addr2, v6addr, extAddr},
			ctx: AddressContext{
				Mode:               ModeNodeIPCIDR,
				NodeIPCIDRPrefixes: dualStackList,
			},
			expected: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: v4Str1},
				{Type: v1.NodeInternalIP, Address: v4Str2},
				{Type: v1.NodeInternalIP, Address: v6Str1},
				{Type: v1.NodeExternalIP, Address: extStr1},
			},
		},
		{
			name: "Priority 3: ModeFallback (First-Win)",
			ips:  []netip.Addr{v4addr1, v4addr2, v6addr},
			ctx: AddressContext{
				Mode: ModeFallback,
			},
			expected: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: v4Str1},
				{Type: v1.NodeExternalIP, Address: v4Str2},
				{Type: v1.NodeInternalIP, Address: v6Str1},
			},
		},
		{
			name: "Legacy Masking: filterByMask in Fallback Mode",
			ips:  []netip.Addr{v4addr1, v4addr2},
			ctx: AddressContext{
				Mode:           ModeFallback,
				LegacyExcludes: []string{v4Str2},
			},
			expected: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: v4Str1},
			},
		},
		{
			name: "ModeProvidedIP: Invalid IP provided",
			ips:  []netip.Addr{v4addr1},
			ctx: AddressContext{
				Mode:       ModeProvidedIP,
				ProvidedIP: extStr2,
			},
			expected: []v1.NodeAddress{
				{Type: v1.NodeExternalIP, Address: v4Str1},
			},
		},
		{
			name: "Edge Case: Empty Input IPs",
			ips:  []netip.Addr{},
			ctx: AddressContext{
				Mode:               ModeNodeIPCIDR,
				NodeIPCIDRPrefixes: v4PrefixList, // Reused
			},
			expected: []v1.NodeAddress{},
		},
		{
			name: "Failure Case: ModeNodeIPCIDR with No Matching IPs as internal",
			ips:  []netip.Addr{extAddr},
			ctx: AddressContext{
				Mode:               ModeNodeIPCIDR,
				NodeIPCIDRPrefixes: v4PrefixList, // Reused
			},
			expected: []v1.NodeAddress{
				{Type: v1.NodeExternalIP, Address: extStr1},
			},
		},
		{
			name: "Failure Case: ModeProvidedIP with Family Mismatch",
			ips:  []netip.Addr{v6addr},
			ctx: AddressContext{
				Mode:       ModeProvidedIP,
				ProvidedIP: v4Str1,
			},
			expected: []v1.NodeAddress{
				{Type: v1.NodeExternalIP, Address: v6Str1},
			},
		},
		{
			name: "Logic Case: InternalIP Protection (Masking)",
			ips:  []netip.Addr{v4addr1},
			ctx: AddressContext{
				Mode:           ModeFallback,
				LegacyExcludes: []string{v4Str1},
			},
			expected: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: v4Str1},
			},
		},
		{
			name: "Edge Case: ModeFallback with Multiple Families",
			ips:  []netip.Addr{v4addr1, v4addr2, v6addr},
			ctx:  AddressContext{Mode: ModeFallback},
			expected: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: v4Str1},
				{Type: v1.NodeExternalIP, Address: v4Str2},
				{Type: v1.NodeInternalIP, Address: v6Str1},
			},
		},
		{
			name: "Failure Case: ModeNodeIPCIDR with empty prefix list",
			ips:  []netip.Addr{v4addr1},
			ctx: AddressContext{
				Mode:               ModeNodeIPCIDR,
				NodeIPCIDRPrefixes: []netip.Prefix{}, // Explicitly empty
			},
			expected: []v1.NodeAddress{
				{Type: v1.NodeExternalIP, Address: v4Str1},
			},
		},
		{
			name:     "Unknown mode, return nil",
			ips:      []netip.Addr{v4addr1},
			ctx:      AddressContext{},
			expected: []v1.NodeAddress{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidates := resolveNodeIPs(tt.ips, &tt.ctx)
			actual := candidates.ToNodeAddresses()

			if !reflect.DeepEqual(actual, tt.expected) {
				t.Errorf("resolveNodeIPs() [%s] failed\nGot:  %v\nWant: %v", tt.name, actual, tt.expected)
			}
		})
	}
}
