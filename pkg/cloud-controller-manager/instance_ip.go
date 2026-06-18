package ccm

import (
	"fmt"
	"net/netip"
	"slices"

	v1 "k8s.io/api/core/v1"
	"k8s.io/cloud-provider/api"

	"github.com/sirupsen/logrus"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester-cloud-provider/pkg/config"
)

type ProcessingMode string

const (
	ModeProvidedIP ProcessingMode = "providedIP" // Priority 1: Legacy Annotation
	ModeNodeIPCIDR ProcessingMode = "nodeIPCIDR" // Priority 2: Modern CIDR Policy
	ModeFallback   ProcessingMode = "fallback"   // Priority 3: First-win Discovery
)

type AddressContext struct {
	Mode                  ProcessingMode
	NodeIPCIDRPrefixes    []netip.Prefix // IP or prefix
	NodeExcludeIPPrefixes []netip.Prefix // IP or prefix
	ProvidedIP            string
	LegacyExcludes        []string // strict string match
	Network               string
}

type CandidateAddress struct {
	Addr     netip.Addr
	NodeAddr v1.NodeAddress
}

type CandidateAddresses []CandidateAddress

// ToNodeAddresses converts the internal rich objects into the final K8s API slice.
func (cas CandidateAddresses) ToNodeAddresses() []v1.NodeAddress {
	res := make([]v1.NodeAddress, 0, len(cas))
	for _, ca := range cas {
		res = append(res, ca.NodeAddr)
	}
	return res
}

func getManagementNetworks(vmi *kubevirtv1.VirtualMachineInstance, cfg *config.Config) []string {
	if mgmt, ok := cfg.GetManagementNetwork(); ok {
		// Build a list of network names (names of NICs) on the VM.
		networkNames := make([]string, 0, 1)
		for _, network := range vmi.Spec.Networks {

			// format:
			// networks:
			//  - multus:
			//	  networkName: default-none/vm-untag3
			// name: nic-0

			if network.Multus == nil {
				// only multus based network is supported on guest cluster
				continue
			}
			// if ManagementNetwork is configured, then strictly match this network
			if network.Multus.NetworkName == mgmt {
				networkNames = append(networkNames, network.Name)
				break
			}
		}
		return networkNames
	}

	// Build a list of network names (names of NICs) on the VM.
	networkNames := make([]string, 0, len(vmi.Spec.Networks))
	for _, network := range vmi.Spec.Networks {
		if network.Multus == nil {
			continue
		}
		networkNames = append(networkNames, network.Name)
	}
	return networkNames
}

func getLegacyModeRelatedParams(node *v1.Node, cfg *config.Config) (bool, bool, string, []string) {
	disableProvidedIP := cfg.DisableAnnotationAlphaProvidedIPAddr
	if disableProvidedIP {
		return disableProvidedIP, false, "", nil
	}

	legacyExcludes := getAdditionalInternalIPs(node)
	// Rule 1: Legacy Alpha Annotation (Highest Priority)
	// We replicate the legacy "ok" check exactly.
	var providedIP string
	useProvidedIP := false
	if val, ok := node.Annotations[api.AnnotationAlphaProvidedIPAddr]; ok {
		// Soft validation: we warn if it's not a valid IP,
		// but we still use it for string comparison to match legacy behavior.
		if _, err := netip.ParseAddr(val); err != nil {
			logrus.Warnf("Node %s has an invalid %s annotation value %q: %v. "+
				"The cloud-provider might not be able to find a matched InternalIP from this annotation, "+
				"and discovered IPs may be treated as ExternalIP instead. Please ensure this annotation contains a valid IP address "+
				"or simply remove it, as the system will fallback to finding the first fit IP automatically. "+
				"For better dual-stack support and reliability, it is recommended to use the --node-ip-cidr flag instead.",
				node.Name, api.AnnotationAlphaProvidedIPAddr, val, err)
		}
		providedIP = val
		useProvidedIP = true
	}

	return disableProvidedIP, useProvidedIP, providedIP, legacyExcludes
}

// buildIPAddressProcessContext determines the processing strategy based on priority:
// 1. Legacy Annotation (if not disabled)
// 2. CIDR Prefix Policy
// 3. First-fit Fallback
func buildIPAddressProcessContext(node *v1.Node, network string, cfg *config.Config) *AddressContext {
	nodeIPCIDRPrefixes := cfg.GetNodeIPCIDRPrefixes()
	disableAnnot, useAnnot, annotIP, excludes := getLegacyModeRelatedParams(node, cfg)

	ctx := AddressContext{
		Network: network, // Explicitly track the target network in the context
	}

	// (1) Priority: Legacy Provided IP (if exists and not disabled)
	if useAnnot && !disableAnnot {
		ctx.Mode = ModeProvidedIP
		ctx.ProvidedIP = annotIP
		ctx.LegacyExcludes = excludes
		return &ctx
	}

	// (2) Priority: Modern CIDR Mode
	if len(nodeIPCIDRPrefixes) > 0 {
		ctx.Mode = ModeNodeIPCIDR
		ctx.NodeIPCIDRPrefixes = nodeIPCIDRPrefixes
		ctx.NodeExcludeIPPrefixes = cfg.GetNodeExcludeIPPrefixes()
		return &ctx
	}

	// (3) Priority: Fallback (First-win)
	ctx.Mode = ModeFallback
	ctx.LegacyExcludes = excludes
	return &ctx
}

