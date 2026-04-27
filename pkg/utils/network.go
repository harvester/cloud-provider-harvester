package utils

import (
	"fmt"
	"net"
	"strings"
)

// NormalizeNetworkName ensures a network string is in the "namespace/name" format.
// It returns the normalized string or an error if the format is fundamentally broken.
func NormalizeNetworkName(networkType, networkName string) (string, error) {
	if networkName == "" {
		return "", fmt.Errorf("network type %s: network name is empty", networkType)
	}

	parts := strings.Split(networkName, "/")
	switch len(parts) {
	case 1:
		// Bare name -> default/name
		return fmt.Sprintf("%s/%s", DefaultNamespace, networkName), nil
	case 2:
		// Ensure neither part is empty (e.g., "ns/" or "/name")
		if parts[0] == "" || parts[1] == "" {
			return "", fmt.Errorf("invalid network type %s format %q, expected 'namespace/name'", networkType, networkName)
		}
		return networkName, nil
	default:
		// Too many slashes
		return "", fmt.Errorf("network type %s name %q has too many slashes", networkType, networkName)
	}
}

// ValidateCIDRFilter checks if the string is a valid comma-separated list of CIDRs or IPs.
// This is called during bootstrap to "Fail Early".
// ValidateCIDRFilter ensures the input is syntactically correct AND logically sound.
func ValidateCIDRFilter(cidrFilter string) error {
	if cidrFilter == "" {
		return nil
	}

	for _, part := range strings.Split(cidrFilter, ",") {
		cidr := strings.TrimSpace(part)
		if cidr == "" {
			continue
		}

		// 1. Strict Syntax Check
		var ip net.IP
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			ip = net.ParseIP(cidr)
			if ip == nil {
				return fmt.Errorf("invalid CIDR or IP format: %q (expected x.x.x.x/bb or x.x.x.x)", cidr)
			}
		} else {
			ip = ipNet.IP
		}

		// 2. Logical Safety Check (The "No-Go" Zone)
		if ip.IsLoopback() {
			return fmt.Errorf("invalid filter %q: loopback addresses are not allowed", cidr)
		}
		if ip.IsLinkLocalUnicast() {
			return fmt.Errorf("invalid filter %q: link-local addresses (APIPA) are not allowed", cidr)
		}

		// 3. Multicast/Global Unicast Check
		// Nodes must have Unicast addresses. 224.0.0.0/4 (Multicast) or 255.255.255.255 are invalid.
		if ip.IsMulticast() || !ip.IsGlobalUnicast() {
			// Note: IsGlobalUnicast() returns true for private ranges like 10.0.0.0/8
			// but false for the limited broadcast 255.255.255.255.
			return fmt.Errorf("invalid filter %q: must be a valid unicast address range", cidr)
		}
	}
	return nil
}

// VerifyNodeIPCIDR is called at runtime by the controller.
// Since ValidateCIDRFilter ran at boot, we know cidrFilter is syntactically correct.
func VerifyNodeIPCIDR(ipStr string, cidrFilter string) bool {
	target := net.ParseIP(ipStr)
	if target == nil {
		return false
	}

	// Always block loopback/link-local regardless of filter
	if target.IsLoopback() || target.IsLinkLocalUnicast() {
		return false
	}

	if cidrFilter == "" {
		return true
	}

	// Logical match (We can skip error checking here because bootstrap already validated it)
	for _, part := range strings.Split(cidrFilter, ",") {
		cidr := strings.TrimSpace(part)
		if _, ipNet, err := net.ParseCIDR(cidr); err == nil {
			if ipNet.Contains(target) {
				return true
			}
		} else if filterIP := net.ParseIP(cidr); filterIP != nil {
			if filterIP.Equal(target) {
				return true
			}
		}
	}
	return false
}

// ValidateIPOrCIDR checks if a string is a valid IP or CIDR range
func ValidateIPOrCIDR(s string) error {
	// Try as CIDR
	_, _, err := net.ParseCIDR(s)
	if err == nil {
		return nil
	}
	// Try as IP
	if ip := net.ParseIP(s); ip != nil {
		return nil
	}
	return fmt.Errorf("must be a valid IP address (e.g. 10.0.0.1) or CIDR (e.g. 10.0.0.0/24)")
}
