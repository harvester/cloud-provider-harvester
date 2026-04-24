package utils

import (
	"fmt"
	"github.com/spf13/cobra"
	"reflect"
	"testing"

	"github.com/harvester/harvester-cloud-provider/pkg/config"
)

func Test_SyncAndValidateHarvesterConfig(t *testing.T) {
	// 1. Save original state and setup restore helper
	originalConfig := *config.GetConfig()
	restoreTo := func(c config.Config) {
		*config.GetConfig() = c
	}
	defer restoreTo(originalConfig)

	tests := []struct {
		name       string
		inputFlags map[string]interface{}
		sliceFlags map[string][]string
		expected   config.Config
		wantErr    bool
	}{
		// --- HAPPY CASES ---
		{
			name: "Full configuration",
			inputFlags: map[string]interface{}{
				FlagClusterName:                     "prod-cluster",
				FlagManagementNetwork:               "harvester-public/vlan100",
				FlagNodeIPCIDR:                      "192.168.0.0/24",
				FlagDisableVmiController:            true,
				FlagAllowSpecifyLoadbalancerNetwork: true,
				FlagShowFullHelpOnError:             true,
			},
			sliceFlags: map[string][]string{
				FlagCloudProviderControllers: {"node", "loadbalancer"},
			},
			wantErr: false,
			expected: config.Config{
				ClusterName:                     "prod-cluster",
				CloudProviderControllers:        "node,loadbalancer",
				ManagementNetwork:               "harvester-public/vlan100",
				NodeIPCIDR:                      "192.168.0.0/24",
				DisableVMIController:            true,
				AllowSpecifyLoadBalancerNetwork: true,
				ShowFullHelpOnError:             true,
			},
		},
		{
			name: "IPv6 CIDR Support",
			inputFlags: map[string]interface{}{
				FlagClusterName: "ipv6-cluster",
				FlagNodeIPCIDR:  "2001:db8::/32",
			},
			wantErr: false,
			expected: config.Config{
				ClusterName: "ipv6-cluster",
				NodeIPCIDR:  "2001:db8::/32",
			},
		},
		{
			name: "Dual-stack CIDR ",
			inputFlags: map[string]interface{}{
				FlagClusterName: "dual-stack-cluster",
				FlagNodeIPCIDR:  "192.168.0.0/24,2001:db8::/32",
			},
			wantErr: false,
			expected: config.Config{
				ClusterName: "dual-stack-cluster",
				NodeIPCIDR:  "192.168.0.0/24,2001:db8::/32",
			},
		},
		{
			name: "Management Network default namespace auto-append",
			inputFlags: map[string]interface{}{
				FlagClusterName:       "net-cluster",
				FlagManagementNetwork: "vlan100", // No namespace provided
			},
			wantErr: false,
			expected: config.Config{
				ClusterName:       "net-cluster",
				ManagementNetwork: "default/vlan100",
			},
		},
		{
			name: "Default values",
			inputFlags: map[string]interface{}{
				FlagClusterName: "minimal-cluster",
			},
			wantErr: false,
			expected: config.Config{
				ClusterName: "minimal-cluster",
				// Other fields remain zero-valued
			},
		},
		{
			name: "Explicit cluster-name flag with empty string",
			inputFlags: map[string]interface{}{
				FlagClusterName: "",
			},
			wantErr: false,
			expected: config.Config{
				ClusterName: "",
				// Other fields remain zero-valued
			},
		},
		{
			name:       "No input flag",
			inputFlags: map[string]interface{}{},
			wantErr:    false,
			expected:   config.Config{
				// note: the cloud-provider framework injects "kubernetes" as cluster-name
				// but on test code this case is not covered
			},
		},
		// --- ERROR CASES ---
		{
			name: "Error: Invalid CIDR",
			inputFlags: map[string]interface{}{
				FlagNodeIPCIDR: "999.999.999.999/invalid",
			},
			wantErr: true,
		},
		{
			name: "Error: Invalid CIDR, IPv4 local host",
			inputFlags: map[string]interface{}{
				FlagNodeIPCIDR: "127.0.0.0/8",
			},
			wantErr: true,
		},
		{
			name: "Error: Invalid CIDR, IPv6 link local",
			inputFlags: map[string]interface{}{
				FlagNodeIPCIDR: "fe80::/10",
			},
			wantErr: true,
		},
		{
			name: "Error: Management Network with too many segments",
			inputFlags: map[string]interface{}{
				FlagManagementNetwork: "namespace/network/invalid-segment",
			},
			wantErr: true,
		},
		{
			name: "Error: IPv4 CIDR with trailing garbage",
			inputFlags: map[string]interface{}{
				FlagNodeIPCIDR: "192.168.1.0/24-invalid-string",
			},
			wantErr: true,
		},
		{
			name: "Error: IPv6 CIDR with invalid range",
			inputFlags: map[string]interface{}{
				FlagNodeIPCIDR: "2001:db8::/129", // IPv6 max is 128
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 2. Wipe global config for isolation
			restoreTo(config.Config{})
			targetCfg := config.GetConfig()

			// 3. Setup Command and register flags
			cmd := &cobra.Command{}
			f := cmd.Flags()
			f.String(FlagClusterName, "", "")
			f.String(FlagManagementNetwork, "", "")
			f.String(FlagNodeIPCIDR, "", "")
			f.Bool(FlagDisableVmiController, false, "")
			f.Bool(FlagAllowSpecifyLoadbalancerNetwork, false, "")
			f.Bool(FlagShowFullHelpOnError, false, "")
			f.StringSlice(FlagCloudProviderControllers, []string{}, "")

			// 4. Inject values
			for k, v := range tt.inputFlags {
				_ = f.Set(k, fmt.Sprintf("%v", v))
			}
			for k, v := range tt.sliceFlags {
				for _, val := range v {
					_ = f.Set(k, val)
				}
			}

			// 5. Execute
			err := SyncAndValidateHarvesterConfig(cmd, targetCfg)

			// 6. Assertions
			if tt.wantErr {
				if err == nil {
					t.Errorf("[%s] expected error but got nil", tt.name)
				}
				return
			}

			if err != nil {
				t.Errorf("[%s] unexpected error: %v", tt.name, err)
				return
			}

			// 7. Deep Comparison
			actual := *targetCfg
			if !reflect.DeepEqual(actual, tt.expected) {
				t.Errorf("[%s] content mismatch!\nExpected: %+v\nActual:   %+v",
					tt.name, tt.expected, actual)
			}
		})
	}
}