// getRawIPsFromVMINetwork extracts the reported IP strings for a specific network
// interface (management/control-plane) from the VMI status.
func getRawIPsFromVMINetwork(vmi *kubevirtv1.VirtualMachineInstance, targetNetwork string) ([]string, error) {
	for _, iface := range vmi.Status.Interfaces {
		if iface.Name != targetNetwork {
			continue
		}

		// Interface identified; prefer the reported IP list when available.
		if len(iface.IPs) > 0 {
			return iface.IPs, nil
		}

		// KubeVirt may populate only the single-IP field; fall back to it when the
		// IP list is empty.
		if iface.IP != "" {
			return []string{iface.IP}, nil
		}

		// Interface identified; check if IPs are reported (usually via Guest Agent)
		return nil, fmt.Errorf("management network %q found but no IPs reported yet for VMI %s/%s", targetNetwork, vmi.Namespace, vmi.Name)
	}

	// The interface is in the spec but hasn't appeared in the status yet
	return nil, fmt.Errorf("management network %q status not yet available for VMI %s/%s", targetNetwork, vmi.Namespace, vmi.Name)
}

// resolveNodeIPs orchestrates the selection, categorization (Internal/External),
// and filtering of raw IP addresses into a CandidateAddresses set based on
// the provided node IP addresses.
func resolveNodeIPs(ips []netip.Addr, ctx *AddressContext) CandidateAddresses {
	switch ctx.Mode {
	case ModeProvidedIP:
		candidates := categorizeByProvidedIP(ips, ctx.ProvidedIP)
		return filterByExcludeList(candidates, ctx.LegacyExcludes)

	case ModeNodeIPCIDR:
		candidates := categorizeByCIDR(ips, ctx.NodeIPCIDRPrefixes)
		return filterByCIDRPolicy(candidates, ctx.NodeExcludeIPPrefixes)

	case ModeFallback:
		candidates := categorizeByFallback(ips)
		return filterByExcludeList(candidates, ctx.LegacyExcludes)

	default:
		return nil
	}
}

func categorizeByProvidedIP(addrs []netip.Addr, providedIP string) CandidateAddresses {
	res := make(CandidateAddresses, 0, len(addrs))
	hasInternalIPv4, hasInternalIPv6 := false, false

	for _, addr := range addrs {
		ipStr := addr.String()
		ipType := v1.NodeExternalIP

		isV4 := addr.Is4()
		needsInternal := (isV4 && !hasInternalIPv4) || (!isV4 && !hasInternalIPv6)

		if needsInternal && ipStr == providedIP {
			ipType = v1.NodeInternalIP
			if isV4 {
				hasInternalIPv4 = true
			} else {
				hasInternalIPv6 = true
			}
		}

		res = append(res, CandidateAddress{
			Addr:     addr,
			NodeAddr: v1.NodeAddress{Type: ipType, Address: ipStr},
		})
	}
	return res
}

func categorizeByCIDR(addrs []netip.Addr, prefixes []netip.Prefix) CandidateAddresses {
	res := make(CandidateAddresses, 0, len(addrs))
	// The first IPv4 address and the first IPv6 address that match any prefix are marked as InternalIP.
	// This avoids misclassifying additional addresses (for example, load balancer VIPs) on the same subnet
	// as InternalIP when they are bound to the target network (for example, the mgmt network).
	hasInternalIPv4, hasInternalIPv6 := false, false
	for _, addr := range addrs {
		ipType := v1.NodeExternalIP
		isV4 := addr.Is4()
		needsInternal := (isV4 && !hasInternalIPv4) || (!isV4 && !hasInternalIPv6)
		if needsInternal {
			for _, pfx := range prefixes {
				if pfx.Contains(addr) {
					ipType = v1.NodeInternalIP
					if isV4 {
						hasInternalIPv4 = true
					} else {
						hasInternalIPv6 = true
					}
					break
				}
			}
		}
		res = append(res, CandidateAddress{
			Addr:     addr,
			NodeAddr: v1.NodeAddress{Type: ipType, Address: addr.String()},
		})
	}
	return res
}

// only the first valid ipv4, ipv6 are marked as internal
func categorizeByFallback(addrs []netip.Addr) CandidateAddresses {
	res := make(CandidateAddresses, 0, len(addrs))
	hasV4, hasV6 := false, false

	for _, addr := range addrs {
		ipType := v1.NodeExternalIP
		isV4 := addr.Is4()

		if (isV4 && !hasV4) || (!isV4 && !hasV6) {
			ipType = v1.NodeInternalIP
			if isV4 {
				hasV4 = true
			} else {
				hasV6 = true
			}
		}

		res = append(res, CandidateAddress{
			Addr:     addr,
			NodeAddr: v1.NodeAddress{Type: ipType, Address: addr.String()},
		})
	}
	return res
}

func filterByCIDRPolicy(addrs CandidateAddresses, prefixes []netip.Prefix) CandidateAddresses {
	final := make(CandidateAddresses, 0, len(addrs))
	for _, ca := range addrs {
		if ca.NodeAddr.Type == v1.NodeInternalIP {
			final = append(final, ca)
			continue
		}
		// only filter ExternalIP
		match := false
		for _, pfx := range prefixes {
			if pfx.Contains(ca.Addr) {
				match = true
				break
			}
		}
		// if an IP is matched, it is excluded from the list, not exposed to upstream node object
		if !match {
			final = append(final, ca)
		}
	}
	return final
}

// filterByExcludeList removes addresses from the candidates list if they match
// the exclude list. This only applies to ExternalIPs; InternalIPs are protected
// and will not be filtered out.
func filterByExcludeList(addrs CandidateAddresses, excludes []string) CandidateAddresses {
	if len(excludes) == 0 {
		return addrs
	}

	final := make(CandidateAddresses, 0, len(addrs))
	for _, ca := range addrs {
		// Precise match against the exclude list for External IPs only
		if ca.NodeAddr.Type == v1.NodeExternalIP && slices.Contains(excludes, ca.NodeAddr.Address) {
			continue
		}
		final = append(final, ca)
	}
	return final
}
