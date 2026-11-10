#!/bin/bash
# Setup test VMs for SDN controller verification using libvirt + OVS.
# Creates lightweight VMs connected to OVS bridges to validate:
#   - VPC isolation via VXLAN
#   - Subnet routing
#   - Firewall rules
#   - Load balancing
#   - NAT gateway
#   - NFV service chains (small VM-based VNFs)
#
# Prerequisites: libvirt, qemu-kvm, openvswitch, cloud-image-utils, wget

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
VM_DIR="/var/lib/libvirt/images/sdn-test"
CLOUD_IMG_URL="https://cloud-images.ubuntu.com/minimal/releases/noble/release/ubuntu-24.04-minimal-cloudimg-amd64.img"
BASE_IMG="${VM_DIR}/ubuntu-base.qcow2"
SSH_KEY="${VM_DIR}/sdn-test-key"

# VM specs (lightweight)
VM_VCPUS=1
VM_MEMORY=512  # MB
VNF_VCPUS=1
VNF_MEMORY=256 # MB for NFV VMs

# Network config
OVS_BRIDGE_INT="br-sdn-int"
OVS_BRIDGE_EXT="br-sdn-ext"
OVS_BRIDGE_TUN="br-sdn-tun"

log() { echo "[$(date '+%H:%M:%S')] $*"; }

