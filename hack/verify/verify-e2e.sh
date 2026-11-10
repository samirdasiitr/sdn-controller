#!/bin/bash
# End-to-end verification script for SDN Controller.
# Deploys CRDs to a k8s cluster, creates test resources, and validates behavior.
#
# Prerequisites: kubectl connected to a cluster, CRDs installed (make install)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
NAMESPACE="sdn-test"
PASSED=0
FAILED=0

log()  { echo "[$(date '+%H:%M:%S')] $*"; }
pass() { PASSED=$((PASSED+1)); log "PASS: $*"; }
fail() { FAILED=$((FAILED+1)); log "FAIL: $*"; }

wait_for_phase() {
    local resource=$1 name=$2 expected=$3 timeout=${4:-30}
    for i in $(seq 1 $timeout); do
        phase=$(kubectl get "$resource" "$name" -n "$NAMESPACE" -o jsonpath='{.status.phase}' 2>/dev/null || echo "")
        if [[ "$phase" == "$expected" ]]; then
            return 0
        fi
        sleep 1
    done
    return 1
}

setup() {
    log "=== Setting up test namespace ==="
    kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

    log "=== Installing CRDs ==="
    cd "$PROJECT_DIR"
    make install 2>/dev/null || kubectl apply -f config/crd/bases/
}

test_vpc() {
    log "=== Test: VPC Creation ==="
    kubectl apply -n "$NAMESPACE" -f - <<EOF
apiVersion: vpc.sdncontroller.io/v1alpha1
kind: VPC
metadata:
  name: test-vpc
spec:
  cidr: "10.200.0.0/16"
  mtu: 9000
  enableDNS: true
  nodeSelector:
    matchLabels:
      network-zone: test
EOF

    if wait_for_phase vpc test-vpc Ready 30; then
        pass "VPC test-vpc reached Ready phase"
    else
        fail "VPC test-vpc did not reach Ready phase"
    fi

    # Verify VNI allocated
    vni=$(kubectl get vpc test-vpc -n "$NAMESPACE" -o jsonpath='{.status.allocatedVNI}')
    if [[ -n "$vni" && "$vni" -gt 0 ]]; then
        pass "VPC VNI allocated: $vni"
    else
        fail "VPC VNI not allocated"
    fi
}

test_subnet() {
    log "=== Test: Subnet Creation ==="
    kubectl apply -n "$NAMESPACE" -f - <<EOF
apiVersion: vpc.sdncontroller.io/v1alpha1
kind: Subnet
metadata:
  name: test-web
spec:
  vpcRef: test-vpc
  cidr: "10.200.1.0/24"
  type: Private
  dhcp:
    enabled: true
    range:
      start: "10.200.1.10"
      end: "10.200.1.250"
    dns:
      - "10.200.0.2"
EOF

    if wait_for_phase subnet test-web Ready 30; then
        pass "Subnet test-web reached Ready phase"
    else
        fail "Subnet test-web did not reach Ready phase"
    fi

    # Verify gateway resolved
    gw=$(kubectl get subnet test-web -n "$NAMESPACE" -o jsonpath='{.status.gateway}')
    if [[ -n "$gw" ]]; then
        pass "Subnet gateway resolved: $gw"
    else
        fail "Subnet gateway not resolved"
    fi

    # Test CIDR overlap detection
    log "=== Test: Subnet CIDR Overlap Detection ==="
    kubectl apply -n "$NAMESPACE" -f - <<EOF
apiVersion: vpc.sdncontroller.io/v1alpha1
kind: Subnet
metadata:
  name: test-overlap
spec:
  vpcRef: test-vpc
  cidr: "10.200.1.0/25"
  type: Private
EOF
    sleep 5
    phase=$(kubectl get subnet test-overlap -n "$NAMESPACE" -o jsonpath='{.status.phase}' 2>/dev/null || echo "")
    if [[ "$phase" == "Failed" ]]; then
        pass "Overlapping subnet correctly detected and failed"
    else
        fail "Overlapping subnet was not rejected (phase: $phase)"
    fi
    kubectl delete subnet test-overlap -n "$NAMESPACE" --ignore-not-found
}

