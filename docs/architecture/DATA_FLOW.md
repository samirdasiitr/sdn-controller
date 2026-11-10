# Data Flow & Reconciliation Patterns

## 1. VPC Creation Flow

```
User creates VPC CR
        |
        v
VPC Controller (reconcile)
  1. Validate CIDR (no overlap with existing VPCs)
  2. Allocate VNI from pool (or use user-specified)
  3. Store VNI mapping in VPC status
  4. Create default route table
  5. Set VPC status = Ready
        |
        v
Node agents on matching nodes (via nodeSelector) detect VPC
  1. Create OVS internal port for VPC on br-int
  2. Add flow rules for VPC isolation (match on VNI)
  3. Report readiness to NodeNetworkConfig status
```

## 2. Subnet Attach Flow

```
User creates Subnet CR (references VPC)
        |
        v
Subnet Controller (reconcile)
  1. Validate CIDR is within VPC CIDR
  2. Validate no overlap with sibling subnets
  3. Allocate gateway IP
  4. Initialize IPAM pool for the subnet
  5. Set Subnet status = Ready
        |
        v
Node agents on matching nodes
  1. Add OVS flows for subnet routing (L2 within subnet, L3 across subnets)
  2. Set up DHCP responder (if enabled)
  3. Program ARP responder flows
```

## 3. Packet Flow: VM-to-VM (Same Subnet, Different Nodes)

```
VM on Node A (10.100.1.11) -> VM on Node B (10.100.1.12)

Node A:
  1. VM sends packet to OVS br-int via tap/veth/VF
  2. br-int: Match on src MAC, learn source
  3. br-int: Lookup dst MAC -> tunnel port to Node B
  4. br-tun: Encapsulate in VXLAN (VNI from VPC)
  5. (If Mellanox offload active: steps 2-4 in hardware via TC flower)
  6. Physical NIC sends VXLAN-encapsulated packet

Node B:
  1. Physical NIC receives VXLAN packet
  2. (If offload: hardware decaps, delivers to VF representor)
  3. br-tun: Decapsulate VXLAN, identify VPC by VNI
  4. br-int: Forward to destination port based on dst MAC
  5. VM receives packet
```

## 4. Packet Flow: VM-to-Internet (via InternetGateway)

```
VM (10.100.1.11) -> Internet (8.8.8.8)

1. VM sends to default gateway (10.100.1.1 on br-int)
2. br-int: Route lookup -> matches default route via IGW
3. Tunnel to gateway node (if VM not on gateway node)
4. Gateway node br-int: Receives packet
5. Gateway node: SNAT (if NATGateway configured) or direct route
6. br-ex: Forward to physical uplink
7. Physical NIC -> Internet

Return path:
1. Physical NIC receives return packet
2. br-ex: Match on DNAT/conntrack entry
3. br-int: Route to destination subnet
4. Tunnel back to VM's node (if different)
5. Deliver to VM
```

## 5. Firewall Processing

```
Packet arrives at br-int
  |
  v
Priority-ordered flow tables on br-int:
  Table 0: Ingress classification (VPC, subnet, source)
  Table 10: Firewall rules (match on Firewall CR rules)
    - For each rule (by priority):
      - Match: protocol, srcCIDR, dstCIDR, ports
      - Action: Allow (goto next table) or Deny (drop)
      - If stateful: Check conntrack first (established = allow)
    - Default action from Firewall CR (Deny/Allow)
  Table 20: Forwarding decision
  Table 30: Egress firewall rules
  Table 40: Output
```

## 6. Load Balancer Flow

```
Client -> VIP (10.100.1.100:80)
  |
  v
OVS br-int on receiving node:
  1. Match on VIP destination
  2. Apply LB algorithm:
     - RoundRobin: Group select bucket (round-robin)
     - LeastConn: Custom controller-managed bucket weights
     - IPHash: Hash(srcIP) mod backends
     - Maglev: Consistent hash table
  3. DNAT to selected backend (10.100.1.11:8080)
  4. Forward to backend (same node or via tunnel)
  5. Conntrack entry for return path

Return path:
  1. Backend reply: src=10.100.1.11:8080, dst=client
  2. OVS conntrack: Reverse DNAT (src -> VIP)
  3. Forward to client
```

## 7. Service Chain (NFV) Flow

```
Packet matches ServiceChain selector
  |
  v
OVS br-int:
  1. Match on chain selector (subnet + protocol + port)
  2. Set NSH/metadata for service chain tracking
  3. Forward to VNF-1 ingress port
  |
  v
VNF-1 (e.g., IDS) processes packet
  -> Forwards to VNF-1 egress port
  |
  v
OVS br-int:
  4. Match on VNF-1 egress port + chain metadata
  5. Forward to VNF-2 ingress port
  |
  v
VNF-2 (e.g., WAF) processes packet
  -> Forwards to VNF-2 egress port
  |
  v
OVS br-int:
  6. End of chain -> resume normal forwarding
  7. Forward to destination
```

## 8. Hardware Offload Decision Tree

```
Node Agent starts
  |
  +-> Scan NICs: /sys/class/net/*/device/vendor
  |     Is vendor 0x15b3 (Mellanox)?
  |       |
  |       +-- YES: Check model (CX6=0x101d, CX7=0x1021)
  |       |     |
  |       |     +-> Check devlink eswitch mode
  |       |     |     Is switchdev supported?
  |       |     |       |
  |       |     |       +-- YES: Enable switchdev mode
  |       |     |       |     Create VFs
  |       |     |       |     Create representor ports
  |       |     |       |     Set offload_capable = true
  |       |     |       |
  |       |     |       +-- NO: Use software OVS datapath
  |       |     |
  |       |     +-> For each flow to program:
  |       |           Try TC flower hardware offload
  |       |             |
  |       |             +-- SUCCESS: Flow in hardware
  |       |             +-- FAIL: Fall back to OVS software flow
  |       |
  |       +-- NO: Use software OVS datapath
  |
  +-> Check for DPU: /sys/class/net/*/device (BF2/BF3 ID)
        Is DPU present?
          |
          +-- YES: Delegate offload to DPU agent
          +-- NO: Offload on host NIC (if capable)
```

## 9. Bonding & HA Flow

```
Bond Creation:
  1. Bond CR created for node
  2. Node agent receives CR via watch
  3. Agent configures Linux bond:
     - ip link add bond0 type bond mode 802.3ad
     - ip link set eth0 master bond0
     - ip link set eth1 master bond0
     - Configure LACP rate, hash policy
  4. Attach bond to OVS br-ex as uplink
  5. Monitor LACP state

Bond Failover:
  1. Agent detects interface down (via netlink)
  2. Linux bonding driver handles failover automatically
  3. Agent updates Bond CR status
  4. If active-backup: traffic continues on surviving interface
  5. If 802.3ad: LACP renegotiates with fewer links

Gateway HA Failover:
  1. Active gateway node fails (health check timeout)
  2. Gateway controller detects via node condition
  3. Controller updates IGW/NATGW CR to point to standby
  4. Standby node agent:
     - Assumes gateway IP (gratuitous ARP)
     - Activates NAT rules
     - Activates routing rules
  5. Tunnel endpoints re-pointed to new gateway
  6. ~2-5 second failover window
```
