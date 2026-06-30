package ccm

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	ccmutil "github.com/harvester/harvester-cloud-provider/pkg/util"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	wranglecorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	cloudprovider "k8s.io/cloud-provider"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester-cloud-provider/pkg/config"
	utils "github.com/harvester/harvester-cloud-provider/pkg/utils"
)

type instanceManager struct {
	vmClient     ctlkubevirtv1.VirtualMachineClient
	vmiClient    ctlkubevirtv1.VirtualMachineInstanceClient
	nodeClient   wranglecorev1.NodeClient
	nodeToVMName *sync.Map
	namespace    string
	clusterName  string
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

	meta.NodeAddresses, err = getNodeAddresses(node, vmi, config.GetConfig())
	if err != nil {
		return nil, err
	}

	if err := i.annotateNodeWithNADInfo(node); err != nil {
		return nil, err
	}

	return meta, nil
}

func (i *instanceManager) annotateNodeWithNADInfo(node *v1.Node) error {
	// List VMIs once. GetCommonVMINADs computes the {nad→interface} intersection across
	// all nodes, from which both the interface annotation and the IPPool annotation are derived.
	// Filter by the guest cluster label so we only see VMIs belonging to this cluster.
	listOpts := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", utils.LabelKeyGuestClusterNameOnVM, i.clusterName),
	}

	vmiList, err := i.vmiClient.List(i.namespace, listOpts)
	if err != nil {
		return err
	}

	commonNADs := ccmutil.GetCommonVMINADs(vmiList.Items) // {nad → interface}

	if len(commonNADs) > 0 {
		if err := i.annotateNodeWithInterfaceMapping(node.Name, commonNADs); err != nil {
			return fmt.Errorf("failed to annotate node with interface-NAD mapping: %w", err)
		}
	}

	return nil
}

func (i *instanceManager) getVM(node *v1.Node) (*kubevirtv1.VirtualMachine, error) {
	nodeName := node.Name
	if vmName, ok := i.nodeToVMName.Load(nodeName); ok {
		nodeName = vmName.(string)
	}
	return i.vmClient.Get(i.namespace, nodeName, metav1.GetOptions{})
}

