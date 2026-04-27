package ccm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"strings"
	"sync"

	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/cloud-provider/api"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester-cloud-provider/pkg/config"
	utils "github.com/harvester/harvester-cloud-provider/pkg/utils"
)

var linkLocalIPv6Range = netip.MustParsePrefix("fe80::/10")

type instanceManager struct {
	vmClient     ctlkubevirtv1.VirtualMachineClient
	vmiClient    ctlkubevirtv1.VirtualMachineInstanceClient
	nodeToVMName *sync.Map
	namespace    string
}

func (i *instanceManager) InstanceExists(ctx context.Context, node *v1.Node) (bool, error) {
	if _, err := i.getVM(node); err != nil {
		if !errors.IsNotFound(err) {
			return false, err
		}
		return false, nil
	}
	return true, nil
}

func (i *instanceManager) InstanceShutdown(ctx context.Context, node *v1.Node) (bool, error) {
	vm, err := i.getVM(node)
	if err != nil {
		return false, err
	}
	return !vm.Status.Ready, nil
}

func (i *instanceManager) InstanceMetadata(ctx context.Context, node *v1.Node) (*cloudprovider.InstanceMetadata, error) {
	vm, err := i.getVM(node)
	if err != nil {
		return nil, err
	}

	// Set node topology metadata from virtual machine annotations
	meta := &cloudprovider.InstanceMetadata{
		ProviderID: ProviderName + "://" + string(vm.UID),
	}

	vmi, err := i.vmiClient.Get(i.namespace, vm.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return meta, nil
		}
		return nil, err
	}

	annotations := vmi.GetAnnotations()
	if region, ok := annotations[v1.LabelTopologyRegion]; ok {
		meta.Region = region
	}
	if zone, ok := annotations[v1.LabelTopologyZone]; ok {
		meta.Zone = zone
	}

	meta.NodeAddresses, err = getNodeAddresses(node, vmi)
	if err != nil {
		return nil, err
	}

	logrus.Infof("node %s gets meta NodeAddresses %v", node.Name, meta.NodeAddresses)

	return meta, nil
}

func (i *instanceManager) getVM(node *v1.Node) (*kubevirtv1.VirtualMachine, error) {
	nodeName := node.Name
	if vmName, ok := i.nodeToVMName.Load(nodeName); ok {
		nodeName = vmName.(string)
	}
	return i.vmClient.Get(i.namespace, nodeName, metav1.GetOptions{})
}

func getManagementNetworks(vmi *kubevirtv1.VirtualMachineInstance) []string {
	if config.IsManagementNetworkConfigured() {
		// Build a list of network names (names of NICs) on the VM.
		networkNames := make([]string, 0, 1)
		for _, network := range vmi.Spec.Networks {

			// format:
			// networks:
			//  - multus:
			//	  networkName: default-none/vm-untag3
			// name: nic-0

			if network.Multus == nil {
				// only NAD multus based network is supported on guest cluster
				continue
			}
			// if ManagementNetwork is configured, then strictly match this network
			if utils.MatchManagementNetwork(network.Multus.NetworkName) {
				networkNames = append(networkNames, network.Name)
				break
			}
		}
		return networkNames
	}

	// Build a list of network names (names of NICs) on the VM.
	networkNames := make([]string, 0, len(vmi.Spec.Networks))
	for _, network := range vmi.Spec.Networks {
		// format:
		// networks:
		//  - multus:
		//	  networkName: default-none/vm-untag3
		// name: nic-0
		if network.Multus == nil {
			// only NAD multus based network is supported on guest cluster
			continue
		}
		networkNames = append(networkNames, network.Name)
	}
	return networkNames
}

