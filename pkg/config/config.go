package config

import "sync"

var (
	instance *Config
	once     sync.Once

	ManagementNetwork               string
	AllowSpecifyLoadBalancerNetwork bool
	ClusterName                     string
)

type Config struct {
	ManagementNetwork               string
	ClusterName                     string
	AllowSpecifyLoadBalancerNetwork bool
}

func Get() *Config {
	once.Do(func() {
		instance = &Config{}
	})
	return instance
}
