package virtualmachineinstance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	cfg "github.com/harvester/harvester-cloud-provider/pkg/config"
	ctlv1 "github.com/harvester/harvester-cloud-provider/pkg/generated/controllers/kubevirt.io/v1"
	utils "github.com/harvester/harvester-cloud-provider/pkg/utils"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	cloudproviderapi "k8s.io/cloud-provider/api"
	cloudnodeutil "k8s.io/cloud-provider/node/helpers"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"
)

// from a vm/vmi.Name to get the hostname
type vmiCacheInfo struct {
	hostname string
	uid      types.UID
}

const (
	vmiControllerName               = "harvester-cloudprovider-resync-topology"
	vmiNetworkMappingControllerName = "harvester-cloudprovider-resync-network-mapping"
)

var errNodeToVMConflict = errors.New("node to vm name mapping conflict")

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
	nodeToVMName *sync.Map,
	namespace string,
) {
	handler := &Handler{
		vmis:            vmis,
		vmiCache:        vmis.Cache(),
		nodeCache:       nodes.Cache(),
		configMapClient: configMaps,
		restClient:      restClient,
		kubevirtClient:  kubevirtClient,
		nodeToVMName:    nodeToVMName,
		vmiToHostname:   &sync.Map{},
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

type Handler struct {
	vmis            ctlv1.VirtualMachineInstanceController
	vmiCache        ctlv1.VirtualMachineInstanceCache
	nodeCache       ctlcorev1.NodeCache
	configMapClient ctlcorev1.ConfigMapClient
	restClient      kubernetes.Interface
	kubevirtClient  kubecli.KubevirtClient

	nodeToVMName  *sync.Map // nodeName (hostname) -> vmi.Name quick lookup
	vmiToHostname *sync.Map // vmi.Name -> vmi.hostname quick lookup

	namespace string
}

// The vmiControllerName controller performs two major tasks:
//  1. Syncs VMI annotations with node labels to ensure cluster topology remains consistent.
//  2. Maintains mapping caches for fast, concurrent-safe lookups:
//     2.1 nodeToVMName: Maps nodeName (hostname) -> vmi.Name.
//     2.2 vmiToHostname: Maps vmi.Name -> hostname (backed by vmiCacheInfo containing
//     the VMI UID to automatically invalidate stale entries across reboots/recreations
//     and avoid expensive GuestOsInfo calls).
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

	// If both HCP and the VMI specify a valid guest cluster name,
	// perform a cluster name check to ensure they match.
	// due to legacy reasons, this check is not strictly enforced if any does not carry a valid name.
	gcName := utils.GetLabelGuestClusterName(vmi)
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
		return h.syncVMIAndNodeTopology(vmi, vmi.Name)
	}

	hostname := vmi.Name

	switch {
	// Found in local cache map
	case func() bool {
		if hName, found := h.getHostnameFromVMIToHostnameMap(vmi); found {
			hostname = hName
			return true
		}
		return false
	}():

	// Agent not connected yet, keep vmi.Name as fallback
	case !utils.IsGuestAgentConnected(vmi):

	// get hostname
	default:
		guestAgentInfo, err := h.kubevirtClient.VirtualMachineInstance(vmi.Namespace).GuestOsInfo(context.TODO(), vmi.Name)

		switch {
		case err != nil:
			logrus.WithFields(logrus.Fields{
				"name":      vmi.Name,
				"namespace": vmi.Namespace,
			}).WithError(err).Error("failed to get guest agent info, fallback to use vmi name as node name")

		case guestAgentInfo.Hostname == "":
			logrus.WithFields(logrus.Fields{
				"name":      vmi.Name,
				"namespace": vmi.Namespace,
			}).Warn("guest agent info returns an empty hostname, fallback to use vmi name as node name")

		default:
			hostname = guestAgentInfo.Hostname
			logrus.WithFields(logrus.Fields{
				"name":      vmi.Name,
				"namespace": vmi.Namespace,
				"hostname":  hostname,
			}).Info("get agent info success, using hostname as node name")

			// Store the hostname mapped to the unique VMI name to avoid expensive GuestOsInfo calls.
			// If the VM is rebooted or recreated, the new VMI instance gets a distinct UID,
			// which automatically invalidates this cached entry.
			//
			// Note: If a user manually changes the hostname inside a running guest OS (e.g.,
			// via hostnamectl), the controller will not detect this change due to the cache.
			// Runtime hostname changes are unsupported and will cause severe topology
			// desynchronization issues in the Kubernetes context since the node name mapping
			// is already permanently bound.
			h.storeHostnameToVMIToHostnameMap(vmi, hostname)
		}
	}

	// conflict check:
	// if multi vmis have same hostname, HCP refuse to store additional one
	vmiName, found := h.getVMFromNodeToVMNameMap(hostname)
	if found && vmi.Name != vmiName {
		return vmi, fmt.Errorf("%w vmi %s hostname %s resolves to node %s, but node points to another vmi %s, cannot sync topology", errNodeToVMConflict, vmi.Name, hostname, hostname, vmiName)
	}

	// for instance.go to use
	logrus.Infof("store [node:%s, vmi:%s] for quick lookup", hostname, vmi.Name)
	h.nodeToVMName.Store(hostname, vmi.Name)

	return h.syncVMIAndNodeTopology(vmi, hostname)
}

