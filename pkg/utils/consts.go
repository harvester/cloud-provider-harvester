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

	// KeyKubevipServiceInterface is the annotation key for kube-vip service interface.
	KeyKubevipServiceInterface = "kube-vip.io/serviceInterface"

	// Note: When a node reports multiple addresses as "InternalIP", Kubernetes typically
	// prioritizes the first entry. This annotation effectively "hides" specific IPs from being
	// categorized as "ExternalIP" without actually making them functional secondary
	// internal addresses in the Kubernetes API.
	KeyAdditionalInternalIPs = HarvesterCloudProviderPrefix + "additional-internal-ips"

	// KeyInterfaceNADMapping is the annotation key used on Node objects (kept for reference).
	KeyInterfaceNADMapping = HarvesterCloudProviderPrefix + "interface-nad-mapping"

	// ConfigMapNADMapping is the name of the ConfigMap (in kube-system) that stores the
	// common NAD->interface mapping for the guest cluster.
	ConfigMapNADMapping = "harvester-nad-mapping"

	// ConfigMapKeyNADMapping is the data key inside the NAD mapping ConfigMap.
	// ConfigMap data keys must match '[-._a-zA-Z0-9]+', so we cannot use the full annotation key.
	// Value is a JSON map of Multus NetworkAttachmentDefinition name -> Linux interface name,
	// e.g. {"default/mgmt-vlan1":"enp1s0","default/net123":"enp2s0"}.
	ConfigMapKeyNADMapping = "interface-nad-mapping"

	// original defined&unexported on pkg/cloud-controller-manager/loadbalancer.go
	// moved to here with adding LB prefix

	// replace `clusterNameKey      = prefix + "cluster"`
	LBClusterNameKey = HarvesterCloudProviderPrefix + "cluster"

	// replace `serviceNamespaceKey = prefix + "serviceNamespace"`
	LBServiceNamespaceKey = HarvesterCloudProviderPrefix + "serviceNamespace"

	// replace `serviceNameKey      = prefix + "serviceName"`
	LBServiceNameKey = HarvesterCloudProviderPrefix + "serviceName"

	// new definitions
	NetworkTypeManagement = "managementNetwork"

	// when a guest cluster has multiple networks, it can explicitly say which one is the management network, instead of guessing or hardcoding
	// value format: `default/vlan100`
	AnnotationKeyGuestClusterManagementNetworkOnLB = HarvesterCloudProviderPrefix + NetworkTypeManagement

	// AnnotationKeyNetworkOnLB stores the raw network name (namespace/name format, e.g. "default/net123")
	// on the LoadBalancer object. This replaces the label-based approach (utils.KeyNetwork label)
	// which required sanitizing '/' to '_' because label values cannot contain slashes.
	AnnotationKeyNetworkOnLB = HarvesterCloudProviderPrefix + "lb-network"

	// cloud-provider framework injects `kubernetes` as cluster-name when runtime env `--cluster-name` is not set
	// if `--cluster-name=abc` then `cluster-name` is `abc`
	// if `--cluster-name=` then `cluster-name` is `` (empty)
	DefaultGuestClusterName = "kubernetes"

	DefaultNamespace = "default"

	// flags defined by framework
	FlagClusterName              = "cluster-name"
	FlagCloudProviderControllers = "controllers"

	// flags defined by Harvester
	FlagDisableVmiController = "disable-vmi-controller"

	// disable the usage of annotation AnnotationAlphaProvidedIPAddr and KeyAdditionalInternalIPs
	// as KeyAdditionalInternalIPs only works if AnnotationAlphaProvidedIPAddr is there
	FlagDisableAnnotationAlphaProvidedIPAddr = "disable-annotation-alpha-provided-ip-addr"

	FlagManagementNetwork = "management-network"

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

	// FlagNodeIPCIDR is the global filter for allowed node IP ranges.
	// Supports dual-stack (e.g., "192.168.122.0/24,2001:db8::/64").
	// When a node has multi-nics or multi-ips, we use this to precisely select
	// the correct node-ip and avoid deterministic "guessing" failures.
	FlagNodeIPCIDR = "node-ip-cidr"

	FlagNodeExcludeIPRanges = "node-exclude-ip-ranges"

	// LabelKeyGuestClusterNameOnVM is the label applied to VMs that belong to a guest cluster.
	// Value is the guest cluster name
	LabelKeyGuestClusterNameOnVM = "guestcluster.harvesterhci.io/name"

	// node-ip related

	// Note:
	// AnnotationAlphaProvidedIPAddr ("alpha.kubernetes.io/provided-node-ip")
	// from "k8s.io/cloud-provider/api/well_known_annotations.go".
	// is always respected first as a legacy override for backward compatibility.
)
