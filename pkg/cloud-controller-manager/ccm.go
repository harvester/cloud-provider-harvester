package ccm

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	ctlkubevirt "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io"
	ctlcore "github.com/rancher/wrangler/pkg/generated/controllers/core"
	"github.com/rancher/wrangler/pkg/kubeconfig"
	"github.com/rancher/wrangler/pkg/signals"
	"github.com/rancher/wrangler/pkg/start"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"

	vmi "github.com/harvester/harvester-cloud-provider/pkg/controller/virtualmachineinstance"
)

const (
	ProviderName = "harvester"

	threadiness = 2
)

type CloudProvider struct {
	loadBalancers cloudprovider.LoadBalancer
	instances     cloudprovider.InstancesV2

	Context      context.Context
	Namespace    string
	ClientConfig *rest.Config
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

	namespace := rawConfig.Contexts[rawConfig.CurrentContext].Namespace

	localCfg, err := kubeconfig.GetNonInteractiveClientConfig(os.Getenv("KUBECONFIG")).ClientConfig()
	if err != nil {
		return nil, err
	}

	loadBalancerManager, err := newLoadBalancerManager(clientConfig, localCfg, namespace)
	if err != nil {
		return nil, fmt.Errorf("create load balancer manager faield, err: %w", err)
	}

	instanceManager, err := newInstanceManager(clientConfig, namespace)
	if err != nil {
		return nil, fmt.Errorf("create instance manager failed, error: %w", err)
	}

	return &CloudProvider{
		loadBalancers: loadBalancerManager,
		instances:     instanceManager,

		Context:      signals.SetupSignalContext(),
		Namespace:    namespace,
		ClientConfig: clientConfig,
	}, nil
}

func (c *CloudProvider) Initialize(clientBuilder cloudprovider.ControllerClientBuilder, stop <-chan struct{}) {
	client := clientBuilder.ClientOrDie(ProviderName)
	config := clientBuilder.ConfigOrDie(ProviderName)

	coreFactory, err := ctlcore.NewFactoryFromConfig(config)
	if err != nil {
		klog.Fatalf("error building core factory: %s", err.Error())
	}

	virts, err := ctlkubevirt.NewFactoryFromConfigWithNamespace(c.ClientConfig, c.Namespace)
	if err != nil {
		klog.Fatalf("error building virt controllers: %s", err.Error())
	}

	vmi.Register(
		c.Context,
		client,
		coreFactory.Core().V1().Node(),
		virts.Kubevirt().V1().VirtualMachineInstance(),
	)

	go func() {
		if err := start.All(c.Context, threadiness, virts, coreFactory); err != nil {
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
