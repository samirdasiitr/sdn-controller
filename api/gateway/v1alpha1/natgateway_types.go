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

// NATType defines the NAT translation type.
// +kubebuilder:validation:Enum=SNAT;DNAT;FullNAT
type NATType string

const (
	NATTypeSNAT    NATType = "SNAT"
	NATTypeDNAT    NATType = "DNAT"
	NATTypeFullNAT NATType = "FullNAT"
)

// PortRange defines a range of L4 ports.
type PortRange struct {
	// start is the first port in the range.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Start int32 `json:"start"`

	// end is the last port in the range.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	End int32 `json:"end"`
}

// PortForward publishes an internal endpoint on the gateway's external IP (DNAT).
type PortForward struct {
	// name is a human-readable name.
	Name string `json:"name"`
	// protocol is TCP or UDP.
	// +kubebuilder:validation:Enum=TCP;UDP
	Protocol string `json:"protocol"`
	// externalPort is the port on the external IP.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	ExternalPort int32 `json:"externalPort"`
	// internalIP is the target address inside the VPC.
	InternalIP string `json:"internalIP"`
	// internalPort is the target port.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	InternalPort int32 `json:"internalPort"`
}

// NATGatewaySpec defines the desired state of NATGateway.
type NATGatewaySpec struct {
	// vpcRef is the name of the VPC this NAT gateway serves.
	// +kubebuilder:validation:Required
	VPCRef string `json:"vpcRef"`

	// subnetRefs are the subnets whose traffic is NATed.
	// +kubebuilder:validation:MinItems=1
	SubnetRefs []string `json:"subnetRefs"`

	// externalIP is the IP used for SNAT translations.
	// +kubebuilder:validation:Required
	ExternalIP string `json:"externalIP"`

	// type is the NAT translation type.
	// +kubebuilder:default=SNAT
	// +optional
	Type NATType `json:"type,omitempty"`

	// portForwards are the DNAT rules (external IP:port -> internal IP:port).
	// Required for type DNAT and FullNAT.
	// +optional
	PortForwards []PortForward `json:"portForwards,omitempty"`

	// portRange limits the ephemeral port range for translations.
	// +optional
	PortRange *PortRange `json:"portRange,omitempty"`

	// ha configures high availability.
	// +optional
	HA *HAConfig `json:"ha,omitempty"`

	// nodeSelector targets nodes that can run this gateway.
	// +optional
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`
}

// EnforcementStatus is reported by each node agent after it programs its OVS
// pipeline for this object. It is keyed by node name in status.enforcement.
type EnforcementStatus struct {
	// observedGeneration is the spec generation the agent compiled and installed.
	ObservedGeneration int64 `json:"observedGeneration"`
	// programmed is true when the flows for this generation are installed.
	Programmed bool `json:"programmed"`
	// flows is the number of OpenFlow rules installed for this object on the node.
	// +optional
	Flows int32 `json:"flows,omitempty"`
	// matchedPackets is unused for gateways.
	// +optional
	MatchedPackets int64 `json:"matchedPackets,omitempty"`
	// newTranslations is the cumulative number of translations created on the node.
	// +optional
	NewTranslations int64 `json:"newTranslations,omitempty"`
	// packets and bytes are cumulative counters of the gateway's flows on the node.
	// +optional
	Packets int64 `json:"packets,omitempty"`
	// +optional
	Bytes int64 `json:"bytes,omitempty"`
	// translations is the number of active NAT translations on the node.
	// +optional
	Translations int64 `json:"translations,omitempty"`
	// message carries the error when programming failed.
	// +optional
	Message string `json:"message,omitempty"`
	// warnings are non-fatal limitations (e.g. health checks not implemented).
	// +optional
	Warnings []string `json:"warnings,omitempty"`
}

// NATGatewayStatus defines the observed state of NATGateway.
type NATGatewayStatus struct {
	// enforcement is what each node agent actually programmed, keyed by node name.
	// +optional
	Enforcement map[string]EnforcementStatus `json:"enforcement,omitempty"`

	// phase is the current lifecycle phase.
	Phase GatewayPhase `json:"phase,omitempty"`

	// totalTranslations is the cumulative number of translations created.
	// +optional
	TotalTranslations int64 `json:"totalTranslations,omitempty"`

	// translationCount is the number of active NAT translations.
	// +optional
	TranslationCount int64 `json:"translationCount,omitempty"`

	// activeNode is the node currently running the gateway.
	// +optional
	ActiveNode string `json:"activeNode,omitempty"`

	// conditions represent the current state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=natgw
// +kubebuilder:printcolumn:name="VPC",type=string,JSONPath=`.spec.vpcRef`
// +kubebuilder:printcolumn:name="ExternalIP",type=string,JSONPath=`.spec.externalIP`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Translations",type=integer,JSONPath=`.status.translationCount`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// NATGateway provides network address translation for private subnets.
type NATGateway struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NATGatewaySpec   `json:"spec,omitempty"`
	Status NATGatewayStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NATGatewayList contains a list of NATGateway.
type NATGatewayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NATGateway `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NATGateway{}, &NATGatewayList{})
}
