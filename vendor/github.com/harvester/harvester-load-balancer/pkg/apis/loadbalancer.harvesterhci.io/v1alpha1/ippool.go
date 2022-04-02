package v1alpha1

import (
	"github.com/rancher/wrangler/pkg/condition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=pool;pools,scope=Cluster
// +kubebuilder:printcolumn:name="DESCRIPTION",type=string,JSONPath=`.spec.description`
// +kubebuilder:printcolumn:name="VLAN",type=string,JSONPath=`.spec.network.vlan`
// +kubebuilder:printcolumn:name="RANGES",type=string,JSONPath=`.spec.ranges`

type IPPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              IPPoolSpec   `json:"spec,omitempty"`
	Status            IPPoolStatus `json:"status,omitempty"`
}

type IPPoolSpec struct {
	// +optional
	Description string `json:"description,omitempty"`

	Ranges []Range `json:"ranges"`
	// +optional
	Network IPPoolNetwork `json:"network,omitempty"`
	// +optional
	Projects []IPPoolProject `json:"projects,omitempty"`
	// +optional
	Namespaces []IPPoolNamespace `json:"namespaces,omitempty"`
}

// Refer to github.com/containernetworking/plugins/plugins/ipam/host-local/backend/allocator.Range
type Range struct {
	RangeStart string `json:"rangeStart,omitempty"` // The first ip, inclusive
	RangeEnd   string `json:"rangeEnd,omitempty"`   // The last ip, inclusive
	Subnet     string `json:"subnet"`
	Gateway    string `json:"gateway,omitempty"`
}

type IPPoolNetwork struct {
	// +optional
	VLAN string `json:"vlan,omitempty"`
}

type IPPoolProject struct {
	Name string `json:"name"`
	// +optional
	Namespaces []IPPoolNamespace `json:"namespaces,omitempty"`
}

type IPPoolNamespace struct {
	Name string `json:"name"`
	// +optional
	GuestClusters []string `json:"guestClusters,omitempty"`
}

type IPPoolStatus struct {
	Total int64 `json:"total"`

	Available int64 `json:"available"`

	LastAllocated string `json:"lastAllocated"`
	// +optional
	Allocated map[string]string `json:"allocated,omitempty"`
	// +optional
	AllocatedHistory map[string]string `json:"allocatedHistory,omitempty"`
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

var (
	IPPoolReady condition.Cond = "Ready"
)
