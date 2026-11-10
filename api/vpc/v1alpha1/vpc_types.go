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

// VPCSpec defines the desired state of VPC.
type VPCSpec struct {
	// cidr is the primary IPv4 CIDR block for this VPC.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^(\d{1,3}\.){3}\d{1,3}/\d{1,2}$`
	CIDR string `json:"cidr"`

	// secondaryCIDRs are additional CIDR blocks for this VPC.
	// +optional
	SecondaryCIDRs []string `json:"secondaryCIDRs,omitempty"`

	// vni is the VXLAN Network Identifier for overlay isolation.
	// Auto-allocated if not specified.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=16777215
	// +optional
	VNI *int32 `json:"vni,omitempty"`

	// mtu is the Maximum Transmission Unit for this VPC.
	// +kubebuilder:validation:Minimum=1280
	// +kubebuilder:validation:Maximum=9216
	// +kubebuilder:default=1500
	// +optional
	MTU int32 `json:"mtu,omitempty"`

	// enableDNS enables internal DNS resolution within the VPC.
	// +kubebuilder:default=true
	// +optional
	EnableDNS bool `json:"enableDNS,omitempty"`

	// nodeSelector targets specific nodes that participate in this VPC.
	// +optional
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`
}

// VPCPhase represents the lifecycle phase of a VPC.
// +kubebuilder:validation:Enum=Pending;Provisioning;Ready;Deleting;Failed
type VPCPhase string

const (
	VPCPhasePending      VPCPhase = "Pending"
	VPCPhaseProvisioning VPCPhase = "Provisioning"
	VPCPhaseReady        VPCPhase = "Ready"
	VPCPhaseDeleting     VPCPhase = "Deleting"
	VPCPhaseFailed       VPCPhase = "Failed"
)

// VPCStatus defines the observed state of VPC.
type VPCStatus struct {
	// phase is the current lifecycle phase of the VPC.
	// +optional
	Phase VPCPhase `json:"phase,omitempty"`

	// allocatedVNI is the VNI assigned to this VPC.
	// +optional
	AllocatedVNI int32 `json:"allocatedVNI,omitempty"`

	// allocatedSubnets is the number of subnets in this VPC.
	// +optional
	AllocatedSubnets int32 `json:"allocatedSubnets,omitempty"`

	// activeNodes is the number of nodes participating in this VPC.
	// +optional
	ActiveNodes int32 `json:"activeNodes,omitempty"`

	// conditions represent the current state of the VPC resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=vpc
// +kubebuilder:printcolumn:name="CIDR",type=string,JSONPath=`.spec.cidr`
// +kubebuilder:printcolumn:name="VNI",type=integer,JSONPath=`.status.allocatedVNI`
// +kubebuilder:printcolumn:name="MTU",type=integer,JSONPath=`.spec.mtu`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// VPC is a virtual private cloud providing isolated network space with its own
// CIDR block, VXLAN overlay, and routing domain.
type VPC struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VPCSpec   `json:"spec,omitempty"`
	Status VPCStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VPCList contains a list of VPC.
type VPCList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VPC `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VPC{}, &VPCList{})
}
