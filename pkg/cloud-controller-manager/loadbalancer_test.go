package ccm

import (
	"bytes"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	lbv1 "github.com/harvester/harvester-load-balancer/pkg/apis/loadbalancer.harvesterhci.io/v1beta1"
	"github.com/sirupsen/logrus"
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
	// 1. Setup a buffer to capture logs
	var buf bytes.Buffer
	originalOutput := logrus.StandardLogger().Out
	logrus.SetOutput(&buf)
	// 2. Ensure we reset logrus after the test
	defer logrus.SetOutput(originalOutput)

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
			buf.Reset() // Clear logs from previous run

			warnClusterName(tt.lbName, tt.clusterName)

			hasWarning := strings.Contains(buf.String(), "ensure a unique name is set")
			if hasWarning != tt.shouldWarn {
				t.Errorf("Expected warning: %v, but got: %v. Log output: %s",
					tt.shouldWarn, hasWarning, buf.String())
			}
		})
	}
}

func Test_patchLB_Priority(t *testing.T) {
	tests := []struct {
		name                string
		mgmtNetwork         string
		allowSpecify        bool
		initialAnnotations  map[string]string
		expectedAnnotations map[string]string
	}{
		{
			name:         "User Override: User input normalized and mgmt network added",
			mgmtNetwork:  "harvester-mgmt/vlan-100",
			allowSpecify: true,
			initialAnnotations: map[string]string{
				utils.AnnotationKeyGuestClusterNetworkNameOnLB: "custom-vlan",
			},
			expectedAnnotations: map[string]string{
				// User input is normalized (bare name -> default/name)
				utils.AnnotationKeyGuestClusterNetworkNameOnLB:       "default/custom-vlan",
				utils.AnnotationKeyGuestClusterManagementNetworkOnLB: "harvester-mgmt/vlan-100",
			},
		},
		{
			name:         "User Override: Invalid user input stripped, mgmt network remains",
			mgmtNetwork:  "harvester-mgmt/vlan-100",
			allowSpecify: true,
			initialAnnotations: map[string]string{
				utils.AnnotationKeyGuestClusterNetworkNameOnLB: "too/many/slashes",
			},
			expectedAnnotations: map[string]string{
				// Invalid user input is deleted, mgmt is still applied
				utils.AnnotationKeyGuestClusterManagementNetworkOnLB: "harvester-mgmt/vlan-100",
			},
		},
		{
			name:               "Management: Global config added when no user input exists",
			mgmtNetwork:        "harvester-mgmt/vlan-100",
			allowSpecify:       true,
			initialAnnotations: map[string]string{},
			expectedAnnotations: map[string]string{
				utils.AnnotationKeyGuestClusterManagementNetworkOnLB: "harvester-mgmt/vlan-100",
			},
		},
		{
			name:               "Management: Global config normalized if bare name",
			mgmtNetwork:        "mgmt-vlan",
			allowSpecify:       true,
			initialAnnotations: map[string]string{},
			expectedAnnotations: map[string]string{
				utils.AnnotationKeyGuestClusterManagementNetworkOnLB: "default/mgmt-vlan",
			},
		},
		{
			name:         "Management: Invalid global config stripped from annotations",
			mgmtNetwork:  "invalid/global/net",
			allowSpecify: true,
			initialAnnotations: map[string]string{
				utils.AnnotationKeyGuestClusterManagementNetworkOnLB: "old-val",
			},
			expectedAnnotations: map[string]string{},
		},
		{
			name:         "Permissions: User input stripped when allowSpecify is false",
			mgmtNetwork:  "harvester-mgmt/vlan-100",
			allowSpecify: false,
			initialAnnotations: map[string]string{
				utils.AnnotationKeyGuestClusterNetworkNameOnLB: "user-vlan",
			},
			expectedAnnotations: map[string]string{
				utils.AnnotationKeyGuestClusterManagementNetworkOnLB: "harvester-mgmt/vlan-100",
			},
		},
		{
			name:         "Fallback: Both annotations removed if mgmt is empty and user input disabled",
			mgmtNetwork:  "",
			allowSpecify: false,
			initialAnnotations: map[string]string{
				utils.AnnotationKeyGuestClusterNetworkNameOnLB:       "user-vlan",
				utils.AnnotationKeyGuestClusterManagementNetworkOnLB: "old-mgmt",
			},
			expectedAnnotations: map[string]string{
				// Result is empty -> Harvester side will perform fallback discovery
			},
		},
		{
			name:         "Preservation: Non-related annotations are not touched",
			mgmtNetwork:  "",
			allowSpecify: false,
			initialAnnotations: map[string]string{
				"harvesterhci.io/other": "important-data",
			},
			expectedAnnotations: map[string]string{
				"harvesterhci.io/other": "important-data",
			},
		},
		{
			name:         "Edge Case: Malformed parts (ns/) are stripped",
			mgmtNetwork:  "",
			allowSpecify: true,
			initialAnnotations: map[string]string{
				utils.AnnotationKeyGuestClusterNetworkNameOnLB: "namespace/",
			},
			expectedAnnotations: map[string]string{},
		},
	}

	originalManagementNetwork := cfg.ManagementNetwork
	originalAllowSpecifyLoadBalancerNetwork := cfg.AllowSpecifyLoadBalancerNetwork

	defer func() {
		cfg.ManagementNetwork = originalManagementNetwork
		cfg.AllowSpecifyLoadBalancerNetwork = originalAllowSpecifyLoadBalancerNetwork
	}()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set global config state
			cfg.ManagementNetwork = tt.mgmtNetwork
			cfg.AllowSpecifyLoadBalancerNetwork = tt.allowSpecify

			lb := &lbv1.LoadBalancer{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   "test-ns",
					Name:        "test-lb",
					Annotations: tt.initialAnnotations,
				},
			}

			patchLB(lb)

			if diff := cmp.Diff(tt.expectedAnnotations, lb.Annotations); diff != "" {
				t.Errorf("patchLB() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
