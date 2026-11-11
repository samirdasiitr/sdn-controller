#!/usr/bin/env bash
# Creates the SDN lab: an underlay libvirt network and three VMs.
#   sdn-cp      192.168.150.10  single-node k3s control plane (controller, UI)
#   sdn-node-1  192.168.150.11  "physical" node: OVS + libvirt/KVM + node agent
#   sdn-node-2  192.168.150.12  "physical" node: OVS + libvirt/KVM + node agent
# Node VMs get host-passthrough CPUs so they can boot real nested guest VMs.
set -euo pipefail

BASE_IMG=${BASE_IMG:-/var/lib/libvirt/images/noble-server-cloudimg-amd64.img}
DIR=${DIR:-/var/lib/libvirt/images/sdn-lab}
NET=sdn-underlay
PUBKEY=${PUBKEY:-$(cat "$HOME/.ssh/id_ed25519.pub")}
PASSWORD=${LAB_PASSWORD:-sdntest}
GW=192.168.150.1

declare -A IP=([sdn-cp]=192.168.150.10 [sdn-node-1]=192.168.150.11 [sdn-node-2]=192.168.150.12)
declare -A MAC=([sdn-cp]=52:54:00:15:00:10 [sdn-node-1]=52:54:00:15:00:11 [sdn-node-2]=52:54:00:15:00:12)
declare -A MEM=([sdn-cp]=4096 [sdn-node-1]=6144 [sdn-node-2]=6144)
declare -A CPU=([sdn-cp]=2 [sdn-node-1]=4 [sdn-node-2]=4)
declare -A DISK=([sdn-cp]=30G [sdn-node-1]=40G [sdn-node-2]=40G)

sudo mkdir -p "$DIR"

if ! sudo virsh net-info $NET >/dev/null 2>&1; then
  cat >/tmp/$NET.xml <<EOF
<network>
  <name>$NET</name>
  <forward mode='nat'/>
  <bridge name='virbr-sdn' stp='on' delay='0'/>
  <ip address='$GW' netmask='255.255.255.0'/>
</network>
EOF
  sudo virsh net-define /tmp/$NET.xml
  sudo virsh net-autostart $NET
  sudo virsh net-start $NET
fi

for vm in sdn-cp sdn-node-1 sdn-node-2; do
  if sudo virsh dominfo "$vm" >/dev/null 2>&1; then echo "$vm exists, skipping"; continue; fi
  seed=$(mktemp -d)
  cat >"$seed/meta-data" <<EOF
instance-id: $vm
local-hostname: $vm
EOF
  extra=""
  if [[ $vm != sdn-cp ]]; then
    extra="
  - openvswitch-switch
  - qemu-system-x86
  - qemu-utils
  - libvirt-daemon-system
  - libvirt-clients
  - virtinst
  - genisoimage
  - bridge-utils
  - iptables"
  fi
  cat >"$seed/user-data" <<EOF
#cloud-config
hostname: $vm
users:
  - name: ubuntu
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    lock_passwd: false
    ssh_authorized_keys:
      - $PUBKEY
chpasswd:
  expire: false
  users:
    - {name: ubuntu, password: $PASSWORD, type: text}
ssh_pwauth: true
package_update: true
packages:
  - qemu-guest-agent
  - curl
  - jq$extra
runcmd:
  - systemctl enable --now qemu-guest-agent
  - touch /var/lib/cloud/instance/sdn-lab-ready
EOF
  cat >"$seed/network-config" <<EOF
version: 2
ethernets:
  nic0:
    match: {macaddress: "${MAC[$vm]}"}
    set-name: eth0
    addresses: [${IP[$vm]}/24]
    routes:
      - {to: default, via: $GW}
    nameservers: {addresses: [8.8.8.8, 1.1.1.1]}
EOF
  sudo genisoimage -quiet -output "$DIR/$vm-seed.iso" -volid cidata -joliet -rock \
    "$seed/meta-data" "$seed/user-data" "$seed/network-config"
  rm -rf "$seed"
  sudo qemu-img create -q -f qcow2 -b "$BASE_IMG" -F qcow2 "$DIR/$vm.qcow2" "${DISK[$vm]}"
  cpu="host-model"
  [[ $vm != sdn-cp ]] && cpu="host-passthrough"
  sudo virt-install --name "$vm" --memory "${MEM[$vm]}" --vcpus "${CPU[$vm]}" --cpu "$cpu" \
    --disk path="$DIR/$vm.qcow2",format=qcow2 --disk path="$DIR/$vm-seed.iso",device=cdrom \
    --network network=$NET,model=virtio,mac="${MAC[$vm]}" \
    --os-variant ubuntu24.04 --graphics none --noautoconsole --import
done
echo "VMs created. SSH: ssh ubuntu@192.168.150.10 (key or password '$PASSWORD')"
