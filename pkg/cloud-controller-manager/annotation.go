// Package ccm contains cloud-controller-manager related constants.
// Deprecated: This package is deprecated. Please use the "github.com/harvester/harvester-cloud-provider/pkg/utils" package instead.
package ccm

import (
	utils "github.com/harvester/harvester-cloud-provider/pkg/utils"
)

const (
	// prefix is the standard prefix for Harvester cloud provider annotations.
	// Deprecated: use utils.HarvesterCloudProviderPrefix instead.
	prefix = utils.HarvesterCloudProviderPrefix

	// KeyIPAM is the annotation key for IPAM configuration.
	// Deprecated: use utils.KeyIPAM instead.
	KeyIPAM = utils.KeyIPAM

	// KeyNetwork is the annotation key for network selection.
	// Deprecated: use utils.KeyNetwork instead.
	KeyNetwork = utils.KeyNetwork

	// KeyProject is the annotation key for project identification.
	// Deprecated: use utils.KeyProject instead.
	KeyProject = utils.KeyProject

	// KeyNamespace is the annotation key for namespace identification.
	// Deprecated: use utils.KeyNamespace instead.
	KeyNamespace = utils.KeyNamespace

	// KeyPrimaryService is the annotation key for identifying the primary service in shared IP scenarios.
	// Deprecated: use utils.KeyPrimaryService instead.
	KeyPrimaryService = utils.KeyPrimaryService

	// KeyKubevipLoadBalancerIP is the annotation key for kube-vip load balancer IPs.
	// Deprecated: use utils.KeyKubevipLoadBalancerIP instead.
	KeyKubevipLoadBalancerIP = utils.KeyKubevipLoadBalancerIP

	// KeyAdditionalInternalIPs is the annotation key for adding extra internal IPs to nodes.
	// Deprecated: use utils.KeyAdditionalInternalIPs instead.
	KeyAdditionalInternalIPs = utils.KeyAdditionalInternalIPs
)
