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

// HAMode defines the high-availability mode.
// +kubebuilder:validation:Enum=ActiveStandby;ActiveActive
type HAMode string

const (
	HAModeActiveStandby HAMode = "ActiveStandby"
	HAModeActiveActive  HAMode = "ActiveActive"
)

// HAConfig defines HA settings for gateways.
type HAConfig struct {
	// enabled activates HA for this gateway.
	// +kubebuilder:default=false
	Enabled bool `json:"enabled"`

	// mode is the HA mode.
	// +kubebuilder:default=ActiveStandby
	// +optional
	Mode HAMode `json:"mode,omitempty"`
}

// InternetGatewaySpec defines the desired state of InternetGateway.
type InternetGatewaySpec struct {
	// vpcRef is the name of the VPC this gateway serves.
	// +kubebuilder:validation:Required
	VPCRef string `json:"vpcRef"`

	// externalInterface is the physical uplink interface name.
	// +kubebuilder:validation:Required
	ExternalInterface string `json:"externalInterface"`

	// externalGateway is the upstream router reachable on externalInterface. The node agent
	// enslaves externalInterface into an OVS bridge, configures externalIPs[0] on it and
	// forwards VPC traffic to this router (SNAT is done in OVS conntrack).
	// +optional
	ExternalGateway string `json:"externalGateway,omitempty"`

	// externalPrefixLength is the prefix length of externalIPs[0] on the uplink network.
	// +kubebuilder:default=24
	// +kubebuilder:validation:Minimum=8
	// +kubebuilder:validation:Maximum=30
	// +optional
	ExternalPrefixLength int32 `json:"externalPrefixLength,omitempty"`

	// externalIPs are the public IP addresses on this gateway.
	// +kubebuilder:validation:MinItems=1
	ExternalIPs []string `json:"externalIPs"`

	// ha configures high availability.
	// +optional
	HA *HAConfig `json:"ha,omitempty"`

	// nodeSelector targets nodes that can run this gateway.
	// +optional
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`
}

// GatewayPhase represents the lifecycle phase of a gateway.
// +kubebuilder:validation:Enum=Pending;Provisioning;Ready;Degraded;Failed
type GatewayPhase string

const (
	GatewayPhasePending      GatewayPhase = "Pending"
	GatewayPhaseProvisioning GatewayPhase = "Provisioning"
	GatewayPhaseReady        GatewayPhase = "Ready"
	GatewayPhaseDegraded     GatewayPhase = "Degraded"
	GatewayPhaseFailed       GatewayPhase = "Failed"
)

// InternetGatewayStatus defines the observed state of InternetGateway.
type InternetGatewayStatus struct {
	// phase is the current lifecycle phase.
	Phase GatewayPhase `json:"phase,omitempty"`

	// activeNode is the node currently running the gateway.
	// +optional
	ActiveNode string `json:"activeNode,omitempty"`

	// standbyNode is the standby node in HA mode.
	// +optional
	StandbyNode string `json:"standbyNode,omitempty"`

	// conditions represent the current state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=igw
// +kubebuilder:printcolumn:name="VPC",type=string,JSONPath=`.spec.vpcRef`
// +kubebuilder:printcolumn:name="ExternalIPs",type=string,JSONPath=`.spec.externalIPs`
// +kubebuilder:printcolumn:name="Active",type=string,JSONPath=`.status.activeNode`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// InternetGateway provides north-south internet connectivity for a VPC.
type InternetGateway struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InternetGatewaySpec   `json:"spec,omitempty"`
	Status InternetGatewayStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// InternetGatewayList contains a list of InternetGateway.
type InternetGatewayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InternetGateway `json:"items"`
}

func init() {
	SchemeBuilder.Register(&InternetGateway{}, &InternetGatewayList{})
}
