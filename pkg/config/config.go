package config

import (
	"fmt"

	"github.com/harvester/harvester-cloud-provider/pkg/utils"
)

var (
	// defined by framework
	ClusterName              string
	CloudProviderControllers string // raw input is string slices, but converted by bootstrap

	// defined by harvester
	ManagementNetwork               string
	AllowSpecifyLoadBalancerNetwork bool
	DisableVMIController            bool
	ShowFullHelpOnError             bool
)

func CurrentConfigString() string {
	return fmt.Sprintf("--%s=%v --%s=%v --%s=%v --%s=%v --%s=%v --%s=%v",
		utils.FlagClusterName, ClusterName,
		utils.FlagCloudProviderControllers, CloudProviderControllers,
		utils.FlagManagementNetwork, ManagementNetwork,
		utils.FlagAllowSpecifyLoadbalancerNetwork, AllowSpecifyLoadBalancerNetwork,
		utils.FlagDisableVmiController, DisableVMIController,
		utils.FlagShowFullHelpOnError, ShowFullHelpOnError)
}
