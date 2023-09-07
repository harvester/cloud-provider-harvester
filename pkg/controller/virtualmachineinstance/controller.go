package virtualmachineinstance

import (
	"context"
	"sync"

	"github.com/harvester/harvester/pkg/builder"
	"github.com/harvester/harvester/pkg/controller/master/virtualmachine"
	ctlv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
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
		restClient:     restClient,
		kubevirtClient: kubevirtClient,
		nodeToVMName:   nodeToVMName,
	}
	vmis.OnChange(ctx, vmiControllerName, handler.OnVmiChanged)
}

type Handler struct {
	vmis           ctlv1.VirtualMachineInstanceController
	vmiCache       ctlv1.VirtualMachineInstanceCache
	nodeCache      ctlcorev1.NodeCache
	restClient     kubernetes.Interface
	kubevirtClient kubecli.KubevirtClient

	nodeToVMName *sync.Map

	namespace string
}

func (h *Handler) OnVmiChanged(_ string, vmi *kubevirtv1.VirtualMachineInstance) (*kubevirtv1.VirtualMachineInstance, error) {
	// TODO: Add some unit tests for this controller

	// only handle the migration completed vmi
	if vmi == nil || vmi.DeletionTimestamp != nil ||
		vmi.Annotations == nil || vmi.Namespace != h.namespace || !isMigrationCompleted(vmi) {
		return vmi, nil
	}

	if creator := vmi.Labels[builder.LabelKeyVirtualMachineCreator]; creator != virtualmachine.VirtualMachineCreatorNodeDriver {
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
