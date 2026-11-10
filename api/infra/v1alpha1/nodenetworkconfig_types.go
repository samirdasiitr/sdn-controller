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

// NodeType defines the compute host type.
// +kubebuilder:validation:Enum=BareMetal;VM
type NodeType string

const (
	NodeTypeBareMetal NodeType = "BareMetal"
	NodeTypeVM        NodeType = "VM"
)

// ComputeType defines the compute workload type.
// +kubebuilder:validation:Enum=CPU;GPU
type ComputeType string

const (
	ComputeTypeCPU ComputeType = "CPU"
	ComputeTypeGPU ComputeType = "GPU"
)

// BridgeType defines the OVS bridge role.
// +kubebuilder:validation:Enum=integration;external;tunnel
type BridgeType string

const (
	BridgeTypeIntegration BridgeType = "integration"
	BridgeTypeExternal    BridgeType = "external"
	BridgeTypeTunnel      BridgeType = "tunnel"
)

// DatapathType defines the OVS datapath type.
// +kubebuilder:validation:Enum=system;netdev
type DatapathType string

const (
	DatapathTypeSystem DatapathType = "system"
	DatapathTypeNetdev DatapathType = "netdev"
)

// OVSBridge defines an OVS bridge configuration.
type OVSBridge struct {
	// name is the bridge name.
	Name string `json:"name"`

	// type is the bridge role.
	Type BridgeType `json:"type"`

	// datapathType is the OVS datapath type.
	// +kubebuilder:default=system
	// +optional
	DatapathType DatapathType `json:"datapathType,omitempty"`

	// uplinks are physical interfaces attached to this bridge.
	// +optional
	Uplinks []string `json:"uplinks,omitempty"`
}

// NICInfo describes a discovered NIC on the node.
type NICInfo struct {
	// device is the Linux device name.
	Device string `json:"device"`

	// vendor is the hardware vendor.
	// +optional
	Vendor string `json:"vendor,omitempty"`

	// model is the hardware model.
	// +optional
	Model string `json:"model,omitempty"`

	// offloadCapable indicates hardware offload support.
	// +optional
	OffloadCapable bool `json:"offloadCapable,omitempty"`

	// switchdevMode indicates if switchdev is enabled.
	// +optional
	SwitchdevMode bool `json:"switchdevMode,omitempty"`

	// numVFs is the number of virtual functions configured.
	// +optional
	NumVFs int32 `json:"numVFs,omitempty"`
}

// HardwareInfo describes the discovered hardware on a node.
type HardwareInfo struct {
	// nics lists all discovered NICs.
	// +optional
	NICs []NICInfo `json:"nics,omitempty"`
}

// LibvirtConfig defines libvirt integration settings.
type LibvirtConfig struct {
	// enabled activates libvirt integration.
	Enabled bool `json:"enabled"`

	// bridgeMapping maps network roles to OVS bridges.
	// +optional
	BridgeMapping map[string]string `json:"bridgeMapping,omitempty"`
}

// NodeNetworkConfigSpec defines the desired state of NodeNetworkConfig.
type NodeNetworkConfigSpec struct {
	// nodeName is the Kubernetes node name.
	// +kubebuilder:validation:Required
	NodeName string `json:"nodeName"`

	// nodeType is the host type.
	// +kubebuilder:default=BareMetal
	// +optional
	NodeType NodeType `json:"nodeType,omitempty"`

	// computeType is the compute workload type.
	// +kubebuilder:default=CPU
	// +optional
	ComputeType ComputeType `json:"computeType,omitempty"`

	// dpuEnabled indicates DPU offload support.
	// +kubebuilder:default=false
	// +optional
	DPUEnabled bool `json:"dpuEnabled,omitempty"`

	// dpuDevice is the DPU model (e.g., bf2, bf3).
	// +optional
	DPUDevice string `json:"dpuDevice,omitempty"`

	// ovsBridges defines the OVS bridges to create on this node.
	// +optional
	OVSBridges []OVSBridge `json:"ovsBridges,omitempty"`

	// defaultMTU is the default MTU for this node.
	// +kubebuilder:validation:Minimum=1280
	// +kubebuilder:validation:Maximum=9216
	// +kubebuilder:default=1500
	// +optional
	DefaultMTU int32 `json:"defaultMTU,omitempty"`

	// tunnelProtocol is the default tunnel encapsulation.
	// +kubebuilder:default=VXLAN
	// +optional
	TunnelProtocol TunnelType `json:"tunnelProtocol,omitempty"`

	// tunnelSourceIP is the underlay IP used as tunnel source.
	// +optional
	TunnelSourceIP string `json:"tunnelSourceIP,omitempty"`

	// hardware describes the physical NIC configuration.
	// +optional
	Hardware *HardwareInfo `json:"hardware,omitempty"`

	// libvirt configures libvirt integration for VM hosts.
	// +optional
	Libvirt *LibvirtConfig `json:"libvirt,omitempty"`
}

// NodeNetworkPhase represents the lifecycle phase.
// +kubebuilder:validation:Enum=Pending;Configured;Degraded;Error
type NodeNetworkPhase string

const (
	NodeNetworkPhasePending    NodeNetworkPhase = "Pending"
	NodeNetworkPhaseConfigured NodeNetworkPhase = "Configured"
	NodeNetworkPhaseDegraded   NodeNetworkPhase = "Degraded"
	NodeNetworkPhaseError      NodeNetworkPhase = "Error"
)

// NodeNetworkConfigStatus defines the observed state of NodeNetworkConfig.
type NodeNetworkConfigStatus struct {
	// phase is the current lifecycle phase.
	Phase NodeNetworkPhase `json:"phase,omitempty"`

	// ovsVersion is the installed OVS version.
	// +optional
	OVSVersion string `json:"ovsVersion,omitempty"`

	// agentVersion is the running node agent version.
	// +optional
	AgentVersion string `json:"agentVersion,omitempty"`

	// tunnelsEstablished is the number of active tunnels from this node.
	// +optional
	TunnelsEstablished int32 `json:"tunnelsEstablished,omitempty"`

	// offloadStatus summarizes the hardware offload state.
	// +optional
	OffloadStatus string `json:"offloadStatus,omitempty"`

	// conditions represent the current state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=nnc
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.spec.nodeName`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.nodeType`
// +kubebuilder:printcolumn:name="Compute",type=string,JSONPath=`.spec.computeType`
// +kubebuilder:printcolumn:name="DPU",type=boolean,JSONPath=`.spec.dpuEnabled`
// +kubebuilder:printcolumn:name="MTU",type=integer,JSONPath=`.spec.defaultMTU`
// +kubebuilder:printcolumn:name="Tunnels",type=integer,JSONPath=`.status.tunnelsEstablished`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// NodeNetworkConfig describes the complete network configuration for a single node.
type NodeNetworkConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeNetworkConfigSpec   `json:"spec,omitempty"`
	Status NodeNetworkConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NodeNetworkConfigList contains a list of NodeNetworkConfig.
type NodeNetworkConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeNetworkConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeNetworkConfig{}, &NodeNetworkConfigList{})
}
