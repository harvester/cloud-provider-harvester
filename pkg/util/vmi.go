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
// pairs that are consistent (same NAD on the same interface) across ALL qualifying VMIs.
//
// A VMI is considered qualifying only if it is Running and has interface status populated
// by the guest agent. VMIs that do not meet these criteria are silently skipped.
//
// Returns nil if no qualifying VMIs exist; callers should treat nil as a signal to clear
// any previously stored mapping.
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
func GetCommonVMINADs(vmis []*kubevirtv1.VirtualMachineInstance) map[string]string {
	// Strip VMIs that are not Running or have no guest-agent interface data.
	active := make([]kubevirtv1.VirtualMachineInstance, 0, len(vmis))
	for _, vmi := range vmis {
		if vmi == nil {
			continue
		}
		if vmi.Status.Phase == kubevirtv1.Running && len(vmi.Status.Interfaces) > 0 {
			active = append(active, *vmi)
		}
	}

	// No qualifying VMIs — signal that any stored mapping should be cleared.
	if len(active) == 0 {
		return nil
	}

	result := getNADToInterfaceMapping(&active[0])
	for _, vmi := range active[1:] {
		curr := getNADToInterfaceMapping(&vmi)
		for nad, iface := range result {
			if curr[nad] != iface {
				delete(result, nad)
			}
		}
	}

	return result
}
