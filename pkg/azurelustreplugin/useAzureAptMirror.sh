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

# Add Azure apt mirrors for Ubuntu packages

set -euo pipefail

ESCAPED_ARCHIVE_URL="http://archive\.ubuntu\.com/ubuntu/"
AZURE_ARCHIVE_URL="http://azure.archive.ubuntu.com/ubuntu/"

ESCAPED_PORTS_URL="http://ports\.ubuntu\.com/ubuntu-ports/"
AZURE_PORTS_URL="http://azure.ports.ubuntu.com/ubuntu-ports/"

if [[ -f /etc/apt/sources.list.d/ubuntu.sources ]]; then
    sed -i "s|${ESCAPED_ARCHIVE_URL}|${AZURE_ARCHIVE_URL}|g" /etc/apt/sources.list.d/ubuntu.sources
    sed -i "s|${ESCAPED_PORTS_URL}|${AZURE_PORTS_URL}|g" /etc/apt/sources.list.d/ubuntu.sources
elif [[ -f /etc/apt/sources.list ]]; then
    sed -i "s|${ESCAPED_ARCHIVE_URL}|${AZURE_ARCHIVE_URL}|g" /etc/apt/sources.list
    sed -i "s|${ESCAPED_PORTS_URL}|${AZURE_PORTS_URL}|g" /etc/apt/sources.list
fi
