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
		name   string
		node   *v1.Node
		vmi    *kubevirtv1.VirtualMachineInstance
		output []v1.NodeAddress
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
							IP:   networkDefaultIP,
						},
						{
							Name: network120,
							IP:   network120IP,
						},
						{
							Name: network130,
							IP:   network130IP,
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
							IP:   networkDefaultIP,
						},
						{
							Name: network120,
							IP:   network120IP,
						},
						{
							Name: network130,
							IP:   network130IP,
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
							IP:   networkDefaultIP,
						},
						{
							Name: network120,
							IP:   network120IP,
						},
						{
							Name: network130,
							IP:   network130IP,
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
							IP:   networkDefaultIP,
						},
						{
							Name: network120,
							IP:   network120IP,
						},
						{
							Name: network130,
							IP:   network130IP,
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
							IP:   networkDefaultIP,
						},
						{
							Name: network120,
							IP:   network120IP,
						},
						{
							Name: network130,
							IP:   network130IP,
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
			ips := getNodeAddresses(tt.node, tt.vmi)
			if !checkOutputEqual(tt.output, ips) {
				t.Errorf("case %v failed, expected output %+v, real output: %+v", tt.name, tt.output, ips)
			}
		})
	}
}
