package utils

import (
	"fmt"
	"testing"

	"github.com/sirupsen/logrus/hooks/test"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"

	"github.com/harvester/harvester-cloud-provider/pkg/config"
)

func getCommandAndFlag() (*cobra.Command, *flag.FlagSet) {
	cmd := &cobra.Command{}
	f := cmd.Flags()
	f.String(FlagClusterName, "", "") // name, value, usage
	f.String(FlagManagementNetwork, "", "")
	f.String(FlagNodeIPCIDR, "", "")
	f.Bool(FlagDisableVmiController, false, "")
	f.Bool(FlagShowFullHelpOnError, false, "")
	f.StringSlice(FlagCloudProviderControllers, []string{}, "")
	f.StringSlice(FlagNodeExcludeIPRanges, []string{}, "")
	f.Bool(FlagDisableAnnotationAlphaProvidedIPAddr, false, "")

	return cmd, f
}

func Test_SyncAndValidateHarvesterConfig(t *testing.T) {
	type expectedResult struct {
		config                     config.Config
		lenExcludeIPRangesPrefixes int
		lenNodeIPCIDRPPrefixes     int
	}
	// helper function to validate the input and generated config and internal data
	validConfig := func(expected *expectedResult, actual *config.Config) error {
		mismatch := "%s mismatch, expected %v got %v"
		if expected.config.ClusterName != actual.ClusterName {
			return fmt.Errorf(mismatch, "ClusterName", expected.config.ClusterName, actual.ClusterName)
		}
		if expected.config.ManagementNetwork != actual.ManagementNetwork {
			return fmt.Errorf(mismatch, "ManagementNetwork", expected.config.ManagementNetwork, actual.ManagementNetwork)
		}
		if expected.config.NodeIPCIDR != actual.NodeIPCIDR {
			return fmt.Errorf(mismatch, "NodeIPCIDR", expected.config.NodeIPCIDR, actual.NodeIPCIDR)
		}
		if expected.lenExcludeIPRangesPrefixes != len(actual.GetNodeExcludeIPPrefixes()) {
			return fmt.Errorf(mismatch, "lenExcludeIPRangesPrefixes", expected.lenExcludeIPRangesPrefixes, len(actual.GetNodeExcludeIPPrefixes()))
		}
		if expected.lenNodeIPCIDRPPrefixes != len(actual.GetNodeIPCIDRPrefixes()) {
			return fmt.Errorf(mismatch, "lenNodeIPCIDRPPrefixes", expected.lenNodeIPCIDRPPrefixes, len(actual.GetNodeIPCIDRPrefixes()))
		}
		return nil
	}

	tests := []struct {
		name       string
		inputFlags map[string]interface{}
		sliceFlags map[string][]string
		expected   expectedResult
		wantErr    bool
	}{
		{
			name: "Full configuration",
			inputFlags: map[string]interface{}{
				FlagClusterName:          "prod-cluster",
				FlagManagementNetwork:    "harvester-public/vlan100",
				FlagNodeIPCIDR:           "192.168.0.0/24",
				FlagDisableVmiController: true,
				FlagShowFullHelpOnError:  true,
			},
			sliceFlags: map[string][]string{
				FlagCloudProviderControllers: {"node", "loadbalancer"},
				FlagNodeExcludeIPRanges:      {},
			},
			wantErr: false,
			expected: expectedResult{
				config: config.Config{
					ClusterName:              "prod-cluster",
					CloudProviderControllers: "node,loadbalancer",
					ManagementNetwork:        "harvester-public/vlan100",
					NodeIPCIDR:               "192.168.0.0/24",
					NodeExcludeIPRanges:      []string{},
					DisableVMIController:     true,
					ShowFullHelpOnError:      true,
				},
				lenNodeIPCIDRPPrefixes: 1,
			},
		},
		{
			name: "IPv6 CIDR Support",
			inputFlags: map[string]interface{}{
				FlagClusterName: "ipv6-cluster",
				FlagNodeIPCIDR:  "2001:db8::/32",
			},
			wantErr: false,
			expected: expectedResult{
				config: config.Config{
					ClusterName: "ipv6-cluster",
					NodeIPCIDR:  "2001:db8::/32",
				},
				lenNodeIPCIDRPPrefixes: 1,
			},
		},
		{
			name: "Dual-stack CIDR ",
			inputFlags: map[string]interface{}{
				FlagClusterName: "dual-stack-cluster",
				FlagNodeIPCIDR:  "192.168.0.0/24,2001:db8::/32",
			},
			wantErr: false,
			expected: expectedResult{
				config: config.Config{
					ClusterName: "dual-stack-cluster",
					NodeIPCIDR:  "192.168.0.0/24,2001:db8::/32",
				},
				lenNodeIPCIDRPPrefixes: 2,
			},
		},
		{
			name: "Dual-stack CIDR, spaces are stripped",
			inputFlags: map[string]interface{}{
				FlagClusterName: "dual-stack-cluster",
				FlagNodeIPCIDR:  "  192.168.0.0/24,   2001:db8::/32  ",
			},
			wantErr: false,
			expected: expectedResult{
				config: config.Config{
					ClusterName: "dual-stack-cluster",
					NodeIPCIDR:  "192.168.0.0/24,2001:db8::/32", // trim the spaces
				},
				lenNodeIPCIDRPPrefixes: 2,
			},
		},
		{
			name: "Single-stack CIDR, spaces are stripped",
			inputFlags: map[string]interface{}{
				FlagClusterName: "single-stack-cluster",
				FlagNodeIPCIDR:  "  192.168.0.0/24,   ",
			},
			wantErr: false,
			expected: expectedResult{
				config: config.Config{
					ClusterName: "single-stack-cluster",
					NodeIPCIDR:  "192.168.0.0/24", // trim the spaces
				},
				lenNodeIPCIDRPPrefixes: 1,
			},
		},
		{
			name: "Multi FlagNodeExcludeIPRanges ",
			inputFlags: map[string]interface{}{
				FlagClusterName: "dual-stack-cluster",
				FlagNodeIPCIDR:  "192.168.0.0/24,2001:db8::/32",
			},
			sliceFlags: map[string][]string{
				FlagNodeExcludeIPRanges: {
					"192.168.0.0/24",
					"192.168.0.255",
					"192.168.0.254",
					"2001:db8::/32",
				},
			},
			wantErr: false,
			expected: expectedResult{
				config: config.Config{
					ClusterName: "dual-stack-cluster",
					NodeIPCIDR:  "192.168.0.0/24,2001:db8::/32",
				},
				lenNodeIPCIDRPPrefixes:     2,
				lenExcludeIPRangesPrefixes: 4,
			},
		},
		{
			name: "Management Network default namespace auto-append",
			inputFlags: map[string]interface{}{
				FlagClusterName:       "net-cluster",
				FlagManagementNetwork: "vlan100", // No namespace provided
			},
			wantErr: false,
			expected: expectedResult{
				config: config.Config{
					ClusterName:       "net-cluster",
					ManagementNetwork: "default/vlan100",
				},
			},
		},
		{
			name: "Default values",
			inputFlags: map[string]interface{}{
				FlagClusterName: "minimal-cluster",
			},
			wantErr: false,
			expected: expectedResult{
				config: config.Config{
					ClusterName: "minimal-cluster",
					// Other fields remain zero-valued
				},
			},
		},
		{
			name: "Explicit cluster-name flag with empty string",
			inputFlags: map[string]interface{}{
				FlagClusterName: "",
			},
			wantErr: false,
			expected: expectedResult{
				config: config.Config{
					ClusterName: "",
					// Other fields remain zero-valued
				},
			},
		},
		{
			name:       "No input flag",
			inputFlags: map[string]interface{}{},
			wantErr:    false,
			expected: expectedResult{
				config: config.Config{
					// note: the cloud-provider framework injects "kubernetes" as cluster-name
					// but on test code this case is not covered
				},
			},
		},
		{
			name: "Error: Invalid CIDR",
			inputFlags: map[string]interface{}{
				FlagClusterName: "test",
				FlagNodeIPCIDR:  "999.999.999.999/invalid",
			},
			wantErr: true,
		},
		{
			name: "Error: Invalid CIDR, IPv4 local host",
			inputFlags: map[string]interface{}{
				FlagClusterName: "test",
				FlagNodeIPCIDR:  "127.0.0.0/8",
			},
			wantErr: true,
		},
		{
			name: "Error: Invalid CIDR, IPv6 link local",
			inputFlags: map[string]interface{}{
				FlagClusterName: "test",
				FlagNodeIPCIDR:  "fe80::/10",
			},
			wantErr: true,
		},
		{
			name: "Error: Management Network with too many segments",
			inputFlags: map[string]interface{}{
				FlagClusterName:       "test",
				FlagManagementNetwork: "namespace/network/invalid-segment",
			},
			wantErr: true,
		},
		{
			name: "Error: IPv4 CIDR with trailing garbage",
			inputFlags: map[string]interface{}{
				FlagClusterName: "test",
				FlagNodeIPCIDR:  "192.168.1.0/24-invalid-string",
			},
			wantErr: true,
		},
		{
			name: "Error: IPv6 CIDR with invalid range",
			inputFlags: map[string]interface{}{
				FlagClusterName: "test",
				FlagNodeIPCIDR:  "2001:db8::/129", // IPv6 max is 128
			},
			wantErr: true,
		},
		{
			name: "Error: More than two CIDRs",
			inputFlags: map[string]interface{}{
				FlagClusterName: "test",
				FlagNodeIPCIDR:  "192.168.0.0/24,192.168.1.0/24,2001:db8::/32",
			},
			wantErr: true,
		},
		{
			name: "Error: Two IPv4 CIDRs",
			inputFlags: map[string]interface{}{
				FlagClusterName: "test",
				FlagNodeIPCIDR:  "192.168.0.0/24,192.168.1.0/24",
			},
			wantErr: true,
		},
		{
			name: "Error: Two IPv6 CIDRs",
			inputFlags: map[string]interface{}{
				FlagClusterName: "test",
				FlagNodeIPCIDR:  "2001:db8::/32,2002:db8::/32",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, f := getCommandAndFlag()
			for k, v := range tt.inputFlags {
				_ = f.Set(k, fmt.Sprintf("%v", v))
			}
			for k, v := range tt.sliceFlags {
				for _, val := range v {
					_ = f.Set(k, val)
				}
			}

			targetCfg := config.Config{}

			err := SyncAndValidateHarvesterConfig(cmd, &targetCfg)

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

			if err := validConfig(&tt.expected, &targetCfg); err != nil {
				t.Errorf("[%s] error: %s", tt.name, err.Error())
			}
		})
	}
}

