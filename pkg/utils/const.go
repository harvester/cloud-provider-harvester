package utils

const (
	HarvesterCloudProvider = "cloudprovider.harvesterhci.io"

	HarvesterCloudProviderPrefix = HarvesterCloudProvider + "/"

	// when guest cluster has multi network, it can explicitly say which one is the management network, instead of guess or hardcode
	LabelKeyGuestClusterManagementNetworkOnLB = HarvesterCloudProviderPrefix + "managementNetwork"

	// if guest cluster sets a target network, then respect it
	LabelKeyGuestClusterNetworkNameOnLB = HarvesterCloudProviderPrefix + "lbNetwork"

	// cloud-prvoider framework injects `kubernetes` as cluster name, when it is not set by runtime env `--cluster-name`
	DefaultGuestClusterName = "kubernetes"
)