func getNodeAddresses(node *v1.Node, vmi *kubevirtv1.VirtualMachineInstance) ([]v1.NodeAddress, error) {
	cfg := config.GetConfig()

	// 1. Fetch categorization and exclusion rules
	internalIPRanges, err := getInternalIPRanges(node)
	if err != nil {
		return nil, err
	}

	// 2. Then check VMI
	if vmi == nil {
		return nil, fmt.Errorf("vmi is empty, can't check node %s/%s", node.Namespace, node.Name)
	}

	// Legacy annotation support (treated as a "Negative" filter)
	legacyExcludes, err := getAdditionalInternalIPs(node)
	if err != nil {
		logrus.Warnf("failed to parse legacy additional IPs for node %s: %v", node.Name, err)
	}

	// 2. Gatekeeper: Identify the correct Management NIC
	networkNames := getManagementNetworks(vmi)
	if len(networkNames) == 0 {
		logrus.Warnf("did not find any valid network from vmi %s/%s, can't effectively fetch IP", vmi.Namespace, vmi.Name)
		return []v1.NodeAddress{{Type: v1.NodeHostName, Address: node.Name}}, nil
	}

	// Find more networks, only fetch IP from the first one, if non management-network is specified
	targetNetwork := networkNames[0]
	if len(networkNames) > 1 {
		logrus.Warnf("found %v networks from vmi %s/%s, only fetch IP from the first network %s", len(networkNames), vmi.Namespace, vmi.Name, networkNames[0])
	}

	// 2. Find the status index for the target interface
	idx := -1
	for i, iface := range vmi.Status.Interfaces {
		if iface.Name == targetNetwork {
			idx = i
			break
		}
	}

	if idx == -1 {
		logrus.Warnf("did not find any valid network from vmi %s/%s, can't effectively fetch IP", vmi.Namespace, vmi.Name)
		return []v1.NodeAddress{{Type: v1.NodeHostName, Address: node.Name}}, nil
	}

	// 3. Process IPs using the new helper function
	nodeAddresses := processInterfaceIPs(
		vmi.Status.Interfaces[idx].IPs,
		internalIPRanges,
		cfg.NodeExcludeIPRanges,
		legacyExcludes,
	)

	// always add node name
	nodeAddresses = append(nodeAddresses, v1.NodeAddress{
		Type:    v1.NodeHostName,
		Address: node.Name,
	})

	return nodeAddresses, nil
}

// processInterfaceIPs transforms raw IP strings into categorized NodeAddresses
func processInterfaceIPs(ips []string, internalRanges []netip.Prefix, globalExcludes []string, legacyExcludes []string) []v1.NodeAddress {
	addresses := make([]v1.NodeAddress, 0, len(ips)+1)
	hasInternalIPv4 := false
	hasInternalIPv6 := false

	for _, ipStr := range ips {
		// STEP A: Exclusion logic (The "Third Option")
		if isIPExcluded(ipStr, globalExcludes, legacyExcludes) {
			continue
		}

		ip, err := netip.ParseAddr(ipStr)
		if err != nil {
			continue
		}

		// Standard link-local check
		if ip.Is6() && linkLocalIPv6Range.Contains(ip) {
			continue
		}

		// STEP B: Categorization logic
		ipType := v1.NodeExternalIP
		isMatchCIDR := false
		for _, prefix := range internalRanges {
			if prefix.Contains(ip) {
				isMatchCIDR = true
				break
			}
		}

		if isMatchCIDR {
			ipType = v1.NodeInternalIP
		} else if len(internalRanges) == 0 {
			// Phase 2 Fallback: If no node-ip-cidr, first of family wins
			if ip.Is4() && !hasInternalIPv4 {
				ipType = v1.NodeInternalIP
				hasInternalIPv4 = true
			} else if ip.Is6() && !hasInternalIPv6 {
				ipType = v1.NodeInternalIP
				hasInternalIPv6 = true
			}
		}

		addresses = append(addresses, v1.NodeAddress{
			Type:    ipType,
			Address: ip.String(),
		})
	}

	return addresses
}

func isIPExcluded(ipStr string, globalExcludes []string, legacyExcludes []string) bool {
	// 1. Check Legacy Annotation (Direct string match)
	for _, ex := range legacyExcludes {
		if ipStr == ex {
			return true
		}
	}

	// 2. Check Global Config (Supports both IP and CIDR)
	ip, err := netip.ParseAddr(ipStr)
	if err != nil {
		return false
	}

	for _, rule := range globalExcludes {
		// Try as CIDR
		if prefix, err := netip.ParsePrefix(rule); err == nil {
			if prefix.Contains(ip) {
				return true
			}
			continue
		}
		// Try as Static IP
		if addr, err := netip.ParseAddr(rule); err == nil {
			if addr == ip {
				return true
			}
		}
	}

	return false
}

