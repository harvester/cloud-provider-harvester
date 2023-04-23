package ccm

import (
	"context"
	"io"
	"io/ioutil"
	"os"

	ctllb "github.com/harvester/harvester-load-balancer/pkg/generated/controllers/loadbalancer.harvesterhci.io"
	ctlkubevirt "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io"
	ctlcore "github.com/rancher/wrangler/pkg/generated/controllers/core"
	"github.com/rancher/wrangler/pkg/kubeconfig"
	"github.com/rancher/wrangler/pkg/start"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	cloudprovider "k8s.io/cloud-provider"
)

const ProviderName = "harvester"

type CloudProvider struct {
	localCoreFactory *ctlcore.Factory
	lbFactory        *ctllb.Factory
	kubevirtFactory  *ctlkubevirt.Factory

	loadBalancers cloudprovider.LoadBalancer
	instances     cloudprovider.InstancesV2
}

func init() {
	cloudprovider.RegisterCloudProvider(ProviderName, newCloudProvider)
}

func newCloudProvider(reader io.Reader) (cloudprovider.Interface, error) {
	bytes, err := ioutil.ReadAll(reader)
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
	localCfg, err := kubeconfig.GetNonInteractiveClientConfig(os.Getenv("KUBECONFIG")).ClientConfig()
	if err != nil {
		return nil, err
	}

	ns := rawConfig.Contexts[rawConfig.CurrentContext].Namespace
	cp := &CloudProvider{
		localCoreFactory: ctlcore.NewFactoryFromConfigOrDie(localCfg),
		lbFactory:        ctllb.NewFactoryFromConfigOrDie(clientConfig),
		kubevirtFactory:  ctlkubevirt.NewFactoryFromConfigOrDie(clientConfig),
	}
	cp.loadBalancers = &LoadBalancerManager{
		lbClient:       cp.lbFactory.Loadbalancer().V1beta1().LoadBalancer(),
		localSvcClient: cp.localCoreFactory.Core().V1().Service(),
		localSvcCache:  cp.localCoreFactory.Core().V1().Service().Cache(),
		namespace:      ns,
	}
	cp.instances = &instanceManager{
		vmClient:  cp.kubevirtFactory.Kubevirt().V1().VirtualMachine(),
		vmiClient: cp.kubevirtFactory.Kubevirt().V1().VirtualMachineInstance(),
		namespace: ns,
	}

	return cp, nil
}

func (c *CloudProvider) Initialize(clientBuilder cloudprovider.ControllerClientBuilder, stop <-chan struct{}) {
	go func() {
		if err := start.All(context.TODO(), 2, c.localCoreFactory); err != nil {
			klog.Fatal(err)
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
