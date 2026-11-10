# Project Directory Structure

```
sdncontroller/
|
+-- cmd/
|   +-- controller-manager/           # Main binary for all controllers
|   |   +-- main.go
|   +-- node-agent/                    # Node agent binary (DaemonSet)
|   |   +-- main.go
|   +-- sdnctl/                        # CLI tool for debugging/admin
|       +-- main.go
|
+-- api/                               # CRD type definitions (kubebuilder generated)
|   +-- vpc/
|   |   +-- v1alpha1/
|   |       +-- vpc_types.go
|   |       +-- subnet_types.go
|   |       +-- groupversion_info.go
|   |       +-- zz_generated.deepcopy.go
|   +-- gateway/
|   |   +-- v1alpha1/
|   |       +-- internetgateway_types.go
|   |       +-- natgateway_types.go
|   |       +-- groupversion_info.go
|   |       +-- zz_generated.deepcopy.go
|   +-- service/
|   |   +-- v1alpha1/
|   |       +-- loadbalancer_types.go
|   |       +-- firewall_types.go
|   |       +-- groupversion_info.go
|   |       +-- zz_generated.deepcopy.go
|   +-- infra/
|   |   +-- v1alpha1/
|   |       +-- networkinterface_types.go
|   |       +-- bond_types.go
|   |       +-- tunnel_types.go
|   |       +-- nodenetworkconfig_types.go
|   |       +-- groupversion_info.go
|   |       +-- zz_generated.deepcopy.go
|   +-- nfv/
|       +-- v1alpha1/
|           +-- servicechain_types.go
|           +-- virtualnetworkfunction_types.go
|           +-- groupversion_info.go
|           +-- zz_generated.deepcopy.go
|
+-- internal/
|   +-- controller/                    # Reconciliation logic
|   |   +-- vpc/
|   |   |   +-- vpc_controller.go
|   |   |   +-- vpc_controller_test.go
|   |   |   +-- subnet_controller.go
|   |   |   +-- subnet_controller_test.go
|   |   +-- gateway/
|   |   |   +-- internetgateway_controller.go
|   |   |   +-- natgateway_controller.go
|   |   +-- service/
|   |   |   +-- loadbalancer_controller.go
|   |   |   +-- firewall_controller.go
|   |   +-- infra/
|   |   |   +-- networkinterface_controller.go
|   |   |   +-- bond_controller.go
|   |   |   +-- tunnel_controller.go
|   |   |   +-- nodenetworkconfig_controller.go
|   |   +-- nfv/
|   |       +-- servicechain_controller.go
|   |       +-- virtualnetworkfunction_controller.go
|   |
|   +-- agent/                         # Node agent implementation
|   |   +-- agent.go                   # Main agent lifecycle
|   |   +-- config.go                  # Agent configuration
|   |   +-- ovs/
|   |   |   +-- manager.go            # OVS bridge/port/flow manager
|   |   |   +-- bridge.go
|   |   |   +-- flow.go
|   |   |   +-- port.go
|   |   |   +-- ovsdb_client.go       # OVSDB protocol client
|   |   |   +-- openflow_client.go    # OpenFlow protocol client
|   |   +-- tunnel/
|   |   |   +-- manager.go            # Tunnel lifecycle
|   |   |   +-- vxlan.go
|   |   |   +-- ipip.go
|   |   |   +-- health.go             # Tunnel health monitoring
|   |   +-- offload/
|   |   |   +-- manager.go            # Offload orchestration
|   |   |   +-- detection.go          # Hardware detection
|   |   |   +-- mellanox.go           # Mellanox-specific offload
|   |   |   +-- switchdev.go          # Switchdev mode management
|   |   |   +-- tc_flower.go          # TC flower rule programming
|   |   |   +-- vf.go                 # VF management
|   |   +-- bond/
|   |   |   +-- manager.go            # Bond configuration
|   |   |   +-- linux_bond.go         # Linux bonding driver
|   |   |   +-- ovs_bond.go           # OVS bond
|   |   +-- nfv/
|   |   |   +-- chain.go              # Service chain flow programming
|   |   |   +-- port_pair.go          # Port pair management
|   |   +-- libvirt/
|   |   |   +-- manager.go            # Libvirt integration
|   |   |   +-- network.go            # VM network interface management
|   |   |   +-- sriov.go              # SR-IOV passthrough
|   |   +-- dpu/
|   |   |   +-- manager.go            # DPU communication
|   |   |   +-- bluefield.go          # BlueField-specific code
|   |   +-- mtu/
|   |   |   +-- calculator.go         # MTU chain calculation
|   |   |   +-- validator.go          # MTU consistency validation
|   |   +-- telemetry/
|   |       +-- metrics.go            # OTel metrics
|   |       +-- collector.go          # Flow stats collection
|   |
|   +-- ipam/                          # IP Address Management
|   |   +-- allocator.go              # CIDR/IP allocation
|   |   +-- pool.go                   # IP pool management
|   |   +-- allocator_test.go
|   |
|   +-- webhook/                       # Admission webhooks
|   |   +-- vpc_webhook.go
|   |   +-- subnet_webhook.go
|   |   +-- firewall_webhook.go
|   |   +-- validation.go             # Shared validation logic
|   |
|   +-- telemetry/                     # Shared telemetry setup
|       +-- otel.go                    # OpenTelemetry initialization
|       +-- metrics.go                 # Metric definitions
|       +-- tracing.go                 # Trace setup
|       +-- logging.go                 # Zap logger with OTel integration
|
+-- pkg/                               # Public libraries (reusable)
|   +-- netutil/                       # Network utilities
|   |   +-- cidr.go                    # CIDR math
|   |   +-- mtu.go                     # MTU calculations
|   |   +-- vni.go                     # VNI allocation
|   +-- ovs/                           # OVS client library
|   |   +-- ovsdb.go
|   |   +-- openflow.go
|   +-- netlink/                       # Netlink wrapper
|   |   +-- link.go
|   |   +-- route.go
|   |   +-- tc.go                      # TC flower
|   +-- hw/                            # Hardware abstraction
|       +-- detect.go                  # NIC detection
|       +-- mellanox.go                # Mellanox utilities
|       +-- dpu.go                     # DPU utilities
|
+-- config/                            # Kubernetes manifests (kubebuilder)
|   +-- crd/
|   |   +-- bases/                     # Generated CRD YAML
|   |   +-- patches/                   # CRD patches (webhook, cainjection)
|   +-- manager/
|   |   +-- manager.yaml               # Controller manager deployment
|   +-- agent/
|   |   +-- daemonset.yaml             # Node agent DaemonSet
|   +-- rbac/
|   |   +-- role.yaml
|   |   +-- role_binding.yaml
|   |   +-- service_account.yaml
|   +-- webhook/
|   |   +-- manifests.yaml
|   +-- samples/                       # Example CRD instances
|   |   +-- vpc_sample.yaml
|   |   +-- subnet_sample.yaml
|   |   +-- internetgateway_sample.yaml
|   |   +-- natgateway_sample.yaml
|   |   +-- loadbalancer_sample.yaml
|   |   +-- firewall_sample.yaml
|   |   +-- bond_sample.yaml
|   |   +-- tunnel_sample.yaml
|   |   +-- nodenetworkconfig_sample.yaml
|   |   +-- servicechain_sample.yaml
|   +-- default/
|       +-- kustomization.yaml
|
+-- deploy/                            # Deployment artifacts
|   +-- helm/
|   |   +-- sdncontroller/
|   |       +-- Chart.yaml
|   |       +-- values.yaml
|   |       +-- templates/
|   |           +-- controller-deployment.yaml
|   |           +-- agent-daemonset.yaml
|   |           +-- crds/
|   |           +-- rbac/
|   +-- kustomize/
|       +-- base/
|       +-- overlays/
|           +-- development/
|           +-- production/
|
+-- hack/                              # Development scripts
|   +-- setup-dev-env.sh               # Dev environment setup
|   +-- generate-crds.sh               # CRD generation
|   +-- e2e-test.sh                    # E2E test runner
|
+-- test/
|   +-- e2e/                           # End-to-end tests
|   |   +-- vpc_test.go
|   |   +-- subnet_test.go
|   |   +-- gateway_test.go
|   |   +-- loadbalancer_test.go
|   |   +-- firewall_test.go
|   +-- integration/                   # Integration tests (with OVS)
|       +-- ovs_test.go
|       +-- tunnel_test.go
|       +-- offload_test.go
|
+-- docs/
|   +-- architecture/
|   |   +-- PLAN.md                    # This document
|   |   +-- PROJECT_STRUCTURE.md       # This file
|   +-- user-guide/
|   +-- api-reference/
|
+-- proto/                             # Protobuf definitions (agent gRPC)
|   +-- agent/
|       +-- v1/
|           +-- agent.proto
|
+-- Dockerfile.controller              # Controller manager image
+-- Dockerfile.agent                   # Node agent image
+-- Makefile                           # Build, test, deploy targets
+-- go.mod
+-- go.sum
+-- PROJECT                            # Kubebuilder project file
```
