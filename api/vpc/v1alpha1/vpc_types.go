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

// TunnelType defines the overlay tunnel encapsulation type.
// +kubebuilder:validation:Enum=VXLAN;IPIP;GRE
type TunnelType string

const (
	TunnelTypeVXLAN TunnelType = "VXLAN"
	TunnelTypeIPIP  TunnelType = "IPIP"
	TunnelTypeGRE   TunnelType = "GRE"
)

// InterSubnetPolicy is the default for traffic between two different subnets of a VPC.
// +kubebuilder:validation:Enum=Deny;Allow
type InterSubnetPolicy string

const (
	InterSubnetDeny  InterSubnetPolicy = "Deny"
	InterSubnetAllow InterSubnetPolicy = "Allow"
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

	// vni is the VXLAN/GRE Network Identifier for overlay isolation.
	// Required for VXLAN and GRE tunnel types. Must be unique across VPCs.
	// The same VNI is used on all nodes to establish tunnels for this VPC.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=16777215
	// +optional
	VNI *int32 `json:"vni,omitempty"`

	// tunnelType specifies the overlay encapsulation for cross-node traffic.
	// +kubebuilder:default=VXLAN
	// +optional
	TunnelType TunnelType `json:"tunnelType,omitempty"`

	// interSubnetPolicy is the default for traffic between different subnets of this VPC.
	// Deny (default) blocks it unless a Firewall rule allows it (rules can name subnets with
	// srcSubnetRef/dstSubnetRef); traffic inside one subnet and to/from outside the VPC is not
	// affected. It is enforced at the destination VNic with connection tracking, so replies to
	// allowed connections pass automatically.
	// +kubebuilder:default=Deny
	// +optional
	InterSubnetPolicy InterSubnetPolicy `json:"interSubnetPolicy,omitempty"`

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

// VPCNodeAttachment describes one node participating in a VPC.
type VPCNodeAttachment struct {
	// nodeName is the Kubernetes node name.
	NodeName string `json:"nodeName"`
	// tunnelIP is the underlay IP the node uses as tunnel endpoint.
	// +optional
	TunnelIP string `json:"tunnelIP,omitempty"`
	// bridge is the OVS bridge created for this VPC on the node.
	// +optional
	Bridge string `json:"bridge,omitempty"`
}

// VPCRoute is a single route in the VPC route table.
type VPCRoute struct {
	// destination is the destination CIDR.
	Destination string `json:"destination"`
	// nextHopNode is the node hosting the destination subnet (tunnel next hop).
	// +optional
	NextHopNode string `json:"nextHopNode,omitempty"`
	// nextHopIP is the tunnel endpoint IP of nextHopNode.
	// +optional
	NextHopIP string `json:"nextHopIP,omitempty"`
	// type is Local, Tunnel or Gateway.
	// +optional
	Type string `json:"type,omitempty"`
}

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

	// attachedNodes lists the nodes that participate in this VPC.
	// Node agents watch this to create bridges, tunnels and flows.
	// +optional
	AttachedNodes []VPCNodeAttachment `json:"attachedNodes,omitempty"`

	// routes is the route table distributed via the Kubernetes API (replaces BGP).
	// Every node agent programs these into its local OVS pipeline.
	// +optional
	Routes []VPCRoute `json:"routes,omitempty"`

	// usedBy lists dependent resources (kind/name) that block deletion.
	// +optional
	UsedBy []string `json:"usedBy,omitempty"`

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
// +kubebuilder:printcolumn:name="Tunnel",type=string,JSONPath=`.spec.tunnelType`
// +kubebuilder:printcolumn:name="VNI",type=integer,JSONPath=`.status.allocatedVNI`
// +kubebuilder:printcolumn:name="Inter-subnet",type=string,JSONPath=`.spec.interSubnetPolicy`
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
