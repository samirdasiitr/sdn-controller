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

// ChainDirection defines the traffic direction for the chain.
// +kubebuilder:validation:Enum=Ingress;Egress;Both
type ChainDirection string

const (
	ChainDirectionIngress ChainDirection = "Ingress"
	ChainDirectionEgress  ChainDirection = "Egress"
	ChainDirectionBoth    ChainDirection = "Both"
)

// PortPair defines the ingress/egress ports for a VNF in a chain.
type PortPair struct {
	// ingress is the VNF ingress port name.
	Ingress string `json:"ingress"`

	// egress is the VNF egress port name.
	Egress string `json:"egress"`
}

// ChainElement is one hop in a service chain.
type ChainElement struct {
	// name is the human-readable name for this hop.
	Name string `json:"name"`

	// vnfRef references the VirtualNetworkFunction.
	VNFRef string `json:"vnfRef"`

	// portPair maps the ingress and egress ports.
	PortPair PortPair `json:"portPair"`
}

// ChainMatch defines what traffic enters the service chain.
type ChainMatch struct {
	// protocol is the L4 protocol to match.
	// +optional
	Protocol string `json:"protocol,omitempty"`

	// srcCIDR is the source network.
	// +optional
	SrcCIDR string `json:"srcCIDR,omitempty"`

	// dstCIDR is the destination network.
	// +optional
	DstCIDR string `json:"dstCIDR,omitempty"`

	// dstPort is the destination port.
	// +optional
	DstPort *int32 `json:"dstPort,omitempty"`
}

// ServiceChainSpec defines the desired state of ServiceChain.
type ServiceChainSpec struct {
	// vpcRef is the name of the VPC.
	// +kubebuilder:validation:Required
	VPCRef string `json:"vpcRef"`

	// subnetRef is the name of the Subnet.
	// +optional
	SubnetRef string `json:"subnetRef,omitempty"`

	// direction defines which traffic direction this chain applies to.
	// +kubebuilder:default=Ingress
	// +optional
	Direction ChainDirection `json:"direction,omitempty"`

	// chain is the ordered list of VNF hops.
	// +kubebuilder:validation:MinItems=1
	Chain []ChainElement `json:"chain"`

	// match defines what traffic enters this chain.
	// +optional
	Match *ChainMatch `json:"match,omitempty"`

	// offload enables hardware offload for chain steering.
	// +optional
	Offload *ChainOffloadConfig `json:"offload,omitempty"`

	// nodeSelector targets specific nodes.
	// +optional
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`
}

// ChainOffloadConfig defines offload settings for service chains.
type ChainOffloadConfig struct {
	// enabled activates hardware offload for chain steering flows.
	// +kubebuilder:default=false
	Enabled bool `json:"enabled"`
}

// ServiceChainPhase represents the lifecycle phase.
// +kubebuilder:validation:Enum=Pending;Active;Degraded;Failed
type ServiceChainPhase string

const (
	ServiceChainPhasePending  ServiceChainPhase = "Pending"
	ServiceChainPhaseActive   ServiceChainPhase = "Active"
	ServiceChainPhaseDegraded ServiceChainPhase = "Degraded"
	ServiceChainPhaseFailed   ServiceChainPhase = "Failed"
)

// ServiceChainStatus defines the observed state of ServiceChain.
type ServiceChainStatus struct {
	// nodes lists the hosts this resource is running on (Ready nodes matching
	// nodeSelector that are attached to the VPC).
	// +optional
	Nodes []string `json:"nodes,omitempty"`

	// phase is the current lifecycle phase.
	Phase ServiceChainPhase `json:"phase,omitempty"`

	// chainLength is the number of active hops.
	// +optional
	ChainLength int32 `json:"chainLength,omitempty"`

	// processedPackets is the total packet count through this chain.
	// +optional
	ProcessedPackets int64 `json:"processedPackets,omitempty"`

	// conditions represent the current state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=sc
// +kubebuilder:printcolumn:name="VPC",type=string,JSONPath=`.spec.vpcRef`
// +kubebuilder:printcolumn:name="Direction",type=string,JSONPath=`.spec.direction`
// +kubebuilder:printcolumn:name="Hops",type=integer,JSONPath=`.status.chainLength`
// +kubebuilder:printcolumn:name="Packets",type=integer,JSONPath=`.status.processedPackets`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ServiceChain steers traffic through an ordered sequence of VNFs.
type ServiceChain struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceChainSpec   `json:"spec,omitempty"`
	Status ServiceChainStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServiceChainList contains a list of ServiceChain.
type ServiceChainList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceChain `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServiceChain{}, &ServiceChainList{})
}
