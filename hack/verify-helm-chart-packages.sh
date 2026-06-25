#!/usr/bin/env bash

# Copyright 2021 The Kubernetes Authors.
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

# Expand a non-matching glob to nothing (not the literal pattern) so the
# empty-array check below correctly detects a chart dir with no .tgz package.
shopt -s nullglob

PKG_ROOT="$(git rev-parse --show-toplevel)"
readonly PKG_ROOT

# Scratch dir for extracting chart packages; always cleaned up on exit so a
# mid-loop failure (e.g. a bad tar) cannot leak temp directories.
TMP_ROOT=$(mktemp -d)
trap 'rm -rf "${TMP_ROOT}"' EXIT

echo "Verifying chart tgz files ..."
git config core.filemode false

issues_found=false
failures=()

# If IMAGE_VERSION is a release version (not "latest"), fetch tags first so we
# have an accurate picture of whether the release has been tagged, then check.
IMAGE_VERSION=$(grep -m1 '^IMAGE_VERSION ?=' "${PKG_ROOT}/Makefile" | awk '{print $3}')
if [[ "${IMAGE_VERSION}" != "latest" ]]; then
  git fetch --tags --quiet

  if [[ -n "$(git tag -l "${IMAGE_VERSION}")" ]]; then
    echo "ERROR: IMAGE_VERSION is '${IMAGE_VERSION}' and tag '${IMAGE_VERSION}' already exists."
    echo "The release has been tagged and cannot be modified. Reset the release to 'latest' to return to development."
    issues_found=true
    failures+=("IMAGE_VERSION '${IMAGE_VERSION}' already tagged (needs release reset)")
  fi

  # Also check for a stale versioned chart directory whose .tgz is out of
  # date (someone ran update-helm-chart-packages.sh which no longer
  # repackages versioned dirs, so changes would not be reflected).
  versioned_dir="${PKG_ROOT}/charts/${IMAGE_VERSION}"
  if [[ -d "${versioned_dir}" ]]; then
    tgz="${versioned_dir}/azurelustre-csi-driver-${IMAGE_VERSION}.tgz"
    if [[ ! -f "${tgz}" ]]; then
      echo "ERROR: Versioned chart directory charts/${IMAGE_VERSION}/ exists but has no .tgz package."
      echo "Run 'hack/update-helm-chart-packages.sh' to create the missing package."
      issues_found=true
      failures+=("Missing .tgz in versioned chart dir charts/${IMAGE_VERSION}/")
    fi
  fi
fi

# Verify whether chart config has uncommitted changes
charts_diff=$(git diff charts)
if [[ -n "${charts_diff}" ]]; then
  echo "${charts_diff}"
  issues_found=true
  failures+=("Uncommitted changes under charts/")
fi

for dir in charts/*/
do
  if [[ -d "${dir}" ]]; then
    echo
    echo "Checking chart package in ${dir} ..."
    tgz_files=("${dir}"*.tgz)
    if (( ${#tgz_files[@]} == 0 )); then
      echo "No chart package found in ${dir}"
      issues_found=true
      failures+=("No chart package in ${dir}")
      continue
    elif (( ${#tgz_files[@]} > 1 )); then
      echo "Multiple chart packages found in ${dir}: ${tgz_files[*]}"
      issues_found=true
      failures+=("Multiple chart packages in ${dir}")
      continue
    fi
    file=${tgz_files[0]}
    if [[ -f "${file}" ]]; then
      echo "Verifying ${file} ..."
      temp_dir=$(mktemp -d "${TMP_ROOT}/extract.XXXXXX")/
      tar -xzf "${file}" -C "${temp_dir}"
      diff_output=$(diff -ru "${dir}azurelustre-csi-driver" "${temp_dir}azurelustre-csi-driver" || true)
      rm -rf "${temp_dir}"
      if [[ -n "${diff_output}" ]]; then
        echo "${diff_output}"
        echo
        echo "Chart package ${file} is out of date."
        echo "Run hack/update-helm-chart-packages.sh to repackage."
        echo
        issues_found=true
        failures+=("Chart package out of date: ${file}")
      fi
    fi
    echo
  fi
done

if [[ "${issues_found}" = true ]]; then
  echo "==== FAILURE SUMMARY ===="
  printf '  - %s\n' "${failures[@]}"
  echo "========================="
  echo "Chart tgz file verification failed."
  exit 1
fi

echo "Chart tgz files verified."