package ccm

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cloud-provider/api"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

const (
	testNamespace = "default"
	nodeName      = "test"

	networkDefault = "default"
	network120     = "vlan120"
	network130     = "vlan130"

	networkDefaultIP = "192.168.100.10"
	network120IP     = "192.168.120.10"
	network130IP     = "192.168.130.10"
)

func Test_getNodeAddresses(t *testing.T) {
	tests := []struct {
		name    string
		node    *v1.Node
		vmi     *kubevirtv1.VirtualMachineInstance
		output  []v1.NodeAddress
		wantErr string
	}{
		{
			name: "1 internal and 2 external IPs",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
					Annotations: map[string]string{
						api.AnnotationAlphaProvidedIPAddr: networkDefaultIP,
					},
				},
			},
			vmi: &kubevirtv1.VirtualMachineInstance{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      nodeName,
				},
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Networks: []kubevirtv1.Network{
						{
							Name: networkDefault,
						},
						{
							Name: network120,
						},
						{
							Name: network130,
						},
					},
				},
				Status: kubevirtv1.VirtualMachineInstanceStatus{
					Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{
						{
							Name: networkDefault,
							IPs:  []string{networkDefaultIP},
						},
						{
							Name: network120,
							IPs:  []string{network120IP},
						},
						{
							Name: network130,
							IPs:  []string{network130IP},
						},
					},
				},
			},
			output: []v1.NodeAddress{
				{
					Type:    v1.NodeInternalIP,
					Address: networkDefaultIP,
				},
				{
					Type:    v1.NodeExternalIP,
					Address: network120IP,
				},
				{
					Type:    v1.NodeExternalIP,
					Address: network130IP,
				},
				{
					Type:    v1.NodeHostName,
					Address: nodeName,
				},
			},
		},
		{
			name: "1 internal and 2 external IPs, additional internal IPs do not match",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
					Annotations: map[string]string{
						api.AnnotationAlphaProvidedIPAddr: networkDefaultIP,
						KeyAdditionalInternalIPs:          "[\"192.168.120.12\", \"192.168.120.11\"]", // match nothing
					},
				},
			},
			vmi: &kubevirtv1.VirtualMachineInstance{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      nodeName,
				},
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Networks: []kubevirtv1.Network{
						{
							Name: networkDefault,
						},
						{
							Name: network120,
						},
						{
							Name: network130,
						},
					},
				},
				Status: kubevirtv1.VirtualMachineInstanceStatus{
					Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{
						{
							Name: networkDefault,
							IPs:  []string{networkDefaultIP},
						},
						{
							Name: network120,
							IPs:  []string{network120IP},
						},
						{
							Name: network130,
							IPs:  []string{network130IP},
						},
					},
				},
			},
			output: []v1.NodeAddress{
				{
					Type:    v1.NodeInternalIP,
					Address: networkDefaultIP,
				},
				{
					Type:    v1.NodeExternalIP,
					Address: network120IP,
				},
				{
					Type:    v1.NodeExternalIP,
					Address: network130IP,
				},
				{
					Type:    v1.NodeHostName,
					Address: nodeName,
				},
			},
		},
		{
			name: "1 internal and 2 external IPs, malformed annotations are skipped",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
					Annotations: map[string]string{
						api.AnnotationAlphaProvidedIPAddr: networkDefaultIP,
						KeyAdditionalInternalIPs:          "192.168.120.11", // not a valid []string converted JSON
					},
				},
			},
			vmi: &kubevirtv1.VirtualMachineInstance{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      nodeName,
				},
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Networks: []kubevirtv1.Network{
						{
							Name: networkDefault,
						},
						{
							Name: network120,
						},
						{
							Name: network130,
						},
					},
				},
				Status: kubevirtv1.VirtualMachineInstanceStatus{
					Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{
						{
							Name: networkDefault,
							IPs:  []string{networkDefaultIP},
						},
						{
							Name: network120,
							IPs:  []string{network120IP},
						},
						{
							Name: network130,
							IPs:  []string{network130IP},
						},
					},
				},
			},
			output: []v1.NodeAddress{
				{
					Type:    v1.NodeInternalIP,
					Address: networkDefaultIP,
				},
				{
					Type:    v1.NodeExternalIP,
					Address: network120IP,
				},
				{
					Type:    v1.NodeExternalIP,
					Address: network130IP,
				},
				{
					Type:    v1.NodeHostName,
					Address: nodeName,
				},
			},
		},
		{
			name: "2 internal and 1 external IPs",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
					Annotations: map[string]string{
						api.AnnotationAlphaProvidedIPAddr: networkDefaultIP,
						KeyAdditionalInternalIPs:          "[\"192.168.120.10\", \"192.168.120.11\"]",
					},
				},
			},
			vmi: &kubevirtv1.VirtualMachineInstance{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      nodeName,
				},
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Networks: []kubevirtv1.Network{
						{
							Name: networkDefault,
						},
						{
							Name: network120,
						},
						{
							Name: network130,
						},
					},
				},
				Status: kubevirtv1.VirtualMachineInstanceStatus{
					Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{
						{
							Name: networkDefault,
							IPs:  []string{networkDefaultIP},
						},
						{
							Name: network120,
							IPs:  []string{network120IP},
						},
						{
							Name: network130,
							IPs:  []string{network130IP},
						},
					},
				},
			},
			output: []v1.NodeAddress{
				{
					Type:    v1.NodeInternalIP,
					Address: networkDefaultIP,
				},
				{
					Type:    v1.NodeInternalIP,
					Address: network120IP,
				},
				{
					Type:    v1.NodeExternalIP,
					Address: network130IP,
				},
				{
					Type:    v1.NodeHostName,
					Address: nodeName,
				},
			},
		},
		{
			name: "3 internal and 0 external IPs",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
					Annotations: map[string]string{
						api.AnnotationAlphaProvidedIPAddr: networkDefaultIP,
						KeyAdditionalInternalIPs:          "[\"192.168.120.10\", \"192.168.130.10\"]",
					},
				},
			},
			vmi: &kubevirtv1.VirtualMachineInstance{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      nodeName,
				},
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Networks: []kubevirtv1.Network{
						{
							Name: networkDefault,
						},
						{
							Name: network120,
						},
						{
							Name: network130,
						},
					},
				},
				Status: kubevirtv1.VirtualMachineInstanceStatus{
					Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{
						{
							Name: networkDefault,
							IPs:  []string{networkDefaultIP},
						},
						{
							Name: network120,
							IPs:  []string{network120IP},
						},
						{
							Name: network130,
							IPs:  []string{network130IP},
						},
					},
				},
			},
			output: []v1.NodeAddress{
				{
					Type:    v1.NodeInternalIP,
					Address: networkDefaultIP,
				},
				{
					Type:    v1.NodeInternalIP,
					Address: network120IP,
				},
				{
					Type:    v1.NodeInternalIP,
					Address: network130IP,
				},
				{
					Type:    v1.NodeHostName,
					Address: nodeName,
				},
			},
		},
		{
			name: "Multiple IPs on one interface, all internal",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
					Annotations: map[string]string{
						api.AnnotationAlphaProvidedIPAddr: networkDefaultIP,
						KeyAdditionalInternalIPs:          "[\"192.168.120.10\", \"192.168.130.10\"]",
					},
				},
			},
			vmi: &kubevirtv1.VirtualMachineInstance{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      nodeName,
				},
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Networks: []kubevirtv1.Network{
						{
							Name: networkDefault,
						},
					},
				},
				Status: kubevirtv1.VirtualMachineInstanceStatus{
					Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{
						{
							Name: networkDefault,
							IPs:  []string{networkDefaultIP, network120IP, network130IP},
						},
					},
				},
			},
			output: []v1.NodeAddress{
				{
					Type:    v1.NodeInternalIP,
					Address: networkDefaultIP,
				},
				{
					Type:    v1.NodeInternalIP,
					Address: network120IP,
				},
				{
					Type:    v1.NodeInternalIP,
					Address: network130IP,
				},
				{
					Type:    v1.NodeHostName,
					Address: nodeName,
				},
			},
		},
		{
			name: "Multiple IPs on one interface, mix internal and external",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
					Annotations: map[string]string{
						api.AnnotationAlphaProvidedIPAddr: networkDefaultIP,
						KeyAdditionalInternalIPs:          "[\"192.168.130.10\"]",
					},
				},
			},
			vmi: &kubevirtv1.VirtualMachineInstance{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      nodeName,
				},
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Networks: []kubevirtv1.Network{
						{
							Name: networkDefault,
						},
					},
				},
				Status: kubevirtv1.VirtualMachineInstanceStatus{
					Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{
						{
							Name: networkDefault,
							IPs:  []string{networkDefaultIP, network120IP, network130IP},
						},
					},
				},
			},
			output: []v1.NodeAddress{
				{
					Type:    v1.NodeInternalIP,
					Address: networkDefaultIP,
				},
				{
					Type:    v1.NodeExternalIP,
					Address: network120IP,
				},
				{
					Type:    v1.NodeInternalIP,
					Address: network130IP,
				},
				{
					Type:    v1.NodeHostName,
					Address: nodeName,
				},
			},
		},
		{
			name: "Extra user defined internal IPs as range",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
					Annotations: map[string]string{
						api.AnnotationAlphaProvidedIPAddr: networkDefaultIP,
						KeyAdditionalInternalIPs:          "[\"172.20.0.0/24\", \"192.168.130.10\", \"2001:db8::1/64\"]",
					},
				},
			},
			vmi: &kubevirtv1.VirtualMachineInstance{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      nodeName,
				},
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Networks: []kubevirtv1.Network{
						{
							Name: networkDefault,
						},
						{
							Name: network130,
						},
					},
				},
				Status: kubevirtv1.VirtualMachineInstanceStatus{
					Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{
						{
							Name: networkDefault,
							IPs:  []string{networkDefaultIP, network120IP, "172.20.0.111", "2001:db8::1", "2001:f00::1"},
						},
						{
							Name: network130,
							IPs:  []string{network130IP, "172.20.0.222", "2001:db8::2"},
						},
					},
				},
			},
			output: []v1.NodeAddress{
				{
					Type:    v1.NodeInternalIP,
					Address: networkDefaultIP,
				},
				{
					Type:    v1.NodeExternalIP,
					Address: network120IP,
				},
				{
					Type:    v1.NodeInternalIP,
					Address: "172.20.0.111",
				},
				{
					Type:    v1.NodeInternalIP,
					Address: "2001:db8::1",
				},
				{
					Type:    v1.NodeExternalIP,
					Address: "2001:f00::1",
				},
				{
					Type:    v1.NodeInternalIP,
					Address: network130IP,
				},
				{
					Type:    v1.NodeInternalIP,
					Address: "172.20.0.222",
				},
				{
					Type:    v1.NodeInternalIP,
					Address: "2001:db8::2",
				},
				{
					Type:    v1.NodeHostName,
					Address: nodeName,
				},
			},
		},
		{
			name: "No provided node IP annotation",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        nodeName,
					Annotations: map[string]string{},
				},
			},
			vmi: &kubevirtv1.VirtualMachineInstance{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      nodeName,
				},
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Networks: []kubevirtv1.Network{
						{
							Name: networkDefault,
						},
						{
							Name: network130,
						},
					},
				},
				Status: kubevirtv1.VirtualMachineInstanceStatus{
					Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{
						{
							Name: networkDefault,
							IPs:  []string{"172.20.0.111", "2001:db8::1", "2001:f00::1"},
						},
						{
							Name: network130,
							IPs:  []string{"172.20.0.222", "2001:db8::2"},
						},
					},
				},
			},
			output: []v1.NodeAddress{
				{
					Type:    v1.NodeInternalIP,
					Address: "172.20.0.111",
				},
				{
					Type:    v1.NodeInternalIP,
					Address: "2001:db8::1",
				},
				{
					Type:    v1.NodeInternalIP,
					Address: "2001:f00::1",
				},
				{
					Type:    v1.NodeInternalIP,
					Address: "172.20.0.222",
				},
				{
					Type:    v1.NodeInternalIP,
					Address: "2001:db8::2",
				},
				{
					Type:    v1.NodeHostName,
					Address: nodeName,
				},
			},
		},
		{
			name: "Malformed node IP annotation",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
					Annotations: map[string]string{
						api.AnnotationAlphaProvidedIPAddr: "broken",
					},
				},
			},
			wantErr: "annotation \"alpha.kubernetes.io/provided-node-ip\" is invalid: failed to parse IP address \"broken\": ParseAddr(\"broken\"): unable to parse IP",
		},
	}

	checkOutputEqual := func(expected, output []v1.NodeAddress) bool {
		if len(expected) != len(output) {
			return false
		}
		for i := range expected {
			if expected[i].Type != output[i].Type || expected[i].Address != output[i].Address {
				return false
			}
		}
		return true
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ips, err := getNodeAddresses(tt.node, tt.vmi)

			var errStr string
			if err != nil {
				errStr = err.Error()
			}

			if errStr != tt.wantErr {
				t.Errorf("getNodeAddresses() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !checkOutputEqual(tt.output, ips) {
				t.Errorf("case %v failed, expected output %+v, real output: %+v", tt.name, tt.output, ips)
			}
		})
	}
}
