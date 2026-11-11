/*
Copyright (c) 2026 Samir Das. All rights reserved.

PROPRIETARY AND CONFIDENTIAL. No license is granted. Any use of this software, and in
particular production or commercial use, requires the prior explicit written permission of
Samir Das <samiruor@gmail.com>. See the LICENSE file.
*/

package agent

// Config holds the configuration for the node agent.
type Config struct {
	// NodeName is the Kubernetes node name this agent runs on.
	NodeName string

	// KubeConfig is the path to a kubeconfig file. If empty, in-cluster config is used.
	KubeConfig string

	// OTLPEndpoint is the OpenTelemetry collector endpoint (e.g., "otel-collector:4317").
	OTLPEndpoint string

	// OVSSocket is the path to the OVS database socket.
	OVSSocket string
}
