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

// LBType defines the load balancer layer.
// +kubebuilder:validation:Enum=L4;L7
type LBType string

const (
	LBTypeL4 LBType = "L4"
	LBTypeL7 LBType = "L7"
)

// LBAlgorithm defines the load balancing algorithm.
// +kubebuilder:validation:Enum=RoundRobin;LeastConn;IPHash;Maglev
type LBAlgorithm string

const (
	LBAlgorithmRoundRobin LBAlgorithm = "RoundRobin"
	LBAlgorithmLeastConn  LBAlgorithm = "LeastConn"
	LBAlgorithmIPHash     LBAlgorithm = "IPHash"
	LBAlgorithmMaglev     LBAlgorithm = "Maglev"
)

// LBFrontend defines the virtual IP and ports exposed by the LB.
type LBFrontend struct {
	// ip is the virtual IP address.
	// +kubebuilder:validation:Required
	IP string `json:"ip"`

	// ports are the frontend listener ports.
	// +kubebuilder:validation:MinItems=1
	Ports []LBPort `json:"ports"`
}

// LBPort defines a single port mapping.
type LBPort struct {
	// protocol is the L4 protocol.
	// +kubebuilder:validation:Enum=TCP;UDP;SCTP
	Protocol string `json:"protocol"`

	// port is the frontend port.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`

	// targetPort is the backend port.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	TargetPort int32 `json:"targetPort"`
}

// LBBackend defines a backend server.
type LBBackend struct {
	// ip is the backend server IP.
	IP string `json:"ip"`

	// weight is the relative weight for weighted algorithms.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1000
	// +kubebuilder:default=100
	// +optional
	Weight int32 `json:"weight,omitempty"`
}

// HealthCheck defines health check parameters for backends.
type HealthCheck struct {
	// protocol is the health check protocol.
	// +kubebuilder:validation:Enum=TCP;HTTP;HTTPS
	Protocol string `json:"protocol"`

	// port is the health check port.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`

	// path is the HTTP health check path (only for HTTP/HTTPS).
	// +optional
	Path string `json:"path,omitempty"`

	// interval is the time between health checks.
	// +kubebuilder:default="10s"
	// +optional
	Interval metav1.Duration `json:"interval,omitempty"`

	// timeout is the health check timeout.
	// +kubebuilder:default="5s"
	// +optional
	Timeout metav1.Duration `json:"timeout,omitempty"`

	// unhealthyThreshold is the number of consecutive failures to mark unhealthy.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=3
	// +optional
	UnhealthyThreshold int32 `json:"unhealthyThreshold,omitempty"`
}

// SessionAffinity configures session persistence.
type SessionAffinity struct {
	// enabled activates session affinity.
	Enabled bool `json:"enabled"`

	// timeout is the session sticky timeout.
	// +kubebuilder:default="3600s"
	// +optional
	Timeout metav1.Duration `json:"timeout,omitempty"`
}

// LoadBalancerSpec defines the desired state of LoadBalancer.
type LoadBalancerSpec struct {
	// vpcRef is the name of the VPC.
	// +kubebuilder:validation:Required
	VPCRef string `json:"vpcRef"`

	// type is the LB layer (L4 or L7).
	// +kubebuilder:default=L4
	// +optional
	Type LBType `json:"type,omitempty"`

	// algorithm is the load balancing algorithm.
	// +kubebuilder:default=RoundRobin
	// +optional
	Algorithm LBAlgorithm `json:"algorithm,omitempty"`

	// frontend defines the VIP and listener ports.
	// +kubebuilder:validation:Required
	Frontend LBFrontend `json:"frontend"`

	// backends are the backend server pool.
	// +kubebuilder:validation:MinItems=1
	Backends []LBBackend `json:"backends"`

	// healthCheck configures backend health monitoring.
	// +optional
	HealthCheck *HealthCheck `json:"healthCheck,omitempty"`

	// sessionAffinity configures session persistence.
	// +optional
	SessionAffinity *SessionAffinity `json:"sessionAffinity,omitempty"`

	// nodeSelector targets specific nodes.
	// +optional
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`
}

// LBPhase represents the lifecycle phase.
// +kubebuilder:validation:Enum=Pending;Provisioning;Ready;Degraded;Failed
type LBPhase string

const (
	LBPhasePending      LBPhase = "Pending"
	LBPhaseProvisioning LBPhase = "Provisioning"
	LBPhaseReady        LBPhase = "Ready"
	LBPhaseDegraded     LBPhase = "Degraded"
	LBPhaseFailed       LBPhase = "Failed"
)

// BackendStatus is the observed health of one backend, written by the router node's prober.
type BackendStatus struct {
	// ip of the backend.
	IP string `json:"ip"`
	// healthy is false after unhealthyThreshold consecutive failed checks.
	Healthy bool `json:"healthy"`
	// mac is the backend's MAC when it is not a VNic (learned from the router node's ARP cache).
	// +optional
	MAC string `json:"mac,omitempty"`
	// message explains an unhealthy backend.
	// +optional
	Message string `json:"message,omitempty"`
}

// LoadBalancerStatus defines the observed state of LoadBalancer.
type LoadBalancerStatus struct {
	// enforcement is what each node agent actually programmed, keyed by node name.
	// +optional
	Enforcement map[string]EnforcementStatus `json:"enforcement,omitempty"`

	// nodes lists the hosts this resource is running on (Ready nodes matching
	// nodeSelector that are attached to the VPC).
	// +optional
	Nodes []string `json:"nodes,omitempty"`

	// phase is the current lifecycle phase.
	Phase LBPhase `json:"phase,omitempty"`

	// activeBackends is the number of healthy backends.
	// +optional
	ActiveBackends int32 `json:"activeBackends,omitempty"`

	// backends is the health of each backend.
	// +listType=map
	// +listMapKey=ip
	// +optional
	Backends []BackendStatus `json:"backends,omitempty"`

	// activeConnections is the number of connections currently tracked, summed over nodes.
	// +optional
	ActiveConnections int64 `json:"activeConnections,omitempty"`

	// totalConnections is the cumulative number of connections load-balanced (all nodes).
	// +optional
	TotalConnections int64 `json:"totalConnections,omitempty"`

	// conditions represent the current state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=lb
// +kubebuilder:printcolumn:name="VPC",type=string,JSONPath=`.spec.vpcRef`
// +kubebuilder:printcolumn:name="VIP",type=string,JSONPath=`.spec.frontend.ip`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Algorithm",type=string,JSONPath=`.spec.algorithm`
// +kubebuilder:printcolumn:name="Backends",type=integer,JSONPath=`.status.activeBackends`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// LoadBalancer distributes traffic across backend servers using configurable algorithms.
type LoadBalancer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LoadBalancerSpec   `json:"spec,omitempty"`
	Status LoadBalancerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// LoadBalancerList contains a list of LoadBalancer.
type LoadBalancerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LoadBalancer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LoadBalancer{}, &LoadBalancerList{})
}
