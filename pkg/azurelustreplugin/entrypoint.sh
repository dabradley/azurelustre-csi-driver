#!/bin/bash

# Copyright 2022 The Kubernetes Authors.
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

#
# Shell script to install Lustre client kernel modules and launch CSI driver
#   $1 is the path to the CSI driver.
#

set -o xtrace
set -o errexit
set -o pipefail
set -o nounset

function add_net_interfaces() {
  echo "$(date -u) Determining ethernet interfaces."
  echo "$(date -u) Route table is:"
  ip route list
  interface_list=$(ip route show | sed -n 's/.*\s\+dev\s\+\([^ ]\+\).*/\1/p' | sort -u)
  ethernet_interfaces=()
  for interface in ${interface_list}; do
    interface_info=$(ip link show "${interface}")
    # Skip interfaces that are not needed
    if [[ "${interface_info}" =~ 'SLAVE' ]]; then
      echo "$(date -u) Not adding slave interface: ${interface}"
    elif [[ "${interface_info}" =~ 'link-netns' ]]; then
      echo "$(date -u) Not adding namespaced interface: ${interface}"
    elif [[ "${interface_info}" =~ 'UNKNOWN' ]]; then
      echo "$(date -u) Not adding state unknown interface: ${interface}"
    # Add remaining link/ether interface
    elif [[ "${interface_info}" =~ 'link/ether' ]]; then
      echo "$(date -u) Including ethernet interface: ${interface}"
      ethernet_interfaces+=("${interface}")
    else
      echo "$(date -u) Skipping non-ethernet interface: ${interface}"
    fi
  done
  echo "$(date -u) List of found ethernet interfaces is: ${ethernet_interfaces[*]}"

  if [[ "${#ethernet_interfaces[@]}" -eq 0 ]]; then
    echo "$(date -u) Cannot find any ethernet network interface"
    exit 1
  fi

  for interface in "${ethernet_interfaces[@]}"; do
    if lnetctl net show --net tcp | grep -q "\b${interface}\b"; then
      echo "$(date -u) Interface already added, skipping: ${interface}"
    else
      echo "$(date -u) Adding interface: ${interface}"
      lnetctl net add --net tcp --if "${interface}"
    fi
  done
}

# Detect OS family from container's os-release
osFamily=""
if [[ -f /etc/os-release ]]; then
  # shellcheck disable=SC1091 # /etc/os-release is a runtime file
  osID=$(. /etc/os-release; echo "${ID:-}")
  if [[ "${osID}" == "azurelinux" || "${osID}" == "mariner" ]]; then
    osFamily="azurelinux"
  elif [[ "${osID}" == "ubuntu" || "${osID}" == "debian" ]]; then
    osFamily="ubuntu"
  fi
fi
if [[ -z "${osFamily}" ]]; then
  echo "$(date -u) Error: Unsupported container OS: ${osID:-unknown}. Supported: ubuntu, debian, azurelinux, mariner."
  exit 1
fi
echo "$(date -u) Detected OS family: ${osFamily}"

# Update CA certificates to ensure HTTPS connections work
if [[ "${osFamily}" == "azurelinux" ]]; then
  update-ca-trust
else
  update-ca-certificates
fi

installClientPackages=${AZURELUSTRE_CSI_INSTALL_LUSTRE_CLIENT:-yes}
echo "installClientPackages: ${installClientPackages}"

