package virtualmachineinstance

import (
	"context"
	"encoding/json"
	"fmt"

	cfg "github.com/harvester/harvester-cloud-provider/pkg/config"
	ctlv1 "github.com/harvester/harvester-cloud-provider/pkg/generated/controllers/kubevirt.io/v1"
	utils "github.com/harvester/harvester-cloud-provider/pkg/utils"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
	vmiControllerName = "harvester-cloudprovider-resync-topology"
)

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
		"controller": vmiControllerName,
		"namespace":  namespace,
	}).Info("start watching virtual machine instance")
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
	if gcName == "" {
		logrus.WithFields(logrus.Fields{
			"namespace": vmi.Namespace,
			"name":      vmi.Name,
		}).Debug("skip processing virtual machine instance which is not carrying cluster name")
		return vmi, nil
	}

	// if HCP is started with an empty/default cluster name, it means the cluster-name param is not correctly passed
	// the cluster-name check is skipped, and fallback to following check
	savedGcName := cfg.GetConfig().ClusterName
	if utils.IsNormalGuestClusterName(savedGcName) {
		if gcName != savedGcName {
			logrus.WithFields(logrus.Fields{
				"namespace":             vmi.Namespace,
				"name":                  vmi.Name,
				"vm-guest-cluster":      gcName,
				"current-guest-cluster": savedGcName,
			}).Debug("skip processing virtual machine instance: VMI does not belong to current cluster")
			return vmi, nil
		}
	} else {
		logrus.WithFields(logrus.Fields{
			"namespace":             vmi.Namespace,
			"name":                  vmi.Name,
			"vm-guest-cluster":      gcName,
			"current-guest-cluster": savedGcName,
		}).Debug("cluster-name parameter is empty or invalid in HCP config; falling back to checking VMI directly, which may be inaccurate in multi-cluster namespaces")
	}

	// Fast Path A: Direct Name Match (Standard RKE2/K3s deployments)
	// For standard RKE2/K3s deployments, this direct lookup is entirely sufficient.
	// We avoid using an in-memory shared map/cache; instead, we persist the mapping
	// directly to the guest node object annotation for reliable, persistent lookup.
	node, err := h.nodeCache.Get(vmi.Name)
	if err == nil {
		if err := h.annotateNodeWithVMIName(node, vmi.Name); err != nil {
			return vmi, fmt.Errorf("failed to annotate node %s which is same with vm name: %w", node.Name, err)
		}
		return h.syncNodeAndVMI(vmi, node)
	} else if !errors.IsNotFound(err) {
		return vmi, err
	}

	// Fast Path B: Search Node Cache for existing Annotation
	annotatedNode, err := h.findNodeByVMIAnnotation(vmi.Name)
	if err != nil {
		return vmi, err
	}
	if annotatedNode != nil {
		return h.syncNodeAndVMI(vmi, annotatedNode)
	}

	// Slow Path / Fallback ONLY: Query Guest Agent RPC
	logrus.WithFields(logrus.Fields{
		"name":      vmi.Name,
		"namespace": vmi.Namespace,
	}).Debug("node not found by name or annotation, querying guest agent info as fallback")

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
		logrus.WithFields(logrus.Fields{
			"name":      vmi.Name,
			"namespace": vmi.Namespace,
		}).WithError(agentErr).Error("failed to get guest agent info")
		return vmi, fmt.Errorf("failed to get guest agent info for VMI %s/%s: %w", vmi.Namespace, vmi.Name, agentErr)
	}

	hostName := guestAgentInfo.Hostname
	// if VMI belongs to current guest cluster, but the hostname is empty, retry
	if hostName == "" {
		logrus.WithFields(logrus.Fields{
			"name":      vmi.Name,
			"namespace": vmi.Namespace,
		}).Info("guest agent info returned an empty hostname for VMI")
		return vmi, fmt.Errorf("VMI %s/%s has invalid empty hostname, cannot decide int's node name", vmi.Namespace, vmi.Name)
	}

	// if VMI belongs to current guest cluster, but cannot find node by hostname, retry
	node, err = h.nodeCache.Get(hostName)
	if err != nil {
		if !errors.IsNotFound(err) {
			return vmi, err
		}
		logrus.WithFields(logrus.Fields{
			"name":      vmi.Name,
			"namespace": vmi.Namespace,
			"hostname":  hostName,
		}).Info("node with guest agent hostname not found in cache")
		return vmi, fmt.Errorf("VMI %s/%s hostname %s, but can't find related node", vmi.Namespace, vmi.Name, hostName)
	}

	// put the vmi info into node, for future quick look up
	// it will ensure the expensive `GuestOsInfo` call runs as few as possible
	if err := h.annotateNodeWithVMIHostName(node, vmi.Name, hostName); err != nil {
		return vmi, fmt.Errorf("failed to annotate node %s: %w", node.Name, err)
	}

	return h.syncNodeAndVMI(vmi, node)
}

