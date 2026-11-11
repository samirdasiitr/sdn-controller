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

// VNicAttachmentKind is the kind of workload a VNic is plugged into.
// +kubebuilder:validation:Enum=VM;VNF;Pod;Other
type VNicAttachmentKind string

const (
	VNicAttachVM    VNicAttachmentKind = "VM"
	VNicAttachVNF   VNicAttachmentKind = "VNF"
	VNicAttachPod   VNicAttachmentKind = "Pod"
	VNicAttachOther VNicAttachmentKind = "Other"
)

// VNicAttachment identifies the workload that owns the VNic.
type VNicAttachment struct {
	// kind of workload.
	// +kubebuilder:default=VM
	Kind VNicAttachmentKind `json:"kind"`

	// name of the workload (libvirt domain, VNF name, pod name).
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// VNicSpec defines the desired state of VNic.
type VNicSpec struct {
	// subnetRef is the Subnet this VNic is attached to. The VPC is derived from it.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	SubnetRef string `json:"subnetRef"`

	// ipAddress requests a specific address from the subnet; allocated automatically if empty.
	// +kubebuilder:validation:Pattern=`^(\d{1,3}\.){3}\d{1,3}$`
	// +optional
	IPAddress string `json:"ipAddress,omitempty"`

	// macAddress requests a specific MAC; generated deterministically if empty.
	// +kubebuilder:validation:Pattern=`^([0-9a-fA-F]{2}:){5}[0-9a-fA-F]{2}$`
	// +optional
	MACAddress string `json:"macAddress,omitempty"`

	// attachment is the workload that owns this VNic.
	// +optional
	Attachment *VNicAttachment `json:"attachment,omitempty"`

	// nodeName pins the VNic to a node. The node agent on that node binds the OVS port.
	// +optional
	NodeName string `json:"nodeName,omitempty"`

	// mtu overrides the VPC MTU for this VNic.
	// +kubebuilder:validation:Minimum=1280
	// +kubebuilder:validation:Maximum=9216
	// +optional
	MTU int32 `json:"mtu,omitempty"`

	// portSecurity drops traffic from this port whose source MAC/IP differ from the VNic's.
	// +kubebuilder:default=true
	// +optional
	PortSecurity *bool `json:"portSecurity,omitempty"`

	// firewallRef optionally names a Firewall that applies to this VNic.
	// +optional
	FirewallRef string `json:"firewallRef,omitempty"`
}

// VNicPhase is the lifecycle phase of a VNic.
// +kubebuilder:validation:Enum=Pending;Allocated;Bound;Deleting;Failed
type VNicPhase string

const (
	VNicPhasePending   VNicPhase = "Pending"
	VNicPhaseAllocated VNicPhase = "Allocated"
	VNicPhaseBound     VNicPhase = "Bound"
	VNicPhaseDeleting  VNicPhase = "Deleting"
	VNicPhaseFailed    VNicPhase = "Failed"
)

// VNicStatus defines the observed state of VNic.
type VNicStatus struct {
	// phase is the lifecycle phase. Bound means a node agent found the port and programmed it.
	// +optional
	Phase VNicPhase `json:"phase,omitempty"`

	// vpcRef is the VPC derived from the subnet.
	// +optional
	VPCRef string `json:"vpcRef,omitempty"`

	// ipAddress is the allocated address.
	// +optional
	IPAddress string `json:"ipAddress,omitempty"`

	// macAddress is the effective MAC.
	// +optional
	MACAddress string `json:"macAddress,omitempty"`

	// gateway is the subnet gateway for this VNic.
	// +optional
	Gateway string `json:"gateway,omitempty"`

	// prefixLength is the subnet prefix length.
	// +optional
	PrefixLength int32 `json:"prefixLength,omitempty"`

	// mtu is the effective MTU.
	// +optional
	MTU int32 `json:"mtu,omitempty"`

	// boundNode is the node whose agent bound the port.
	// +optional
	BoundNode string `json:"boundNode,omitempty"`

	// bridge is the OVS bridge holding the port.
	// +optional
	Bridge string `json:"bridge,omitempty"`

	// ovsPort is the OVS interface name (e.g. the libvirt vnetN tap).
	// +optional
	OVSPort string `json:"ovsPort,omitempty"`

	// ofPort is the OpenFlow port number.
	// +optional
	OFPort int32 `json:"ofPort,omitempty"`

	// conditions represent the current state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=vnic
// +kubebuilder:printcolumn:name="Subnet",type=string,JSONPath=`.spec.subnetRef`
// +kubebuilder:printcolumn:name="IP",type=string,JSONPath=`.status.ipAddress`
// +kubebuilder:printcolumn:name="MAC",type=string,JSONPath=`.status.macAddress`
// +kubebuilder:printcolumn:name="Workload",type=string,JSONPath=`.spec.attachment.name`
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.status.boundNode`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// VNic is a virtual network interface of a workload (VM, VNF, pod) in a subnet.
// It owns an IPAM address and a MAC, and is bound to an OVS port by the node agent.
type VNic struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VNicSpec   `json:"spec,omitempty"`
	Status VNicStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VNicList contains a list of VNic.
type VNicList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VNic `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VNic{}, &VNicList{})
}
