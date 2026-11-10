# SDN Controller - Project Reference

## Quick Start
```bash
# Prerequisites: Go 1.24+, kubectl, kind, docker, openvswitch-switch
# Build
go build ./...
go vet ./...

# Deploy local cluster + observability
kind create cluster --name sdn-test
make install                                    # Install 12 CRDs
go run cmd/main.go \                            # Run controller
  --metrics-bind-address=:8080 \
  --metrics-secure=false \
  --health-probe-bind-address=:8081
cd deploy && docker compose up -d               # Grafana + Prometheus + OTel

# Apply sample resources
kubectl apply -f config/samples/full_example.yaml
```

## Build Commands
```bash
go build ./...          # Build all packages
go vet ./...            # Static analysis
make generate           # Generate deepcopy methods
make manifests          # Regenerate CRD YAML from Go types
make install            # Install CRDs into current cluster
make uninstall          # Remove CRDs from cluster
```

## Binaries
| Binary | Path | Description |
|---|---|---|
| controller-manager | `cmd/main.go` | Runs all 12 CRD reconcilers with leader election |
| node-agent | `cmd/node-agent/main.go` | DaemonSet agent: programs OVS, tunnels, offload, bonds, NFV |
| sdnctl | `cmd/sdnctl/main.go` | CLI debug tool: `status`, `flows`, `tunnels`, `hw` |

## CRD API Groups (12 CRDs, 5 groups)
| Group | Resources | Short Names |
|---|---|---|
| `vpc.sdncontroller.io` | VPC, Subnet | `vpc`, `sn` |
| `gateway.sdncontroller.io` | InternetGateway, NATGateway | `igw`, `natgw` |
| `service.sdncontroller.io` | LoadBalancer, Firewall | `lb`, `fw` |
| `infra.sdncontroller.io` | NetworkInterface, Bond, Tunnel, NodeNetworkConfig | `nic`, `bond`, `tun`, `nnc` |
| `nfv.sdncontroller.io` | ServiceChain, VirtualNetworkFunction | — , `vnf` |

Note: `sc` short name conflicts with StorageClass; use `servicechains.nfv.sdncontroller.io` instead.

## Key Source Directories
```
api/                        # CRD type definitions (5 groups x v1alpha1)
internal/controller/        # 12 reconcilers (vpc, gateway, service, infra, nfv)
internal/agent/             # Node agent subsystems (ovs, tunnel, offload, bond, nfv, libvirt, dpu)
internal/ipam/              # IP address management (per-subnet allocators)
internal/telemetry/         # OpenTelemetry init + metric instruments
pkg/ovs/                    # OVS client (ovs-vsctl/ovs-ofctl wrapper)
pkg/netutil/                # CIDR math, MTU calculation, VNI allocator
pkg/hw/                     # Hardware detection (Mellanox CX6/7, DPU, sysfs)
deploy/                     # Helm chart, docker-compose, Grafana dashboards, OTel config
config/crd/bases/           # Generated CRD manifests (12 files)
config/samples/             # Example CRs (vpc_sample, subnet_sample, full_example)
hack/verify/                # E2E test scripts + libvirt VM setup
```

## Verification
```bash
# Automated E2E (requires running cluster + controller)
hack/verify/verify-e2e.sh run

# Libvirt VM test environment (requires libvirt, OVS, cloud-image-utils)
sudo hack/verify/setup-test-vms.sh setup    # Create 4 VMs on OVS bridges
sudo hack/verify/setup-test-vms.sh cleanup  # Tear down

# Manual test
kubectl apply -f config/samples/full_example.yaml -n default
kubectl get vpc,subnet,igw,natgw,lb,fw,tunnel,vnf -o wide
```

## Observability Stack
```bash
cd deploy && docker compose up -d
# Grafana:    http://localhost:3000 (admin / admin)
# Prometheus: http://localhost:9090
# OTel OTLP:  localhost:4317 (gRPC), localhost:4318 (HTTP)

# Dashboards (auto-provisioned):
#   - SDN Controller Overview (uid: sdn-overview)
#   - SDN Node Agent (uid: sdn-node-agent)
```

## Docker Images
```bash
docker build -f Dockerfile.controller -t sdncontroller/controller .
docker build -f Dockerfile.agent -t sdncontroller/agent .
```

## Helm
```bash
helm install sdncontroller deploy/helm/sdncontroller/
helm upgrade sdncontroller deploy/helm/sdncontroller/ --set controller.replicas=3
```

## Known Issues / Design Notes
- VNI allocator is in-memory; on controller restart it restores from VPC status fields
- ServiceChain short name `sc` conflicts with StorageClass; use full resource name
- LB DNAT in OVS requires conntrack (`ct()` action) for return-path reverse NAT
- Subnet overlap check skips Failed-phase subnets to avoid false positives on re-reconcile
- Node agent requires privileged DaemonSet (OVS, netns, sysfs access)
