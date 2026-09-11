package virtualmachineinstance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	cfg "github.com/harvester/harvester-cloud-provider/pkg/config"
	ctlv1 "github.com/harvester/harvester-cloud-provider/pkg/generated/controllers/kubevirt.io/v1"
	utils "github.com/harvester/harvester-cloud-provider/pkg/utils"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	cloudproviderapi "k8s.io/cloud-provider/api"
	cloudnodeutil "k8s.io/cloud-provider/node/helpers"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"
)

const (
	vmiControllerName               = "harvester-cloudprovider-resync-topology"
	vmiNetworkMappingControllerName = "harvester-cloudprovider-resync-network-mapping"
)

var ErrAnnotationConflict = errors.New("node annotation conflict")

type Handler struct {
	vmis            ctlv1.VirtualMachineInstanceController
	vmiCache        ctlv1.VirtualMachineInstanceCache
	nodeClient      ctlcorev1.NodeClient
	nodeCache       ctlcorev1.NodeCache
	configMapClient ctlcorev1.ConfigMapClient
	restClient      kubernetes.Interface
	kubevirtClient  kubecli.KubevirtClient

	namespace string
}

// Register the controller is helping to re-sync harvester node topology labels to guest cluster nodes.
// when the migration is completed, the controller will re-sync the labels to guest cluster nodes.
// this is to make sure the node topology labels are always up-to-date.
func Register(
	ctx context.Context,
	restClient kubernetes.Interface,
	nodes ctlcorev1.NodeController,
	configMaps ctlcorev1.ConfigMapController,
	vmis ctlv1.VirtualMachineInstanceController,
	kubevirtClient kubecli.KubevirtClient,

	namespace string,
) {
	handler := &Handler{
		vmis:            vmis,
		vmiCache:        vmis.Cache(),
		nodeClient:      nodes,
		nodeCache:       nodes.Cache(),
		configMapClient: configMaps,
		restClient:      restClient,
		kubevirtClient:  kubevirtClient,
		namespace:       namespace,
	}
	logrus.WithFields(logrus.Fields{
		"controller1": vmiControllerName,
		"controller2": vmiNetworkMappingControllerName,
		"namespace":   namespace,
	}).Info("start watching virtual machine instance")

	vmis.OnChange(ctx, vmiNetworkMappingControllerName, handler.OnVmiChangedNetworkMapping)
	vmis.OnChange(ctx, vmiControllerName, handler.OnVmiChanged)
}