func Test_normalizeAndWarnClusterName(t *testing.T) {
	// Setup the null logger and hook to capture logs
	logger, hook := test.NewNullLogger()

	tests := []struct {
		name           string
		input          string
		expectedResult string
		expectedLogs   int // Number of warnings expected
	}{
		{
			name:           "Empty string (after trimming)",
			input:          "  \"\"  ",
			expectedResult: "",
			expectedLogs:   2, // 1 for trimming, 1 for being empty
		},
		{
			name:           "Default value",
			input:          DefaultGuestClusterName,
			expectedResult: DefaultGuestClusterName,
			expectedLogs:   1, // Warning for using default
		},
		{
			name:           "Valid normal value",
			input:          "good-guest-cluster",
			expectedResult: "good-guest-cluster",
			expectedLogs:   0, // No warnings
		},
		{
			name:           "Invalid DNS format (uppercase and dots)",
			input:          "Invalid.Cluster.Name",
			expectedResult: "Invalid.Cluster.Name",
			expectedLogs:   1, // Warning for DNS validation failure
		},
		{
			name:           "Value requiring trimming (quotes and backticks)",
			input:          "\"`clean-me`\"",
			expectedResult: "clean-me",
			expectedLogs:   1, // Warning for trimming
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook.Reset()
			result := normalizeAndWarnClusterName(logger, tt.input)
			if result != tt.expectedResult {
				t.Errorf("Result mismatch: expected %q, got %q", tt.expectedResult, result)
			}
			if len(hook.Entries) != tt.expectedLogs {
				t.Errorf("Log count mismatch: expected %d warnings, got %d", tt.expectedLogs, len(hook.Entries))
			}
		})
	}
}
