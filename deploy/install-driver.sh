#!/bin/bash

# Copyright 2020 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
set -euo pipefail

CONFIGMAP_NAME="csi-azurelustre-entrypoint"

function usage {
    echo "Usage: $0 [--custom-entrypoint <file>] [branch|local|url]"
    echo
    echo "branch: The branch from which to install the Azure Lustre CSI Driver to install. Default is 'main'."
    echo "local: Deploy out of local filesystem."
    echo
    echo "Options:"
    echo "  --custom-entrypoint <file>  Use a custom entrypoint script via ConfigMap instead of the"
    echo "                              built-in entrypoint. The file will be mounted into the CSI driver"
    echo "                              containers. Without this flag, the built-in entrypoint is used."
    echo
    echo "Example:"
    echo "$0 # install from remote main"
    echo "$0 main # install from remote branch or reference"
    echo "$0 local # install from locally checked out branch"
    echo "$0 https://raw.githubusercontent.com/csmuell/azurelustre-csi-driver/main # install from given remote repository/branch"
    echo "$0 --custom-entrypoint ./my-entrypoint.sh local # install with custom entrypoint"
    exit 1
}

custom_entrypoint=""

# Parse --custom-entrypoint flag (must come before positional args)
while [[ "$#" -gt 0 ]]; do
  case "$1" in
    --custom-entrypoint)
      if [[ "$#" -lt 2 ]]; then
        echo "Error: --custom-entrypoint requires a file path argument."
        usage
      fi
      custom_entrypoint="$2"
      shift 2
      ;;
    --help)
      usage
      ;;
    *)
      break
      ;;
  esac
done

if [[ "$#" -gt 1 ]]; then
  usage
fi

branch="main"
repo="https://raw.githubusercontent.com/kubernetes-sigs/azurelustre-csi-driver/${branch}/deploy"

if [[ "$#" -eq 1 ]]; then
  case "$1" in
    local)
      repo="$(git rev-parse --show-toplevel)/deploy"
      ;;
    http*)
      repo="${1}/deploy"
      ;;
    *)
      branch="${1}"
      repo="https://raw.githubusercontent.com/kubernetes-sigs/azurelustre-csi-driver/${branch}/deploy"
      ;;
  esac
fi

verify="${repo}/install-driver.sh"
if ! [ -f "${verify}" ]; then
  if ! curl -L -Is --fail "${verify}" > /dev/null; then
    echo "Unknown repository: ${repo} ${verify} does not exist."
    usage
  fi
fi

echo
echo "Installing Azure Lustre CSI Driver branch: $branch, repo: $repo ..."

# Handle custom entrypoint ConfigMap
configmap_changed="false"
if [[ -n "${custom_entrypoint}" ]]; then
  if [[ ! -f "${custom_entrypoint}" ]]; then
    echo "Error: Custom entrypoint file not found: ${custom_entrypoint}"
    exit 1
  fi
  echo "Creating ConfigMap '${CONFIGMAP_NAME}' from custom entrypoint: ${custom_entrypoint}"
  kubectl create configmap "${CONFIGMAP_NAME}" \
    --from-file=entrypoint.sh="${custom_entrypoint}" \
    -n kube-system --dry-run=client -o yaml | kubectl apply -f - | grep -q "configured\|created" && configmap_changed="true"
else
  # Clean up any previously created custom entrypoint ConfigMap
  if kubectl get configmap "${CONFIGMAP_NAME}" -n kube-system &>/dev/null; then
    kubectl delete configmap "${CONFIGMAP_NAME}" -n kube-system
    configmap_changed="true"
  fi
fi

kubectl delete -n kube-system daemonset csi-azurelustre-node --ignore-not-found

kubectl apply -f "$repo/rbac-csi-azurelustre-controller.yaml"
kubectl apply -f "$repo/rbac-csi-azurelustre-node.yaml"
kubectl apply -f "$repo/csi-azurelustre-driver.yaml"
kubectl apply -f "$repo/csi-azurelustre-controller.yaml"
kubectl apply -f "$repo/csi-azurelustre-node-jammy.yaml"
kubectl apply -f "$repo/csi-azurelustre-node-noble.yaml"

# Restart node DaemonSet pods only if the ConfigMap state changed.
# The custom entrypoint ConfigMap is only mounted into node DaemonSets,
# not the controller, so only node pods need restarting.
if [[ "${configmap_changed}" == "true" ]]; then
  echo "Custom entrypoint configuration changed, restarting node pods..."
  kubectl rollout restart daemonset csi-azurelustre-node-jammy -n kube-system
  kubectl rollout restart daemonset csi-azurelustre-node-noble -n kube-system
fi

kubectl rollout status deployment csi-azurelustre-controller -nkube-system --timeout=300s
kubectl rollout status daemonset csi-azurelustre-node-jammy -nkube-system --timeout=1800s
kubectl rollout status daemonset csi-azurelustre-node-noble -nkube-system --timeout=1800s
echo 'Azure Lustre CSI driver installed successfully.'
