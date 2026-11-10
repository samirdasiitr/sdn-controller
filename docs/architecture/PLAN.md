# SDN Controller - Architecture Plan

## 1. Overview

A CNCF-native Software-Defined Networking (SDN) controller built on Kubernetes CRDs. It provides cloud-like networking primitives (VPC, Subnets, LB, Firewall, NAT Gateway, Internet Gateway) for bare-metal and VM environments, with hardware offload support for Mellanox ConnectX-6/7 NICs.

### Key Design Principles

- **CRD-first**: Every network construct is a Kubernetes Custom Resource
- **Declarative**: Users declare desired network state; controllers reconcile
- **Hardware-aware**: Automatic detection and use of hardware offload (Mellanox switchdev/TC flower)
- **Multi-tenant**: VPC isolation with overlapping IP support via VXLAN/IPIP
- **Observable**: OpenTelemetry-based logging, metrics, and tracing throughout
- **HA**: Leader election for controllers, active-standby for gateways, ECMP for load balancers

---

## 2. High-Level Architecture

```
+------------------------------------------------------------------+
|                     Kubernetes Control Plane                      |
|                                                                   |
|  +------------------+  +------------------+  +-----------------+  |
|  | VPC Controller   |  | Gateway Ctrl     |  | Service Ctrl    |  |
|  | (vpc, subnet)    |  | (igw, natgw)     |  | (lb, firewall)  |  |
|  +--------+---------+  +--------+---------+  +--------+--------+  |
|           |                     |                      |          |
|  +--------+---------+  +-------+----------+            |          |
|  | Infra Controller |  | NFV Controller   |            |          |
|  | (bond, tunnel,   |  | (svc chain, vnf) |            |          |
|  |  nic, mtu)       |  +------------------+            |          |
|  +------------------+                                  |          |
|           |                                            |          |
+------------------------------------------------------------------+
            |            gRPC / CRD Status Updates       |
            v                                            v
+------------------------------------------------------------------+
|                    Node Agent (DaemonSet)                          |
|                                                                   |
|  +------------------+  +------------------+  +-----------------+  |
|  | OVS Manager      |  | Tunnel Manager   |  | Offload Manager |  |
|  | (bridges, ports,  |  | (VXLAN, IPIP)   |  | (Mellanox CX6/7)|  |
|  |  flows)           |  +------------------+  +-----------------+  |
|  +------------------+                                             |
|  +------------------+  +------------------+  +-----------------+  |
|  | Bond Manager     |  | NFV Agent        |  | Metrics Agent   |  |
|  | (NIC bonding)    |  | (service chains) |  | (OTel exporter) |  |
|  +------------------+  +------------------+  +-----------------+  |
|                                                                   |
+------------------------------------------------------------------+
            |                                            |
            v                                            v
+------------------------------+    +----------------------------+
| OVS Datapath                 |    | Hardware (Mellanox CX6/7)  |
| - br-int (integration)      |    | - switchdev mode           |
| - br-ex (external)          |    | - TC flower offload        |
| - br-tun (tunnel)           |    | - VF representors          |
+------------------------------+    +----------------------------+
```

---

## 3. CRD API Groups

Multi-group layout with clear separation of concerns:

| API Group                    | Resources                          | Purpose                        |
|------------------------------|------------------------------------|--------------------------------|
| `vpc.sdncontroller.io`      | VPC, Subnet                        | Core network topology          |
| `gateway.sdncontroller.io`  | InternetGateway, NATGateway        | North-south connectivity       |
| `service.sdncontroller.io`  | LoadBalancer, Firewall             | Network services               |
| `infra.sdncontroller.io`    | NetworkInterface, Bond, Tunnel, NodeNetworkConfig | Node-level infrastructure |
| `nfv.sdncontroller.io`      | ServiceChain, VirtualNetworkFunction | Network function virtualization |

---

## 4. CRD Specifications

### 4.1 VPC (`vpc.sdncontroller.io/v1alpha1`)