func (h *Handler) OnVmiChanged(_ string, vmi *kubevirtv1.VirtualMachineInstance) (*kubevirtv1.VirtualMachineInstance, error) {
	if vmi == nil || vmi.DeletionTimestamp != nil {
		return vmi, nil
	}

	// unrelated vmis, don't log anything
	if vmi.Namespace != h.namespace || !utils.IsRunning(vmi) || !utils.IsMigrationCompleted(vmi) {
		return vmi, nil
	}

	if !utils.IsVmiCreatedFromHarvesterCreator(vmi) {
		logrus.WithFields(logrus.Fields{
			"namespace": vmi.Namespace,
			"name":      vmi.Name,
		}).Debug("skip processing virtual machine instance which is not created by Harvester creator")
		return vmi, nil
	}

	gcName := utils.GetLabelGuestClusterName(vmi)

	// If both the configured cluster and the VMI specify a valid guest cluster name,
	// perform a strict check to ensure they match.
	savedGcName := cfg.GetConfig().ClusterName
	if utils.IsNormalGuestClusterName(savedGcName) && utils.IsNormalGuestClusterName(gcName) {
		if gcName != savedGcName {
			logrus.WithFields(logrus.Fields{
				"namespace":             vmi.Namespace,
				"name":                  vmi.Name,
				"vm-guest-cluster":      gcName,
				"current-guest-cluster": savedGcName,
			}).Debug("skip processing virtual machine instance: VMI does not belong to current cluster")
			return vmi, nil
		}
		// match, continue
	} else {
		// Fallback for legacy environments where guest cluster info is missing or non-standard;
		// continue processing, noting that behavior may be ambiguous in multi-cluster namespaces.
		logrus.WithFields(logrus.Fields{
			"namespace":           vmi.Namespace,
			"name":                vmi.Name,
			"vmGuestCluster":      gcName,
			"currentGuestCluster": savedGcName,
		}).Debug("Insufficient guest cluster information; proceeding")
	}

	// when hostname lookup is disabled, controller works solely via vmi.Name == node.Name
	// avoiding tricky, error-prone guest OS lookup scenarios.
	if cfg.GetConfig().DisableHostnameLookup {
		return h.handleStrictVMNameNodeMapping(vmi)
	}

	// WARNING: legacy behaviour
	// HCP supports node.Name=vmi.hostname where vmi.hostname != vmi.Name.
	// Lacking explicit annotations or labels on the node/object, it blindly guesses the relationship between these names,
	// which is inherently error-prone.
	// The following code tries its best to handle this, but cannot deal with all tricky scenarios.
	// To eliminate this entirely, use the config option `--disable-hostname-lookup=true`.

	// Fast Path A: Direct Name Match (Standard RKE2/K3s deployments)
	// For standard RKE2/K3s deployments, this direct lookup is entirely sufficient.
	// We avoid using an in-memory shared map/cache; instead, we persist the mapping
	// directly to the guest node object annotation for reliable, persistent lookup.
	node, err := h.nodeCache.Get(vmi.Name)
	var conflictErr error
	if err == nil {
		if conflictErr = h.annotateNodeWithVMIName(node, vmi.Name); conflictErr != nil {
			if !errors.Is(conflictErr, ErrAnnotationConflict) {
				return vmi, fmt.Errorf("failed to annotate node %s which is same with vm name: %w", node.Name, conflictErr)
			}
			// conflict, will be handled after Fast Path B
		} else {
			// node matches vmi
			return h.syncNodeAndVMI(vmi, node)
		}
	} else if !apierrors.IsNotFound(err) {
		return vmi, err
	}

	// Fast Path B: Search Node Cache for existing Annotation
	annotatedNode, err := h.findNodeByVMIAnnotation(vmi.Name)
	if err != nil {
		return vmi, err
	}

	// Find the node via vmi.Name
	if annotatedNode != nil {
		return h.syncNodeAndVMI(vmi, annotatedNode)
	}

	// Fallback: Executed if node lookup fails or an ErrAnnotationConflict occurs
	// (e.g., a node matching the VMI name is already claimed by a different VMI).
	// The controller proceeds to check if the VMI's guest agent hostname resolves to a node.
	if conflictErr != nil {
		logrus.Infof("%s, will continue to check if hostname matches", conflictErr.Error())
	}

	// A few possibilities:
	//  (1) node is same as hostname, but hostname is not available yet
	//  (2) node is not registered, new guest vm is still on the bootstrap stage, neither vmi.Name nor hostname could resolve a node
	// HCP can only wait & retry
	//  (3) something is wrong, e.g. node name: example-0, vmi.Name: example-1, hostname: example-2
	// before further extend HCP to support inject <node,vm> map info into vmi, HCP cannot handle case (3)
	// customize hostname in guest-cluster provision scenario brings more trouble than benefits

	// Slow Path / Fallback ONLY: Query Guest Agent RPC
	if !utils.IsGuestAgentConnected(vmi) {
		return vmi, fmt.Errorf("guest agent is not connected for VMI %s/%s, cannot resolve node via hostname fallback", vmi.Namespace, vmi.Name)
	}

	logrus.WithFields(logrus.Fields{
		"name":      vmi.Name,
		"namespace": vmi.Namespace,
	}).Debug("node not found name or annotation, querying guest agent info as fallback")

	// NOTE: Avoid calling GuestOsInfo unless strictly necessary. It is an expensive
	// subresource call reaching into the guest agent that can easily bottleneck the controller.
	// GuestOsInfo is used here as a slow-path fallback solely to handle edge cases where
	// users configure custom VM hostnames.
	//
	// Architectural Limitation: Relying on custom or fixed cloud-init-based hostnames
	// conflicts with Rancher machine pool design. A single machine pool can scale to
	// multiple instances; enforcing static custom hostnames limits this operational
	// flexibility and risks severe name collisions.
	//
	// Optimization: Even when custom hostnames are used, HCP smartly persists the
	// resolved VMI-to-node mapping (via node annotations/cache) upon the initial lookup.
	// Subsequent reconciliations hit the fast path directly, entirely bypassing
	// redundant GuestOsInfo calls.

	guestAgentInfo, agentErr := h.kubevirtClient.VirtualMachineInstance(vmi.Namespace).GuestOsInfo(context.TODO(), vmi.Name)
	if agentErr != nil {
		return vmi, fmt.Errorf("failed to get guest agent info of VMI %s/%s: %w", vmi.Namespace, vmi.Name, agentErr)
	}

	hostname := guestAgentInfo.Hostname
	// if VMI belongs to current guest cluster, but the hostname is empty, retry
	if hostname == "" {
		return vmi, fmt.Errorf("VMI %s/%s get an empty hostname, cannot decide its node name", vmi.Namespace, vmi.Name)
	}

	node, err = h.nodeCache.Get(hostname)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return vmi, fmt.Errorf("failed to find node via vmi %s/%s hostname %s: %w", vmi.Namespace, vmi.Name, hostname, err)
		}
		// if VMI belongs to current guest cluster, but it occurs early than guest cluster node, retry
		// if it's totally wrong, still retry
		return vmi, fmt.Errorf("VMI %s/%s hostname %s, but did not find related node", vmi.Namespace, vmi.Name, hostname)
	}

	// Store the VMI info into the node for future quick lookup and lock this mapping;
	// it ensures the expensive `GuestOsInfo` call runs as few as possible.
	// Once bound, subsequent guest hostname changes will not affect controller lookups:
	// if a user or script deliberately changes the VMI hostname after initial booting,
	// HCP does not resync this value.
	// In a Kubernetes context, don't play with the hostname;
	// while changing it causes Kubernetes issues, that is out of scope for HCP.
	if err := h.annotateNodeWithVMIName(node, vmi.Name); err != nil {
		return vmi, fmt.Errorf("failed to annotate node %s (via hostname %s look) with vm name %s: %w", node.Name, hostname, vmi.Name, err)
	}

	return h.syncNodeAndVMI(vmi, node)
}

