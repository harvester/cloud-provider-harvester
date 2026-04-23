package utils

import (
	"fmt"
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