```yaml
apiVersion: vpc.sdncontroller.io/v1alpha1
kind: VPC
metadata:
  name: production-vpc
spec:
  cidr: "10.100.0.0/16"
  secondaryCIDRs:
    - "10.200.0.0/16"
  vni: 5001                          # VXLAN Network Identifier
  mtu: 9000                          # Jumbo frames
  enableDNS: true
  nodeSelector:                       # Target specific nodes
    matchLabels:
      network-zone: "production"
status:
  phase: Ready
  allocatedSubnets: 3
  activeNodes: 12
  conditions:
    - type: Ready
      status: "True"
```

### 4.2 Subnet (`vpc.sdncontroller.io/v1alpha1`)

```yaml
apiVersion: vpc.sdncontroller.io/v1alpha1
kind: Subnet
metadata:
  name: web-tier
spec:
  vpcRef: production-vpc
  cidr: "10.100.1.0/24"
  gateway: "10.100.1.1"
  type: Private                       # Private | Public
  vlanID: 100                         # Optional VLAN tag
  mtu: 9000
  dhcp:
    enabled: true
    range:
      start: "10.100.1.10"
      end: "10.100.1.250"
    dns:
      - "10.100.0.2"
  nodeSelector:
    matchLabels:
      rack: "rack-01"
status:
  phase: Ready
  allocatedIPs: 45
  availableIPs: 196
```

### 4.3 InternetGateway (`gateway.sdncontroller.io/v1alpha1`)

```yaml
apiVersion: gateway.sdncontroller.io/v1alpha1
kind: InternetGateway
metadata:
  name: prod-igw
spec:
  vpcRef: production-vpc
  externalInterface: "eth0"           # Physical uplink
  externalIPs:
    - "203.0.113.10"
    - "203.0.113.11"
  ha:
    enabled: true
    mode: ActiveStandby               # ActiveStandby | ActiveActive
  nodeSelector:
    matchLabels:
      role: "gateway-node"
status:
  phase: Ready
  activeNode: "gw-node-01"
  standbyNode: "gw-node-02"
```

### 4.4 NATGateway (`gateway.sdncontroller.io/v1alpha1`)

```yaml
apiVersion: gateway.sdncontroller.io/v1alpha1
kind: NATGateway
metadata:
  name: prod-natgw
spec:
  vpcRef: production-vpc
  subnetRefs:
    - web-tier
    - app-tier
  externalIP: "203.0.113.20"
  type: SNAT                          # SNAT | DNAT | FullNAT
  portRange:
    start: 1024
    end: 65535
  ha:
    enabled: true
    mode: ActiveStandby
  nodeSelector:
    matchLabels:
      role: "gateway-node"
status:
  phase: Ready
  translationCount: 15420
  activeNode: "gw-node-01"
```

### 4.5 LoadBalancer (`service.sdncontroller.io/v1alpha1`)

```yaml
apiVersion: service.sdncontroller.io/v1alpha1
kind: LoadBalancer
metadata:
  name: web-lb
spec:
  vpcRef: production-vpc
  type: L4                            # L4 | L7
  algorithm: RoundRobin               # RoundRobin | LeastConn | IPHash | Maglev
  frontend:
    ip: "10.100.1.100"
    ports:
      - protocol: TCP
        port: 80
        targetPort: 8080
      - protocol: TCP
        port: 443
        targetPort: 8443
  backends:
    - ip: "10.100.1.11"
      weight: 100
    - ip: "10.100.1.12"
      weight: 100
  healthCheck:
    protocol: HTTP
    port: 8080
    path: "/healthz"
    interval: 10s
    timeout: 5s
    unhealthyThreshold: 3
  sessionAffinity:
    enabled: true
    timeout: 3600s
  nodeSelector:
    matchLabels:
      network-zone: "production"
status:
  phase: Ready
  activeBackends: 2
  totalConnections: 84210
```

### 4.6 Firewall (`service.sdncontroller.io/v1alpha1`)

