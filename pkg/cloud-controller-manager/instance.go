package ccm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"slices"
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

	return meta, nil
}

func (i *instanceManager) getVM(node *v1.Node) (*kubevirtv1.VirtualMachine, error) {
	nodeName := node.Name
	if vmName, ok := i.nodeToVMName.Load(nodeName); ok {
		nodeName = vmName.(string)
	}
	return i.vmClient.Get(i.namespace, nodeName, metav1.GetOptions{})
}

// getNodeAddresses return nodeAddresses only when the value of annotation `alpha.kubernetes.io/provided-node-ip` is not empty
func getNodeAddresses(node *v1.Node, vmi *kubevirtv1.VirtualMachineInstance) ([]v1.NodeAddress, error) {
	internalIPRanges, err := getInternalIPRanges(node)
	if err != nil {
		return nil, err
	}

	// Optimistically assume that for every interface have one IP. Add one for the hostname address that we add later.
	// Since the amount of IP addresses is probably very limited this should be fine.
	nodeAddresses := make([]v1.NodeAddress, 0, len(vmi.Status.Interfaces)+1)

	// Build a list of network names (names of NICs) on the VM.
	networkNames := make([]string, 0, len(vmi.Spec.Networks))
	for _, network := range vmi.Spec.Networks {
		networkNames = append(networkNames, network.Name)
	}

	// Find all IP addresses of the VM
	for _, networkInterface := range vmi.Status.Interfaces {
		// The interface list might contain interfaces that do not belong to any NIC of the VM. Filter them out.
		if !slices.Contains(networkNames, networkInterface.Name) {
			// Ignore interface since it does not belong to one of the NICs.
			continue
		}

		for _, ipStr := range networkInterface.IPs {
			ip, err := netip.ParseAddr(ipStr)
			if err != nil {
				// Failed to parse IP, skip it
				logrus.WithFields(logrus.Fields{
					"namespace": node.Namespace,
					"name":      node.Name,
				}).Warnf("Unable to parse IP %s, skip it: %s", ipStr, err.Error())
				continue
			}

			// Skip addresses in link local range, other nodes don't seem to be able to reach this address during cluster bootstrapping.
			if ip.Is6() && linkLocalIPv6Range.Contains(ip) {
				continue
			}

			// Determine if the IP should be listed as an internal or external IP.
			ipType := v1.NodeExternalIP
			for _, internalPrefix := range internalIPRanges {
				if internalPrefix.Contains(ip) {
					// IP is an internal IP, no need to check further.
					ipType = v1.NodeInternalIP
					break
				}
			}

			nodeAddresses = append(nodeAddresses, v1.NodeAddress{
				Type:    ipType,
				Address: ip.String(),
			})
		}
	}

	nodeAddresses = append(nodeAddresses, v1.NodeAddress{
		Type:    v1.NodeHostName,
		Address: node.Name,
	})

	return nodeAddresses, nil
}

func getInternalIPRanges(node *v1.Node) ([]netip.Prefix, error) {
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
	aiIPs, ok := node.Annotations[KeyAdditionalInternalIPs]
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
