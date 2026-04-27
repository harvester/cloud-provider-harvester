package utils

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/harvester/harvester-cloud-provider/pkg/config"
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

// validateAndParseNodeIPCIDR ensures the NodeIPCIDR is syntactically correct and logically sound.
// It strictly enforces a "Single or Dual-Stack" policy (max one IPv4 and one IPv6).
//
// Valid Examples:
//   - "10.0.0.0/24"                 (Single v4)
//   - "fd00::/8"                    (Single v6)
//   - "10.0.0.0/24, fd00::/8"       (v4 + v6)
//   - "fd00::/8, 10.0.0.1"          (v6 + v4)
//
// Invalid Examples:
//   - "10.0.0.0/24, 192.168.1.0/24" (Multiple v4 - Error)
//   - "127.0.0.1"                   (Loopback - Error)
//   - "224.0.0.1, fd00::/8"         (Multicast - Error)
//   - "not-an-ip"                   (Malformed - Error)
func validateAndParseNodeIPCIDR(cfg *config.Config) error {
	var (
		hasIPv4, hasIPv6, configured bool
		cidrFilter                   = cfg.NodeIPCIDR
		parts                        = strings.Split(cidrFilter, ",")
		prefixes                     = make([]netip.Prefix, 0, len(parts))
		updatedCidr                  = make([]string, 0, len(parts))
	)

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}

		configured = true
		// 1. Parse and Determine Family
		var addr netip.Addr
		var prefix netip.Prefix

		if p, err := netip.ParsePrefix(trimmed); err == nil {
			prefix = p
			addr = p.Addr()
		} else if a, err := netip.ParseAddr(trimmed); err == nil {
			prefix = netip.PrefixFrom(a, a.BitLen())
			addr = a
		} else {
			return fmt.Errorf("invalid configuration for --%s: invalid CIDR or IP format %q", FlagNodeIPCIDR, trimmed)
		}

		// 2. Strict Family Count (Max 1 per family)
		switch {
		case addr.Is4():
			if hasIPv4 {
				return fmt.Errorf("invalid configuration for --%s: multiple IPv4 entries in %q", FlagNodeIPCIDR, cidrFilter)
			}
			hasIPv4 = true
		case addr.Is6():
			if hasIPv6 {
				return fmt.Errorf("invalid configuration for --%s: multiple IPv6 entries in %q", FlagNodeIPCIDR, cidrFilter)
			}
			hasIPv6 = true
		default:
			return fmt.Errorf("invalid configuration for --%s (%q): unsupported IP family", FlagNodeIPCIDR, trimmed)
		}

		// 3. Logical Safety Checks
		if addr.IsLoopback() {
			return fmt.Errorf("invalid configuration for --%s (%q): loopback addresses not allowed", FlagNodeIPCIDR, trimmed)
		}
		if addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() {
			return fmt.Errorf("invalid configuration for --%s (%q): link-local addresses not allowed", FlagNodeIPCIDR, trimmed)
		}
		if addr.IsMulticast() || addr.IsUnspecified() {
			return fmt.Errorf("invalid configuration for --%s (%q): must be a valid unicast address", FlagNodeIPCIDR, trimmed)
		}

		// 4. IPv4 Broadcast Check
		if addr.Is4() && addr.As4() == [4]byte{255, 255, 255, 255} {
			return fmt.Errorf("invalid configuration for --%s (%q): broadcast address not allowed", FlagNodeIPCIDR, trimmed)
		}

		updatedCidr = append(updatedCidr, trimmed)
		prefixes = append(prefixes, prefix)
	}

	if configured && len(prefixes) == 0 {
		return fmt.Errorf("invalid configuration for --%s (%q): no valid CIDR or IP entries found", FlagNodeIPCIDR, cidrFilter)
	}

	// Save results to internal state
	cfg.SetNodeIPCIDRPrefixes(prefixes)
	cfg.NodeIPCIDR = strings.Join(updatedCidr, ",") // save the trimmed result
	return nil
}

func validateAndParseNodeExcludeIPRanges(cfg *config.Config) error {
	var cleanRanges []string
	var excludePrefixes []netip.Prefix

	// Loop through the slice and ensure every entry is either a valid IP or a valid CIDR
	for _, entry := range cfg.NodeExcludeIPRanges {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}

		prefix, err := parseStringToIPPrefix(trimmed)
		if err != nil {
			return fmt.Errorf("invalid entry in --%s (%q): %w", FlagNodeExcludeIPRanges, trimmed, err)
		}

		cleanRanges = append(cleanRanges, trimmed)
		excludePrefixes = append(excludePrefixes, prefix)
	}

	cfg.NodeExcludeIPRanges = cleanRanges
	cfg.SetNodeExcludeIPPrefixes(excludePrefixes)
	return nil
}

// parseStringToIPPrefix converts a string representation of an IP address or a CIDR
// range into a netip.Prefix.
//
// Usage Tips:
//   - To check if the result represents a single specific host (e.g., /32 or /128),
//     use prefix.IsSingleIP().
//   - To get the underlying address, use prefix.Addr().
//   - To check if an IP is within the range, use prefix.Contains(targetAddr).
func parseStringToIPPrefix(s string) (netip.Prefix, error) {
	// 1. Attempt to parse as a CIDR prefix (e.g., "10.0.0.0/24")
	if p, err := netip.ParsePrefix(s); err == nil {
		return p, nil
	}

	// 2. Attempt to parse as a single IP address (e.g., "10.0.0.1")
	if a, err := netip.ParseAddr(s); err == nil {
		// Convert the address to a full-mask prefix.
		// This will result in a Prefix where IsSingleIP() returns true.
		return netip.PrefixFrom(a, a.BitLen()), nil
	}

	return netip.Prefix{}, fmt.Errorf("invalid IP or CIDR format: %q", s)
}

// ConvertAndFilterIPs converts the string list to netip.Addr list and
// filters out loopback, link-local, multicast and broadcast IPs.
// As IPs are fetched from vmi interface status, we expect valid IP strings;
// if parsing fails, it returns an error to allow the caller to handle the malformed data.
func ConvertAndFilterIPs(ips []string) ([]netip.Addr, error) {
	if len(ips) == 0 {
		return nil, nil
	}

	isInternalOnly := func(addr netip.Addr) bool {
		return !addr.IsValid() ||
			addr.IsLoopback() ||
			addr.IsMulticast() ||
			addr.IsUnspecified() ||
			addr.IsLinkLocalUnicast() ||
			addr.IsLinkLocalMulticast() ||
			(addr.Is4() && addr == netip.AddrFrom4([4]byte{255, 255, 255, 255}))
	}

	validIPs := make([]netip.Addr, 0, len(ips))
	for _, ipStr := range ips {
		addr, err := netip.ParseAddr(ipStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse IP %q: %w", ipStr, err)
		}

		if !isInternalOnly(addr) {
			validIPs = append(validIPs, addr)
		}
	}

	return validIPs, nil
}
