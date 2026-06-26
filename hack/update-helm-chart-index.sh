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

# Regenerates charts/index.yaml deterministically from git tags and the local
# charts/latest/ package.  No --merge is used — the index is built from
# scratch every time, so it never gets out of sync.
#
# For each v* tag that contains a chart .tgz at charts/<tag>/, the package is
# extracted from git and included in the index with a URL pointing at the tag.
# The local charts/latest/ package (if present) is included with a URL
# pointing at the current branch.
#
# Tags that predate the Helm chart work (no charts/<tag>/ directory) are
# silently skipped.
#
# Usage:
#   hack/update-helm-chart-index.sh
#
# Environment variables:
#   REPO_URL  - Base GitHub raw content URL (default: https://raw.githubusercontent.com/kubernetes-sigs/azurelustre-csi-driver)
#   BRANCH    - Branch name used as the initial base in --url (default: main)

PKG_ROOT="$(git rev-parse --show-toplevel)"
readonly PKG_ROOT

REPO_URL="${REPO_URL:-https://raw.githubusercontent.com/kubernetes-sigs/azurelustre-csi-driver}"
BRANCH="${BRANCH:-main}"

if [[ -z "$(command -v helm)" ]]; then
  echo "Cannot find helm. Please install helm first."
  exit 1
fi

# Output path for the generated index. Overridable (e.g. by
# verify-helm-chart-index.sh) so verification can regenerate into a temp file
# instead of overwriting the committed index.
INDEX="${OUTPUT_INDEX:-${PKG_ROOT}/charts/index.yaml}"

temp_dir=$(mktemp -d)
trap 'rm -rf "${temp_dir}"' EXIT

# Collect chart .tgz files from version tags.
echo "Scanning version tags for chart packages ..."
for tag in $(git tag -l 'v*' --sort=version:refname); do
  tgz_path="charts/${tag}/azurelustre-csi-driver-${tag}.tgz"

  # Check if this tag has a chart package (skip legacy tags that predate charts)
  if ! git cat-file -e "${tag}:${tgz_path}" 2>/dev/null; then
    continue
  fi

  echo "  ${tag}: extracting ${tgz_path}"
  mkdir -p "${temp_dir}/${tag}"
  git show "${tag}:${tgz_path}" > "${temp_dir}/${tag}/azurelustre-csi-driver-${tag}.tgz"
done

# Copy the local latest package (development chart).
if [[ -f "${PKG_ROOT}/charts/latest/azurelustre-csi-driver-v0.0.0.tgz" ]]; then
  echo "  latest: using local package"
  mkdir -p "${temp_dir}/latest"
  cp "${PKG_ROOT}/charts/latest/azurelustre-csi-driver-v0.0.0.tgz" "${temp_dir}/latest/"
fi

# Also copy any locally-present versioned chart package (exists during
# the release process, before the tag is pushed).
for dir in "${PKG_ROOT}"/charts/v*/; do
  [[ -d "${dir}" ]] || continue
  version=$(basename "${dir}")
  tgz="${dir}azurelustre-csi-driver-${version}.tgz"
  if [[ -f "${tgz}" && ! -f "${temp_dir}/${version}/azurelustre-csi-driver-${version}.tgz" ]]; then
    echo "  ${version}: using local package (not yet tagged)"
    mkdir -p "${temp_dir}/${version}"
    cp "${tgz}" "${temp_dir}/${version}/"
  fi
done

# Build the index from the collected packages.
# Use a dummy --url that we will rewrite per-entry below.
echo
echo "Generating charts/index.yaml ..."
helm repo index \
  --url "${REPO_URL}/${BRANCH}/charts/" \
  "${temp_dir}"

# Rewrite versioned chart URLs from branch to the matching git tag.
# e.g. .../main/charts/v0.5.0/... -> .../v0.5.0/charts/v0.5.0/...
# The regex v[^/]+ intentionally does not match "latest", so the
# development chart URL stays on main.
sed -i "s|${BRANCH}/charts/\\(v[^/]\\+\\)/|\\1/charts/\\1/|" "${temp_dir}/index.yaml"

cp "${temp_dir}/index.yaml" "${INDEX}"
echo "Updated ${INDEX}"
