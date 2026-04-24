package utils

const (
	HarvesterCloudProvider = "cloudprovider.harvesterhci.io"

	HarvesterCloudProviderPrefix = HarvesterCloudProvider + "/"

	// original defined on pkg/cloud-controller-manager/annotation.go, moved to here
	KeyIPAM           = HarvesterCloudProviderPrefix + "ipam"
	KeyNetwork        = HarvesterCloudProviderPrefix + "network"
	KeyProject        = HarvesterCloudProviderPrefix + "project"
	KeyNamespace      = HarvesterCloudProviderPrefix + "namespace"
	KeyPrimaryService = HarvesterCloudProviderPrefix + "primary-service"

	KeyKubevipLoadBalancerIP = "kube-vip.io/loadbalancerIPs"

	KeyAdditionalInternalIPs = HarvesterCloudProviderPrefix + "additional-internal-ips"

	// original defined&unexported on pkg/cloud-controller-manager/loadbalancer.go
	// moved to here with adding LB prefix

	// replace `clusterNameKey      = prefix + "cluster"`
	LBClusterNameKey = HarvesterCloudProviderPrefix + "cluster"

	// replace `serviceNamespaceKey = prefix + "serviceNamespace"`
	LBServiceNamespaceKey = HarvesterCloudProviderPrefix + "serviceNamespace"

	// replace `serviceNameKey      = prefix + "serviceName"`
	LBServiceNameKey = HarvesterCloudProviderPrefix + "serviceName"

	// LabelKeyGuestClusterManagementNetworkOnLB = HarvesterCloudProviderPrefix + "managementNetwork"

	NetworkTypeManagement = "managementNetwork"

	NetworkTypeLB = "lbNetwork"

	// when a guest cluster has multiple networks, it can explicitly say which one is the management network, instead of guessing or hardcoding
	// value format: `default/vlan100`
	AnnotationKeyGuestClusterManagementNetworkOnLB = HarvesterCloudProviderPrefix + NetworkTypeManagement

	// LabelKeyGuestClusterNetworkNameOnLB = HarvesterCloudProviderPrefix + "lbNetwork"

	// if guest cluster sets a target network, then respect it
	// // value format: `project200/vlan200`
	AnnotationKeyGuestClusterNetworkNameOnLB = HarvesterCloudProviderPrefix + NetworkTypeLB

	// cloud-provider framework injects `kubernetes` as cluster name when it is not set by runtime env `--cluster-name`
	DefaultGuestClusterName = "kubernetes"

	DefaultNamespace = "default"

	// flags defined by framework
	FlagClusterName              = "cluster-name"
	FlagCloudProviderControllers = "controllers"

	// flags defined by harvester
	FlagDisableVmiController = "disable-vmi-controller"

	FlagManagementNetwork = "management-network"

	FlagAllowSpecifyLoadbalancerNetwork = "allow-specify-loadbalancer-network"

	// FlagShowFullHelpOnError toggles the display of the full framework help menu on startup failure.
	// Since users utilize '.Values.extraArgs' to tune cloud-provider framework features—such as
	// utilizing '--controllers' to enable or disable specific sub-controllers—we disable the
	// verbose help dump by default. This ensures configuration errors remain the focus of the logs.
	//
	// Example of a framework-level error handled by this logic:
	//   Command: ... --controllers=cloud-node-controller,node-route-controller,unknown
	//   Output:
	//     ERRO: =============================================================================================
	//     ERRO: FATAL: cloudprovider.harvesterhci.io failed to start
	//     ERRO: Error detail: "unknown" is not in the list of known controllers
	//     ERRO: =============================================================================================
	FlagShowFullHelpOnError = "show-full-help-on-error"
)
