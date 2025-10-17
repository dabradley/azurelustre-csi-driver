#!/bin/bash

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

echo "Verifying chart tgz files ..."
git config core.filemode false

issues_found=false

# Verify whether chart config has uncommitted changes
diff=$(git diff charts)
if [[ -n "${diff}" ]]; then
  echo "${diff}"
  issues_found=true
fi

for dir in charts/*/
do
  if [[ -d "${dir}" ]]; then
    echo "Linting chart in ${dir} ..."
    if ! helm lint "${dir}azurelustre-csi-driver"; then
      echo "Helm lint failed for chart in ${dir}"
      issues_found=true
      continue
    fi

    echo
    echo "Checking chart package in ${dir} ..."
    tgz_files=("${dir}"*.tgz)
    if (( ${#tgz_files[@]} == 0 )); then
      echo "No chart package found in ${dir}"
      issues_found=true
      continue
    elif (( ${#tgz_files[@]} > 1 )); then
      echo "Multiple chart packages found in ${dir}: ${tgz_files[*]}"
      issues_found=true
      continue
    fi
    file=${tgz_files[0]}
    if [[ -f "${file}" ]]; then
      echo "Verifying ${file} ..."
      temp_dir=$(mktemp -d)/
      tar -xzf "${file}" -C "${temp_dir}"
      diff=$(diff -qr "${dir}azurelustre-csi-driver" "${temp_dir}azurelustre-csi-driver" || true)
      if [[ -n "${diff}" ]]; then
        version=$(basename "${dir}")
        echo "Chart package ${file} is out of date, please run \"helm package charts/${version}/azurelustre-csi-driver -d charts/${version}/\" to update tgz file"
        issues_found=true
      fi
      echo "Removing temp dir ${temp_dir} ..."
      rm -rf "${temp_dir}"
    fi
    echo
  fi
done

if [[ "${issues_found}" = true ]]; then
  echo "Chart tgz file verification failed."
  exit 1
fi

echo "Chart tgz files verified."  
echo