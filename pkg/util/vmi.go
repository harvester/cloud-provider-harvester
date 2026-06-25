package util

import kubevirtv1 "kubevirt.io/api/core/v1"

// getInterfaceToNADMapping builds a map of Linux interface name -> Multus NAD name
// by joining VMI spec networks with VMI status interfaces (reported by the guest agent).
// Example result: {"enp1s0": "default/mgmt-vlan1", "enp2s0": "default/net123"}
// Interfaces without a Multus network (e.g. calico, kube-vip macvlan) are excluded.
func getInterfaceToNADMapping(vmi *kubevirtv1.VirtualMachineInstance) map[string]string {
	nameToNAD := make(map[string]string, len(vmi.Spec.Networks))
	for _, net := range vmi.Spec.Networks {
		if net.Multus != nil {
			nameToNAD[net.Name] = net.Multus.NetworkName
		}
	}

	result := make(map[string]string)
	for _, iface := range vmi.Status.Interfaces {
		if iface.InterfaceName == "" || iface.Name == "" {
			continue
		}
		if nad, ok := nameToNAD[iface.Name]; ok {
			result[iface.InterfaceName] = nad
		}
	}
	return result
}

// GetCommonVMINADs returns a map of NAD name -> Linux interface name for all (NAD, interface)
// pairs that are consistent (same NAD on the same interface) across ALL provided VMIs.
// Returns nil if the VMI list is empty; callers should treat nil as "unknown".
//
// This is the primary intersection function. It handles both topology cases:
//   - Asymmetric: a NAD present only on some nodes is excluded.
//   - Misorder: a NAD on different interfaces across nodes is excluded.
//
// Example:
//
//	vm1: enp1s0->default/mgmt, enp2s0->default/net123
//	vm2: enp1s0->default/mgmt, enp3s0->default/net123  (net123 on different interface)
//	result: {"default/mgmt": "enp1s0"}
func GetCommonVMINADs(vmis []kubevirtv1.VirtualMachineInstance) map[string]string {
	if len(vmis) == 0 {
		return nil
	}
	// Invert the first VMI's interface->NAD mapping to get NAD->interface.
	result := invertStringMap(getInterfaceToNADMapping(&vmis[0]))
	for _, vmi := range vmis[1:] {
		curr := invertStringMap(getInterfaceToNADMapping(&vmi))
		for nad, iface := range result {
			if curr[nad] != iface {
				delete(result, nad)
			}
		}
	}
	return result
}

// invertStringMap returns a new map with keys and values swapped.
func invertStringMap(m map[string]string) map[string]string {
	result := make(map[string]string, len(m))
	for k, v := range m {
		result[v] = k
	}
	return result
}
