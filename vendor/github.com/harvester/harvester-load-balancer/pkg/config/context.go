package config

import (
	"context"

	ctlcni "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io"
	ctlcore "github.com/rancher/wrangler/pkg/generated/controllers/core"
	"github.com/rancher/wrangler/pkg/start"
	"k8s.io/client-go/rest"

	ctldiscovery "github.com/harvester/harvester-load-balancer/pkg/generated/controllers/discovery.k8s.io"
	ctllb "github.com/harvester/harvester-load-balancer/pkg/generated/controllers/loadbalancer.harvesterhci.io"
	"github.com/harvester/harvester-load-balancer/pkg/ipam"
)

type Management struct {
	Ctx context.Context

	CoreFactory      *ctlcore.Factory
	LbFactory        *ctllb.Factory
	CniFactory       *ctlcni.Factory
	DiscoveryFactory *ctldiscovery.Factory

	starters []start.Starter

	AllocatorMap *ipam.SafeAllocatorMap
}

func SetupManagement(ctx context.Context, cfg *rest.Config) *Management {
	lbFactory := ctllb.NewFactoryFromConfigOrDie(cfg)
	cniFactory := ctlcni.NewFactoryFromConfigOrDie(cfg)
	coreFactory := ctlcore.NewFactoryFromConfigOrDie(cfg)
	discoveryFactory := ctldiscovery.NewFactoryFromConfigOrDie(cfg)
	management := &Management{
		Ctx: ctx,

		CoreFactory:      coreFactory,
		LbFactory:        lbFactory,
		CniFactory:       cniFactory,
		DiscoveryFactory: discoveryFactory,

		AllocatorMap: ipam.NewSafeAllocatorMap(),
	}

	management.starters = append(management.starters, coreFactory)
	management.starters = append(management.starters, discoveryFactory)
	management.starters = append(management.starters, cniFactory)
	management.starters = append(management.starters, lbFactory)

	return management
}

func (m *Management) Start(threadiness int) error {
	for _, starter := range m.starters {
		if err := starter.Start(m.Ctx, threadiness); err != nil {
			return err
		}
	}

	return nil
}
