package util

import (
	kubevirtv1 "kubevirt.io/api/core/v1"
)

// getNADToInterfaceMapping builds a map of Multus NAD name -> Linux interface name
// by joining VMI spec networks with VMI status interfaces (reported by the guest agent).
// Example result: {"default/mgmt-vlan1": "enp1s0", "default/net123": "enp2s0"}
// Interfaces without a Multus network (e.g. calico, kube-vip macvlan) are excluded.
func getNADToInterfaceMapping(vmi *kubevirtv1.VirtualMachineInstance) map[string]string {
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
			result[nad] = iface.InterfaceName
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

	// Early exit if the first VMI doesn't have status info yet
	if len(vmis[0].Status.Interfaces) == 0 {
		return nil
	}

	result := getNADToInterfaceMapping(&vmis[0])
	for _, vmi := range vmis[1:] {
		if len(vmi.Status.Interfaces) == 0 {
			return nil // Guest agent data missing on one VM; consensus unknown
		}

		curr := getNADToInterfaceMapping(&vmi)

		for nad, iface := range result {
			currIface, exists := curr[nad]
			if !exists || currIface != iface {
				delete(result, nad)
			}
		}
	}

	return result
}