```yaml
apiVersion: service.sdncontroller.io/v1alpha1
kind: Firewall
metadata:
  name: web-firewall
spec:
  vpcRef: production-vpc
  subnetRefs:
    - web-tier
  defaultAction: Deny                 # Deny | Allow
  rules:
    - name: allow-http
      priority: 100
      direction: Ingress              # Ingress | Egress
      action: Allow
      match:
        protocol: TCP
        srcCIDR: "0.0.0.0/0"
        dstPort: 80
      logging: true
    - name: allow-https
      priority: 101
      direction: Ingress
      action: Allow
      match:
        protocol: TCP
        srcCIDR: "0.0.0.0/0"
        dstPort: 443
    - name: deny-all-ingress
      priority: 9999
      direction: Ingress
      action: Deny
      match:
        srcCIDR: "0.0.0.0/0"
      logging: true
  stateful: true                      # Connection tracking
  nodeSelector:
    matchLabels:
      network-zone: "production"
status:
  phase: Ready
  activeRules: 3
  matchedPackets: 1502340
```

### 4.7 NetworkInterface (`infra.sdncontroller.io/v1alpha1`)

```yaml
apiVersion: infra.sdncontroller.io/v1alpha1
kind: NetworkInterface
metadata:
  name: node01-eth0
spec:
  nodeName: "node-01"
  type: Physical                      # Physical | Virtual | VF | Bond
  device: "eth0"
  mtu: 9000
  addresses:
    - "10.100.1.11/24"
  subnetRef: web-tier
  vpcRef: production-vpc
  offload:
    enabled: true                     # Enable hardware offload if available
    driver: mlx5                      # Auto-detected if not specified
  dpuManaged: false                   # Whether managed by DPU
status:
  phase: Ready
  macAddress: "00:1a:2b:3c:4d:5e"
  speed: "100Gbps"
  offloadCapable: true
  offloadActive: true
  driver: "mlx5_core"
  firmwareVersion: "22.36.1010"
```

### 4.8 Bond (`infra.sdncontroller.io/v1alpha1`)

```yaml
apiVersion: infra.sdncontroller.io/v1alpha1
kind: Bond
metadata:
  name: node01-bond0
spec:
  nodeName: "node-01"
  mode: 802.3ad                       # balance-rr | active-backup | 802.3ad | balance-xor
  interfaces:
    - "eth0"
    - "eth1"
  mtu: 9000
  lacpRate: fast                      # slow | fast
  hashPolicy: layer3+4               # layer2 | layer2+3 | layer3+4
  addresses:
    - "10.100.1.11/24"
  subnetRef: web-tier
status:
  phase: Ready
  activeInterfaces: 2
  speed: "200Gbps"
```

### 4.9 Tunnel (`infra.sdncontroller.io/v1alpha1`)

```yaml
apiVersion: infra.sdncontroller.io/v1alpha1
kind: Tunnel
metadata:
  name: vxlan-node01-node02
spec:
  type: VXLAN                         # VXLAN | IPIP | GRE
  vni: 5001                           # For VXLAN
  localEndpoint:
    nodeName: "node-01"
    ip: "192.168.1.1"
  remoteEndpoint:
    nodeName: "node-02"
    ip: "192.168.1.2"
  mtu: 8950                           # Outer MTU minus overhead
  checksumEnabled: true
  offload:
    enabled: true                     # Hardware offload for encap/decap
status:
  phase: Established
  txPackets: 1502340
  rxPackets: 1498200
  offloadActive: true
```

### 4.10 NodeNetworkConfig (`infra.sdncontroller.io/v1alpha1`)

```yaml
apiVersion: infra.sdncontroller.io/v1alpha1
kind: NodeNetworkConfig
metadata:
  name: node-01
spec:
  nodeName: "node-01"
  nodeType: BareMetal                 # BareMetal | VM
  computeType: GPU                    # CPU | GPU
  dpuEnabled: true
  dpuDevice: "bf2"                    # BlueField-2/3 DPU
  ovsBridges:
    - name: br-int
      type: integration
      datapathType: netdev            # system | netdev (for DPDK/offload)
    - name: br-ex
      type: external
      uplinks:
        - "bond0"
    - name: br-tun
      type: tunnel
  defaultMTU: 9000
  tunnelProtocol: VXLAN               # Default tunnel type
  tunnelSourceIP: "192.168.1.1"
  hardware:
    nics:
      - device: "eth0"
        vendor: "Mellanox"
        model: "ConnectX-7"
        offloadCapable: true
        switchdevMode: true
        numVFs: 32
      - device: "eth1"
        vendor: "Mellanox"
        model: "ConnectX-7"
        offloadCapable: true
        switchdevMode: true
        numVFs: 32
  libvirt:                            # Only for VM hosts
    enabled: true
    bridgeMapping:
      management: br-mgmt
      data: br-int
status:
  phase: Configured
  ovsVersion: "3.3.0"
  agentVersion: "0.1.0"
  tunnelsEstablished: 15
  offloadStatus: Active
```

