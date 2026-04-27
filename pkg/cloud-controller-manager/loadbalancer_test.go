package ccm

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	lbv1 "github.com/harvester/harvester-load-balancer/pkg/apis/loadbalancer.harvesterhci.io/v1beta1"
	"github.com/sirupsen/logrus/hooks/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cfg "github.com/harvester/harvester-cloud-provider/pkg/config"
	utils "github.com/harvester/harvester-cloud-provider/pkg/utils"
)

const defaultUID = "d4b50d98-39ec-4d88-8098-36579de5db4a"

func Test_getLoadBalancerName(t *testing.T) {
	type args struct {
		clusterName      string
		serviceNamespace string
		serviceName      string
		uid              string
	}
	tests := []struct {
		name       string
		args       args
		wantPrefix string
	}{
		{"case_1", args{"test", "default", "abcd", defaultUID}, "test-default-abcd-"},
		{"case_2", args{"1test", "default", "abcd", defaultUID}, "a1test-default-abcd-"},
		{"case_3", args{"test", "default-default", "kube-system-rke2-ingress-nginx-controller", defaultUID}, "test-default-default-kube-system-rke2-ingress-nginx-con"},
		{"case_4", args{"test", "default-default", "kube-system-rke2-ingress-nginx-co-abcd", defaultUID}, "test-default-default-kube-system-rke2-ingress-nginx-co-"},
		{"case_5", args{"1test", "default-default", "kube-system-rke2-ingress-nginx-controller", defaultUID}, "a1test-default-default-kube-system-rke2-ingress-nginx-c"},
		{"case_6", args{"kubernetes", "default-default", "kube-system-rke2-ingress-nginx-controller", defaultUID}, "kubernetes-default-default-kube-system-rke2-"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := loadBalancerName(tt.args.clusterName, tt.args.serviceNamespace, tt.args.serviceName, tt.args.uid)
			if !strings.HasPrefix(name, tt.wantPrefix) {
				t.Errorf("invalid name %s, args: %+v, wantPrefix: %s", name, tt.args, tt.wantPrefix)
			}
		})
	}
}

func Test_warnClusterName(t *testing.T) {
	tests := []struct {
		name        string
		clusterName string
		lbName      string
		shouldWarn  bool
	}{
		{
			name:        "Empty cluster name triggers warning",
			clusterName: "",
			lbName:      "lb1",
			shouldWarn:  true,
		},
		{
			name:        "Default cluster name triggers warning",
			clusterName: utils.DefaultGuestClusterName,
			lbName:      "lb2",
			shouldWarn:  true,
		},
		{
			name:        "Unique cluster name does not trigger warning",
			clusterName: "production-cluster",
			lbName:      "lb3",
			shouldWarn:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observerLogger, hook := test.NewNullLogger()
			warnClusterName(observerLogger, tt.lbName, tt.clusterName)

			hasWarning := false
			msg := ""
			for _, entry := range hook.AllEntries() {
				if strings.Contains(entry.Message, "ensure a unique name is set") {
					hasWarning = true
					msg = entry.Message
					break
				}
			}
			if hasWarning != tt.shouldWarn {
				t.Errorf("Expected warning: %v, but got: %v. Log output: %s",
					tt.shouldWarn, hasWarning, msg)
			}
		})
	}
}

func Test_patchLB_Priority(t *testing.T) {
	tests := []struct {
		name                string
		mgmtNetwork         string
		lbNetwork           string
		initialAnnotations  map[string]string
		expectedAnnotations map[string]string
	}{
		{
			name:               "Both networks set: normalization and application",
			mgmtNetwork:        "harvester-mgmt/vlan-100",
			lbNetwork:          "custom-vlan",
			initialAnnotations: map[string]string{},
			expectedAnnotations: map[string]string{
				utils.AnnotationKeyGuestClusterNetworkNameOnLB:       "default/custom-vlan",
				utils.AnnotationKeyGuestClusterManagementNetworkOnLB: "harvester-mgmt/vlan-100",
			},
		},
		{
			name:        "LB network empty: existing LB annotation is stripped",
			mgmtNetwork: "harvester-mgmt/vlan-100",
			lbNetwork:   "",
			initialAnnotations: map[string]string{
				utils.AnnotationKeyGuestClusterNetworkNameOnLB: "too/many/slashes",
			},
			expectedAnnotations: map[string]string{
				utils.AnnotationKeyGuestClusterManagementNetworkOnLB: "harvester-mgmt/vlan-100",
			},
		},
		{
			name:               "Management network normalization: bare name to default namespace",
			mgmtNetwork:        "mgmt-vlan",
			lbNetwork:          "",
			initialAnnotations: map[string]string{},
			expectedAnnotations: map[string]string{
				utils.AnnotationKeyGuestClusterManagementNetworkOnLB: "default/mgmt-vlan",
			},
		},
		{
			name:        "Management network empty: existing mgmt annotation is stripped",
			mgmtNetwork: "",
			lbNetwork:   "",
			initialAnnotations: map[string]string{
				utils.AnnotationKeyGuestClusterManagementNetworkOnLB: "default/mgmt-vlan",
			},
			expectedAnnotations: map[string]string{},
		},
		{
			name:        "Invalid global config: results in stripped annotations",
			mgmtNetwork: "invalid/global/net/too/deep",
			lbNetwork:   "",
			initialAnnotations: map[string]string{
				utils.AnnotationKeyGuestClusterManagementNetworkOnLB: "old-val",
			},
			expectedAnnotations: map[string]string{},
		},
		{
			name:        "Preservation: unrelated annotations remain untouched",
			mgmtNetwork: "",
			lbNetwork:   "",
			initialAnnotations: map[string]string{
				"harvesterhci.io/other": "important-data",
			},
			expectedAnnotations: map[string]string{
				"harvesterhci.io/other": "important-data",
			},
		},
		{
			name:        "Malformed input: validation failure leads to removal",
			mgmtNetwork: "",
			lbNetwork:   "",
			initialAnnotations: map[string]string{
				utils.AnnotationKeyGuestClusterNetworkNameOnLB: "namespace/",
			},
			expectedAnnotations: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup a clean config for every sub-test to avoid side-effects
			currentCfg := cfg.GetConfig()
			oldMgmt := currentCfg.ManagementNetwork
			oldLB := currentCfg.LoadbalancerNetwork

			currentCfg.ManagementNetwork = tt.mgmtNetwork
			currentCfg.LoadbalancerNetwork = tt.lbNetwork

			// Cleanup after each subtest
			defer func() {
				currentCfg.ManagementNetwork = oldMgmt
				currentCfg.LoadbalancerNetwork = oldLB
			}()

			lb := &lbv1.LoadBalancer{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   "test-ns",
					Name:        "test-lb",
					Annotations: tt.initialAnnotations,
				},
			}

			patchLB(lb)

			// Using cmp.Diff for high-quality error messages
			// Ensure we compare against an empty map if expected is nil
			if tt.expectedAnnotations == nil {
				tt.expectedAnnotations = make(map[string]string)
			}
			// patchLB might leave map as nil if it clears everything,
			// so we normalize for the comparison if needed.
			actual := lb.Annotations
			if actual == nil {
				actual = make(map[string]string)
			}

			if diff := cmp.Diff(tt.expectedAnnotations, actual); diff != "" {
				t.Errorf("patchLB() result mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
