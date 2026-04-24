package main

import (
	"fmt"
	"os"

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
	utils.BootstrapLogrus()

	pflag.CommandLine.SetNormalizeFunc(cliflag.WordSepNormalizeFunc)

	ccmOptions, err := options.NewCloudControllerManagerOptions()
	if err != nil {
		klog.Fatalf("unable to initialize command options: %v", err)
	}

	controllerInitializers := app.DefaultInitFuncConstructors

	fss := cliflag.NamedFlagSets{}
	harv := fss.FlagSet("harvester")
	harv.BoolVar(&cfg.GetConfig().DisableVMIController, utils.FlagDisableVmiController, false,
		"Disable sync topology to nodes and not affect the custom cluster.")

	harv.StringVar(&cfg.GetConfig().ManagementNetwork, utils.FlagManagementNetwork, "",
		"Define the Harvester network name (e.g., 'default/vlan-100'). The provider will fetch node-ip and "+
			"allocate loadbalancer-ip from the VMI interface (guest cluster node) associated with this network.")

	harv.StringVar(&cfg.GetConfig().NodeIPCIDR, utils.FlagNodeIPCIDR, "",
		"Comma-separated list of CIDRs to filter node IPs (e.g., '192.168.122.0/24,2001:db8::/64'). "+
			"When used with --management-network, it further refines which IPs on that specific interface are selected. "+
			"Used to avoid non-deterministic guessing in multi-NIC/multi-IP setups.")

	harv.BoolVar(&cfg.GetConfig().AllowSpecifyLoadBalancerNetwork, utils.FlagAllowSpecifyLoadbalancerNetwork, false,
		"Allow loadbalancer to use user input annotation to specify the target network, otherwise the target network is always refetched. (default false)")

	harv.BoolVar(&cfg.GetConfig().ShowFullHelpOnError, utils.FlagShowFullHelpOnError, false,
		"Show the full help menu and flag list will be displayed if a configuration error occurs at startup. (default false)")

	command := app.NewCloudControllerManagerCommand(ccmOptions, cloudInitializer, controllerInitializers, map[string]string{}, fss, wait.NeverStop)

	// Check if we should silence the framework's verbose help output
	utils.CheckFlagShowFullHelpOnError(command, cfg.GetConfig())

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
		if err := utils.SyncAndValidateHarvesterConfig(cmd, cfg.GetConfig()); err != nil {
			return err
		}
		if originalRunE == nil {
			return fmt.Errorf("the original runE command was nil, initialization failed")
		}
		return originalRunE(cmd, args)
	}

	if err := command.Execute(); err != nil {
		// Pass the error, the value of your custom flag, and the command object
		utils.HandleStartupError(cfg.GetConfig(), err)
	}
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
