#!/usr/bin/env bash
# Boots a real nested guest VM on a node VM, plugged into the VPC's OVS bridge.
# Usage: create-guest.sh <node-ip> <name> <ip> <mac> <gateway> [bridge] [mtu]
set -euo pipefail
NODE=$1 NAME=$2 IP=$3 MAC=$4 GW=$5 BRIDGE=${6:-br-production-v} MTU=${7:-1450}
KEY=$(cat "$HOME/.ssh/id_ed25519.pub")
ssh -o StrictHostKeyChecking=no ubuntu@"$NODE" "sudo bash -s" <<EOF
set -e
D=/var/lib/libvirt/images/guests
virsh dominfo $NAME >/dev/null 2>&1 && { echo "$NAME exists"; exit 0; }
S=\$(mktemp -d)
cat >\$S/meta-data <<M
instance-id: $NAME
local-hostname: $NAME
M
cat >\$S/user-data <<U
#cloud-config
users:
  - name: ubuntu
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    lock_passwd: false
    ssh_authorized_keys: ["$KEY"]
chpasswd: {expire: false, users: [{name: ubuntu, password: sdntest, type: text}]}
ssh_pwauth: true
package_update: true
packages: [nginx, qemu-guest-agent]
write_files:
  - path: /var/www/html/index.html
    content: "<h1>$NAME ($IP) on $(ssh -o StrictHostKeyChecking=no ubuntu@$NODE hostname 2>/dev/null || echo node)</h1>\n"
runcmd:
  - systemctl enable --now qemu-guest-agent nginx
U
cat >\$S/network-config <<N
version: 2
ethernets:
  nic0:
    match: {macaddress: "$MAC"}
    set-name: eth0
    mtu: $MTU
    addresses: [$IP/24]
    routes: [{to: default, via: $GW}]
    nameservers: {addresses: [8.8.8.8]}
N
genisoimage -quiet -output \$D/$NAME-seed.iso -volid cidata -joliet -rock \$S/meta-data \$S/user-data \$S/network-config
qemu-img create -q -f qcow2 -b \$D/base.img -F qcow2 \$D/$NAME.qcow2 10G
virt-install --name $NAME --memory 1024 --vcpus 1 --cpu host-passthrough \
  --disk path=\$D/$NAME.qcow2 --disk path=\$D/$NAME-seed.iso,device=cdrom \
  --network bridge=$BRIDGE,virtualport_type=openvswitch,mac=$MAC,model=virtio \
  --os-variant ubuntu24.04 --graphics none --noautoconsole --import
rm -rf \$S
EOF
