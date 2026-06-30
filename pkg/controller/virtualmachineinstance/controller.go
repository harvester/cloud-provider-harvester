package virtualmachineinstance

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	cfg "github.com/harvester/harvester-cloud-provider/pkg/config"
	ccmutil "github.com/harvester/harvester-cloud-provider/pkg/util"
	utils "github.com/harvester/harvester-cloud-provider/pkg/utils"
	"github.com/harvester/harvester/pkg/builder"
	ctlv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	harvesterutil "github.com/harvester/harvester/pkg/util"
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

// Register the controller is helping to re-sync harvester node topology labels to guest cluster nodes.
// when the migration is completed, the controller will re-sync the labels to guest cluster nodes.
// this is to make sure the node topology labels are always up-to-date.
func Register(
	ctx context.Context,
	restClient kubernetes.Interface,
	nodes ctlcorev1.NodeController,
	vmis ctlv1.VirtualMachineInstanceController,
	kubevirtClient kubecli.KubevirtClient,
	nodeToVMName *sync.Map,
	namespace string,
) {
	handler := &Handler{
		vmis:           vmis,
		vmiCache:       vmis.Cache(),
		nodeCache:      nodes.Cache(),
		nodeClient:     nodes,
		restClient:     restClient,
		kubevirtClient: kubevirtClient,
		nodeToVMName:   nodeToVMName,
		namespace:      namespace,
	}
	logrus.WithFields(logrus.Fields{
		"controller": vmiControllerName,
		"namespace":  namespace,
	}).Info("start watching virtual machine instance")
	vmis.OnChange(ctx, vmiControllerName, handler.OnVmiChanged)
}

type Handler struct {
	vmis           ctlv1.VirtualMachineInstanceController
	vmiCache       ctlv1.VirtualMachineInstanceCache
	nodeCache      ctlcorev1.NodeCache
	nodeClient     ctlcorev1.NodeClient
	restClient     kubernetes.Interface
	kubevirtClient kubecli.KubevirtClient

	nodeToVMName *sync.Map

	namespace string
}

func (h *Handler) OnVmiChanged(_ string, vmi *kubevirtv1.VirtualMachineInstance) (*kubevirtv1.VirtualMachineInstance, error) {
	// TODO: Add some unit tests for this controller

	if vmi == nil || vmi.DeletionTimestamp != nil {
		return vmi, nil
	}

	// only handle the migration completed vmi
	if vmi.Annotations == nil || vmi.Labels == nil || vmi.Namespace != h.namespace || !isMigrationCompleted(vmi) {
		logrus.WithFields(logrus.Fields{
			"namespace": vmi.Namespace,
			"name":      vmi.Name,
		}).Info("skip processing virtual machine instance")
		return vmi, nil
	}

	if creator := vmi.Labels[builder.LabelKeyVirtualMachineCreator]; creator != harvesterutil.VirtualMachineCreatorNodeDriver {
		logrus.WithFields(logrus.Fields{
			"namespace": vmi.Namespace,
			"name":      vmi.Name,
		}).Info("skip processing virtual machine instance")
		return vmi, nil
	}

	nodeName := vmi.Name
	guestAgentInfo, err := h.kubevirtClient.VirtualMachineInstance(vmi.Namespace).GuestOsInfo(context.TODO(), vmi.Name)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"name":      vmi.Name,
			"namespace": vmi.Namespace,
		}).WithError(err).Error("failed to get guest agent info, fallback to use vmi name as node name")
	} else {
		logrus.WithFields(logrus.Fields{
			"name":      vmi.Name,
			"namespace": vmi.Namespace,
			"hostname":  guestAgentInfo.Hostname,
		}).Info("get agent info success, using hostname as node name")
		nodeName = guestAgentInfo.Hostname
		h.nodeToVMName.Store(nodeName, vmi.Name)
	}

	node, err := h.nodeCache.Get(nodeName)
	if err != nil {
		if !errors.IsNotFound(err) {
			return vmi, err
		}
		// This vm does not belong to current cluster if the node is not found
		return vmi, nil
	}

	if err := h.annotateNodeWithNADInfo(node); err != nil {
		return vmi, fmt.Errorf("failed to annotate node %s with NAD info: %w", node.Name, err)
	}

	if !compareTopology(vmi.GetAnnotations(), node.GetLabels()) {
		if err := h.reSync(vmi); err != nil {
			return vmi, err
		}
	}

	return vmi, nil
}

func (h *Handler) reSync(vmi *kubevirtv1.VirtualMachineInstance) error {
	return cloudnodeutil.AddOrUpdateTaintOnNode(h.restClient, vmi.Name, &corev1.Taint{
		Key:    cloudproviderapi.TaintExternalCloudProvider,
		Value:  "true",
		Effect: corev1.TaintEffectPreferNoSchedule,
	})
}

func compareTopology(a map[string]string, b map[string]string) bool {
	return a[corev1.LabelTopologyRegion] == b[corev1.LabelTopologyRegion] &&
		a[corev1.LabelTopologyZone] == b[corev1.LabelTopologyZone]
}

func isMigrationCompleted(vmi *kubevirtv1.VirtualMachineInstance) bool {
	return vmi.Status.MigrationState == nil || vmi.Status.MigrationState.Completed
}

// annotateNodeWithNADInfo computes the common NAD→interface mapping across all VMIs
// in this guest cluster and writes it onto the Kubernetes Node annotation.
func (h *Handler) annotateNodeWithNADInfo(node *corev1.Node) error {
	sel := labels.Set{utils.LabelKeyGuestClusterNameOnVM: cfg.GetConfig().ClusterName}.AsSelector()
	vmiPtrs, err := h.vmiCache.List(h.namespace, sel)
	if err != nil {
		return err
	}

	vmis := make([]kubevirtv1.VirtualMachineInstance, 0, len(vmiPtrs))
	for _, v := range vmiPtrs {
		if v != nil {
			vmis = append(vmis, *v)
		}
	}

	commonNADs := ccmutil.GetCommonVMINADs(vmis) // {nad → interface}
	if len(commonNADs) > 0 {
		if err := h.annotateNodeWithInterfaceMapping(node.Name, commonNADs); err != nil {
			return fmt.Errorf("failed to annotate node with interface-NAD mapping: %w", err)
		}
	}

	return nil
}

// annotateNodeWithInterfaceMapping stores the NAD->interface mapping as a JSON annotation
// on the Kubernetes Node so that frontends can query it via the K8s API.
func (h *Handler) annotateNodeWithInterfaceMapping(nodeName string, mapping map[string]string) error {
	data, err := json.Marshal(mapping)
	if err != nil {
		return fmt.Errorf("marshal interface mapping: %w", err)
	}
	value := string(data)

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		node, err := h.nodeClient.Get(nodeName, metav1.GetOptions{})
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
		_, err = h.nodeClient.Update(nodeCopy)
		return err
	})
}
