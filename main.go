package main

import (
	"os"

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
	harv.BoolVar(&ccm.DisableVMIController, "disable-vmi-controller", ccm.DisableVMIController,
		"The disable-vmi-controller will disable sync topology to nodes and not affect the custom cluster.")

	command := app.NewCloudControllerManagerCommand(ccmOptions, cloudInitializer, controllerInitializers, fss, wait.NeverStop)

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

	if err := command.Execute(); err != nil {
		os.Exit(1)
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
