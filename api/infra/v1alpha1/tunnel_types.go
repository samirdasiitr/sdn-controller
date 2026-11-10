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

// TunnelType defines the encapsulation protocol.
// +kubebuilder:validation:Enum=VXLAN;IPIP;GRE
type TunnelType string

const (
	TunnelTypeVXLAN TunnelType = "VXLAN"
	TunnelTypeIPIP  TunnelType = "IPIP"
	TunnelTypeGRE   TunnelType = "GRE"
)

// TunnelEndpoint defines one side of a tunnel.
type TunnelEndpoint struct {
	// nodeName is the Kubernetes node name.
	// +kubebuilder:validation:Required
	NodeName string `json:"nodeName"`

	// ip is the tunnel endpoint IP (underlay).
	// +kubebuilder:validation:Required
	IP string `json:"ip"`
}

// TunnelSpec defines the desired state of Tunnel.
type TunnelSpec struct {
	// type is the tunnel encapsulation type.
	// +kubebuilder:default=VXLAN
	// +optional
	Type TunnelType `json:"type,omitempty"`

	// vni is the VXLAN Network Identifier (only for VXLAN).
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=16777215
	// +optional
	VNI *int32 `json:"vni,omitempty"`

	// localEndpoint is this side of the tunnel.
	// +kubebuilder:validation:Required
	LocalEndpoint TunnelEndpoint `json:"localEndpoint"`

	// remoteEndpoint is the far side of the tunnel.
	// +kubebuilder:validation:Required
	RemoteEndpoint TunnelEndpoint `json:"remoteEndpoint"`

	// mtu is the tunnel interface MTU (should account for encap overhead).
	// +kubebuilder:validation:Minimum=1280
	// +kubebuilder:validation:Maximum=9216
	// +optional
	MTU *int32 `json:"mtu,omitempty"`

	// checksumEnabled enables outer header checksum.
	// +kubebuilder:default=true
	// +optional
	ChecksumEnabled bool `json:"checksumEnabled,omitempty"`

	// offload configures tunnel hardware offload.
	// +optional
	Offload *OffloadConfig `json:"offload,omitempty"`
}

// TunnelPhase represents the lifecycle phase.
// +kubebuilder:validation:Enum=Pending;Establishing;Established;Error
type TunnelPhase string

const (
	TunnelPhasePending      TunnelPhase = "Pending"
	TunnelPhaseEstablishing TunnelPhase = "Establishing"
	TunnelPhaseEstablished  TunnelPhase = "Established"
	TunnelPhaseError        TunnelPhase = "Error"
)

// TunnelStatus defines the observed state of Tunnel.
type TunnelStatus struct {
	// phase is the current lifecycle phase.
	Phase TunnelPhase `json:"phase,omitempty"`

	// txPackets is the total transmitted packet count.
	// +optional
	TxPackets int64 `json:"txPackets,omitempty"`

	// rxPackets is the total received packet count.
	// +optional
	RxPackets int64 `json:"rxPackets,omitempty"`

	// offloadActive indicates whether tunnel offload is active.
	// +optional
	OffloadActive bool `json:"offloadActive,omitempty"`

	// conditions represent the current state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=tun
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Local",type=string,JSONPath=`.spec.localEndpoint.nodeName`
// +kubebuilder:printcolumn:name="Remote",type=string,JSONPath=`.spec.remoteEndpoint.nodeName`
// +kubebuilder:printcolumn:name="VNI",type=integer,JSONPath=`.spec.vni`
// +kubebuilder:printcolumn:name="Offload",type=boolean,JSONPath=`.status.offloadActive`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// Tunnel is a point-to-point overlay tunnel (VXLAN, IPIP, or GRE) between two nodes.
type Tunnel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TunnelSpec   `json:"spec,omitempty"`
	Status TunnelStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TunnelList contains a list of Tunnel.
type TunnelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Tunnel `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Tunnel{}, &TunnelList{})
}
