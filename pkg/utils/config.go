package utils

import (
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/harvester/harvester-cloud-provider/pkg/config" // Import data store
)

func GetCurrentConfigString(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	return fmt.Sprintf("--%s=%v --%s=%v --%s=%v --%s=%v --%s=%v --%s=%v --%s=%v",
		FlagClusterName, cfg.ClusterName,
		FlagCloudProviderControllers, cfg.CloudProviderControllers,
		FlagManagementNetwork, cfg.ManagementNetwork,
		FlagNodeIPCIDR, cfg.NodeIPCIDR,
		FlagAllowSpecifyLoadbalancerNetwork, cfg.AllowSpecifyLoadBalancerNetwork,
		FlagDisableVmiController, cfg.DisableVMIController,
		FlagShowFullHelpOnError, cfg.ShowFullHelpOnError)
}

// syncAndValidateHarvesterConfig bridges the gap between the K8s framework and Harvester needs.
//
// WHY:
// The command flags are registered and processed by the underlying 'cloud-provider'
// framework rather than our direct logic. The framework does not expose these
// flags directly to the entire application; they are typically passed only to
// specific plugins as needed. To log or verify these values globally before
// the provider starts, this manual fetch is the most reliable method.
//
// CRITICALITY (Identity & Optional Network Precision):
//  1. Cluster Name: Critical for identity. Defaults lead to resource collisions.
//  2. Management Network (Optional): If provided, defines the specific Harvester
//     network for node traffic. If omitted, discovery follows default logic.
//  3. Node IP CIDR (Optional): If provided, acts as a deterministic filter for
//     IP selection (vital for Dual-Stack, Multi-Home). If omitted, all valid IPs are candidates.
//
// VALIDATION POLICY (Strictly Optional, Strictly Validated):
// To ensure environment stability, this function treats networking flags as
// "Opt-in for Precision." They are not required to start the provider, but
// if present, they must be correct:
//
// The provider will return an error and terminate (Fail-Fast) only if:
//   - A provided --management-network name is malformed.
//   - A provided --node-ip-cidr contains syntax errors, restricted ranges
//     (loopback/link-local), or non-unicast addresses.
//
// This prevents the provider from falling back to "guessing" when the user
// has explicitly expressed a configuration intent.
//
// DEPLOYMENT NOTE:
//
//   - Direct Deployment: Ensure container args include "--cluster-name=a-unique-name".
//
//   - Helm Chart Deployment: Ensure the following configuration is set (which
//     is eventually converted into the deployment container args):
//
//     rkeConfig:
//     chartValues:
//     harvester-cloud-provider:
//     global:
//     cattle:
//     clusterName: a-unique-name
//
// HOW:
// We fetch values here via RunE to ensure global synchronization before the
// framework branches into multi-threaded controller execution.
//
// TROUBLESHOOTING:
// We log these values clearly at boot time to allow for immediate verification
// of the runtime configuration in production logs.
func SyncAndValidateHarvesterConfig(cmd *cobra.Command, cfg *config.Config) error {
	flags := cmd.Flags()

	// 1. Helper to retrieve flags safely without panicking.
	// Errors here indicate a code-level mismatch (flag not registered).
	getStr := func(name string) (string, error) {
		val, err := flags.GetString(name)
		if err != nil {
			return "", fmt.Errorf("internal error: flag %q not registered: %w", name, err)
		}
		return val, nil
	}
	getBool := func(name string) (bool, error) {
		val, err := flags.GetBool(name)
		if err != nil {
			return false, fmt.Errorf("internal error: flag %q not registered: %w", name, err)
		}
		return val, nil
	}
	getStrSlice := func(name string) ([]string, error) {
		val, err := flags.GetStringSlice(name)
		if err != nil {
			return nil, fmt.Errorf("internal error: flag %q not registered as stringSlice: %w", name, err)
		}
		return val, nil
	}

	// 2. Sync values and check for registration errors
	var err error
	if cfg.ClusterName, err = getStr(FlagClusterName); err != nil {
		return err
	}
	if cfg.ManagementNetwork, err = getStr(FlagManagementNetwork); err != nil {
		return err
	}
	if cfg.NodeIPCIDR, err = getStr(FlagNodeIPCIDR); err != nil {
		return err
	}
	if cfg.DisableVMIController, err = getBool(FlagDisableVmiController); err != nil {
		return err
	}
	if cfg.AllowSpecifyLoadBalancerNetwork, err = getBool(FlagAllowSpecifyLoadbalancerNetwork); err != nil {
		return err
	}
	if cfg.ShowFullHelpOnError, err = getBool(FlagShowFullHelpOnError); err != nil {
		return err
	}

	controllerSlice, err := getStrSlice(FlagCloudProviderControllers)
	if err != nil {
		return err
	}
	cfg.CloudProviderControllers = strings.Join(controllerSlice, ",")

	// 3. Logic Validation: ClusterName (Warning only, as per existing logic)
	if cfg.ClusterName == "" || cfg.ClusterName == DefaultGuestClusterName {
		logrus.Warnf("%s WARNING: the flag --%s is using an empty or default value (current value: %q). "+
			"A unique cluster name is required for remote systems to identify this cluster.",
			HarvesterCloudProvider, FlagClusterName, cfg.ClusterName)
	}

	// 4. Strict Validation: Management Network
	// If the user provided a value, it MUST be valid. We no longer drop and continue.
	if cfg.ManagementNetwork != "" {
		normalized, err := NormalizeNetworkName(NetworkTypeManagement, cfg.ManagementNetwork)
		if err != nil {
			return fmt.Errorf("invalid configuration for --%s: %w", FlagManagementNetwork, err)
		}
		cfg.ManagementNetwork = normalized
	}

	// 5. Strict Validation: Node IP CIDR
	// Fail early if the CIDR format or range is invalid.
	if cfg.NodeIPCIDR != "" {
		if err := ValidateCIDRFilter(cfg.NodeIPCIDR); err != nil {
			return fmt.Errorf("invalid configuration for --%s: %w", FlagNodeIPCIDR, err)
		}
	}

	logrus.Infof("%s effective configurations: %s", HarvesterCloudProvider, GetCurrentConfigString(cfg))
	return nil
}

