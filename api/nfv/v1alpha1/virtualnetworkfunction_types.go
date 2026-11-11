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

// VNFType defines the VNF implementation type.
// +kubebuilder:validation:Enum=Container;VM
type VNFType string

const (
	VNFTypeContainer VNFType = "Container"
	VNFTypeVM        VNFType = "VM"
)

// VNFInterfaceType defines the interface attachment mode.
// +kubebuilder:validation:Enum=OVSPort;SRIOV;Tap
type VNFInterfaceType string

const (
	VNFInterfaceTypeOVSPort VNFInterfaceType = "OVSPort"
	VNFInterfaceTypeSRIOV   VNFInterfaceType = "SRIOV"
	VNFInterfaceTypeTap     VNFInterfaceType = "Tap"
)

// VNFInterface defines a network interface on a VNF.
type VNFInterface struct {
	// name is the interface name inside the VNF.
	Name string `json:"name"`

	// type is the interface attachment mode.
	// +kubebuilder:default=OVSPort
	// +optional
	Type VNFInterfaceType `json:"type,omitempty"`

	// subnetRef attaches this interface to a Subnet. For every instance a VNic is
	// created in that subnet (IPAM address + MAC) and bound to the VNF's port.
	// +optional
	SubnetRef string `json:"subnetRef,omitempty"`
}

// VNFResources defines the compute resources for a VNF.
type VNFResources struct {
	// cpu is the CPU allocation (e.g., "2" or "500m").
	// +optional
	CPU string `json:"cpu,omitempty"`

	// memory is the memory allocation (e.g., "512Mi", "2Gi").
	// +optional
	Memory string `json:"memory,omitempty"`
}

// VMConfig defines VM-specific settings for VM-type VNFs.
type VMConfig struct {
	// image is the disk image path or URL.
	// +optional
	Image string `json:"image,omitempty"`

	// vcpus is the number of virtual CPUs.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	// +optional
	VCPUs int32 `json:"vcpus,omitempty"`

	// memoryMB is the VM memory in megabytes.
	// +kubebuilder:validation:Minimum=128
	// +kubebuilder:default=512
	// +optional
	MemoryMB int32 `json:"memoryMB,omitempty"`

	// packages are installed in the guest on first boot (cloud-init), e.g. suricata.
	// The guest needs internet access (Internet Gateway) for this.
	// +optional
	Packages []string `json:"packages,omitempty"`

	// commands run in the guest on first boot after packages are installed.
	// +optional
	Commands []string `json:"commands,omitempty"`
}

// VirtualNetworkFunctionSpec defines the desired state of VirtualNetworkFunction.
type VirtualNetworkFunctionSpec struct {
	// type is the VNF implementation type.
	// +kubebuilder:default=VM
	// +optional
	Type VNFType `json:"type,omitempty"`

	// image is the container image (for Container type).
	// +optional
	Image string `json:"image,omitempty"`

	// vmConfig is the VM configuration (for VM type).
	// +optional
	VMConfig *VMConfig `json:"vmConfig,omitempty"`

	// replicas is the desired number of VNF instances.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// interfaces are the network interfaces for this VNF.
	// +kubebuilder:validation:MinItems=1
	Interfaces []VNFInterface `json:"interfaces"`

	// resources defines compute resource limits.
	// +optional
	Resources *VNFResources `json:"resources,omitempty"`

	// nodeSelector targets specific nodes.
	// +optional
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`
}

// VNFPhase represents the lifecycle phase.
// +kubebuilder:validation:Enum=Pending;Creating;Running;Failed;Terminating
type VNFPhase string

const (
	VNFPhasePending     VNFPhase = "Pending"
	VNFPhaseCreating    VNFPhase = "Creating"
	VNFPhaseRunning     VNFPhase = "Running"
	VNFPhaseFailed      VNFPhase = "Failed"
	VNFPhaseTerminating VNFPhase = "Terminating"
)

// VNFInstanceStatus is the state of one VNF instance (one guest VM).
type VNFInstanceStatus struct {
	// name of the instance (<vnf>-<index>); also the guest VM name suffix.
	Name string `json:"name"`
	// node is the host running this instance.
	Node string `json:"node"`
	// ready is true once the guest is up and all its VNics are bound to OVS ports.
	Ready bool `json:"ready"`
	// message explains why the instance is not ready.
	// +optional
	Message string `json:"message,omitempty"`
}

// VirtualNetworkFunctionStatus defines the observed state of VirtualNetworkFunction.
type VirtualNetworkFunctionStatus struct {
	// nodes lists the hosts this resource is running on (Ready nodes matching
	// nodeSelector that are attached to the VPC).
	// +optional
	Nodes []string `json:"nodes,omitempty"`

	// phase is the current lifecycle phase.
	Phase VNFPhase `json:"phase,omitempty"`

	// instances lists every instance, its host and readiness.
	// +listType=map
	// +listMapKey=name
	// +optional
	Instances []VNFInstanceStatus `json:"instances,omitempty"`

	// readyReplicas is the number of running VNF instances.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// conditions represent the current state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=vnf
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.spec.replicas`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// VirtualNetworkFunction is a network function (firewall, IDS, NAT, etc.)
// running as a container or VM, wired into OVS via service chains.
type VirtualNetworkFunction struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualNetworkFunctionSpec   `json:"spec,omitempty"`
	Status VirtualNetworkFunctionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VirtualNetworkFunctionList contains a list of VirtualNetworkFunction.
type VirtualNetworkFunctionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualNetworkFunction `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VirtualNetworkFunction{}, &VirtualNetworkFunctionList{})
}
