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
