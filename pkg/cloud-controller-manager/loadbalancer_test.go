package ccm

import (
	"bytes"
	"os"
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
	logrus.SetOutput(&buf)

	// 2. Ensure we reset logrus after the test
	defer logrus.SetOutput(os.Stderr)

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
	// Setup/Restore cfg logic here...

	tests := []struct {
		name                string
		mgmtNetwork         string
		allowSpecify        bool
		initialAnnotations  map[string]string
		expectedAnnotations map[string]string
	}{
		{
			name:         "Priority 1 Enforced: Mgmt network exists",
			mgmtNetwork:  "harvester-mgmt/vlan-100",
			allowSpecify: true,
			initialAnnotations: map[string]string{
				// User tries to specify a custom one
				utils.AnnotationKeyGuestClusterNetworkNameOnLB: "user/vlan-200",
			},
			expectedAnnotations: map[string]string{
				// Both are present, but your controller logic will read Mgmt first
				utils.AnnotationKeyGuestClusterManagementNetworkOnLB: "harvester-mgmt/vlan-100",
				utils.AnnotationKeyGuestClusterNetworkNameOnLB:       "user/vlan-200",
			},
		},
		{
			name:         "Priority 2 Stripped: User specified, but not allowed globally",
			mgmtNetwork:  "",
			allowSpecify: false,
			initialAnnotations: map[string]string{
				utils.AnnotationKeyGuestClusterNetworkNameOnLB: "user/vlan-200",
			},
			expectedAnnotations: map[string]string{
				// Result is empty -> Triggers Priority 3 (Fallback/Guess)
			},
		},
		{
			name:         "Priority 2 Allowed: User specified and allowed globally",
			mgmtNetwork:  "",
			allowSpecify: true,
			initialAnnotations: map[string]string{
				utils.AnnotationKeyGuestClusterNetworkNameOnLB: "user/vlan-200",
			},
			expectedAnnotations: map[string]string{
				utils.AnnotationKeyGuestClusterNetworkNameOnLB: "user/vlan-200",
			},
		},
		{
			name:         "Legacy Case: No global config and no LB annotations",
			mgmtNetwork:  "",    // No global flag set
			allowSpecify: false, // Default/Legacy state
			initialAnnotations: map[string]string{
				"other-annotation": "stays-untouched",
			},
			expectedAnnotations: map[string]string{
				"other-annotation": "stays-untouched",
				// No new annotations added, none removed.
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg.ManagementNetwork = tt.mgmtNetwork
			cfg.AllowSpecifyLoadBalancerNetwork = tt.allowSpecify

			lb := &lbv1.LoadBalancer{
				ObjectMeta: metav1.ObjectMeta{
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
