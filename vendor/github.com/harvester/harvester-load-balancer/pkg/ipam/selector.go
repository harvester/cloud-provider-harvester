package ipam

import (
	"fmt"

	lbv1 "github.com/harvester/harvester-load-balancer/pkg/apis/loadbalancer.harvesterhci.io/v1alpha1"
	"github.com/harvester/harvester-load-balancer/pkg/utils"
)

type Selector struct {
	list       []*lbv1.IPPool
	conditions Conditions
}

type Conditions struct {
	HardCond    HardConditions
	ElasticCond ElasticConditions
}

// HardConditions is must during selection
type HardConditions struct {
	VLAN string
}

// ElasticConditions are used to select the more suitable IP Pool
type ElasticConditions struct {
	Project   string
	Namespace string
	Cluster   string
}

func NewSelector(pools []*lbv1.IPPool, conditions Conditions) *Selector {
	return &Selector{
		list:       pools,
		conditions: conditions,
	}
}

func (s *Selector) Select() (*lbv1.IPPool, error) {
	shortlist, err := s.preSelect()
	if err != nil {
		return nil, fmt.Errorf("pre-select failed, error: %w", err)
	}

	return s.finalSelect(shortlist)
}

// preSelect filters the pools which unfit the hard conditions
func (s *Selector) preSelect() ([]*lbv1.IPPool, error) {
	var shortlist []*lbv1.IPPool
	for _, pool := range s.list {
		if pool.Labels[utils.KeyVid] == s.conditions.HardCond.VLAN {
			shortlist = append(shortlist, pool)
		}
	}

	return shortlist, nil
}

// finalSelect selects the most suitable pool based on the elastic conditions
func (s *Selector) finalSelect(shortlist []*lbv1.IPPool) (*lbv1.IPPool, error) {
	if len(shortlist) == 0 {
		return nil, fmt.Errorf("no matching IP pool")
	}

	elect := struct {
		pool     *lbv1.IPPool
		priority int
	}{nil, 0}

	for _, pool := range shortlist {
		priority := 1
		var namespaces []lbv1.IPPoolNamespace

		if s.conditions.ElasticCond.Project != "" {
			// match project
			for _, project := range pool.Spec.Projects {
				if project.Name == s.conditions.ElasticCond.Project {
					priority++
					namespaces = project.Namespaces
				}
			}
		} else {
			namespaces = pool.Spec.Namespaces
		}

		// match namespace
		for _, namespace := range namespaces {
			if namespace.Name == s.conditions.ElasticCond.Namespace {
				priority++
				// match cluster
				for _, cluster := range namespace.GuestClusters {
					if cluster == s.conditions.ElasticCond.Cluster {
						priority++
					}
				}
			}
		}

		// select the pool with high priority
		if priority > elect.priority {
			elect.pool = pool
			elect.priority = priority
		}
	}

	return elect.pool, nil
}
