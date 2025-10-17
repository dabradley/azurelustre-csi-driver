#!/bin/bash

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

# This script verifies that the helm chart files in the charts/ directory
# are consistent with the Kubernetes deployment files in the deploy/ directory.
# It checks that for each deploy file, the corresponding helm chart template
# generates the same Kubernetes manifests when rendered with helm template.
# It also checks that there are no unlisted files between the two directories.
#
# The REPOSITORY environment variable can be set to specify the image repository
# to use when rendering the helm charts, which can be useful for testing or
# development.
# If not set, it defaults to "mcr.microsoft.com/oss/v2/kubernetes-csi/azurelustre-csi".
# 
# The COLOR environment variable can be set to control diff coloring.
# It defaults to "always".

COLOR=${COLOR:-always}

if [[ -z "$(command -v pip)" ]]; then
  echo "Cannot find pip. Installing pip3..."
  apt install python3-pip -y
  update-alternatives --install /usr/bin/pip pip /usr/bin/pip3 1
fi

if [[ -z "$(command -v jq)" ]]; then
  echo "Cannot find jq. Installing jq..."
  apt install jq -y
fi

if [[ -z "$(command -v yq)" ]]; then
  echo "Cannot find yq. Installing yq..."
  pip install yq
fi

# Map of deploy files to chart template files
declare -A CHARTS_FOR_DEPLOY_FILE=(
["deploy/csi-azurelustre-node-jammy.yaml"]="templates/node-daemonset-jammy.yaml"
["deploy/csi-azurelustre-node-noble.yaml"]="templates/node-daemonset-noble.yaml"
["deploy/csi-azurelustre-controller.yaml"]="templates/controller-deployment.yaml"
["deploy/csi-azurelustre-driver.yaml"]="templates/csidriver.yaml"
["deploy/rbac-csi-azurelustre-controller.yaml"]="templates/controller-serviceaccount.yaml templates/controller-clusterrole.yaml templates/controller-clusterrolebinding.yaml templates/controller-secret-clusterrole.yaml templates/controller-secret-clusterrolebinding.yaml"
["deploy/rbac-csi-azurelustre-node.yaml"]="templates/node-serviceaccount.yaml templates/node-secret-clusterrole.yaml templates/node-secret-clusterrolebinding.yaml"
)

yq_format() {
  # Format yaml for diffing
  yq eval -o=props --properties-array-brackets '
    ... comments="" | # Remove comments from files
    del( # Helm-specific things can be ignored
        .metadata.labels.[
            "helm.sh/chart",
            "app.kubernetes.io/instance",
            "app.kubernetes.io/managed-by"
        ],
        .spec.template.metadata.labels.[
            "app.kubernetes.io/instance",
            "app.kubernetes.io/managed-by",
            "helm.sh/chart"
        ]
    ) |
    {(documentIndex | tostring): .} | # Split multi-doc yaml into separate documents
    style="" # Pretty print
    sort_keys(..) |
    (.. | select( (tag == "!!map" or tag =="!!seq") and length == 0)) = "" # This is necessary to detect empty maps and arrays
    ' "${1}"
}

