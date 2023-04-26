package lb

import (
	"k8s.io/apimachinery/pkg/types"

	lbv1 "github.com/harvester/harvester-load-balancer/pkg/apis/loadbalancer.harvesterhci.io/v1beta1"
)

type Manager interface {
	EnsureLoadBalancer(lb *lbv1.LoadBalancer) error
	DeleteLoadBalancer(lb *lbv1.LoadBalancer) error
	GetBackendServers(lb *lbv1.LoadBalancer) ([]BackendServer, error)
	AddBackendServers(lb *lbv1.LoadBalancer, servers []BackendServer) error
	RemoveBackendServers(lb *lbv1.LoadBalancer, servers []BackendServer) error
}

type BackendServer interface {
	GetUID() types.UID
	GetKind() string
	GetNamespace() string
	GetName() string
	GetAddress() (string, bool)
}