// CheckFlagShowFullHelpOnError determines if the user wants the standard, concise
// or the full cloud-provider framework help dump.
//
// DEBUGGING TIP: If you need to see the full list of supported flags,
// manually edit the deployment/statefulset and add the following argument:
//
//	args:
//	  - --show-full-help-on-error=true
//
// This is intentionally not exposed in the standard Helm Chart to keep
// the user interface clean and prevent accidentally flooding production logs.
func CheckFlagShowFullHelpOnError(cmd *cobra.Command, cfg *config.Config) {
	showFullHelp := false
	for _, arg := range os.Args {
		// We check os.Args directly because this happens before cmd.Execute()
		if arg == "--"+FlagShowFullHelpOnError+"=true" || arg == "--"+FlagShowFullHelpOnError {
			showFullHelp = true
			break
		}
	}

	if !showFullHelp {
		// Silence the massive help wall and internal Cobra error printing
		// so we can provide our own high-signal troubleshooting logs.
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
	}
	cfg.ShowFullHelpOnError = showFullHelp
}

// HandleStartupError provides a clean, user-friendly exit when the application fails to start.
// Since the harvester-cloud-provider Helm chart allows users to inject custom flags via '.Values.extraArgs',
// it is essential to check if the failure is due to an unknown flag. This ensures users
// get clear feedback regarding typos in their Helm configuration rather than a generic crash.
func HandleStartupError(cfg *config.Config, err error) {
	errStr := err.Error()

	// Visual boundary to separate the error from standard container logs
	logrus.Errorf("=============================================================================================")
	logrus.Errorf("FATAL: %s failed to start", HarvesterCloudProvider)

	// Detect flag-related errors (typos, unsupported flags, etc.)
	isFlagError := strings.Contains(errStr, "unknown flag") ||
		strings.Contains(errStr, "flag provided but not defined") ||
		strings.Contains(errStr, "bad flag syntax")

	logrus.Errorf("Error detail: %v", err)

	if isFlagError {
		logrus.Errorf("Potential cause: invalid flag(s) detected.")
		logrus.Errorf("Helm check: verify the list in '.Values.extraArgs' within your values.yaml.")
		logrus.Errorf("Action: ensure flags use the '--name=value' format and are supported by this version.")

		if !cfg.ShowFullHelpOnError {
			logrus.Infof("Hint: set '--show-full-help-on-error=true' to see all valid flags in the logs.")
		}
	}

	// other logic-based errors (RBAC, Network, API, etc.)
	logrus.Errorf("=============================================================================================")

	// Always exit with a non-zero code to trigger a Pod restart/error state
	os.Exit(1)
}
