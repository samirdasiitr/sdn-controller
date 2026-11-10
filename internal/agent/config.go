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