func getInternalIPRanges(node *v1.Node) ([]netip.Prefix, error) {
	cfg := config.GetConfig()
	internalIPRanges := make([]netip.Prefix, 0, 2)

	// Priority 1: User-defined Global CIDR (The most stable way)
	if cfg.NodeIPCIDR != "" {
		// Handle comma-separated CIDRs if the user provided multiple
		for _, cidr := range strings.Split(cfg.NodeIPCIDR, ",") {
			prefix, err := netip.ParsePrefix(strings.TrimSpace(cidr))
			if err == nil {
				internalIPRanges = append(internalIPRanges, prefix)
			}
		}
		// If we have a global CIDR, we use it strictly.
		return internalIPRanges, nil
	}

	// Priority 2: Kubelet provided IP (--node-ip)
	providedNodeIP, ok := node.Annotations[api.AnnotationAlphaProvidedIPAddr]
	if ok {
		nodeIPRange, err := ipStringToPrefix(providedNodeIP)
		if err == nil {
			internalIPRanges = append(internalIPRanges, nodeIPRange)
		}
	}

	// Priority 3: Legacy Annotations
	extraInternalIPs, err := getAdditionalInternalIPs(node)
	if err != nil {
		logrus.Warnf("skip legacy annotation, error: %s", err.Error())
	} else {
		for _, extra := range extraInternalIPs {
			if extraRange, err := ipStringToPrefix(extra); err == nil {
				internalIPRanges = append(internalIPRanges, extraRange)
			}
		}
	}

	// NOTE: If the list is empty here, processInterfaceIPs will
	// automatically trigger the "First-Win" fallback.
	// We no longer need to append 0.0.0.0/0.
	return internalIPRanges, nil
}

func getInternalIPRangesBackup(node *v1.Node) ([]netip.Prefix, error) {
	internalIPRanges := make([]netip.Prefix, 0, 1) // Most of the time we would only have 1 internal range defined, the provided node IP

	// Kubelet sets this node annotation if the --node-ip flag is set and an external cloud provider is used
	providedNodeIP, ok := node.Annotations[api.AnnotationAlphaProvidedIPAddr]
	if !ok {
		// Annotation is not set, this could be because we are running in a dual stack setup.
		// Assume all IPs are internal IPs.
		internalIPRanges = append(internalIPRanges, netip.MustParsePrefix("0.0.0.0/0"))
		internalIPRanges = append(internalIPRanges, netip.MustParsePrefix("::/0"))
		return internalIPRanges, nil
	}

	// We got an IP from kubelet, parse it and convert it to a prefix containing only this IP
	nodeIPRange, err := ipStringToPrefix(providedNodeIP)
	if err != nil {
		return nil, fmt.Errorf("annotation \"%s\" is invalid: %w", api.AnnotationAlphaProvidedIPAddr, err)
	}
	internalIPRanges = append(internalIPRanges, nodeIPRange)

	// Support marking extra IPs as internal
	extraInternalIPs, err := getAdditionalInternalIPs(node)
	if err != nil {
		// Unable to parse extra provided internal IP ranges, ignore them.
		logrus.WithFields(logrus.Fields{
			"namespace": node.Namespace,
			"name":      node.Name,
		}).Warnf("%s, skip it", err.Error())

		// Return list without extra user defined IP ranges.
		return internalIPRanges, nil
	}

	for _, extraInternalIP := range extraInternalIPs {
		extraRange, err := ipStringToPrefix(extraInternalIP)
		if err != nil {
			// IP (range) malformed, skip it.
			logrus.WithFields(logrus.Fields{
				"namespace": node.Namespace,
				"name":      node.Name,
			}).Warnf("Unable to parse IP %s, skip it: %s", extraInternalIP, err.Error())
			continue
		}
		internalIPRanges = append(internalIPRanges, extraRange)
	}

	return internalIPRanges, nil
}

// ipStringToPrefix converts an IP / CIDR range to a netip.Prefix. It supports IPv4 and IPv6 addresses.
// If a plain IP address is given, it returns a Prefix that only contains this IP.
// If a CIDR range is given, it returns a Prefix that contains the whole range.
func ipStringToPrefix(str string) (netip.Prefix, error) {
	if strings.Contains(str, "/") {
		// CIDR notation
		return netip.ParsePrefix(str)
	}

	// Plain IP address
	addr, err := netip.ParseAddr(str)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("failed to parse IP address \"%s\": %w", str, err)
	}

	// For a single IPv4 address, the prefix length is 32; for IPv6, it's 128.
	prefixLen := 32
	if addr.Is6() {
		prefixLen = 128
	}

	// Create a prefix with the single address in it.
	return addr.Prefix(prefixLen)
}

// User may want to mark some IPs of the node also as internal
func getAdditionalInternalIPs(node *v1.Node) ([]string, error) {
	aiIPs, ok := node.Annotations[utils.KeyAdditionalInternalIPs]
	if !ok {
		return nil, nil
	}
	var ips []string
	err := json.Unmarshal([]byte(aiIPs), &ips)
	if err != nil {
		return nil, fmt.Errorf("failed to decode additional external IPs from %v: %w", aiIPs, err)
	}
	return ips, nil
}