func (h *Handler) reSync(nodeName string) error {
	logrus.Infof("prepare to taint %s on node %s to trigger the sync", cloudproviderapi.TaintExternalCloudProvider, nodeName)
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
//
// Note: This is a new feature introduced in HCP 0.2.14. It requires both the HCP
// and the VMI to have a valid, matching guest cluster name to function properly.
func (h *Handler) syncNADMappingConfigMap() error {
	clusterName := cfg.GetConfig().ClusterName

	if !utils.IsNormalGuestClusterName(clusterName) {
		// Return an error and exit early to prevent cross-cluster pollution
		return fmt.Errorf("failed to sync NAD mapping ConfigMap: guest cluster name configuration is empty/default, cannot identify the cluster")
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

func (h *Handler) syncVMIAndNodeTopology(vmi *kubevirtv1.VirtualMachineInstance, hostname string) (*kubevirtv1.VirtualMachineInstance, error) {
	node, err := h.nodeCache.Get(hostname)
	if err != nil {
		// This vm does not belong to current cluster, or the node object is not created yet, return error to retry
		return vmi, fmt.Errorf("failed to get node via vm name %s/%s hostname %s: %w", vmi.Namespace, vmi.Name, hostname, err)
	}
	if !compareTopology(vmi.GetAnnotations(), node.GetLabels()) {
		if err := h.reSync(node.Name); err != nil {
			return vmi, err
		}
	}
	return vmi, nil
}

// node->vm map
func (h *Handler) getVMFromNodeToVMNameMap(nodeName string) (string, bool) {
	if val, ok := h.nodeToVMName.Load(nodeName); ok {
		return val.(string), ok
	}

	return "", false
}

// vmi -> cacheInfo map check with built-in UID validation
func (h *Handler) getHostnameFromVMIToHostnameMap(vmi *kubevirtv1.VirtualMachineInstance) (string, bool) {
	if val, ok := h.vmiToHostname.Load(vmi.Name); ok {
		if info, ok := val.(vmiCacheInfo); ok && info.uid == vmi.UID {
			return info.hostname, true
		}
		// UID mismatch means the VMI was recreated/rebooted; evict the stale entry
		h.vmiToHostname.Delete(vmi.Name)
	}

	return "", false
}

// storeHostnameToVMIToHostnameMap stores the resolved hostname and VMI UID for quick lookup.
// Encapsulating this allows for easy expansion if additional cache fields are needed later.
func (h *Handler) storeHostnameToVMIToHostnameMap(vmi *kubevirtv1.VirtualMachineInstance, hostname string) {
	h.vmiToHostname.Store(vmi.Name, vmiCacheInfo{
		hostname: hostname,
		uid:      vmi.UID,
	})
}
