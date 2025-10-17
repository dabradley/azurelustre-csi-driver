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

# (Re-)packages the charts/latest/ chart and regenerates the chart index.
# Versioned chart directories (charts/v*/) are NOT repackaged here — they are
# created during the release process and should not be modified after
# the fact.
#
# Usage:
#   hack/update-helm-chart-packages.sh

PKG_ROOT="$(git rev-parse --show-toplevel)"
readonly PKG_ROOT

if [[ -z "$(command -v helm)" ]]; then
  echo "Cannot find helm. Please install helm first."
  exit 1
fi

LATEST_DIR="${PKG_ROOT}/charts/latest"
CHART_DIR="${LATEST_DIR}/azurelustre-csi-driver"

if [[ ! -d "${CHART_DIR}" ]]; then
  echo "ERROR: ${CHART_DIR} does not exist"
  exit 1
fi

echo "Packaging chart in ${LATEST_DIR}/ ..."

# Remove any existing .tgz so we don't accumulate stale packages
rm -f "${LATEST_DIR}"/*.tgz

helm package "${CHART_DIR}" -d "${LATEST_DIR}/"

# helm package re-serializes Chart.yaml (may reorder keys and change quoting).
# Extract the packaged Chart.yaml back into the chart directory so the two stay
# byte-identical, which is what verify-helm-chart-packages.sh checks.
tgz_file=("${LATEST_DIR}"/*.tgz)
if [[ -f "${tgz_file[0]}" ]]; then
  tar -xzf "${tgz_file[0]}" -C "${LATEST_DIR}/" azurelustre-csi-driver/Chart.yaml
  echo "Synced Chart.yaml from packaged .tgz back to chart directory"
fi

echo
echo "Regenerating chart index ..."
"${PKG_ROOT}/hack/update-helm-chart-index.sh"

echo
echo "Chart packages and index updated."
