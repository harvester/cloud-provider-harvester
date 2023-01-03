package ccm

import (
	"strings"
	"testing"
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
		{"case_1", args{"bvis", "default", "abcd", defaultUID}, "bvis-default-abcd-"},
		{"case_2", args{"1bvis", "default", "abcd", defaultUID}, "a1bvis-default-abcd-"},
		{"case_3", args{"bvis", "central-rundeck", "kube-system-rke2-ingress-nginx-controller", defaultUID}, "bvis-central-rundeck-kube-system-rke2-ingress-nginx-con"},
		{"case_4", args{"bvis", "central-rundeck", "kube-system-rke2-ingress-nginx-co-abcd", defaultUID}, "bvis-central-rundeck-kube-system-rke2-ingress-nginx-co-"},
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
