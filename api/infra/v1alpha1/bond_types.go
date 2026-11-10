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

// BondMode defines the bonding mode.
// +kubebuilder:validation:Enum="balance-rr";"active-backup";"802.3ad";"balance-xor";"balance-tlb";"balance-alb"
type BondMode string

const (
	BondModeBalanceRR     BondMode = "balance-rr"
	BondModeActiveBackup  BondMode = "active-backup"
	BondMode8023AD        BondMode = "802.3ad"
	BondModeBalanceXOR    BondMode = "balance-xor"
	BondModeBalanceTLB    BondMode = "balance-tlb"
	BondModeBalanceALB    BondMode = "balance-alb"
)

// LACPRate defines the LACP negotiation rate.
// +kubebuilder:validation:Enum=slow;fast
type LACPRate string

const (
	LACPRateSlow LACPRate = "slow"
	LACPRateFast LACPRate = "fast"
)

// HashPolicy defines the transmit hash policy.
// +kubebuilder:validation:Enum=layer2;"layer2+3";"layer3+4"
type HashPolicy string

const (
	HashPolicyLayer2   HashPolicy = "layer2"
	HashPolicyLayer2_3 HashPolicy = "layer2+3"
	HashPolicyLayer3_4 HashPolicy = "layer3+4"
)

// BondSpec defines the desired state of Bond.
type BondSpec struct {
	// nodeName is the node this bond belongs to.
	// +kubebuilder:validation:Required
	NodeName string `json:"nodeName"`

	// mode is the bonding mode.
	// +kubebuilder:default="802.3ad"
	// +optional
	Mode BondMode `json:"mode,omitempty"`

	// interfaces are the member interface device names.
	// +kubebuilder:validation:MinItems=2
	Interfaces []string `json:"interfaces"`

	// mtu is the bond MTU.
	// +kubebuilder:validation:Minimum=1280
	// +kubebuilder:validation:Maximum=9216
	// +kubebuilder:default=1500
	// +optional
	MTU int32 `json:"mtu,omitempty"`

	// lacpRate is the LACP negotiation rate (only for 802.3ad).
	// +kubebuilder:default=fast
	// +optional
	LACPRate LACPRate `json:"lacpRate,omitempty"`

	// hashPolicy is the transmit hash policy.
	// +kubebuilder:default="layer3+4"
	// +optional
	HashPolicy HashPolicy `json:"hashPolicy,omitempty"`

	// addresses are IP addresses assigned to the bond.
	// +optional
	Addresses []string `json:"addresses,omitempty"`

	// subnetRef links this bond to a Subnet.
	// +optional
	SubnetRef string `json:"subnetRef,omitempty"`
}

// BondPhase represents the lifecycle phase.
// +kubebuilder:validation:Enum=Pending;Configured;Degraded;Error
type BondPhase string

const (
	BondPhasePending    BondPhase = "Pending"
	BondPhaseConfigured BondPhase = "Configured"
	BondPhaseDegraded   BondPhase = "Degraded"
	BondPhaseError      BondPhase = "Error"
)

// BondStatus defines the observed state of Bond.
type BondStatus struct {
	// phase is the current lifecycle phase.
	Phase BondPhase `json:"phase,omitempty"`

	// activeInterfaces is the number of active member interfaces.
	// +optional
	ActiveInterfaces int32 `json:"activeInterfaces,omitempty"`

	// speed is the aggregated link speed.
	// +optional
	Speed string `json:"speed,omitempty"`

	// conditions represent the current state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=bond
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.spec.nodeName`
// +kubebuilder:printcolumn:name="Mode",type=string,JSONPath=`.spec.mode`
// +kubebuilder:printcolumn:name="MTU",type=integer,JSONPath=`.spec.mtu`
// +kubebuilder:printcolumn:name="Active",type=integer,JSONPath=`.status.activeInterfaces`
// +kubebuilder:printcolumn:name="Speed",type=string,JSONPath=`.status.speed`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// Bond aggregates multiple NICs into a single logical interface for redundancy and throughput.
type Bond struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BondSpec   `json:"spec,omitempty"`
	Status BondStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BondList contains a list of Bond.
type BondList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Bond `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Bond{}, &BondList{})
}