test_gateway() {
    log "=== Test: InternetGateway Creation ==="
    kubectl apply -n "$NAMESPACE" -f - <<EOF
apiVersion: gateway.sdncontroller.io/v1alpha1
kind: InternetGateway
metadata:
  name: test-igw
spec:
  vpcRef: test-vpc
  externalInterface: eth0
  externalIPs:
    - "203.0.113.10"
  ha:
    enabled: true
    mode: ActiveStandby
  nodeSelector:
    matchLabels:
      role: gateway
EOF

    if wait_for_phase internetgateway test-igw Ready 30; then
        pass "InternetGateway test-igw reached Ready"
    else
        fail "InternetGateway test-igw did not reach Ready"
    fi

    log "=== Test: NATGateway Creation ==="
    kubectl apply -n "$NAMESPACE" -f - <<EOF
apiVersion: gateway.sdncontroller.io/v1alpha1
kind: NATGateway
metadata:
  name: test-natgw
spec:
  vpcRef: test-vpc
  subnetRefs:
    - test-web
  externalIP: "203.0.113.20"
  type: SNAT
  portRange:
    start: 1024
    end: 65535
EOF

    if wait_for_phase natgateway test-natgw Ready 30; then
        pass "NATGateway test-natgw reached Ready"
    else
        fail "NATGateway test-natgw did not reach Ready"
    fi
}

test_services() {
    log "=== Test: LoadBalancer Creation ==="
    kubectl apply -n "$NAMESPACE" -f - <<EOF
apiVersion: service.sdncontroller.io/v1alpha1
kind: LoadBalancer
metadata:
  name: test-lb
spec:
  vpcRef: test-vpc
  type: L4
  algorithm: RoundRobin
  frontend:
    ip: "10.200.1.100"
    ports:
      - protocol: TCP
        port: 80
        targetPort: 8080
  backends:
    - ip: "10.200.1.11"
      weight: 100
    - ip: "10.200.1.12"
      weight: 100
  healthCheck:
    protocol: HTTP
    port: 8080
    path: "/healthz"
EOF

    if wait_for_phase loadbalancer test-lb Ready 30; then
        pass "LoadBalancer test-lb reached Ready"
    else
        fail "LoadBalancer test-lb did not reach Ready"
    fi

    backends=$(kubectl get lb test-lb -n "$NAMESPACE" -o jsonpath='{.status.activeBackends}')
    if [[ "$backends" == "2" ]]; then
        pass "LoadBalancer has 2 active backends"
    else
        fail "LoadBalancer backends count: $backends (expected 2)"
    fi

    log "=== Test: Firewall Creation ==="
    kubectl apply -n "$NAMESPACE" -f - <<EOF
apiVersion: service.sdncontroller.io/v1alpha1
kind: Firewall
metadata:
  name: test-fw
spec:
  vpcRef: test-vpc
  subnetRefs:
    - test-web
  defaultAction: Deny
  stateful: true
  rules:
    - name: allow-http
      priority: 100
      direction: Ingress
      action: Allow
      match:
        protocol: TCP
        srcCIDR: "0.0.0.0/0"
        dstPort: "80"
      logging: true
    - name: allow-https
      priority: 101
      direction: Ingress
      action: Allow
      match:
        protocol: TCP
        dstPort: "443"
    - name: deny-all
      priority: 9999
      direction: Ingress
      action: Deny
      match:
        srcCIDR: "0.0.0.0/0"
      logging: true
EOF

    if wait_for_phase firewall test-fw Active 30; then
        pass "Firewall test-fw reached Active"
    else
        fail "Firewall test-fw did not reach Active"
    fi

    rules=$(kubectl get fw test-fw -n "$NAMESPACE" -o jsonpath='{.status.activeRules}')
    if [[ "$rules" == "3" ]]; then
        pass "Firewall has 3 active rules"
    else
        fail "Firewall rules count: $rules (expected 3)"
    fi
}