func (h *Handler) syncNodeAndVMI(vmi *kubevirtv1.VirtualMachineInstance, node *corev1.Node) (*kubevirtv1.VirtualMachineInstance, error) {
	if topologyChanged(vmi.GetAnnotations(), node.GetLabels()) {
		if err := h.reSync(node.Name); err != nil {
			return vmi, fmt.Errorf("failed to reSync node %s and vmi due to topology difference %s/%s: %w", node.Name, vmi.Namespace, vmi.Name, err)
		}
	}
	return vmi, nil
}

func (h *Handler) findNodeByVMIAnnotation(vmiName string) (*corev1.Node, error) {
	nodes, err := h.nodeCache.List(labels.Everything())
	if err != nil {
		return nil, err
	}

	for _, node := range nodes {
		if node.Annotations[utils.AnnotationVMNameOfGuestClusterNode] == vmiName {
			return node, nil
		}
	}
	return nil, nil
}

// annotateNodeWithVMIName enforces a persistent mapping between the guest node and the VMI name.
// HCP refuses to update an existing annotation with a different VMI name, returning
// ErrAnnotationConflict if a manual hack or cross-wiring collision is detected.
func (h *Handler) annotateNodeWithVMIName(node *corev1.Node, vmiName string) error {
	if node == nil {
		return nil
	}

	existing := node.Annotations[utils.AnnotationVMNameOfGuestClusterNode]
	if existing == vmiName {
		return nil
	}

	if existing != "" {
		return fmt.Errorf("%w: node (%s) annotation already points to vm(%s), cannot annotate to new vm(%s)", ErrAnnotationConflict, node.Name, existing, vmiName)
	}

	nodeCopy := node.DeepCopy()
	if nodeCopy.Annotations == nil {
		nodeCopy.Annotations = make(map[string]string)
	}

	nodeCopy.Annotations[utils.AnnotationVMNameOfGuestClusterNode] = vmiName

	logrus.WithFields(logrus.Fields{
		"node":    node.Name,
		"vmiName": vmiName,
	}).Info("annotating node with harvester VM metadata")

	if _, err := h.nodeClient.Update(nodeCopy); err != nil {
		return fmt.Errorf("failed to update node %s annotations: %w", node.Name, err)
	}

	return nil
}

func (h *Handler) reSync(nodeName string) error {
	logrus.Infof("prepare to taint node %s with %s to trigger a sync", nodeName, cloudproviderapi.TaintExternalCloudProvider)
	return cloudnodeutil.AddOrUpdateTaintOnNode(h.restClient, nodeName, &corev1.Taint{
		Key:    cloudproviderapi.TaintExternalCloudProvider,
		Value:  "true",
		Effect: corev1.TaintEffectPreferNoSchedule,
	})
}

