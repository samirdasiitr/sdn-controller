/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SubnetType defines whether a subnet is public or private.
// +kubebuilder:validation:Enum=Private;Public
type SubnetType string

const (
	SubnetTypePrivate SubnetType = "Private"
	SubnetTypePublic  SubnetType = "Public"
)

// DHCPConfig defines DHCP settings for a subnet.
type DHCPConfig struct {
	// enabled activates the DHCP server for this subnet.
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`

	// range defines the IP allocation range.
	// +optional
	Range *DHCPRange `json:"range,omitempty"`

	// dns specifies DNS server addresses.
	// +optional
	DNS []string `json:"dns,omitempty"`
}

// DHCPRange defines start/end of DHCP allocation range.
type DHCPRange struct {
	// start is the first IP in the DHCP range.
	Start string `json:"start"`
	// end is the last IP in the DHCP range.
	End string `json:"end"`
}

// SubnetSpec defines the desired state of Subnet.
type SubnetSpec struct {
	// vpcRef is the name of the parent VPC.
	// +kubebuilder:validation:Required
	VPCRef string `json:"vpcRef"`

	// cidr is the IPv4 CIDR block for this subnet.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^(\d{1,3}\.){3}\d{1,3}/\d{1,2}$`
	CIDR string `json:"cidr"`

	// gateway is the default gateway IP for this subnet.
	// Auto-calculated as first usable IP if not set.
	// +optional
	Gateway string `json:"gateway,omitempty"`

	// type defines whether this subnet is Public or Private.
	// +kubebuilder:default=Private
	// +optional
	Type SubnetType `json:"type,omitempty"`

	// vlanID is an optional 802.1Q VLAN tag for this subnet.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=4094
	// +optional
	VLANID *int32 `json:"vlanID,omitempty"`

	// mtu overrides the VPC MTU for this subnet.
	// +kubebuilder:validation:Minimum=1280
	// +kubebuilder:validation:Maximum=9216
	// +optional
	MTU *int32 `json:"mtu,omitempty"`

	// dhcp configures the DHCP server for this subnet.
	// +optional
	DHCP *DHCPConfig `json:"dhcp,omitempty"`

	// nodeSelector targets specific nodes for this subnet.
	// +optional
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`
}

// SubnetPhase represents the lifecycle phase of a Subnet.
// +kubebuilder:validation:Enum=Pending;Provisioning;Ready;Deleting;Failed
type SubnetPhase string

const (
	SubnetPhasePending      SubnetPhase = "Pending"
	SubnetPhaseProvisioning SubnetPhase = "Provisioning"
	SubnetPhaseReady        SubnetPhase = "Ready"
	SubnetPhaseDeleting     SubnetPhase = "Deleting"
	SubnetPhaseFailed       SubnetPhase = "Failed"
)

// SubnetStatus defines the observed state of Subnet.
type SubnetStatus struct {
	// phase is the current lifecycle phase.
	// +optional
	Phase SubnetPhase `json:"phase,omitempty"`

	// allocatedIPs is the count of IPs currently allocated.
	// +optional
	AllocatedIPs int32 `json:"allocatedIPs,omitempty"`

	// availableIPs is the count of IPs remaining in the pool.
	// +optional
	AvailableIPs int32 `json:"availableIPs,omitempty"`

	// gateway is the resolved gateway IP.
	// +optional
	Gateway string `json:"gateway,omitempty"`

	// conditions represent the current state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=sn
// +kubebuilder:printcolumn:name="VPC",type=string,JSONPath=`.spec.vpcRef`
// +kubebuilder:printcolumn:name="CIDR",type=string,JSONPath=`.spec.cidr`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Gateway",type=string,JSONPath=`.status.gateway`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Subnet is a network segment within a VPC with its own CIDR, gateway, and DHCP.
type Subnet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SubnetSpec   `json:"spec,omitempty"`
	Status SubnetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SubnetList contains a list of Subnet.
type SubnetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Subnet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Subnet{}, &SubnetList{})
}
