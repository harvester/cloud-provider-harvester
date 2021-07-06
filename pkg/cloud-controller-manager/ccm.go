package ccm

import (
	"fmt"
	"io"
	"io/ioutil"

	"k8s.io/client-go/tools/clientcmd"
	cloudprovider "k8s.io/cloud-provider"
)

const ProviderName = "harvester"

type CloudProvider struct {
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

	ns := rawConfig.Contexts[rawConfig.CurrentContext].Namespace

	loadBalancerManager, err := newLoadBalancerManager(clientConfig, ns)
	if err != nil {
		return nil, fmt.Errorf("create load balancer manager faield, err: %w", err)
	}

	instanceManager, err := newInstanceManager(clientConfig, ns)
	if err != nil {
		return nil, fmt.Errorf("create instance manager failed, error: %w", err)
	}
	return &CloudProvider{
		loadBalancers: loadBalancerManager,
		instances:     instanceManager,
	}, nil
}

func (c *CloudProvider) Initialize(clientBuilder cloudprovider.ControllerClientBuilder, stop <-chan struct{}) {
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
