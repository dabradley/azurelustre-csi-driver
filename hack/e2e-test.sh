#!/usr/bin/env bash

# Copyright 2026 The Kubernetes Authors.
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

# Installs the Azure Lustre CSI driver from the local Helm chart and runs the
# Kubernetes external storage e2e tests.  Designed for developer use — handles
# helm installation, driver deployment, test execution, and cleanup.
#
# Usage:
#   hack/e2e-test.sh --lustre-fs-name <name> --lustre-mgs-ip <ip> [options]
#
# Required:
#   --lustre-fs-name   Lustre filesystem name (or set LUSTRE_FS_NAME)
#   --lustre-mgs-ip    Lustre MGS IP address  (or set LUSTRE_MGS_IP)
#
# Options:
#   --image-repo       Override driver image repository
#   --image-tag        Override driver image tag
#   --helm-args        Extra arguments passed to helm upgrade --install
#   --skip-install     Skip driver installation (assume already installed)
#   --skip-cleanup     Don't uninstall the driver after tests
#   --force            Remove existing non-helm driver installation before installing
#   --help             Show this help message

REPO_ROOT_PATH="$(git rev-parse --show-toplevel)"
readonly REPO_ROOT_PATH

LUSTRE_FS_NAME="${LUSTRE_FS_NAME:-}"
LUSTRE_MGS_IP="${LUSTRE_MGS_IP:-}"
IMAGE_REPO=""
IMAGE_TAG=""
HELM_EXTRA_ARGS=""
SKIP_INSTALL=false
SKIP_CLEANUP=false
FORCE=false
RELEASE_NAME="azurelustre"
NAMESPACE="kube-system"

usage() {
  sed -n '/^# Usage:/,/^$/p' "$0" | sed 's/^# \?//'
  exit "${1:-0}"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --lustre-fs-name)  LUSTRE_FS_NAME="$2";  shift 2 ;;
    --lustre-mgs-ip)   LUSTRE_MGS_IP="$2";   shift 2 ;;
    --image-repo)      IMAGE_REPO="$2";      shift 2 ;;
    --image-tag)       IMAGE_TAG="$2";       shift 2 ;;
    --helm-args)       HELM_EXTRA_ARGS="$2"; shift 2 ;;
    --skip-install)    SKIP_INSTALL=true;    shift ;;
    --skip-cleanup)    SKIP_CLEANUP=true;    shift ;;
    --force)           FORCE=true;           shift ;;
    --help)            usage 0 ;;
    *)                 echo "Unknown option: $1"; usage 1 ;;
  esac
done

if [[ -z "${LUSTRE_FS_NAME}" || -z "${LUSTRE_MGS_IP}" ]]; then
  echo "ERROR: --lustre-fs-name and --lustre-mgs-ip are required (or set LUSTRE_FS_NAME and LUSTRE_MGS_IP)."
  usage 1
fi

# --- Ensure helm is available ---
if ! command -v helm &>/dev/null; then
   echo "ERROR: helm not found. Please install Helm v3+ (https://helm.sh/docs/intro/install/) and re-run."
   exit 1
fi

echo "Using helm: $(helm version --short)"

# --- Cleanup on exit ---
# Only uninstall the driver on success. On failure we deliberately leave it
# installed: the external-storage tests may have left PVCs/PVs behind, and the
# driver is what detaches and deletes those volumes — tearing it down first
# would orphan them. Leaving it also preserves cluster state for debugging.
# Registered before install so a failure during install or rollout still runs it.
cleanup() {
  local exit_code=$?
  if [[ "${SKIP_CLEANUP}" == "false" && "${SKIP_INSTALL}" == "false" ]]; then
    if [[ "${exit_code}" -eq 0 ]]; then
      echo
      echo "=== Cleaning up: uninstalling driver ==="
      helm uninstall "${RELEASE_NAME}" -n "${NAMESPACE}" --wait 2>/dev/null || true
    else
      echo
      echo "=== Tests failed (exit ${exit_code}): leaving the driver installed for investigation. ==="
      echo "    Uninstalling now could orphan volumes the driver still needs to clean up."
      echo "    To tear down manually once the cluster is clean: helm uninstall ${RELEASE_NAME} -n ${NAMESPACE}"
    fi
  fi
  exit "${exit_code}"
}
trap cleanup EXIT

# --- Check for existing non-helm installation ---
if [[ "${SKIP_INSTALL}" == "false" ]]; then
  if kubectl get csidriver azurelustre.csi.azure.com &>/dev/null \
     && ! helm status "${RELEASE_NAME}" -n "${NAMESPACE}" &>/dev/null; then
    if [[ "${FORCE}" == "true" ]]; then
      echo "Existing non-helm driver installation detected. Removing it (--force) ..."
      "${REPO_ROOT_PATH}/deploy/uninstall-driver.sh"
    else
      echo "ERROR: The driver is already installed but not managed by helm."
      echo "Run deploy/uninstall-driver.sh first, or re-run with --force to remove it automatically."
      exit 1
    fi
  fi

  echo
  echo "=== Installing Azure Lustre CSI driver from local chart ==="

  CHART_DIR="${REPO_ROOT_PATH}/charts/latest/azurelustre-csi-driver"

  set_args=()
  if [[ -n "${IMAGE_REPO}" ]]; then
    set_args+=(--set "image.repository=${IMAGE_REPO}")
  fi
  if [[ -n "${IMAGE_TAG}" ]]; then
    set_args+=(--set "image.tag=${IMAGE_TAG}")
  fi

  # shellcheck disable=SC2086 # HELM_EXTRA_ARGS is intentionally word-split into multiple helm flags
  helm upgrade --install "${RELEASE_NAME}" "${CHART_DIR}" \
    --namespace "${NAMESPACE}" \
    --create-namespace \
    --wait \
    --timeout 10m \
    "${set_args[@]+"${set_args[@]}"}" \
    ${HELM_EXTRA_ARGS}

  echo "Driver installed successfully."
  echo

  echo "Waiting for controller rollout ..."
  controller_deploy="$(kubectl get deployment -n "${NAMESPACE}" -l app=csi-azurelustre-controller -o name)"
  [[ -n "${controller_deploy}" ]] || { echo "ERROR: controller deployment not found"; exit 1; }
  kubectl rollout status "${controller_deploy}" -n "${NAMESPACE}" --timeout=300s

  echo "Waiting for node daemonset rollout ..."
  for ds in $(kubectl get daemonset -n "${NAMESPACE}" -l app=csi-azurelustre-node -o name 2>/dev/null); do
    kubectl rollout status "${ds}" -n "${NAMESPACE}" --timeout=1800s
  done

  echo "All driver pods are ready."
fi

# --- Run the e2e tests ---
echo
echo "=== Running external e2e tests ==="

export LUSTRE_FS_NAME
export LUSTRE_MGS_IP
export REPO_ROOT_PATH

# Run as a child process (not exec) so the EXIT trap above runs cleanup.
# set -e plus cleanup's `exit "${exit_code}"` propagate run.sh's exit code.
"${REPO_ROOT_PATH}/test/external-e2e/run.sh"
