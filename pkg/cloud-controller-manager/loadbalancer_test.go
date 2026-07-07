package ccm

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	lbv1 "github.com/harvester/harvester-load-balancer/pkg/apis/loadbalancer.harvesterhci.io/v1beta1"
	pkgctllb "github.com/harvester/harvester-load-balancer/pkg/controller/loadbalancer"
	"github.com/sirupsen/logrus/hooks/test"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	cfg "github.com/harvester/harvester-cloud-provider/pkg/config"
	utils "github.com/harvester/harvester-cloud-provider/pkg/utils"
	"github.com/harvester/harvester-cloud-provider/pkg/utils/fakeclients"
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

func newServiceWithAnnotations(annotations map[string]string, labels map[string]string) *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "default",
			Name:        "test-svc",
			UID:         types.UID(defaultUID),
			Annotations: annotations,
			Labels:      labels,
		},
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

func Test_isPrimaryServiceUpdatedWithIP(t *testing.T) {
	const ip = "192.168.101.57"

	tests := []struct {
		name          string
		annotations   map[string]string
		labels        map[string]string
		lbAddress     string
		lbNetwork     string
		ip            string
		resolvedIface string
		want          bool
	}{
		{
			// No network annotation → no interface resolved → serviceInterface absent is correct.
			name: "fully_updated_no_network",
			annotations: map[string]string{
				utils.KeyKubevipLoadBalancerIP: ip,
			},
			labels:    map[string]string{utils.KeyPrimaryService: ""},
			lbAddress: ip,
			ip:        ip,
			want:      true,
		},
		{
			// When resolvedIface is empty, any existing serviceInterface is left untouched.
			name: "no_network_existing_interface_preserved",
			annotations: map[string]string{
				utils.KeyKubevipLoadBalancerIP:   ip,
				utils.KeyKubevipServiceInterface: "eth0",
			},
			labels:    map[string]string{utils.KeyPrimaryService: ""},
			lbAddress: ip,
			ip:        ip,
			want:      true,
		},
		{
			name: "ip_mismatch",
			annotations: map[string]string{
				utils.KeyKubevipLoadBalancerIP: "192.168.101.1",
			},
			labels:    map[string]string{utils.KeyPrimaryService: ""},
			lbAddress: ip,
			ip:        ip,
			want:      false,
		},
		{
			// DHCP first run: user provided network but serviceInterface not yet written.
			name: "dhcp_network_present_not_yet_converted",
			annotations: map[string]string{
				utils.KeyKubevipLoadBalancerIP: ip,
				utils.KeyIPAM:                  string(lbv1.DHCP),
				utils.KeyNetwork:               "default/mgmt-vlan1",
			},
			labels:        map[string]string{utils.KeyPrimaryService: ""},
			lbAddress:     ip,
			lbNetwork:     "default/mgmt-vlan1",
			ip:            ip,
			resolvedIface: "enp1s0",
			want:          false,
		},
		{
			// DHCP: NAD mapping unavailable (resolvedIface empty) — treated as up-to-date
			// so the network annotation is preserved for the next reconciliation.
			name: "dhcp_mapping_unavailable_network_preserved",
			annotations: map[string]string{
				utils.KeyKubevipLoadBalancerIP: ip,
				utils.KeyIPAM:                  string(lbv1.DHCP),
				utils.KeyNetwork:               "default/mgmt-vlan1",
			},
			labels:    map[string]string{utils.KeyPrimaryService: ""},
			lbAddress: ip,
			lbNetwork: "default/mgmt-vlan1",
			ip:        ip,
			// resolvedIface left empty (zero value) — mapping unavailable
			want: true,
		},
		{
			// DHCP fully converted on a previous reconciliation; no network annotation
			// remains. resolvedIface is empty because there is nothing to look up, but
			// the interface must not be erased.
			name: "dhcp_after_conversion_interface_preserved",
			annotations: map[string]string{
				utils.KeyKubevipLoadBalancerIP:   ip,
				utils.KeyKubevipServiceInterface: "enp1s0",
				utils.KeyIPAM:                    string(lbv1.DHCP),
			},
			labels:    map[string]string{utils.KeyPrimaryService: ""},
			lbAddress: ip,
			ip:        ip,
			// resolvedIface empty — no network annotation left, mapping not consulted
			want: true,
		},
		{
			// DHCP: interface written but network annotation not yet removed.
			name: "dhcp_interface_set_but_network_not_removed",
			annotations: map[string]string{
				utils.KeyKubevipLoadBalancerIP:   ip,
				utils.KeyKubevipServiceInterface: "enp1s0",
				utils.KeyIPAM:                    string(lbv1.DHCP),
				utils.KeyNetwork:                 "default/mgmt-vlan1",
			},
			labels:        map[string]string{utils.KeyPrimaryService: ""},
			lbAddress:     ip,
			lbNetwork:     "default/mgmt-vlan1",
			ip:            ip,
			resolvedIface: "enp1s0",
			want:          true,
		},
		{
			// DHCP fully converted: serviceInterface set and network annotation removed.
			name: "dhcp_fully_converted",
			annotations: map[string]string{
				utils.KeyKubevipLoadBalancerIP:   ip,
				utils.KeyKubevipServiceInterface: "enp1s0",
				utils.KeyIPAM:                    string(lbv1.DHCP),
			},
			labels:        map[string]string{utils.KeyPrimaryService: ""},
			lbAddress:     ip,
			ip:            ip,
			resolvedIface: "enp1s0",
			want:          true,
		},
		{
			// IPPool: interface not yet resolved.
			name: "ippool_interface_not_yet_set",
			annotations: map[string]string{
				utils.KeyKubevipLoadBalancerIP: ip,
				utils.KeyIPAM:                  string(lbv1.Pool),
				utils.KeyNetwork:               "default/mgmt-vlan1",
			},
			labels:        map[string]string{utils.KeyPrimaryService: ""},
			lbAddress:     ip,
			lbNetwork:     "default/mgmt-vlan1",
			ip:            ip,
			resolvedIface: "enp1s0",
			want:          false,
		},
		{
			// IPPool fully converted: serviceInterface set, network annotation kept.
			name: "ippool_fully_converted",
			annotations: map[string]string{
				utils.KeyKubevipLoadBalancerIP:   ip,
				utils.KeyKubevipServiceInterface: "enp1s0",
				utils.KeyIPAM:                    string(lbv1.Pool),
				utils.KeyNetwork:                 "default/mgmt-vlan1",
			},
			labels:        map[string]string{utils.KeyPrimaryService: ""},
			lbAddress:     ip,
			lbNetwork:     "default/mgmt-vlan1",
			ip:            ip,
			resolvedIface: "enp1s0",
			want:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newServiceWithAnnotations(tt.annotations, tt.labels)
			lb := &lbv1.LoadBalancer{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						pkgctllb.AnnotationKeyNetwork: tt.lbNetwork,
					},
				},
				Status: lbv1.LoadBalancerStatus{
					Address: tt.lbAddress,
				},
			}
			got := isPrimaryServiceUpdatedWithIP(svc, lb, tt.ip, tt.resolvedIface)
			if got != tt.want {
				t.Errorf("isPrimaryServiceUpdatedWithIP() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_patchLB_Priority(t *testing.T) {
	tests := []struct {
		name                string
		mgmtNetwork         string
		initialAnnotations  map[string]string
		expectedAnnotations map[string]string
	}{
		{
			name:               "Management networks sets: normalization and application",
			mgmtNetwork:        "harvester-mgmt/vlan-100",
			initialAnnotations: map[string]string{},
			expectedAnnotations: map[string]string{
				utils.AnnotationKeyGuestClusterManagementNetworkOnLB: "harvester-mgmt/vlan-100",
			},
		},
		{
			name:               "Management network normalization: bare name to default namespace",
			mgmtNetwork:        "mgmt-vlan",
			initialAnnotations: map[string]string{},
			expectedAnnotations: map[string]string{
				utils.AnnotationKeyGuestClusterManagementNetworkOnLB: "default/mgmt-vlan",
			},
		},
		{
			name:        "Management network empty: existing mgmt annotation is stripped",
			mgmtNetwork: "",
			initialAnnotations: map[string]string{
				utils.AnnotationKeyGuestClusterManagementNetworkOnLB: "default/mgmt-vlan",
			},
			expectedAnnotations: map[string]string{},
		},
		{
			name:        "Invalid global config: results in stripped annotations",
			mgmtNetwork: "invalid/global/net/too/deep",
			initialAnnotations: map[string]string{
				utils.AnnotationKeyGuestClusterManagementNetworkOnLB: "old-val",
			},
			expectedAnnotations: map[string]string{},
		},
		{
			name:        "Preservation: unrelated annotations remain untouched",
			mgmtNetwork: "",
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
			initialAnnotations: map[string]string{
				utils.AnnotationKeyGuestClusterManagementNetworkOnLB: "namespace/",
			},
			expectedAnnotations: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup a clean config for every sub-test to avoid side-effects
			currentCfg := cfg.GetConfig()
			oldMgmt := currentCfg.ManagementNetwork

			currentCfg.ManagementNetwork = tt.mgmtNetwork

			// Cleanup after each subtest
			defer func() {
				currentCfg.ManagementNetwork = oldMgmt
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

func Test_isSecondaryServiceUpdatedWithPrimary(t *testing.T) {
	const (
		ip         = "192.168.101.57"
		labelValue = "default.primary-svc"
	)

	tests := []struct {
		name        string
		annotations map[string]string
		labels      map[string]string
		want        bool
	}{
		{
			// Fully updated: no serviceInterface annotation (interface is not set for secondary).
			name: "fully_updated",
			annotations: map[string]string{
				utils.KeyKubevipLoadBalancerIP: ip,
				utils.KeyIPAM:                  "",
			},
			labels: map[string]string{utils.KeyPrimaryService: labelValue},
			want:   true,
		},
		{
			name: "ip_mismatch",
			annotations: map[string]string{
				utils.KeyKubevipLoadBalancerIP: "192.168.101.1",
				utils.KeyIPAM:                  "",
			},
			labels: map[string]string{utils.KeyPrimaryService: labelValue},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newServiceWithAnnotations(tt.annotations, tt.labels)
			primary := newServiceWithAnnotations(nil, nil)
			got := isSecondaryServiceUpdatedWithPrimary(primary, svc, ip, labelValue)
			if got != tt.want {
				t.Errorf("isSecondaryServiceUpdatedWithPrimary() = %v, want %v", got, tt.want)
			}
		})
	}
}
func newNADMappingConfigMap(mapping map[string]string) *v1.ConfigMap {
	data, _ := json.Marshal(mapping)
	return &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.ConfigMapNADMapping,
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			utils.ConfigMapKeyNADMapping: string(data),
		},
	}
}

func Test_resolveNetworkInterface(t *testing.T) {
	const svcNS, svcName = "default", "my-svc"

	validMapping := map[string]string{
		"default/net123": "enp2s0",
		"default/mgmt":   "enp1s0",
	}

	tests := []struct {
		name        string
		annotations map[string]string
		cache       *fakeclients.ConfigMapCache
		wantErr     bool
	}{
		{
			name:        "no relevant annotations: always pass",
			annotations: map[string]string{},
			cache:       fakeclients.NewConfigMapCache(nil, nil),
			wantErr:     false,
		},
		{
			name: "ConfigMap not found: pass through",
			annotations: map[string]string{
				utils.KeyNetwork: "default/mgmt",
				utils.KeyIPAM:    string(lbv1.DHCP),
			},
			cache:   fakeclients.NewConfigMapCache(nil, nil),
			wantErr: false,
		},
		{
			name: "DHCP: network present in mapping",
			annotations: map[string]string{
				utils.KeyNetwork: "default/net123",
				utils.KeyIPAM:    string(lbv1.DHCP),
			},
			cache:   fakeclients.NewConfigMapCache(newNADMappingConfigMap(validMapping), nil),
			wantErr: false,
		},
		{
			name: "DHCP: network not in mapping",
			annotations: map[string]string{
				utils.KeyNetwork: "default/unknown-net",
				utils.KeyIPAM:    string(lbv1.DHCP),
			},
			cache:   fakeclients.NewConfigMapCache(newNADMappingConfigMap(validMapping), nil),
			wantErr: true,
		},
		{
			name: "Pool: network NAD present in mapping",
			annotations: map[string]string{
				utils.KeyNetwork: "default/net123",
				utils.KeyIPAM:    string(lbv1.Pool),
			},
			cache:   fakeclients.NewConfigMapCache(newNADMappingConfigMap(validMapping), nil),
			wantErr: false,
		},
		{
			name: "Pool: network NAD not in mapping",
			annotations: map[string]string{
				utils.KeyNetwork: "default/unknown-net",
				utils.KeyIPAM:    string(lbv1.Pool),
			},
			cache:   fakeclients.NewConfigMapCache(newNADMappingConfigMap(validMapping), nil),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newServiceWithAnnotations(tt.annotations, nil)
			svc.Namespace = svcNS
			svc.Name = svcName

			lbm := &LoadBalancerManager{configMapCache: tt.cache}
			_, err := lbm.resolveNetworkInterface(svc)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveNetworkInterface() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