check_unlisted_files() {
  # Check for files that aren't listed in the CHARTS_FOR_DEPLOY_FILE between deploy and charts
  version=${1}
  file_not_found=false

  echo "== Checking for unlisted files between deploy and charts for version: ${version} =="

  referenced_deploy_files=$(printf "%s\n" "${!CHARTS_FOR_DEPLOY_FILE[@]}" | sort)
  referenced_charts_files=$(printf "%s\n" "${CHARTS_FOR_DEPLOY_FILE[@]}" | sort)
  all_deploy_files=$(ls deploy/*.yaml)
  all_charts_files=$(ls charts/"${version}"/azurelustre-csi-driver/templates/*.yaml)

  for file in ${all_deploy_files}; do
    # Check for all actual deploy files in charts references
    if ! grep -q -R -F "${file}" - <<<"${referenced_deploy_files}"; then
      echo "File ${file} missing from list of charts files!"
      file_not_found=true
    fi
  done
  for file in ${all_charts_files}; do
    # Check for all actual chart files in deploy references
    if ! grep -q -R -F "templates/$(basename "${file}")" - <<<"${referenced_charts_files}"; then
      echo "File ${file} missing from list of deploy files!"
      file_not_found=true
    fi
  done
  if [[ "${file_not_found}" == true ]]; then
    echo "Inconsistent chart and deploy files found!"
    echo
    return 1
  fi
  echo "No unlisted files found between deploy and charts for version: ${version}"
  echo
  return 0
}

helm_template() {
  # Generate yaml from helm templates matching specific deploy file
  version=${1}
  version_override=${VERSION_OVERRIDE:-${DRIVER_VERSION}}
  deploy_file=${2}
  repository=${REPOSITORY:-mcr.microsoft.com/oss/v2/kubernetes-csi/azurelustre-csi}
  show_only=()
  for value in ${CHARTS_FOR_DEPLOY_FILE[${deploy_file}]}; do
    # Collect the templates that correspond to the deploy file
    show_only+=("--show-only" "${value}")
  done
  helm template \
    --set "fullnameOverride=csi-azurelustre" \
    --set "image.repository=${repository}" \
    --set "image.tag=${version_override}" \
    --namespace kube-system \
    chart-test \
    "${show_only[@]}" \
    ./charts/"${version}"/azurelustre-csi-driver/
}

chart_mapping() {
  # Return list of chart file names by document index
  # Meant for display purposes
  deploy_file=${1}
  IFS=" " read -r -a charts <<< "${CHARTS_FOR_DEPLOY_FILE[${deploy_file}]}"
  for index in "${!charts[@]}"; do
    echo
    echo -n "${index}: ${charts[${index}]}"
  done
  echo
}

pad_to_length() {
  # Pad string to length with spaces to align diff output
  str=${1}
  len=${2}
  printf "%-${len}s" "${str}"
}

diff_outputs() {
  # Show diff output between deploy file and generated chart template
  version=${1}
  deploy_file=${2}
  chart_list=$(chart_mapping "${deploy_file}")
  color_match=$"\\(\x1b\\[[0-9;]*m\\)\?" # Optionally match color codes at start of line
  IFS=" " read -r -a charts_for_deploy_file <<< "${CHARTS_FOR_DEPLOY_FILE[${deploy_file}]}"

  replacements=()
  for index in "${!charts_for_deploy_file[@]}"; do
    max_length=${#deploy_file}
    replacement=${charts_for_deploy_file[${index}]}
    if (( ${#replacement} > max_length )); then
      # Adjust padding length if replacement is longer than deploy file name
      max_length=${#replacement}
    fi
    replacement=$(pad_to_length "${replacement}" "${max_length}")
    # Replace start of chart diff line with chart file name
    replacements+=("-e" $"s|^${color_match}+${index}|\\1${replacement}: |")

    deploy_file_replacement=$(pad_to_length "${deploy_file}" "${max_length}")
    # Replace start of deploy diff line with deploy file name
    replacements+=("-e" $"s|^${color_match}-${index}|\\1${deploy_file_replacement}: |")
  done
  # Replace any remaining +{number} with deploy file name (extra documents in yaml beyond chart files)
  replacements+=("-e" $"s|^${color_match}-[0-9]\+|\\1${deploy_file}: |")

  if output=$(diff -L"Deploy file ${deploy_file}" -L"Charts:${chart_list}" --color="${COLOR}" -u --ignore-space-change <(yq_format "${deploy_file}") <(helm_template  "${version}" "${deploy_file}" | yq_format -)); then
    echo "No significant differences found"
    return 0
  else
    # Show diff with more readable file names
    sed "${replacements[@]}" <<<"${output}"
    return 1
  fi
}

check_file_diffs() {
  # Check for diffs between deploy files and generated chart templates
  version=${1}
  diff_issues=false
  sorted_deploy_files=$(printf "%s\n" "${!CHARTS_FOR_DEPLOY_FILE[@]}" | sort)
  echo "== Checking file differences between deploy and charts for version: ${version} =="
  for deploy_file in ${sorted_deploy_files}; do
    echo -n "Checking for differences between chart and deploy yaml for file: ${deploy_file}: "
    # Replace start of diff output with file names?
    if ! diff_outputs  "${version}" "${deploy_file}"; then
      diff_issues=true
    fi
  done
  if [[ "${diff_issues}" == true ]]; then
    return 1
  fi
  return 0
}

echo "Verifying helm chart files against deploy yamls ..."

issues_found=false

# Get expected image version from deploy files
DRIVER_VERSION=$(grep -ohP "image:.*azurelustre-csi:\K[^-]*" deploy/*.yaml | sort -u)
if [[ $(echo "${DRIVER_VERSION}" | wc -l) -ne 1 ]]; then
  echo "Failed to get expected image version from deploy files! Found versions:"
  echo "${DRIVER_VERSION}"
  exit 1
fi
echo "Using expected driver version: ${DRIVER_VERSION}"

for version in charts/*/; do
  version=$(basename "${version}")
  echo
  echo "=== Checking version: ${version} ==="

  if ! check_unlisted_files "${version}"; then
    issues_found=true
  fi

  if ! check_file_diffs "${version}"; then
    issues_found=true
  fi
done

echo

if [[ "${issues_found}" == true ]]; then
  echo "Helm chart verification failed!"
  exit 1
else
  echo "Helm chart verification succeeded!"
  echo
fi
