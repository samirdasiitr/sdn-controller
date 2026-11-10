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

// FirewallAction defines what to do with matched traffic.
// +kubebuilder:validation:Enum=Allow;Deny;Log
type FirewallAction string

const (
	FirewallActionAllow FirewallAction = "Allow"
	FirewallActionDeny  FirewallAction = "Deny"
	FirewallActionLog   FirewallAction = "Log"
)

// FirewallDirection defines the traffic direction.
// +kubebuilder:validation:Enum=Ingress;Egress
type FirewallDirection string

const (
	FirewallDirectionIngress FirewallDirection = "Ingress"
	FirewallDirectionEgress  FirewallDirection = "Egress"
)

// FirewallRuleMatch defines packet matching criteria.
type FirewallRuleMatch struct {
	// protocol is the L4 protocol to match.
	// +kubebuilder:validation:Enum=TCP;UDP;ICMP;SCTP;Any
	// +optional
	Protocol string `json:"protocol,omitempty"`

	// srcCIDR is the source network to match.
	// +optional
	SrcCIDR string `json:"srcCIDR,omitempty"`

	// dstCIDR is the destination network to match.
	// +optional
	DstCIDR string `json:"dstCIDR,omitempty"`

	// srcPort is the source port or range (e.g., "80" or "1024-65535").
	// +optional
	SrcPort string `json:"srcPort,omitempty"`

	// dstPort is the destination port or range.
	// +optional
	DstPort string `json:"dstPort,omitempty"`
}

// FirewallRule defines a single firewall rule.
type FirewallRule struct {
	// name is the human-readable rule name.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// priority determines evaluation order (lower = higher priority).
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Priority int32 `json:"priority"`

	// direction is the traffic direction.
	Direction FirewallDirection `json:"direction"`

	// action is what to do with matched traffic.
	Action FirewallAction `json:"action"`

	// match defines packet matching criteria.
	Match FirewallRuleMatch `json:"match"`

	// logging enables per-rule packet logging.
	// +optional
	Logging bool `json:"logging,omitempty"`
}

// FirewallSpec defines the desired state of Firewall.
type FirewallSpec struct {
	// vpcRef is the name of the VPC.
	// +kubebuilder:validation:Required
	VPCRef string `json:"vpcRef"`

	// subnetRefs are the subnets this firewall protects.
	// +optional
	SubnetRefs []string `json:"subnetRefs,omitempty"`

	// defaultAction is applied when no rules match.
	// +kubebuilder:default=Deny
	// +optional
	DefaultAction FirewallAction `json:"defaultAction,omitempty"`

	// rules are ordered firewall rules evaluated by priority.
	// +optional
	Rules []FirewallRule `json:"rules,omitempty"`

	// stateful enables connection tracking so return traffic is auto-allowed.
	// +kubebuilder:default=true
	// +optional
	Stateful bool `json:"stateful,omitempty"`

	// nodeSelector targets specific nodes.
	// +optional
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`
}

// FirewallPhase represents the lifecycle phase.
// +kubebuilder:validation:Enum=Pending;Active;Failed
type FirewallPhase string

const (
	FirewallPhasePending FirewallPhase = "Pending"
	FirewallPhaseActive  FirewallPhase = "Active"
	FirewallPhaseFailed  FirewallPhase = "Failed"
)

// FirewallStatus defines the observed state of Firewall.
type FirewallStatus struct {
	// phase is the current lifecycle phase.
	Phase FirewallPhase `json:"phase,omitempty"`

	// activeRules is the number of installed rules.
	// +optional
	ActiveRules int32 `json:"activeRules,omitempty"`

	// matchedPackets is the total matched packet count.
	// +optional
	MatchedPackets int64 `json:"matchedPackets,omitempty"`

	// conditions represent the current state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=fw
// +kubebuilder:printcolumn:name="VPC",type=string,JSONPath=`.spec.vpcRef`
// +kubebuilder:printcolumn:name="Default",type=string,JSONPath=`.spec.defaultAction`
// +kubebuilder:printcolumn:name="Rules",type=integer,JSONPath=`.status.activeRules`
// +kubebuilder:printcolumn:name="Stateful",type=boolean,JSONPath=`.spec.stateful`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Firewall provides stateful packet filtering for VPC subnets.
type Firewall struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FirewallSpec   `json:"spec,omitempty"`
	Status FirewallStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FirewallList contains a list of Firewall.
type FirewallList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Firewall `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Firewall{}, &FirewallList{})
}
