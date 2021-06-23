package ccm

import (
	"fmt"
	"io"

	"github.com/BurntSushi/toml"
	"k8s.io/client-go/tools/clientcmd"
	cloudprovider "k8s.io/cloud-provider"
)

const ProviderName = "harvester"

type CloudProvider struct {
	config        *cloudConfig
	loadBalancers cloudprovider.LoadBalancer
	instances     cloudprovider.InstancesV2
}

func init() {
	cloudprovider.RegisterCloudProvider(ProviderName, newCloudProvider)
}

func newCloudProvider(reader io.Reader) (cloudprovider.Interface, error) {
	var config cloudConfig
	if _, err := toml.DecodeReader(reader, &config); err != nil {
		return nil, fmt.Errorf("decode toml file failed, error: %w", err)
	}
	if config.Harvester.Server == "" || config.Harvester.Token == "" || config.Harvester.Certificate == "" {
		return nil, fmt.Errorf("server, token or certicate can not be empty")
	}

	cfg, err := clientcmd.BuildConfigFromFlags(config.Harvester.Server, "")
	if err != nil {
		return nil, err
	}
	cfg.BearerToken = config.Harvester.Token
	cfg.CAData = []byte(config.Harvester.Certificate)

	loadBalancerManager, err := newLoadBalancerManager(cfg, config.Harvester.Namespace, config.Cluster.Name)
	if err != nil {
		return nil, fmt.Errorf("create load balancer manager faield, err: %w", err)
	}

	instanceManager, err := newInstanceManager(cfg, config.Harvester.Namespace)
	if err != nil {
		return nil, fmt.Errorf("create instance manager failed, error: %w", err)
	}
	return &CloudProvider{
		config:        &config,
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