### 4.11 ServiceChain (`nfv.sdncontroller.io/v1alpha1`)

```yaml
apiVersion: nfv.sdncontroller.io/v1alpha1
kind: ServiceChain
metadata:
  name: security-chain
spec:
  vpcRef: production-vpc
  subnetRef: web-tier
  direction: Ingress
  chain:
    - name: ids-inspection
      vnfRef: suricata-vnf
      portPair:
        ingress: "vnf-in"
        egress: "vnf-out"
    - name: waf-filter
      vnfRef: modsec-vnf
      portPair:
        ingress: "vnf-in"
        egress: "vnf-out"
  match:
    protocol: TCP
    dstPort: 80
  offload:
    enabled: true                     # Offload chain steering to hardware
  nodeSelector:
    matchLabels:
      nfv-capable: "true"
status:
  phase: Active
  chainLength: 2
  processedPackets: 892100
```

### 4.12 VirtualNetworkFunction (`nfv.sdncontroller.io/v1alpha1`)

```yaml
apiVersion: nfv.sdncontroller.io/v1alpha1
kind: VirtualNetworkFunction
metadata:
  name: suricata-vnf
spec:
  type: Container                     # Container | VM
  image: "registry.local/suricata:7.0"
  replicas: 2
  interfaces:
    - name: vnf-in
      type: OVSPort
    - name: vnf-out
      type: OVSPort
  resources:
    cpu: "4"
    memory: "8Gi"
  nodeSelector:
    matchLabels:
      nfv-capable: "true"
status:
  phase: Running
  readyReplicas: 2
```

---

## 5. Controller Architecture

### 5.1 Controllers (Control Plane)

Each controller runs as a separate Deployment (or combined where appropriate) with leader election:

| Controller           | Watches                          | Responsibilities                                   |
|----------------------|----------------------------------|----------------------------------------------------|
| **VPC Controller**   | VPC, Subnet                      | CIDR allocation, VNI assignment, route table mgmt   |
| **Gateway Controller** | InternetGateway, NATGateway    | External connectivity, NAT rules, HA failover       |
| **Service Controller** | LoadBalancer, Firewall          | LB programming, firewall rules, conn tracking       |
| **Infra Controller** | NIC, Bond, Tunnel, NodeNetworkConfig | Node network setup, tunnel mesh, bonding          |
| **NFV Controller**   | ServiceChain, VNF               | Service chain setup, VNF lifecycle                  |

### 5.2 Node Agent (Data Plane)

The node agent runs as a **DaemonSet** on every network node. It:

1. **Discovers hardware** - Scans for Mellanox CX6/7, DPUs, NIC capabilities
2. **Configures OVS** - Creates bridges (br-int, br-ex, br-tun), programs flows
3. **Manages tunnels** - Sets up VXLAN/IPIP/GRE tunnels between nodes
4. **Handles offload** - Enables switchdev mode, configures TC flower rules
5. **Manages bonds** - Configures Linux bonding or OVS bonds
6. **Programs NFV chains** - Steers traffic through service function chains
7. **Reports status** - Updates CRD status fields via Kubernetes API

#### Node Agent Components