if [[ "${installClientPackages}" == "yes" ]]; then
  if [[ -z ${LUSTRE_VERSION} ]]; then
    echo "LUSTRE_VERSION environment variable is not set"
    exit 1
  fi
  requiredLustreVersion=${LUSTRE_VERSION}
  echo "requiredLustreVersion: ${requiredLustreVersion}"

  if [[ -z ${CLIENT_SHA_SUFFIX} ]]; then
    echo "CLIENT_SHA_SUFFIX environment variable is not set"
    exit 1
  fi
  requiredClientSha="${CLIENT_SHA_SUFFIX}"
  echo "requiredClientSha: ${requiredClientSha}"

  # Construct package version and name (OS-specific naming convention)
  if [[ "${osFamily}" == "azurelinux" ]]; then
    # Azure Linux RPM packages use underscores: 2.16.1_21_g153e389
    pkgVersion="${requiredLustreVersion}_${requiredClientSha//-/_}"
  else
    # Ubuntu deb packages use dashes: 2.15.7-33-g79ddf99
    pkgVersion="${requiredLustreVersion}-${requiredClientSha}"
  fi
  echo "pkgVersion: ${pkgVersion}"

  pkgName="amlfs-lustre-client-${pkgVersion}"
  echo "pkgName: ${pkgName}"

  echo "$(date -u) Command line arguments: $*"

  kernelVersion=$(uname -r)

  if [[ "${osFamily}" == "azurelinux" ]]; then
    # ----- Azure Linux path -----

    # Host OS validation: check ID and major VERSION_ID
    # shellcheck disable=SC1091 # /etc/os-release is a runtime file
    containerVersionID=$(. /etc/os-release; echo "${VERSION_ID:-}")
    # shellcheck disable=SC1091 # /etc/host-os-release is mounted at runtime
    # shellcheck disable=SC2031 # intentional: read ID in a subshell to avoid polluting the current scope
    hostID=$(. /etc/host-os-release; echo "${ID:-}")
    # shellcheck disable=SC1091 # /etc/host-os-release is mounted at runtime
    # shellcheck disable=SC2031 # intentional: read VERSION_ID in a subshell to avoid polluting the current scope
    hostVersionID=$(. /etc/host-os-release; echo "${VERSION_ID:-}")
    if [[ -z "${containerVersionID}" ]]; then
      echo "Could not determine container OS VERSION_ID from /etc/os-release"
      exit 1
    fi
    if [[ -z "${hostVersionID}" ]]; then
      echo "Could not determine host OS VERSION_ID from /etc/host-os-release"
      exit 1
    fi
    if [[ "${hostID}" != "azurelinux" && "${hostID}" != "mariner" ]]; then
      echo "Incompatible host OS detected: ${hostID}, expected: azurelinux"
      exit 1
    fi
    # Compare major version (e.g., "3" from "3.0" or "3.0.20260517")
    if [[ "${hostVersionID%%.*}" != "${containerVersionID%%.*}" ]]; then
      echo "Incompatible host OS version: ${hostVersionID}, expected major version: ${containerVersionID%%.*}"
      exit 1
    fi

    echo "$(date -u) Installing Lustre client packages for OS=azurelinux${containerVersionID%%.*}, kernel=${kernelVersion}"

    # Repo setup: import GPG key and add yum repo
    echo "$(date -u) Adding Microsoft package repository for Lustre client modules."
    rpm --import https://packages.microsoft.com/keys/microsoft.asc
    cat > /etc/yum.repos.d/amlfs.repo <<REPOEOF
