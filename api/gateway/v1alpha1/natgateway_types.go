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
