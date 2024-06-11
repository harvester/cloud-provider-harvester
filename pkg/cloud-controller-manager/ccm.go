package ccm

import (
	"context"
	"io"
	"os"
	"sync"

	ctllb "github.com/harvester/harvester-load-balancer/pkg/generated/controllers/loadbalancer.harvesterhci.io"
	ctlkubevirt "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io"
	ctlcore "github.com/rancher/wrangler/pkg/generated/controllers/core"
	"github.com/rancher/wrangler/pkg/kubeconfig"
	"github.com/rancher/wrangler/pkg/signals"
	"github.com/rancher/wrangler/pkg/start"
	"k8s.io/client-go/tools/clientcmd"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"
	"kubevirt.io/client-go/kubecli"

	vmi "github.com/harvester/harvester-cloud-provider/pkg/controller/virtualmachineinstance"
)

const (
	ProviderName = "harvester"

	threadiness = 2
)

var DisableVMIController bool

type CloudProvider struct {
	localCoreFactory *ctlcore.Factory
	lbFactory        *ctllb.Factory
	kubevirtFactory  *ctlkubevirt.Factory

	loadBalancers cloudprovider.LoadBalancer
	instances     cloudprovider.InstancesV2

	kubevirtClient kubecli.KubevirtClient

	nodeToVMName *sync.Map

	Context context.Context

	namespace string
}

func init() {
	cloudprovider.RegisterCloudProvider(ProviderName, newCloudProvider)
}

func newCloudProvider(reader io.Reader) (cloudprovider.Interface, error) {
	bytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	config, err := clientcmd.NewClientConfigFromBytes(bytes)
	if err != nil {
		return nil, err
	}
	clientConfig, err := config.ClientConfig()
	if err != nil {
		return nil, err
	}
	rawConfig, err := config.RawConfig()
	if err != nil {
		return nil, err
	}

	namespace := rawConfig.Contexts[rawConfig.CurrentContext].Namespace

	localCfg, err := kubeconfig.GetNonInteractiveClientConfig(os.Getenv("KUBECONFIG")).ClientConfig()
	if err != nil {
		return nil, err
	}
	kubevirtFactory := ctlkubevirt.NewFactoryFromConfigWithOptionsOrDie(clientConfig, &ctlkubevirt.FactoryOptions{
		Namespace: namespace,
	})

	kubevirtClient, err := kubecli.GetKubevirtClientFromClientConfig(config)
	if err != nil {
		return nil, err
	}

	nodeToVMName := &sync.Map{}
	cp := &CloudProvider{
		localCoreFactory: ctlcore.NewFactoryFromConfigOrDie(localCfg),
		lbFactory:        ctllb.NewFactoryFromConfigOrDie(clientConfig),
		kubevirtFactory:  kubevirtFactory,

		kubevirtClient: kubevirtClient,

		nodeToVMName: nodeToVMName,

		Context: signals.SetupSignalContext(),

		namespace: namespace,
	}
	cp.loadBalancers = &LoadBalancerManager{
		lbClient:       cp.lbFactory.Loadbalancer().V1beta1().LoadBalancer(),
		localSvcClient: cp.localCoreFactory.Core().V1().Service(),
		localSvcCache:  cp.localCoreFactory.Core().V1().Service().Cache(),
		namespace:      namespace,
	}
	cp.instances = &instanceManager{
		vmClient:     cp.kubevirtFactory.Kubevirt().V1().VirtualMachine(),
		vmiClient:    cp.kubevirtFactory.Kubevirt().V1().VirtualMachineInstance(),
		nodeToVMName: nodeToVMName,
		namespace:    namespace,
	}

	return cp, nil
}

func (c *CloudProvider) Initialize(clientBuilder cloudprovider.ControllerClientBuilder, stop <-chan struct{}) {
	client := clientBuilder.ClientOrDie(ProviderName)

	if !DisableVMIController {
		vmi.Register(
			c.Context,
			client,
			c.localCoreFactory.Core().V1().Node(),
			c.kubevirtFactory.Kubevirt().V1().VirtualMachineInstance(),
			c.kubevirtClient,
			c.nodeToVMName,
			c.namespace,
		)
	}

	go func() {
		if err := start.All(c.Context, threadiness, c.kubevirtFactory, c.localCoreFactory); err != nil {
			klog.Fatalf("error starting controllers: %s", err.Error())
		}
		<-stop
	}()
}

func (c *CloudProvider) LoadBalancer() (cloudprovider.LoadBalancer, bool) {
	return c.loadBalancers, true
}

func (c *CloudProvider) Instances() (cloudprovider.Instances, bool) {
	return nil, false
}

func (c *CloudProvider) InstancesV2() (cloudprovider.InstancesV2, bool) {
	return c.instances, true
}

func (c *CloudProvider) Zones() (cloudprovider.Zones, bool) {
	return nil, false
}

func (c *CloudProvider) Clusters() (cloudprovider.Clusters, bool) {
	return nil, false
}

func (c *CloudProvider) Routes() (cloudprovider.Routes, bool) {
	return nil, false
}

func (c *CloudProvider) ProviderName() string {
	return ProviderName
}

func (c *CloudProvider) HasClusterID() bool {
	return false
}
