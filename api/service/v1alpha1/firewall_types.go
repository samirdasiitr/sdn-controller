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

	// srcSubnetRef matches traffic from a Subnet of the VPC by name (compiled to its CIDR, which
	// OVS matches natively as a masked prefix). Mutually exclusive with srcCIDR.
	// +optional
	SrcSubnetRef string `json:"srcSubnetRef,omitempty"`

	// dstSubnetRef matches traffic to a Subnet of the VPC by name. Mutually exclusive with dstCIDR.
	// +optional
	DstSubnetRef string `json:"dstSubnetRef,omitempty"`

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

// RuleCounter is the cumulative traffic matched by one firewall rule.
type RuleCounter struct {
	Name    string `json:"name"`
	Action  string `json:"action"`
	Packets int64  `json:"packets"`
	Bytes   int64  `json:"bytes"`
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
	// matchedPackets is the packet count of this object's rules on the node (firewalls).
	// +optional
	MatchedPackets int64 `json:"matchedPackets,omitempty"`
	// connections is the number of connections currently tracked on the node (load balancers).
	// +optional
	Connections int64 `json:"connections,omitempty"`
	// newConnections is the cumulative number of connections load-balanced on the node.
	// +optional
	NewConnections int64 `json:"newConnections,omitempty"`
	// packets and bytes are cumulative counters of the object's main flows on the node.
	// +optional
	Packets int64 `json:"packets,omitempty"`
	// +optional
	Bytes int64 `json:"bytes,omitempty"`
	// rules are cumulative per-rule counters on the node (firewalls).
	// +optional
	Rules []RuleCounter `json:"rules,omitempty"`
	// message carries the error when programming failed.
	// +optional
	Message string `json:"message,omitempty"`
	// warnings are non-fatal limitations (e.g. health checks not implemented).
	// +optional
	Warnings []string `json:"warnings,omitempty"`
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
	// enforcement is what each node agent actually programmed, keyed by node name.
	// +optional
	Enforcement map[string]EnforcementStatus `json:"enforcement,omitempty"`

	// nodes lists the hosts this resource is running on (Ready nodes matching
	// nodeSelector that are attached to the VPC).
	// +optional
	Nodes []string `json:"nodes,omitempty"`

	// phase is the current lifecycle phase.
	Phase FirewallPhase `json:"phase,omitempty"`

	// activeRules is the number of installed rules.
	// +optional
	ActiveRules int32 `json:"activeRules,omitempty"`

	// ruleCounters are the per-rule counters summed over all nodes.
	// +listType=map
	// +listMapKey=name
	// +optional
	RuleCounters []RuleCounter `json:"ruleCounters,omitempty"`

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