/*
getNodeAddresses executes a 4-stage processing pipeline to resolve Node addresses
from the underlying KubeVirt VMI. It is designed to be deterministic and "operator-friendly,"
ensuring that any failure to find an IP results in a clear, actionable WARN log.

Pipeline Stages:

 1. Decision Logic (Strategy Selection):
    Determines the processing mode based on strict priority:
    - Priority 1: Legacy Manual Override via 'alpha.kubernetes.io/provided-node-ip' annotation.
    This is respected primarily for backward compatibility to avoid breaking legacy
    systems. It is a first-class citizen unless explicitly disabled via the
    '--disable-annotation-alpha-provided-ip-addr' flag.
    - Priority 2: Policy-based filtering via CIDR prefixes (--node-ip-cidr).
    RECOMMENDED: This is the preferred mode for multi-nic or multi-ip clusters to
    ensure predictable IP selection.
    - Priority 3: First-fit fallback (automatic discovery of the first valid IPv4/IPv6).

 2. Data Retrieval (Management Network Discovery):
    Identifies which VMI interface to use for Node addressing:
    - Explicit: Uses the network name provided via the '--management-network' flag.
    - Implicit: Defaults to the first multus/secondary network found (first-found rule).
    NOTE: In multi-network environments, it is STRONGLY recommended to set the
    '--management-network' flag explicitly to avoid non-deterministic IP selection.

3. Processing & Filtering (Safety & Scope):

  - Validates IP syntax and discards loopback (127.0.0.1) or invalid strings.

  - Global Exclusion (--node-exclude-ip-ranges): Only active when '--node-ip-cidr'
    is set (Priority 2), allowing strict control over which IPs within a CIDR
    range are permissible.

  - Harvester Filter Annotation ('cloudprovider.harvesterhci.io/additional-internal-ips'):
    Works in Priority 1 (Annotation) or Priority 3 (Fallback) modes. Matched `ExternalIP` IPs
    are excluded from the Node object, hiding them from 'kubectl get nodes'.

    4. Finalization:
    Ensures the NodeHostName is always appended. If no IPs survive the filtration
    gauntlet, the function returns only the Hostname to maintain controller stability
    while logging a specific warning for troubleshooting.

Maintenance Note:
Node IP fetching is event-driven and may not be called frequently by the K8s framework.
Always ensure exit points have unique WARN logs to identify if a missing IP is due
to VMI status lag, configuration mismatch, or strict filtering policies.
*/
func getNodeAddresses(node *v1.Node, vmi *kubevirtv1.VirtualMachineInstance, cfg *config.Config) ([]v1.NodeAddress, error) {
	if vmi == nil {
		return nil, fmt.Errorf("unable to fetch IPs from node %s as its VMI is nil", node.Name)
	}

	getHostNameAddress := func() v1.NodeAddress {
		return v1.NodeAddress{Type: v1.NodeHostName, Address: node.Name}
	}

	// Returns the hostname address anyway.
	// Fallback: In case of error, logs the IP fetching failure but still returns the hostname.
	getNodeAddressWithHostNameOnly := func() []v1.NodeAddress {
		return []v1.NodeAddress{getHostNameAddress()}
	}

	// --- STAGE 1: Decision Logic ---
	networkNames := getManagementNetworks(vmi, cfg)
	if len(networkNames) == 0 {
		logrus.Warnf("No management networks found for node %s via its VMI %s/%s",
			node.Name, vmi.Namespace, vmi.Name)
		return getNodeAddressWithHostNameOnly(), nil
	}

	if len(networkNames) > 1 {
		logrus.Warnf("Multi-network mode detected for node %s via its VMI %s/%s (discovered: %v). "+
			"No --management-network flag provided; falling back to %q. "+
			"Results may be unpredictable—please use the flag to specify the management network.",
			node.Name, vmi.Namespace, vmi.Name, networkNames, networkNames[0])
	}

	targetNetwork := networkNames[0]
	ctx := buildIPAddressProcessContext(node, targetNetwork, cfg)

	// --- STAGE 2: Data Fetching ---
	rawIPStrings, err := getRawIPsFromVMINetwork(vmi, targetNetwork)
	if err != nil {
		logrus.Warnf("Unable to fetch IPs for node %s via its VMI %s/%s on network %s: %v",
			node.Name, vmi.Namespace, vmi.Name, targetNetwork, err)
		return getNodeAddressWithHostNameOnly(), nil
	}

	// --- STAGE 3: Processing Pipeline ---
	validIPs, err := utils.ConvertAndFilterIPs(rawIPStrings)
	if err != nil {
		// rawIPStrings has content, but it's "garbage", log it
		logrus.Errorf("Malformed IP data %q detected for node %s via its VMI %s/%s on network %s: %v",
			rawIPStrings, node.Name, vmi.Namespace, vmi.Name, targetNetwork, err)
		return getNodeAddressWithHostNameOnly(), nil
	}

	if len(validIPs) == 0 {
		logrus.Warnf("Found 0 valid IPs for node %s via its VMI %s/%s on network %s",
			node.Name, vmi.Namespace, vmi.Name, targetNetwork)
		return getNodeAddressWithHostNameOnly(), nil
	}

	// selection, categorization (Internal/External) and filtering
	candidates := resolveNodeIPs(validIPs, ctx)
	if len(candidates) == 0 {
		logrus.Warnf("Found %d IPs but all were filtered for node %s via its VMI %s/%s on network %s",
			len(validIPs), node.Name, vmi.Namespace, vmi.Name, targetNetwork)
		return getNodeAddressWithHostNameOnly(), nil
	}

	// --- STAGE 4: Finalize ---
	finalAddresses := candidates.ToNodeAddresses()
	finalAddresses = append(finalAddresses, getHostNameAddress())

	logrus.Infof("Successfully resolved (fetched, checked and filtered) addresses for node %s via its VMI %s/%s on network %s: %v",
		node.Name, vmi.Namespace, vmi.Name, targetNetwork, finalAddresses)

	return finalAddresses, nil
}

// User may want to mark some IPs of the node also as internal (not exposed on `kubectl get nodes -A -owide`)
// getAdditionalInternalIPs returns a list of IPs from the legacy annotation.
// this is optional; if the annotation is missing or malformed, it returns nil.
//
// Note: When a node reports multiple addresses as "InternalIP", Kubernetes typically
// prioritizes the first entry. This logic effectively "hides" specific IPs from being
// categorized as "ExternalIP" without actually making them functional secondary
// internal addresses in the Kubernetes API.
func getAdditionalInternalIPs(node *v1.Node) []string {
	aiIPs, ok := node.Annotations[utils.KeyAdditionalInternalIPs]
	if !ok || aiIPs == "" || aiIPs == "[]" || aiIPs == "null" {
		return nil
	}

	var ips []string
	if err := json.Unmarshal([]byte(aiIPs), &ips); err != nil {
		logrus.Errorf("skipping optional internal IP filtering for node %s due to malformed annotation: %v", node.Name, err)
		return nil
	}

	return ips
}

// annotateNodeWithInterfaceMapping stores the NAD->interface mapping as a JSON annotation
// on the Kubernetes Node so that frontends can query it via the K8s API.
func (i *instanceManager) annotateNodeWithInterfaceMapping(nodeName string, mapping map[string]string) error {
	data, err := json.Marshal(mapping)
	if err != nil {
		return fmt.Errorf("marshal interface mapping: %w", err)
	}
	value := string(data)

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		node, err := i.nodeClient.Get(nodeName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if node.Annotations[utils.KeyInterfaceNADMapping] == value {
			return nil
		}
		nodeCopy := node.DeepCopy()
		if nodeCopy.Annotations == nil {
			nodeCopy.Annotations = make(map[string]string)
		}
		nodeCopy.Annotations[utils.KeyInterfaceNADMapping] = value
		_, err = i.nodeClient.Update(nodeCopy)
		return err
	})
}
