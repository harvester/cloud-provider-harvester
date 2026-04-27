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
	registerHarvesterFlags(harv)

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

func registerHarvesterFlags(harv *pflag.FlagSet) {
	config := cfg.GetConfig()

	harv.BoolVar(&config.DisableVMIController, utils.FlagDisableVmiController, false,
		"Disable sync topology to nodes and not affect the custom cluster.")

	harv.BoolVar(&config.DisableAnnotationAlphaProvidedIPAddr, utils.FlagDisableAnnotationAlphaProvidedIPAddr, false,
		"By default, if the 'alpha.kubernetes.io/provided-node-ip' annotation is present, the cloud-provider \n"+
			"    limits internal IP reporting to that specific address. Setting this to true causes the provider \n"+
			"    to ignore this legacy annotation and instead determine the node IP based on the discovery pipeline \n"+
			"    defined by --management-network and --node-ip-cidr.")

	harv.StringVar(&config.ManagementNetwork, utils.FlagManagementNetwork, "",
		"Define the management network of this guest cluster, which is carried by a Harvester network \n"+
			"    (e.g., 'default/vlan-100'). This setting serves two primary purposes: \n"+
			"    1. Node IP Reporting: Guides the instance manager to the specific network interface from which \n"+
			"       to fetch the node's internal/external IP addresses. \n"+
			"    2. LoadBalancer Allocation: Guides the loadbalancer plugin to allocate Service IPs from the \n"+
			"       IPPool associated with this network.")

	harv.StringVar(&config.NodeIPCIDR, utils.FlagNodeIPCIDR, "",
		"Comma-separated list of CIDRs to filter node IPs (e.g., '192.168.122.0/24'). When used with \n"+
			"    --management-network, the instance manager will use this as a secondary filter to mark specific \n"+
			"    IPs on that interface as InternalIP. This prevents non-deterministic selection when a single \n"+
			"    network interface has multiple IP addresses.")

	harv.StringSliceVar(&config.NodeExcludeIPRanges, utils.FlagNodeExcludeIPRanges, []string{},
		"Define IP ranges or single IPs to exclude (e.g., '10.0.0.0/8,2001:db8::/64,192.168.0.5'). This is the \n"+
			"    final safety filter; any IP matching these ranges will not be marked as InternalIP or ExternalIP. \n"+
			"    Consequently, they will be suppressed and will not appear in 'kubectl get nodes -o wide'. \n"+
			"    This global setting replaces the legacy 'cloudprovider.harvesterhci.io/additional-internal-ips' \n"+
			"    node annotation.")

	harv.StringVar(&config.LoadbalancerNetwork, utils.FlagLoadbalancerNetwork, "",
		"(Experimental) Define the Harvester network name for LoadBalancer services (e.g., 'poc/vlan300'). \n"+
			"    When set, all LoadBalancer IPs will be allocated from this specific network. Successful routing \n"+
			"    requires alignment with kube-vip configuration and potential guest OS kernel tuning.")

	harv.BoolVar(&config.ShowFullHelpOnError, utils.FlagShowFullHelpOnError, false,
		"If a configuration error occurs at startup, the full help menu and flag list will be displayed. (default false)")
}
