package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/util/wait"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/cloud-provider/app"
	cloudcontrollerconfig "k8s.io/cloud-provider/app/config"
	"k8s.io/cloud-provider/options"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"
	"k8s.io/klog/v2"

	ccm "github.com/harvester/harvester-cloud-provider/pkg/cloud-controller-manager"
	cfg "github.com/harvester/harvester-cloud-provider/pkg/config"
	"github.com/harvester/harvester-cloud-provider/pkg/utils"
)

func main() {
	pflag.CommandLine.SetNormalizeFunc(cliflag.WordSepNormalizeFunc)

	ccmOptions, err := options.NewCloudControllerManagerOptions()
	if err != nil {
		klog.Fatalf("unable to initialize command options: %v", err)
	}

	controllerInitializers := app.DefaultInitFuncConstructors

	fss := cliflag.NamedFlagSets{}
	harv := fss.FlagSet("harvester")
	harv.BoolVar(&cfg.DisableVMIController, utils.FlagDisableVmiController, false,
		"Disable sync topology to nodes and not affect the custom cluster.")

	harv.StringVar(&cfg.ManagementNetwork, utils.FlagMgmtNetwork, "",
		"Define the management network of the cluster, otherwise it selects the first network.")

	harv.BoolVar(&cfg.AllowSpecifyLoadBalancerNetwork, utils.FlagAllowSpecifyLoadbalancerNetwork, false,
		"Allow loadbalancer to use user input annotation to specify the target network, otherwise the target network is always refetched. (default false)")

	harv.BoolVar(&cfg.ShowFullHelpOnError, utils.FlagShowFullHelpOnError, false,
		"Show the full help menu and flag list will be displayed if a configuration error occurs at startup. (default false)")

	command := app.NewCloudControllerManagerCommand(ccmOptions, cloudInitializer, controllerInitializers, map[string]string{}, fss, wait.NeverStop)

	// Check if we should silence the framework's verbose help output
	configureErrorReporting(command)

	// Set static flags for which we know the values.
	command.Flags().VisitAll(func(fl *pflag.Flag) {
		var err error
		switch fl.Name {
		case "allow-untagged-cloud",
			// Untagged clouds must be enabled explicitly as they were once marked
			// deprecated. See
			// https://github.com/kubernetes/cloud-provider/issues/12 for an ongoing
			// discussion on whether that is to be changed or not.
			"authentication-skip-lookup":
			// Prevent reaching out to an authentication-related ConfigMap that
			// we do not need, and thus do not intend to create RBAC permissions
			// for. See also
			// https://github.com/digitalocean/digitalocean-cloud-controller-manager/issues/217
			// and https://github.com/kubernetes/cloud-provider/issues/29.
			err = fl.Value.Set("true")
		case "cloud-provider":
			// Specify the name we register our own cloud provider implementation
			// for.
			err = fl.Value.Set(ccm.ProviderName)
		}
		if err != nil {
			klog.Errorf("set flag %s failed, error: %s", fl.Name, err.Error())
			os.Exit(1)
		}
	})

	logs.InitLogs()
	defer logs.FlushLogs()

	// Wrap the framework's RunE with our custom configuration sync and validation
	originalRunE := command.RunE
	command.RunE = func(cmd *cobra.Command, args []string) error {
		if err := syncAndValidateHarvesterConfig(cmd); err != nil {
			return err
		}
		if originalRunE == nil {
			return fmt.Errorf("the original runE command was nil, initialization failed")
		}
		return originalRunE(cmd, args)
	}

	if err := command.Execute(); err != nil {
		// Pass the error, the value of your custom flag, and the command object
		handleStartupError(err)
	}
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
// CRITICALITY (Cluster Identity):
// A unique 'cluster-name' is critical in multi-cluster management systems. If
// left as the default "kubernetes", remote systems cannot distinguish which
// cluster is requesting resources (like LoadBalancer IPs), leading to identity
// collisions.
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
// We actively fetch them here via RunE to ensure our global configuration is
// synchronized. This initialization is thread-safe because the various controller
// loops and plugins have not been started yet; we are capturing the finalized
// state before the framework branches into multi-threaded execution.
//
// TROUBLESHOOTING:
// We log these values clearly at boot time to allow for immediate verification
// of the runtime configuration in production logs.
func syncAndValidateHarvesterConfig(cmd *cobra.Command) error {
	cfg.ClusterName, _ = cmd.Flags().GetString(utils.FlagClusterName)
	cfg.ManagementNetwork, _ = cmd.Flags().GetString(utils.FlagMgmtNetwork)
	cfg.DisableVMIController, _ = cmd.Flags().GetBool(utils.FlagDisableVmiController)
	cfg.AllowSpecifyLoadBalancerNetwork, _ = cmd.Flags().GetBool(utils.FlagAllowSpecifyLoadbalancerNetwork)

	if cfg.ClusterName == "" || cfg.ClusterName == utils.DefaultGuestClusterName {
		logrus.Warnf("%s WARNING: the flag --%s=%s is using an empty or default value (%q). A unique cluster name is "+
			"required for remote systems to identify this cluster in multi-cluster "+
			"environments. This may cause resource collisions.",
			utils.HarvesterCloudProvider, utils.FlagClusterName, cfg.ClusterName, utils.DefaultGuestClusterName)
	}

	logrus.Infof("%s effective configurations: %s", utils.HarvesterCloudProvider, cfg.CurrentConfigString())
	return nil
}

// configureErrorReporting determines if the user wants the standard, concise
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
func configureErrorReporting(cmd *cobra.Command) {
	showFullHelp := false
	for _, arg := range os.Args {
		// We check os.Args directly because this happens before cmd.Execute()
		if arg == "--"+utils.FlagShowFullHelpOnError+"=true" || arg == "--"+utils.FlagShowFullHelpOnError {
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

// handleStartupError provides a clean, user-friendly exit when the application fails to start.
// Since the harvester-cloud-provider Helm chart allows users to inject custom flags via '.Values.extraArgs',
// it is essential to check if the failure is due to an unknown flag. This ensures users
// get clear feedback regarding typos in their Helm configuration rather than a generic crash.
func handleStartupError(err error) {
	errStr := err.Error()

	// Visual boundary to separate the error from standard container logs
	logrus.Errorf("=============================================================================================")
	logrus.Errorf("FATAL: %s failed to start", utils.HarvesterCloudProvider)

	// Log the exact raw arguments received by the OS
	// This is the ultimate "truth" for debugging Helm/Shell injection issues
	logrus.Errorf("Raw arguments: %v", os.Args)

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

func cloudInitializer(config *cloudcontrollerconfig.CompletedConfig) cloudprovider.Interface {
	cloudConfig := config.ComponentConfig.KubeCloudShared.CloudProvider
	// initialize cloud harvester with the cloud provider name and config file provided
	cloud, err := cloudprovider.InitCloudProvider(cloudConfig.Name, cloudConfig.CloudConfigFile)
	if err != nil {
		klog.Fatalf("Cloud provider harvester could not be initialized: %v", err)
	}
	if cloud == nil {
		klog.Fatalf("Cloud provider harvester is nil")
	}

	if !cloud.HasClusterID() {
		if config.ComponentConfig.KubeCloudShared.AllowUntaggedCloud {
			klog.Warning("detected a cluster without a ClusterID.  A ClusterID will be required in the future.  Please tag your cluster to avoid any future issues")
		} else {
			klog.Fatalf("no ClusterID found.  A ClusterID is required for the cloud harvester to function properly.  This check can be bypassed by setting the allow-untagged-cloud option")
		}
	}

	return cloud
}