func (h *Handler) syncNodeAndVMI(vmi *kubevirtv1.VirtualMachineInstance, node *corev1.Node) (*kubevirtv1.VirtualMachineInstance, error) {
	if !compareTopology(vmi.GetAnnotations(), node.GetLabels()) {
		if err := h.reSync(node.Name); err != nil {
			return vmi, fmt.Errorf("failed to reSync node %s and vmi due to topology difference %s/%s", node.Name, vmi.Namespace, vmi.Name)
		}
	}

	if err := h.syncNADMappingConfigMap(); err != nil {
		return vmi, fmt.Errorf("failed to sync NAD mapping ConfigMap: %w", err)
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

func (h *Handler) annotateNodeWithVMIName(node *corev1.Node, vmiName string) error {
	if node == nil {
		return nil
	}

	existing := node.Annotations[utils.AnnotationVMNameOfGuestClusterNode]
	if existing == vmiName {
		return nil
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

// annotateNodeWithVMIHostName handles custom hostname mappings where vmiName != hostName.
// It enforces strict safety checks: if the node is already annotated with a different VMI,
// it rejects the update to prevent accidental overwrites or conflicts (e.g., from hostname reassignments).
func (h *Handler) annotateNodeWithVMIHostName(node *corev1.Node, vmiName string, hostName string) error {
	if node == nil {
		return nil
	}

	existing := node.Annotations[utils.AnnotationVMNameOfGuestClusterNode]
	if existing == vmiName {
		return nil
	}

	// customized hostname case: plans to overwrite others, something must be wrong, deny
	if existing != "" {
		err := fmt.Errorf("node annotation already points to vm(%s), can not annotate to new vm(%s) via it's hostname(%s) lookup", existing, vmiName, hostName)
		logrus.Warnf("%s", err.Error())
		return err
	}

	nodeCopy := node.DeepCopy()
	if nodeCopy.Annotations == nil {
		nodeCopy.Annotations = make(map[string]string)
	}

	nodeCopy.Annotations[utils.AnnotationVMNameOfGuestClusterNode] = vmiName

	logrus.WithFields(logrus.Fields{
		"node":     node.Name,
		"vmiName":  vmiName,
		"hostName": hostName,
	}).Info("annotating node with harvester VM via the hostName lookup")

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

func compareTopology(a map[string]string, b map[string]string) bool {
	return a[corev1.LabelTopologyRegion] == b[corev1.LabelTopologyRegion] &&
		a[corev1.LabelTopologyZone] == b[corev1.LabelTopologyZone]
}

// syncNADMappingConfigMap computes the common NAD→interface mapping across all VMIs
// in this guest cluster and stores it in a ConfigMap in kube-system.
// If the mapping is empty, the value is cleared (set to "").
func (h *Handler) syncNADMappingConfigMap() error {
	clusterName := cfg.GetConfig().ClusterName

	if !utils.IsNormalGuestClusterName(clusterName) {
		// Return an error and exit early to prevent cross-cluster pollution
		logrus.Warnf("failed to sync NAD mapping ConfigMap: guest cluster name %s is empty/default, cannot identify the cluster", clusterName)
		// note: as the clusterName in injected at pod start time, don't return error to trigger reconciller
		// as try makes no sense
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
			if !errors.IsNotFound(err) {
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
