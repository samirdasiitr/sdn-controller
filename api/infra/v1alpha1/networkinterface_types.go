/*
Copyright (c) 2026 Samir Das. All rights reserved.

PROPRIETARY AND CONFIDENTIAL. No license is granted. Any use of this software, and in
particular production or commercial use, requires the prior explicit written permission of
Samir Das <samiruor@gmail.com>. See the LICENSE file.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NICType defines the type of network interface.
// +kubebuilder:validation:Enum=Physical;Virtual;VF;Bond
type NICType string

const (
	NICTypePhysical NICType = "Physical"
	NICTypeVirtual  NICType = "Virtual"
	NICTypeVF       NICType = "VF"
	NICTypeBond     NICType = "Bond"
)

// OffloadConfig defines hardware offload settings.
type OffloadConfig struct {
	// enabled activates hardware offload if the NIC supports it.
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`

	// driver is the NIC driver name (auto-detected if empty).
	// +optional
	Driver string `json:"driver,omitempty"`
}

// NetworkInterfaceSpec defines the desired state of NetworkInterface.
type NetworkInterfaceSpec struct {
	// nodeName is the node this interface belongs to.
	// +kubebuilder:validation:Required
	NodeName string `json:"nodeName"`

	// type is the interface type.
	// +kubebuilder:default=Physical
	// +optional
	Type NICType `json:"type,omitempty"`

	// device is the Linux device name (e.g., eth0, ens3f0).
	// +kubebuilder:validation:Required
	Device string `json:"device"`

	// mtu is the interface MTU.
	// +kubebuilder:validation:Minimum=1280
	// +kubebuilder:validation:Maximum=9216
	// +kubebuilder:default=1500
	// +optional
	MTU int32 `json:"mtu,omitempty"`

	// addresses are the IP addresses assigned to this interface.
	// +optional
	Addresses []string `json:"addresses,omitempty"`

	// subnetRef links this interface to a Subnet.
	// +optional
	SubnetRef string `json:"subnetRef,omitempty"`

	// vpcRef links this interface to a VPC.
	// +optional
	VPCRef string `json:"vpcRef,omitempty"`

	// offload configures hardware offload.
	// +optional
	Offload *OffloadConfig `json:"offload,omitempty"`

	// dpuManaged indicates whether this interface is managed by a DPU.
	// +kubebuilder:default=false
	// +optional
	DPUManaged bool `json:"dpuManaged,omitempty"`
}

// NICPhase represents the lifecycle phase.
// +kubebuilder:validation:Enum=Pending;Configured;Error
type NICPhase string

const (
	NICPhasePending    NICPhase = "Pending"
	NICPhaseConfigured NICPhase = "Configured"
	NICPhaseError      NICPhase = "Error"
)

// NetworkInterfaceStatus defines the observed state of NetworkInterface.
type NetworkInterfaceStatus struct {
	// phase is the current lifecycle phase.
	Phase NICPhase `json:"phase,omitempty"`

	// macAddress is the hardware MAC address.
	// +optional
	MACAddress string `json:"macAddress,omitempty"`

	// speed is the link speed (e.g., "100Gbps").
	// +optional
	Speed string `json:"speed,omitempty"`

	// offloadCapable indicates whether the NIC supports offload.
	// +optional
	OffloadCapable bool `json:"offloadCapable,omitempty"`

	// offloadActive indicates whether offload is currently active.
	// +optional
	OffloadActive bool `json:"offloadActive,omitempty"`

	// driver is the detected NIC driver.
	// +optional
	Driver string `json:"driver,omitempty"`

	// firmwareVersion is the NIC firmware version.
	// +optional
	FirmwareVersion string `json:"firmwareVersion,omitempty"`

	// conditions represent the current state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=nic
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.spec.nodeName`
// +kubebuilder:printcolumn:name="Device",type=string,JSONPath=`.spec.device`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="MTU",type=integer,JSONPath=`.spec.mtu`
// +kubebuilder:printcolumn:name="Speed",type=string,JSONPath=`.status.speed`
// +kubebuilder:printcolumn:name="Offload",type=boolean,JSONPath=`.status.offloadActive`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// NetworkInterface represents a network interface (physical, virtual, or VF) on a node.
type NetworkInterface struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NetworkInterfaceSpec   `json:"spec,omitempty"`
	Status NetworkInterfaceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NetworkInterfaceList contains a list of NetworkInterface.
type NetworkInterfaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NetworkInterface `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NetworkInterface{}, &NetworkInterfaceList{})
}
