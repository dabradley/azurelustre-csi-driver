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

PKG_ROOT="$(git rev-parse --show-toplevel)"
readonly PKG_ROOT

INDEX=${PKG_ROOT}/charts/index.yaml

issues_found=false

strip_timestamps() {
    # Remove created/generated timestamps so regenerated index can be compared
    # structurally against the committed one.
    sed '/^[[:space:]]*created:/d; /^generated:/d' "$1"
}

function check_index_structure() {
    # Verify that charts/index.yaml matches what the update script would generate.
    echo "== Checking index.yaml structure =="

    if [[ -z "$(command -v helm)" ]]; then
        echo "Cannot find helm. Skipping structural check."
        return 0
    fi

    local temp_dir
    temp_dir=$(mktemp -d)
    trap 'rm -rf "${temp_dir}"' RETURN

    # The regeneration below derives versioned entries from local git tags.
    # Once a released (versioned) chart is referenced in index.yaml, those tags
    # must be present locally or the regeneration would omit them and falsely
    # report drift. Fetch tags in that case (offline-tolerant no-op otherwise).
    if grep -qE 'azurelustre-csi-driver/v[0-9]' "${INDEX}"; then
        git fetch --tags --quiet 2>/dev/null || true
    fi

    # Regenerate into a temp file using the same tag-based logic as
    # update-helm-chart-index.sh (no --merge), WITHOUT touching the committed
    # index.yaml, so an interrupted run can never leave it modified.
    local regenerated="${temp_dir}/regenerated.yaml"
    if ! OUTPUT_INDEX="${regenerated}" "${PKG_ROOT}/hack/update-helm-chart-index.sh"; then
        echo "ERROR: update-helm-chart-index.sh failed"
        return 1
    fi

    # Compare the committed index against the regenerated one, ignoring timestamps.
    if ! diff -u \
        <(strip_timestamps "${INDEX}") \
        <(strip_timestamps "${regenerated}"); then
        echo ""
        echo "ERROR: charts/index.yaml is out of date!"
        echo "Run hack/update-helm-chart-index.sh to regenerate it."
        return 1
    fi

    echo "index.yaml structure is correct"
    echo ""
    return 0
}

function check_url() {
    local url=${1}
    local result
    result=$(curl -I -m 5 -s -w "%{http_code}\n" -o /dev/null "${url}")

    if [[ "${result}" -ne 200 ]]; then
        echo "Warning: ${url} returned HTTP ${result}"

        local path_after_charts="${url#*githubusercontent.com/*/azurelustre-csi-driver/*/charts/}"
        local local_path="${PKG_ROOT}/charts/${path_after_charts}"

        echo "Checking whether local file exists: ${local_path}"

        if [[ -f "${local_path}" ]]; then
            echo "Local file exists: ${local_path}"

            return 0
        else
            echo "Warning: Local file does not exist: ${local_path}"

            # Only ignore if this is a "latest" chart (development version)
            if [[ "${url}" == *"/latest/"* ]]; then
                echo "Ignoring missing remote URL for development 'latest' chart"
                return 0
            else
                echo "ERROR: Referenced chart URL is not accessible and local file does not exist"
                echo "ERROR: ${url} is invalid"
                echo "If the index.yaml is stale, run hack/update-helm-chart-index.sh to regenerate it."
                return 1
            fi
        fi
    else
        echo "${url} is valid"
    fi
}

function check_yaml() {
    local line url

    while IFS= read -r line; do
        url=$(awk '{print $2}' <<< "${line}")
        echo ""
        echo "Checking ${url} ..."

        if ! check_url "${url}"; then
            issues_found=true
        fi
    done < <(grep -F "https://" "${INDEX}")
}

echo "Verifying chart index ${INDEX} ..."
echo ""

if ! check_index_structure; then
    issues_found=true
fi

echo "== Checking URL reachability =="
check_yaml

echo ""
if [[ "${issues_found}" = true ]]; then
    echo "Chart index verification failed!"
    echo "If the index.yaml is out of date, run hack/update-helm-chart-index.sh to regenerate it."
    exit 1
else
    echo "Chart index verification succeeded!"
fi