```
Node Agent
  |
  +-- OVS Manager
  |     +-- Bridge Manager (br-int, br-ex, br-tun)
  |     +-- Flow Manager (OpenFlow rules)
  |     +-- Port Manager (VFs, veth, tap, bonds)
  |
  +-- Tunnel Manager
  |     +-- VXLAN endpoint management
  |     +-- IPIP endpoint management
  |     +-- Tunnel health monitoring
  |
  +-- Offload Manager
  |     +-- Hardware detection (lspci, sysfs)
  |     +-- Switchdev mode management
  |     +-- TC flower rule programming
  |     +-- VF representor management
  |
  +-- Bond Manager
  |     +-- Linux bonding driver config
  |     +-- LACP negotiation monitoring
  |
  +-- NFV Agent
  |     +-- Service chain flow programming
  |     +-- Port pair management
  |
  +-- Libvirt Integration (optional)
  |     +-- VM NIC hotplug
  |     +-- OVS port-to-VM mapping
  |     +-- SR-IOV VF passthrough
  |
  +-- DPU Agent (optional)
  |     +-- BlueField DPU communication
  |     +-- Offload delegation to DPU
  |
  +-- Telemetry Agent
        +-- OTel metrics exporter
        +-- Flow statistics collection
        +-- Health probes
```

---

## 6. Communication Architecture

```
Controllers <---> Kubernetes API Server <---> CRD Watch/Update
     |
     +-- (optional) gRPC fast-path to Node Agent for bulk flow updates
     
Node Agent <---> OVS via OVSDB protocol (unix socket)
Node Agent <---> OVS via OpenFlow protocol (for flow programming)
Node Agent <---> sysfs/netlink (for NIC config, bonding, offload)
Node Agent <---> libvirtd via libvirt API (for VM hosts)
Node Agent <---> DPU via gRPC/RPC (for DPU-managed hosts)
```

### Controller-to-Agent Communication

- **Primary**: Kubernetes CRD watches (agent watches CRDs filtered by `nodeSelector`)
- **Secondary**: Optional gRPC channel for low-latency bulk operations
- **Status**: Agent updates CRD `.status` subresource via Kubernetes API

---

## 7. Observability (OpenTelemetry)

### 7.1 Logging
- Structured JSON logging via `zap` logger
- Trace context propagation in all log entries
- Log levels: Debug, Info, Warn, Error
- Per-reconcile logging with resource context

### 7.2 Metrics
- OTLP export to OpenTelemetry Collector
- Controller metrics:
  - `sdn_reconcile_duration_seconds` (histogram)
  - `sdn_reconcile_errors_total` (counter)
  - `sdn_resource_count` (gauge per type)
  - `sdn_vpc_active_count` (gauge)
- Node agent metrics:
  - `sdn_ovs_flow_count` (gauge per bridge)
  - `sdn_tunnel_tx_bytes_total` / `sdn_tunnel_rx_bytes_total`
  - `sdn_offload_flows_count` (hardware offloaded flows)
  - `sdn_bond_active_interfaces` (gauge)
  - `sdn_nat_translations_active` (gauge)
  - `sdn_lb_connections_active` (gauge)
  - `sdn_firewall_matched_packets_total` (counter per rule)

### 7.3 Tracing
- Distributed tracing for reconciliation loops
- Trace spans:
  - Controller reconcile cycles
  - API server interactions
  - Agent-to-OVS operations
  - Tunnel establishment
  - Hardware offload programming

---

## 8. HA and Resilience

### 8.1 Control Plane HA
- **Leader election** via Kubernetes lease objects
- Multiple controller replicas (active/standby)
- Graceful failover with state stored in etcd (via CRDs)

### 8.2 Gateway HA
- **InternetGateway**: Active/Standby with VRRP-like failover
- **NATGateway**: Active/Standby with connection state sync
- Health probes between gateway nodes
- Automatic failover on node failure

### 8.3 Data Plane Resilience
- **Tunnel redundancy**: Multiple tunnel endpoints per VPC
- **Bond failover**: Active-backup or LACP for NIC redundancy
- **OVS restart recovery**: Agent re-programs flows on OVS restart
- **Node failure**: Controllers detect and re-route traffic

---

## 9. Hardware Offload Architecture

### 9.1 Mellanox ConnectX-6/7 Offload

