// Package config stores global configuration data initialized during the cloud-provider boot sequence.
//
// ARCHITECTURAL DESIGN & TRADEOFFS:
// The cloud-provider app lifecycle is managed by the underlying Kubernetes framework, not by our
// direct code. Because the framework does not expose parsed flags directly to plugins after
// initialization, storing them in this package is the current best tradeoff solution.
//
// CONCURRENCY & SAFETY:
// These variables are populated once by the bootstrap process. Because the framework completes
// flag parsing before starting any plugins or controller loops, there is no risk of race
// conditions. After the boot phase, these variables are treated as effectively immutable;
// no additional writes occur, making them safe for concurrent reads by plugins and controllers.
package config

type Config struct {
	// defined by cloud-provider framework
	ClusterName              string
	CloudProviderControllers string

	// defined by Harvester, refer pkg/utils/consts.go for more information
	ManagementNetwork               string
	NodeIPCIDR                      string
	NodeExcludeIPRanges             []string
	AllowSpecifyLoadBalancerNetwork bool
	DisableVMIController            bool
	ShowFullHelpOnError             bool
}

var _config = &Config{}

func GetConfig() *Config {
	return _config
}

func IsManagementNetworkConfigured() bool {
	return _config.ManagementNetwork != ""
}
