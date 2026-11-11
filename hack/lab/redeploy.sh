#!/usr/bin/env bash
# Rebuild lab images, load them into the VMs' containerd, and roll the workloads.
# Usage: hack/lab/redeploy.sh [agent|controller|all]
set -euo pipefail
cd "$(dirname "$0")/../.."
what=${1:-all}
export KUBECONFIG=${KUBECONFIG:-$HOME/.kube/sdn-lab.yaml}
CP=192.168.150.10; NODES="192.168.150.11 192.168.150.12"
SSH="ssh -o StrictHostKeyChecking=no ubuntu@"

if [[ $what == agent || $what == all ]]; then
  docker build -q -f Dockerfile.agent -t sdn/agent:lab . >/dev/null
  for n in $NODES; do docker save sdn/agent:lab | ${SSH}$n 'sudo k3s ctr images import - >/dev/null' & done; wait
  kubectl -n sdn-system rollout restart ds/sdn-agent
  kubectl -n sdn-system rollout status ds/sdn-agent --timeout=120s
fi
if [[ $what == controller || $what == all ]]; then
  docker build -q -f Dockerfile.controller -t sdn/controller:lab . >/dev/null
  docker save sdn/controller:lab | ${SSH}$CP 'sudo k3s ctr images import - >/dev/null'
  kubectl -n sdn-system rollout restart deploy/sdn-controller
  kubectl -n sdn-system rollout status deploy/sdn-controller --timeout=120s
fi