test_infra() {
    log "=== Test: Tunnel Creation ==="
    kubectl apply -n "$NAMESPACE" -f - <<EOF
apiVersion: infra.sdncontroller.io/v1alpha1
kind: Tunnel
metadata:
  name: test-tunnel
spec:
  type: VXLAN
  vni: 5001
  localEndpoint:
    nodeName: node-01
    ip: "192.168.1.1"
  remoteEndpoint:
    nodeName: node-02
    ip: "192.168.1.2"
  mtu: 8950
  offload:
    enabled: true
EOF

    if wait_for_phase tunnel test-tunnel Established 30; then
        pass "Tunnel test-tunnel reached Established"
    else
        fail "Tunnel test-tunnel did not reach Established"
    fi
}

test_nfv() {
    log "=== Test: VNF Creation ==="
    kubectl apply -n "$NAMESPACE" -f - <<EOF
apiVersion: nfv.sdncontroller.io/v1alpha1
kind: VirtualNetworkFunction
metadata:
  name: test-ids
spec:
  type: VM
  vmConfig:
    image: "/var/lib/libvirt/images/vnf-ids.qcow2"
    vcpus: 1
    memoryMB: 256
  replicas: 1
  interfaces:
    - name: vnf-in
      type: OVSPort
    - name: vnf-out
      type: OVSPort
  nodeSelector:
    matchLabels:
      nfv-capable: "true"
EOF

    if wait_for_phase virtualnetworkfunction test-ids Running 30; then
        pass "VNF test-ids reached Running"
    else
        fail "VNF test-ids did not reach Running"
    fi

    log "=== Test: ServiceChain Creation ==="
    kubectl apply -n "$NAMESPACE" -f - <<EOF
apiVersion: nfv.sdncontroller.io/v1alpha1
kind: ServiceChain
metadata:
  name: test-chain
spec:
  vpcRef: test-vpc
  subnetRef: test-web
  direction: Ingress
  chain:
    - name: ids-inspection
      vnfRef: test-ids
      portPair:
        ingress: vnf-in
        egress: vnf-out
  match:
    protocol: TCP
    dstPort: 80
EOF

    if wait_for_phase servicechain test-chain Active 30; then
        pass "ServiceChain test-chain reached Active"
    else
        fail "ServiceChain test-chain did not reach Active"
    fi
}

test_kubectl_output() {
    log "=== Test: kubectl get output formatting ==="
    echo "--- VPCs ---"
    kubectl get vpc -n "$NAMESPACE" -o wide
    echo "--- Subnets ---"
    kubectl get subnet -n "$NAMESPACE" -o wide
    echo "--- Gateways ---"
    kubectl get igw,natgw -n "$NAMESPACE" -o wide
    echo "--- Services ---"
    kubectl get lb,fw -n "$NAMESPACE" -o wide
    echo "--- Tunnels ---"
    kubectl get tunnel -n "$NAMESPACE" -o wide
    echo "--- NFV ---"
    kubectl get vnf,sc -n "$NAMESPACE" -o wide
    pass "kubectl output formatting works"
}

cleanup() {
    log "=== Cleaning up ==="
    kubectl delete namespace "$NAMESPACE" --ignore-not-found --timeout=60s
}

report() {
    echo ""
    echo "========================================="
    echo "  Test Results: ${PASSED} passed, ${FAILED} failed"
    echo "========================================="
    if [[ $FAILED -gt 0 ]]; then
        exit 1
    fi
}

# --- Entry point ---
case "${1:-run}" in
    run)
        setup
        test_vpc
        test_subnet
        test_gateway
        test_services
        test_infra
        test_nfv
        test_kubectl_output
        report
        ;;
    cleanup)
        cleanup
        ;;
    *)
        echo "Usage: $0 {run|cleanup}"
        exit 1
        ;;
esac
