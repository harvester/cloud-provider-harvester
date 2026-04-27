package utils

import (
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/harvester/harvester-cloud-provider/pkg/config"
)

// GetCurrentConfigString serializes the current configuration into a string of
// CLI flags. The output is formatted with spaces between flags to allow
// users to copy-paste the configuration directly into a terminal for debugging.
func GetCurrentConfigString(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}

	return fmt.Sprintf("--%s=%v --%s=%v --%s=%v --%s=%v --%s=%v --%s=%v --%s=%v --%s=%v --%s=%v",
		FlagClusterName, cfg.ClusterName,
		FlagCloudProviderControllers, cfg.CloudProviderControllers,
		FlagManagementNetwork, cfg.ManagementNetwork,
		FlagNodeIPCIDR, cfg.NodeIPCIDR,
		// Use helper to ensure a comma-separated string instead of a Go slice [a b]
		FlagNodeExcludeIPRanges, cfg.GetNodeExcludeIPRangesCmdString(),
		FlagDisableAnnotationAlphaProvidedIPAddr, cfg.DisableAnnotationAlphaProvidedIPAddr,
		FlagLoadbalancerNetwork, cfg.LoadbalancerNetwork,
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
// Note: keep following indent
/*
   rkeConfig:
     chartValues:
       harvester-cloud-provider:
         global:
           cattle:
             clusterName: a-unique-name
*/
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
	var rawClusterName string
	if cfg.ClusterName, err = getStr(FlagClusterName); err != nil {
		return err
	}
	if rawClusterName, err = getStr(FlagClusterName); err != nil {
		return err
	}

	if cfg.ManagementNetwork, err = getStr(FlagManagementNetwork); err != nil {
		return err
	}
	if cfg.NodeIPCIDR, err = getStr(FlagNodeIPCIDR); err != nil {
		return err
	}
	if cfg.NodeExcludeIPRanges, err = getStrSlice(FlagNodeExcludeIPRanges); err != nil {
		return err
	}

	if cfg.DisableAnnotationAlphaProvidedIPAddr, err = getBool(FlagDisableAnnotationAlphaProvidedIPAddr); err != nil {
		return err
	}
	if cfg.DisableVMIController, err = getBool(FlagDisableVmiController); err != nil {
		return err
	}
	if cfg.LoadbalancerNetwork, err = getStr(FlagLoadbalancerNetwork); err != nil {
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

	// 3. Normalize and Warn: Cluster Name
	cfg.ClusterName = normalizeAndWarnClusterName(logrus.StandardLogger(), rawClusterName)

	// 4. Strict Validation: Management Network
	// If the user provided a value, it MUST be valid.
	if cfg.ManagementNetwork != "" {
		normalized, err := NormalizeNetworkName(NetworkTypeManagement, cfg.ManagementNetwork)
		if err != nil {
			return fmt.Errorf("invalid configuration for --%s: %w", FlagManagementNetwork, err)
		}
		cfg.ManagementNetwork = normalized
	}

	// 5. Strict Validation: Node IP CIDR
	// Fail early if the CIDR format or range is invalid.
	if err := validateAndParseNodeIPCIDR(cfg); err != nil {
		return err
	}

	// 6. Strict Validation: Node Exclude IP Ranges
	if err := validateAndParseNodeExcludeIPRanges(cfg); err != nil {
		return err
	}

	// 7. Strict Validation: Loadbalancer Network
	// If the user provided a value, it MUST be valid.
	if cfg.LoadbalancerNetwork != "" {
		normalized, err := NormalizeNetworkName(NetworkTypeLB, cfg.LoadbalancerNetwork)
		if err != nil {
			return fmt.Errorf("invalid configuration for --%s: %w", FlagLoadbalancerNetwork, err)
		}
		cfg.LoadbalancerNetwork = normalized
	}

	logrus.Infof("%s effective configurations: %s", HarvesterCloudProvider, GetCurrentConfigString(cfg))
	if cfg.ManagementNetwork == "" {
		logrus.Warnf("The '--%s' is not specified. Falling back to default discovery:", FlagManagementNetwork)
		logrus.Warnf("    - Node IPs: Fetched from the first available interface.")
		logrus.Warnf("    - LoadBalancers: Allocated from the first available network/IPPool.")
		logrus.Warnf("    Note: In multi-network environments, this can lead to non-deterministic IP reporting/allocating.")
	}
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

// normalizeAndWarnClusterName cleans the raw input and logs warnings for
// invalid or default states to ensure downstream K8s compatibility.
func normalizeAndWarnClusterName(logger logrus.FieldLogger, rawName string) string {
	// 1. Strip literal quotes, backticks, and whitespace.
	// When the --cluster-name is wrapped in literal quotes (e.g., "\"abc\""),
	// it causes downstream LoadBalancer name generation to fail K8s validation.
	normalized := strings.Trim(rawName, " \t\n\r\"'`")

	if normalized != rawName {
		logger.WithFields(logrus.Fields{
			"raw":        rawName,
			"normalized": normalized,
		}).Warnf("the --%s value was trimmed of whitespace or quotes, using normalized valu", FlagClusterName)
	}

	if normalized != rawName {
		logrus.Warnf("the --%s value %q was trimmed of whitespace or quotes; using normalized value: %q",
			FlagClusterName, rawName, normalized)
	}

	// 2. Logic Validation: Check for empty or default values
	if normalized == "" || normalized == DefaultGuestClusterName {
		logger.WithFields(logrus.Fields{
			"provided": rawName,
			"result":   normalized,
		}).Warnf("the flag --%s is empty or using the default value. A unique cluster name is recommended for remote systems to identify this cluster.", FlagClusterName)

		if normalized == "" {
			return normalized
		}
	}

	// 3. RFC 1123 Validation
	// We warn on failure but return the value anyway to maintain backward compatibility,
	// allowing loadBalancerName() to attempt its own mitigations (like the "a" prefix).
	errs := validation.IsDNS1123Label(normalized)
	if len(errs) > 0 {
		logger.WithFields(logrus.Fields{
			"value":  normalized,
			"errors": strings.Join(errs, "; "),
		}).Warnf("the --%s value is not a valid DNS label; this may cause downstream issues", FlagClusterName)
	}

	return normalized
}