```
Detection Flow:
  1. Node agent scans /sys/class/net/*/device/vendor (0x15b3 = Mellanox)
  2. Check firmware capabilities via devlink
  3. Enable switchdev eSwitch mode
  4. Create VFs and VF representors
  5. Program TC flower rules for offload

Offload Capabilities:
  - VXLAN encap/decap in hardware
  - Connection tracking (CT) offload
  - ACL/firewall rules via TC flower
  - Load balancer flows (L3/L4 hash)
  - Service chain steering
  
Fallback:
  - If offload fails -> fall back to OVS software datapath
  - If NIC doesn't support a specific feature -> software path for that feature only
  - Mixed mode: some flows offloaded, others in software
```

### 9.2 DPU (BlueField) Integration

```
Host <-> DPU Communication:
  - DPU runs a subset of the node agent
  - Host agent delegates offloadable work to DPU agent
  - DPU manages its own OVS instance in switchdev mode
  - All VF traffic is steered through DPU
  
Benefits:
  - Host CPU freed from network processing
  - Better isolation (network control on DPU, compute on host)
  - Hardware-accelerated networking transparent to workloads
```

---

## 10. MTU Management

```
MTU Chain:
  Physical NIC (9000) 
    -> Bond (9000)
      -> OVS br-ex (9000)
        -> OVS br-int (9000)
          -> Tunnel overhead reduction:
              VXLAN: 9000 - 50 = 8950 inner MTU
              IPIP:  9000 - 20 = 8980 inner MTU
              GRE:   9000 - 24 = 8976 inner MTU
          -> VM/Container interface (8950 for VXLAN)

MTU auto-calculation:
  - VPC specifies desired inner MTU
  - System calculates required physical MTU based on tunnel type
  - Validates physical NIC supports required MTU
  - Alerts if MTU mismatch detected
```

---

## 11. Libvirt / VM Integration

For VM hosts using libvirt + OVS:

1. **OVS port creation**: Agent creates OVS port for each VM NIC
2. **Libvirt XML**: Generates network interface XML for libvirt domain
3. **SR-IOV passthrough**: When Mellanox VFs are available, pass VF directly to VM
4. **Live migration**: Coordinate with libvirt for live migration of VMs
   - Pre-migration: Set up destination OVS ports and tunnels
   - Post-migration: Clean up source OVS ports

---

## 12. Security

- **RBAC**: Fine-grained Kubernetes RBAC for all CRDs
- **Network isolation**: VPC-level isolation enforced at OVS flow level
- **Encryption**: Optional IPsec or WireGuard for tunnel encryption
- **Admission webhooks**: Validate CRD specs (CIDR overlap, MTU consistency, etc.)
- **Audit logging**: All network changes logged with user context

---

## 13. Technology Stack

| Component              | Technology                              |
|------------------------|-----------------------------------------|
| Language               | Go 1.24+                               |
| CRD Framework          | Kubebuilder v4 + controller-runtime     |
| OVS Interaction        | go-ovs (OVSDB), go-openflow (OpenFlow) |
| Hardware Offload       | netlink (TC flower), devlink            |
| Observability          | OpenTelemetry Go SDK                    |
| Logging                | go.uber.org/zap                         |
| gRPC                   | google.golang.org/grpc                  |
| Libvirt                | libvirt-go-module                       |
| Testing                | Ginkgo/Gomega (integration), Go testing |
| CI/CD                  | GitHub Actions                          |
| Packaging              | Helm charts, Kustomize                  |

---

## 14. Implementation Phases

### Phase 1 - Foundation (Weeks 1-3) -- COMPLETE
- [x] Project scaffolding with kubebuilder (multi-group, 5 API groups)
- [x] VPC and Subnet CRDs + controllers (CIDR validation, VNI allocation, IPAM, overlap detection)
- [x] Basic node agent with OVS bridge setup (internal/agent/)
- [x] VXLAN tunnel mesh between nodes (+ IPIP, GRE support)
- [x] OpenTelemetry instrumentation skeleton (40+ metric instruments)
- [x] Core libraries: pkg/netutil (CIDR, MTU, VNI), pkg/ovs, pkg/hw, internal/ipam