check_deps() {
    local missing=()
    for cmd in virsh qemu-img virt-install ovs-vsctl cloud-localds; do
        command -v "$cmd" &>/dev/null || missing+=("$cmd")
    done
    if [[ ${#missing[@]} -gt 0 ]]; then
        echo "Missing dependencies: ${missing[*]}"
        echo "Install with: sudo apt-get install -y libvirt-daemon-system qemu-kvm openvswitch-switch cloud-image-utils virtinst"
        exit 1
    fi
}

setup_directories() {
    log "Creating VM directory ${VM_DIR}"
    sudo mkdir -p "${VM_DIR}"
    sudo chown "$(id -u):$(id -g)" "${VM_DIR}"
}

download_base_image() {
    if [[ -f "${BASE_IMG}" ]]; then
        log "Base image already exists"
        return
    fi
    log "Downloading Ubuntu cloud image..."
    wget -q --show-progress -O "${BASE_IMG}" "${CLOUD_IMG_URL}"
    # Resize to 4GB
    qemu-img resize "${BASE_IMG}" 4G
}

generate_ssh_key() {
    if [[ -f "${SSH_KEY}" ]]; then
        log "SSH key already exists"
        return
    fi
    log "Generating SSH key pair"
    ssh-keygen -t ed25519 -f "${SSH_KEY}" -N "" -q
}

setup_ovs_bridges() {
    log "Setting up OVS bridges"
    for br in ${OVS_BRIDGE_INT} ${OVS_BRIDGE_EXT} ${OVS_BRIDGE_TUN}; do
        sudo ovs-vsctl --may-exist add-br "$br"
    done
    # Add VXLAN tunnel port for testing (loopback to self)
    sudo ovs-vsctl --may-exist add-port ${OVS_BRIDGE_TUN} vxlan-loop \
        -- set interface vxlan-loop type=vxlan options:remote_ip=127.0.0.1 options:key=5001
    log "OVS bridges created: ${OVS_BRIDGE_INT}, ${OVS_BRIDGE_EXT}, ${OVS_BRIDGE_TUN}"
    sudo ovs-vsctl show
}

create_cloud_init() {
    local vm_name=$1
    local ip_addr=$2
    local gateway=$3
    local role=${4:-"workload"} # workload or vnf

    local ci_dir="${VM_DIR}/${vm_name}"
    mkdir -p "${ci_dir}"

    # user-data
    cat > "${ci_dir}/user-data" <<USERDATA
#cloud-config
hostname: ${vm_name}
users:
  - name: sdn
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    ssh_authorized_keys:
      - $(cat "${SSH_KEY}.pub")
packages:
  - iproute2
  - iperf3
  - tcpdump
  - curl
  - nftables
runcmd:
  - ip addr add ${ip_addr}/24 dev enp1s0 || true
  - ip link set enp1s0 up
  - ip route add default via ${gateway} || true
  - echo "${vm_name} ready" > /tmp/sdn-ready
USERDATA

    # network-config (disable cloud-init networking, use runcmd above)
    cat > "${ci_dir}/network-config" <<NETCFG
version: 2
ethernets:
  enp1s0:
    dhcp4: false
NETCFG

    # meta-data
    cat > "${ci_dir}/meta-data" <<META
instance-id: ${vm_name}
local-hostname: ${vm_name}
META

    # Create cloud-init ISO
    cloud-localds -v \
        --network-config "${ci_dir}/network-config" \
        "${ci_dir}/cloud-init.iso" \
        "${ci_dir}/user-data" \
        "${ci_dir}/meta-data"
}

create_vm() {
    local vm_name=$1
    local ip_addr=$2
    local gateway=$3
    local bridge=$4
    local vcpus=${5:-$VM_VCPUS}
    local memory=${6:-$VM_MEMORY}
    local role=${7:-"workload"}

    # Check if VM already exists
    if virsh dominfo "${vm_name}" &>/dev/null; then
        log "VM ${vm_name} already exists, skipping"
        return
    fi

    log "Creating VM: ${vm_name} (${ip_addr}, bridge=${bridge}, ${vcpus}vCPU, ${memory}MB)"

    # Create disk from base image
    local disk="${VM_DIR}/${vm_name}.qcow2"
    qemu-img create -f qcow2 -b "${BASE_IMG}" -F qcow2 "${disk}"

    # Create cloud-init
    create_cloud_init "${vm_name}" "${ip_addr}" "${gateway}" "${role}"

    # Create VM
    virt-install \
        --name "${vm_name}" \
        --vcpus "${vcpus}" \
        --memory "${memory}" \
        --disk "path=${disk},format=qcow2" \
        --disk "path=${VM_DIR}/${vm_name}/cloud-init.iso,device=cdrom" \
        --network "bridge=${bridge},model=virtio,virtualport_type=openvswitch" \
        --os-variant ubuntu24.04 \
        --graphics none \
        --console pty,target_type=serial \
        --noautoconsole \
        --import \
        --quiet

    log "VM ${vm_name} created and starting"
}

# --- Main Test Environment ---

create_workload_vms() {
    log "=== Creating workload VMs ==="

    # VPC: 10.100.0.0/16
    # Subnet web-tier: 10.100.1.0/24  (gateway 10.100.1.1)
    # Subnet app-tier: 10.100.2.0/24  (gateway 10.100.2.1)

    # Web tier VMs
    create_vm "sdn-web-1" "10.100.1.11" "10.100.1.1" "${OVS_BRIDGE_INT}" 1 512 "workload"
    create_vm "sdn-web-2" "10.100.1.12" "10.100.1.1" "${OVS_BRIDGE_INT}" 1 512 "workload"

    # App tier VMs
    create_vm "sdn-app-1" "10.100.2.11" "10.100.2.1" "${OVS_BRIDGE_INT}" 1 512 "workload"
}

create_nfv_vms() {
    log "=== Creating NFV VMs (small VNFs) ==="

    # VNF: IDS (simulated with tcpdump/nftables)
    create_vm "sdn-vnf-ids" "10.100.10.11" "10.100.10.1" "${OVS_BRIDGE_INT}" 1 256 "vnf"

    # VNF: WAF (simulated with nftables)
    create_vm "sdn-vnf-waf" "10.100.10.12" "10.100.10.1" "${OVS_BRIDGE_INT}" 1 256 "vnf"
}

create_gateway_vm() {
    log "=== Creating gateway VM ==="
    # Gateway VM has interfaces on both internal and external bridges
    create_vm "sdn-gateway" "10.100.0.1" "0.0.0.0" "${OVS_BRIDGE_EXT}" 1 512 "gateway"
}

apply_ovs_flows() {
    log "=== Programming OVS flows for test topology ==="

    # Table 0: Classification
    sudo ovs-ofctl add-flow ${OVS_BRIDGE_INT} "table=0,priority=100,dl_type=0x0800,nw_dst=10.100.1.0/24,actions=goto_table:10"
    sudo ovs-ofctl add-flow ${OVS_BRIDGE_INT} "table=0,priority=100,dl_type=0x0800,nw_dst=10.100.2.0/24,actions=goto_table:10"
    sudo ovs-ofctl add-flow ${OVS_BRIDGE_INT} "table=0,priority=0,actions=NORMAL"

    # Table 10: Firewall (allow HTTP, deny rest from external)
    sudo ovs-ofctl add-flow ${OVS_BRIDGE_INT} "table=10,priority=200,dl_type=0x0800,nw_proto=6,tp_dst=80,actions=goto_table:20"
    sudo ovs-ofctl add-flow ${OVS_BRIDGE_INT} "table=10,priority=200,dl_type=0x0800,nw_proto=6,tp_dst=443,actions=goto_table:20"
    sudo ovs-ofctl add-flow ${OVS_BRIDGE_INT} "table=10,priority=100,dl_type=0x0800,nw_src=10.100.0.0/16,actions=goto_table:20"
    sudo ovs-ofctl add-flow ${OVS_BRIDGE_INT} "table=10,priority=0,actions=drop"

    # Table 20: Forwarding
    sudo ovs-ofctl add-flow ${OVS_BRIDGE_INT} "table=20,priority=0,actions=NORMAL"

    log "OVS flows programmed"
    sudo ovs-ofctl dump-flows ${OVS_BRIDGE_INT}
}

print_status() {
    log "=== Test Environment Status ==="
    echo ""
    echo "VMs:"
    virsh list --all | grep sdn-
    echo ""
    echo "OVS Bridges:"
    sudo ovs-vsctl show
    echo ""
    echo "SSH access: ssh -i ${SSH_KEY} sdn@<vm-ip>"
    echo ""
    echo "Test connectivity:"
    echo "  ssh -i ${SSH_KEY} sdn@10.100.1.11 ping -c3 10.100.1.12  # same subnet"
    echo "  ssh -i ${SSH_KEY} sdn@10.100.1.11 ping -c3 10.100.2.11  # cross subnet"
    echo "  ssh -i ${SSH_KEY} sdn@10.100.1.11 iperf3 -s &            # start iperf server"
    echo "  ssh -i ${SSH_KEY} sdn@10.100.1.12 iperf3 -c 10.100.1.11  # bandwidth test"
}

# --- Cleanup ---
cleanup() {
    log "=== Cleaning up test environment ==="
    for vm in sdn-web-1 sdn-web-2 sdn-app-1 sdn-vnf-ids sdn-vnf-waf sdn-gateway; do
        if virsh dominfo "${vm}" &>/dev/null; then
            virsh destroy "${vm}" 2>/dev/null || true
            virsh undefine "${vm}" --remove-all-storage 2>/dev/null || true
            log "Removed VM: ${vm}"
        fi
    done
    for br in ${OVS_BRIDGE_INT} ${OVS_BRIDGE_EXT} ${OVS_BRIDGE_TUN}; do
        sudo ovs-vsctl --if-exists del-br "$br"
    done
    sudo rm -rf "${VM_DIR}"
    log "Cleanup complete"
}

# --- Entry point ---
case "${1:-setup}" in
    setup)
        check_deps
        setup_directories
        download_base_image
        generate_ssh_key
        setup_ovs_bridges
        create_workload_vms
        create_nfv_vms
        create_gateway_vm
        apply_ovs_flows
        print_status
        ;;
    cleanup)
        cleanup
        ;;
    status)
        print_status
        ;;
    *)
        echo "Usage: $0 {setup|cleanup|status}"
        exit 1
        ;;
esac