func topologyChanged(a map[string]string, b map[string]string) bool {
	return a[corev1.LabelTopologyRegion] != b[corev1.LabelTopologyRegion] ||
		a[corev1.LabelTopologyZone] != b[corev1.LabelTopologyZone]
}

// OnVmiChangedNetworkMapping is decoupled from individual VMI-to-node resolution,
// using Harvester VMI definitions as the single source of truth for global network mapping.
//
// Note: While individual running/completed VMI changes trigger this reconciliation,
// syncNADMappingConfigMap evaluates the aggregate state across the cluster's VMI list
// while properly filters out non-running or migrating VMs.
func (h *Handler) OnVmiChangedNetworkMapping(_ string, vmi *kubevirtv1.VirtualMachineInstance) (*kubevirtv1.VirtualMachineInstance, error) {
	// Note: Since there is no OnRemove controller, OnChange handles actual object deletions
	// where Wrangler passes a nil object (along with instances having a DeletionTimestamp).
	// Calling syncNADMappingConfigMap here is safe and ensures proper cleanup of related VMs.
	if vmi == nil {
		return vmi, h.syncNADMappingConfigMap()
	}

	// unrelated vmis, don't log anything
	if vmi.Namespace != h.namespace {
		return vmi, nil
	}

	if !utils.IsVmiCreatedFromHarvesterCreator(vmi) {
		logrus.WithFields(logrus.Fields{
			"namespace": vmi.Namespace,
			"name":      vmi.Name,
		}).Debug("skip processing virtual machine instance which is not created by Harvester creator")
		return vmi, nil
	}
	return vmi, h.syncNADMappingConfigMap()
}

// syncNADMappingConfigMap computes the common NAD→interface mapping across all VMIs
// in this guest cluster and stores it in a ConfigMap in kube-system.
// If the mapping is empty, the value is cleared (set to "").
func (h *Handler) syncNADMappingConfigMap() error {
	clusterName := cfg.GetConfig().ClusterName

	if !utils.IsNormalGuestClusterName(clusterName) {
		logrus.Warnf("failed to sync NAD mapping ConfigMap: guest cluster name %s is empty/default, cannot identify the cluster", clusterName)
		/// Note: Since clusterName is injected at pod start time, returning an error to trigger a reconciler retry is unnecessary.
		return nil
	}

	sel := labels.Set{utils.LabelKeyGuestClusterNameOnVM: clusterName}.AsSelector()
	vmis, err := h.vmiCache.List(h.namespace, sel)
	if err != nil {
		return err
	}

	var value string
	if mapping := utils.GetCommonVMINADs(vmis); len(mapping) > 0 {
		data, err := json.Marshal(mapping)
		if err != nil {
			return fmt.Errorf("marshal NAD mapping: %w", err)
		}
		value = string(data)
	}

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		existing, err := h.configMapClient.Get(metav1.NamespaceSystem, utils.ConfigMapNADMapping, metav1.GetOptions{})
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
			_, err = h.configMapClient.Create(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      utils.ConfigMapNADMapping,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{
					utils.ConfigMapKeyNADMapping: value,
				},
			})
			return err
		}
		if existing.Data[utils.ConfigMapKeyNADMapping] == value {
			return nil
		}
		cmCopy := existing.DeepCopy()
		if cmCopy.Data == nil {
			cmCopy.Data = make(map[string]string)
		}
		cmCopy.Data[utils.ConfigMapKeyNADMapping] = value
		_, err = h.configMapClient.Update(cmCopy)
		return err
	})
}

// handleStrictVMNameNodeMapping enforces a strict 1:1 mapping between the VirtualMachineInstance
// name and the Kubernetes Node name. When hostname lookup is disabled, it bypasses annotations
// and guest agent lookups, directly fetching the corresponding node from the cache using vmi.Name
// and synchronizing them.
func (h *Handler) handleStrictVMNameNodeMapping(vmi *kubevirtv1.VirtualMachineInstance) (*kubevirtv1.VirtualMachineInstance, error) {
	node, err := h.nodeCache.Get(vmi.Name)
	if err != nil {
		return vmi, fmt.Errorf("failed to get node via vm name %s/%s: %w", vmi.Namespace, vmi.Name, err)
	}
	return h.syncNodeAndVMI(vmi, node)
}