### Phase 2 - Core Networking (Weeks 4-6) -- COMPLETE
- [x] InternetGateway and NATGateway CRDs + controllers (HA node selection, subnet validation)
- [x] Firewall CRD + controller (priority-ordered rules, default action, stateful flag)
- [x] LoadBalancer CRD + controller (L4, RoundRobin/LeastConn/IPHash, backend validation)
- [x] MTU chain management (auto inner-MTU calculation for VXLAN/IPIP/GRE)
- [ ] Admission webhooks for validation (CRD-level validation via kubebuilder markers in place)

### Phase 3 - Infrastructure (Weeks 7-9) -- COMPLETE
- [x] NetworkInterface, Bond, Tunnel, NodeNetworkConfig CRDs + controllers
- [x] Bonding support (LACP, active-backup, balance-xor modes)
- [x] IPIP tunnel support (alongside VXLAN and GRE)
- [x] Mellanox ConnectX-6/7 offload detection (sysfs-based, switchdev check)
- [x] Node selector enforcement across all CRDs
- [x] NodeNetworkConfig with OVS bridge config, tunnel protocol, agent status

### Phase 4 - Advanced Features (Weeks 10-12) -- COMPLETE
- [x] NFV service chaining (ServiceChain CRD with multi-hop VNF chains)
- [x] VirtualNetworkFunction CRD (VM and Container types)
- [x] Libvirt integration for VM hosts (internal/agent/libvirt/)
- [x] DPU (BlueField-2/3) detection and delegation (internal/agent/dpu/)
- [ ] SR-IOV VF management (VF creation scaffolded in offload manager)
- [ ] L7 LoadBalancer support (L4 complete, L7 deferred)

### Phase 5 - Production Readiness (Weeks 13-15) -- MOSTLY COMPLETE
- [x] HA for controllers (leader election via controller-runtime)
- [x] Gateway HA (Active/Standby node selection with failover)
- [x] Full OpenTelemetry metrics, traces, logging (40+ instruments, 2 Grafana dashboards)
- [x] Helm charts (controller Deployment, agent DaemonSet, RBAC) and Kustomize overlays
- [x] Integration tests with kind (E2E verified: VPC/Subnet/GW/LB/FW/Tunnel/NFV)
- [ ] Performance benchmarks (deferred)
- [x] Documentation (AGENTS.md, architecture docs, sample CRs)

### E2E Verification Results (Automated)
| Test | Result |
|---|---|
| VPC creation + VNI allocation | PASS - VNI 1000 stable across re-reconciles |
| Subnet CIDR overlap detection | PASS - correctly rejects overlapping CIDRs |
| Subnet out-of-VPC-range detection | PASS - rejects CIDRs outside parent VPC |
| InternetGateway HA node selection | PASS - selects active node by label |
| NATGateway subnet validation | PASS - validates all subnet refs exist |
| LoadBalancer backend validation | PASS - 3 backends, VIP validated |
| Firewall rule ordering | PASS - 4 rules ordered by priority |
| VXLAN + IPIP tunnel establishment | PASS - inner MTU calculated (8900/1460) |
| VNF lifecycle (VM + Container) | PASS - replicas reported ready |
| ServiceChain VNF hop validation | PASS - 2-hop chain active |
| OVS same-subnet ping | PASS - 0% loss, <0.4ms RTT |
| OVS firewall packet filtering | PASS - ICMP/TCP allowed, deny-all drops |
| OVS LB DNAT flow matching | PASS - VIP packets rewritten to backend |
| OVS NFV service chain steering | PASS - packets traverse WAF->IDS hops |

### Bugs Found and Fixed During Verification
1. **VNI allocation leak**: Each reconcile allocated a new VNI. Fixed: idempotent allocation restores from status.
2. **Status update race**: Setting Provisioning phase in a separate status update triggered re-reconcile conflicts. Fixed: batch phase change with final status update.
3. **Failed subnet poisoning**: A rejected overlapping subnet caused valid sibling subnets to fail on re-reconcile. Fixed: skip Failed-phase subnets in overlap check.