[amlfs]
name=Azure Lustre Packages
baseurl=https://packages.microsoft.com/yumrepos/amlfs-al${containerVersionID%%.*}
enabled=1
gpgcheck=1
gpgkey=https://packages.microsoft.com/keys/microsoft.asc
REPOEOF

    # Transform kernel version for RPM package name:
    # e.g. 6.6.139.1-1.azl3.x86_64 -> 6.6.139.1.1.azl3
    kernelPkgVersion=$(echo "${kernelVersion}" | sed -e "s/\.$(uname -m)$//" | sed -re 's/[-_]/\./g')

    installPkgName="${pkgName}-${kernelPkgVersion}"

    echo "$(date -u) Installing Lustre client modules: ${installPkgName}"

    tries=3
    sleep_before_retry=15
    install_success=false
    while [[ tries -gt 0 ]]; do
      if ! tdnf install -y --disablerepo="*" --enablerepo="amlfs" --enablerepo="azurelinux-official-base" "${installPkgName}"; then
        echo "$(date -u) Error installing Lustre client modules. Will try removing existing versions"
        if type lustre_rmmod >/dev/null 2>&1 && ! lustre_rmmod; then
          echo "$(date -u) Error: Unable to unload running module. Are there still mounted Lustre filesystems on this node? Old Lustre client version may continue running."
        fi
        # Query exact RPM names for logging and removal
        mapfile -t existing_pkgs < <(rpm -qa '*lustre-client*' 2>/dev/null || true)
        if [[ ${#existing_pkgs[@]} -gt 0 ]]; then
          echo "$(date -u) The following existing versions of the Lustre client are installed and will be removed: ${existing_pkgs[*]}"
          echo "$(date -u) Uninstalling existing Lustre client versions."
          tdnf remove -y "${existing_pkgs[@]}" || true
        fi
        tries=$((tries - 1))
        sleep "${sleep_before_retry}"
        sleep_before_retry=$((sleep_before_retry * 2))
      else
        install_success=true
        break
      fi
    done

    echo "$(date -u) Install success: ${install_success}, Tries left: ${tries}"

    if ! ${install_success}; then
      echo "$(date -u) Error: Could not install necessary Lustre drivers for: ${installPkgName}"
    else
      echo "$(date -u) Installed Lustre client packages for: ${installPkgName}"
    fi

  else
    # ----- Ubuntu path -----

    # shellcheck disable=SC1091,SC2154 # /etc/os-release sets VERSION_CODENAME
    osReleaseCodeName=$(. /etc/os-release; echo "${VERSION_CODENAME}")
    if [[ -z "${osReleaseCodeName}" ]]; then
      echo "Could not determine OS release codename"
      exit 1
    fi

    # shellcheck disable=SC1091 # /etc/host-os-release exists only at runtime inside the container
    if ! grep -q -R "${osReleaseCodeName}" /etc/host-os-release; then
      # shellcheck disable=SC2031 # intentional: read VERSION_CODENAME in a subshell to avoid polluting the current scope
      hostCodeName=$(. /etc/host-os-release; echo "${VERSION_CODENAME}")
      if [[ "${hostCodeName}" == "focal" && "${osReleaseCodeName}" == "jammy" ]]; then
        echo "Allowing jammy container on focal host, this usage is deprecated and will be removed in future"
      else
        echo "Incompatible host OS detected: ${hostCodeName}, expected: ${osReleaseCodeName}"
        exit 1
      fi
    fi

    echo "$(date -u) Installing Lustre client packages for OS=${osReleaseCodeName}, kernel=${kernelVersion} "

    ARCH=$(uname -m)
    if [[ "${ARCH}" != "x86_64" && "${ARCH}" != "aarch64" ]]; then
      echo "$(date -u) Error: Unsupported architecture: ${ARCH}"
      exit 1
    fi
    if [[ "${ARCH}" == "x86_64" ]]; then
      ARCH="amd64"
    else
      ARCH="arm64"
    fi

    # Prefer the Ubuntu CDN; keep the Azure mirror as a lower-priority apt fallback.
    /app/useAzureAptMirror.sh

    echo "$(date -u) Adding Microsoft package repository for Lustre client modules, architecture=${ARCH}."
    curl -sL https://packages.microsoft.com/keys/microsoft.asc | gpg --dearmor | tee /etc/apt/trusted.gpg.d/microsoft.gpg > /dev/null
    echo "deb [arch=${ARCH}] https://packages.microsoft.com/repos/amlfs-${osReleaseCodeName}/ ${osReleaseCodeName} main" | tee /etc/apt/sources.list.d/amlfs.list
    apt-get update

    echo "$(date -u) Installing Lustre client modules: ${pkgName}=${kernelVersion}"

    tries=3
    sleep_before_retry=15
    install_success=false
    while [[ tries -gt 0 ]]; do
      # grub issue
      # https://stackoverflow.com/questions/40748363/virtual-machine-apt-get-grub-issue/40751712
      if ! DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends -o DPkg::options::="--force-confdef" -o DPkg::options::="--force-confold" \
        "${pkgName}=${kernelVersion}"; then
        echo "$(date -u) Error installing Lustre client modules. Will try removing existing versions"
        # Check if lustre_rmmod is available, attempt to unload the modules if so.
        # If modules are already uninstalled, this will still pass
        if type lustre_rmmod >/dev/null 2>&1 && ! lustre_rmmod; then
          echo "$(date -u) Error: Unable to unload running module. Are there still mounted Lustre filesystems on this node? Old Lustre client version may continue running."
        fi
        if existing_versions=$(dpkg-query --showformat=' ${Package}=${Version}' --show '*lustre-client*'); then
          echo  "$(date -u) The following existing versions of the Lustre client are installed and will be removed:${existing_versions}"
        fi
        echo "$(date -u) Uninstalling existing Lustre client versions."
        apt-get remove --purge -y '*lustre-client*' || true
        tries=$((tries - 1))
        sleep "${sleep_before_retry}"
        sleep_before_retry=$((sleep_before_retry * 2))
      else
        install_success=true
        break
      fi
    done

    echo "$(date -u) Install success: ${install_success}, Tries left: ${tries}"

    if ! ${install_success}; then
      echo "$(date -u) Error: Could not install necessary Lustre drivers for: ${pkgName}=${kernelVersion}"
    else
      echo "$(date -u) Installed Lustre client packages for: ${pkgName}=${kernelVersion}"
    fi

  fi

  init_lnet="true"

  if lsmod | grep "^lnet"; then
    if lnetctl net show --net tcp | grep interfaces; then
      echo "$(date -u) LNet is loaded skip the load"
      echo "$(date -u) Adding missing interfaces"
      add_net_interfaces
      init_lnet="false"
    elif lnetctl net show | grep "net type: tcp"; then
    # There may be a default configuration with no interface.
    # This is configured by an old version CSI.
      lnetctl net del --net tcp
    fi
  fi

  if [[ "${init_lnet}" == "true" ]]; then
    echo "$(date -u) Loading the LNet."
    modprobe -v lnet
    modprobe -v ksocklnd skip_mr_route_setup=1
    lnetctl lnet configure

    add_net_interfaces

    echo "$(date -u) Done"
  fi

  echo "$(date -u) Enabling Lustre client kernel modules."
  modprobe -v mgc
  modprobe -v lustre

  echo "$(date -u) Enabled Lustre client kernel modules."

fi

echo "$(date -u) Entering Lustre CSI driver"

if [[ $# -eq 0 ]]; then
  echo "$(date -u) Error: No command provided to execute. Usage: entrypoint.sh <csi-driver-binary> [args...]"
  exit 1
fi

echo "Executing: $*"
exec "$@"
