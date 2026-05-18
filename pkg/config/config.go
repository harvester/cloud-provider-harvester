// Package config stores global configuration data initialized during the cloud-provider boot sequence.
//
// ARCHITECTURAL DESIGN & TRADEOFFS:
// The cloud-provider app lifecycle is managed by the underlying Kubernetes framework, not by our
// direct code. Because the framework does not expose parsed flags directly to plugins after
// initialization, storing them in this package is the current best tradeoff solution.
//
// IMPORT RESTRICTIONS:
// This package must remain a low-level dependency to serve as a common data source. Most IP
// and data processing logic resides in 'package utils'. To avoid circular (loop) imports,
// DO NOT import 'package utils' into this package. All data transformation should happen
// within 'utils' by consuming the structures defined here.
//
// CONCURRENCY & SAFETY:
// These variables are populated once by the bootstrap process. Because the framework completes
// flag parsing before starting any plugins or controller loops, there is no risk of race
// conditions. After the boot phase, these variables are treated as effectively immutable;
// no additional writes occur, making them safe for concurrent reads by plugins and controllers.
package config

import (
	"net/netip"
	"strings"
	// NOTE: To prevent circular dependencies, DO NOT import
	// "github.com/harvester/harvester-cloud-provider/pkg/utils" here,
	// as that package already imports this config package.
)

type Config struct {
	// defined by cloud-provider framework
	ClusterName              string
	CloudProviderControllers string

	// DisableAnnotationAlphaProvidedIPAddr governs the handling of the legacy
	// "alpha.kubernetes.io/provided-node-ip" annotation.
	// When set to true, the cloud-provider will ignore this annotation even
	// if present on the Node object, forcing the selection logic to use
	// alternative methods (CIDR or Fallback) instead.
	DisableAnnotationAlphaProvidedIPAddr bool

	// defined by Harvester, refer pkg/utils/consts.go for more information
	ManagementNetwork    string
	NodeIPCIDR           string
	NodeExcludeIPRanges  []string
	DisableVMIController bool
	ShowFullHelpOnError  bool

	// internalNodeIPCIDRPrefixes is the pre-parsed representation of NodeIPCIDR.
	// NOTE: This is populated during bootstrap validation. By storing the
	// parsed prefixes here, we ensure that the rest of the application
	// can use the CIDRs without needing to re-parse or handle parsing
	// errors at every call site.
	internalNodeIPCIDRPrefixes []netip.Prefix

	// internalNodeExcludeIPPrefixes is the pre-parsed representation of NodeExcludeIPRanges.
	// It is used to quickly filter out specific IPs or subnets during the node
	// address discovery process.
	internalNodeExcludeIPPrefixes []netip.Prefix
}

// GetConfig returns a pointer to the global configuration instance.
// The returned pointer is guaranteed to be non-nil.
//
// ACCESS & SAFETY:
// While the returned struct members are exported and directly visible, they are
// populated ONLY during the bootstrap stage. Controllers and plugins must treat
// this configuration as READ-ONLY. Do not modify these values during the
// controller/plugin runtime, as it may lead to inconsistent state or race
// conditions across the provider.
func GetConfig() *Config {
	return instance
}

// instance is the internal singleton. It is explicitly initialized to a
// non-nil pointer to ensure GetConfig() is always safe to call.
var instance = &Config{}

// GetManagementNetwork returns the configured management network name.
// The boolean return value indicates whether the configuration is effectively
// set by the user with a valid (non-empty) value.
func (c *Config) GetManagementNetwork() (string, bool) {
	if c != nil && c.ManagementNetwork != "" {
		return c.ManagementNetwork, true
	}
	return "", false
}

func (c *Config) GetNodeIPCIDRPrefixes() []netip.Prefix {
	if c == nil {
		return nil
	}
	return append([]netip.Prefix(nil), c.internalNodeIPCIDRPrefixes...)
}

// SetNodeIPCIDRPrefixes populates the parsed CIDR prefixes.
//
// LIFECYCLE & USAGE:
//  1. Bootstrap: Called once during the global configuration initialization to cache
//     parsed netip.Prefix data, avoiding redundant parsing overhead during runtime.
//  2. Testing: Used to inject mock network configurations into independent config instances.
//
// WARNING:
// This must NOT be called within controller loops or plugin runtimes. The configuration
// is intended to be effectively immutable after the boot sequence to ensure thread-safety
// for concurrent readers.
func (c *Config) SetNodeIPCIDRPrefixes(prefixes []netip.Prefix) {
	if c == nil {
		return
	}
	c.internalNodeIPCIDRPrefixes = prefixes
}

func (c *Config) GetNodeExcludeIPPrefixes() []netip.Prefix {
	if c == nil {
		return nil
	}
	return append([]netip.Prefix(nil), c.internalNodeExcludeIPPrefixes...)
}

// SetNodeExcludeIPPrefixes populates the parsed exclusion IP/CIDR prefixes.
//
// LIFECYCLE & USAGE:
//  1. Bootstrap: Called once during initialization to cache parsed exclusion ranges,
//     ensuring efficient lookup (contains checks) during the node address discovery.
//  2. Testing: Allows for manual injection of specific exclusion sets to verify
//     discovery filtering logic.
//
// WARNING:
// To maintain thread-safety, this field should only be set during the application's
// initial validation phase. It must be treated as read-only once the controller
// runtime or discovery sync has started.
func (c *Config) SetNodeExcludeIPPrefixes(prefixes []netip.Prefix) {
	if c == nil {
		return
	}
	c.internalNodeExcludeIPPrefixes = prefixes
}

// GetNodeExcludeIPRangesCmdString reconstructs the original comma-separated
// command-line string from the NodeExcludeIPRanges slice.
//
// For example, if the slice is ["192.168.10.1/24", "10.0.0.0/8"], it returns
// "192.168.10.1/24,10.0.0.0/8", which can be passed back into the
// --node-exclude-ip-ranges flag.
func (c *Config) GetNodeExcludeIPRangesCmdString() string {
	if c == nil || len(c.NodeExcludeIPRanges) == 0 {
		return ""
	}
	return strings.Join(c.NodeExcludeIPRanges, ",")
}
