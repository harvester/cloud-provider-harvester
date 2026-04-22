package utils

const (
	HarvesterCloudProvider = "cloudprovider.harvesterhci.io"

	HarvesterCloudProviderPrefix = HarvesterCloudProvider + "/"

	// when a guest cluster has multiple networks, it can explicitly say which one is the management network, instead of guessing or hardcoding
	LabelKeyGuestClusterManagementNetworkOnLB = HarvesterCloudProviderPrefix + "managementNetwork"

	// if guest cluster sets a target network, then respect it
	LabelKeyGuestClusterNetworkNameOnLB = HarvesterCloudProviderPrefix + "lbNetwork"

	// cloud-provider framework injects `kubernetes` as cluster name when it is not set by runtime env `--cluster-name`
	DefaultGuestClusterName = "kubernetes"
)
