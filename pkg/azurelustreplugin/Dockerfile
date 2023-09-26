# Copyright 2019 The Kubernetes Authors.
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

FROM ubuntu:22.04

RUN apt-get update && apt --only-upgrade -y install tar libudev1 libsystemd0
RUN apt -y install gpg curl ca-certificates iproute2 kmod libc-dev-bin libc6 libc6-dev  \
  libcrypt-dev libgdbm-compat4 libgdbm6 libkmod2     \
  libmysqlclient21 libnetsnmptrapd40 libnl-3-200 libnl-genl-3-200 libnsl-dev libpci-dev libpci3    \
  libperl5.34 libsensors-config libsensors-dev libsensors5 libsnmp-base libsnmp-dev libsnmp40      \
  libssl-dev libtirpc-dev libudev-dev libwrap0 libwrap0-dev libyaml-0-2 linux-base linux-base-sgx  \
  linux-libc-dev mysql-common pci.ids perl perl-modules-5.34 rpcsvc-proto zlib1g-dev
RUN apt-get clean all

# Install all Lustre packages dependencies
# RUN apt-get install -y --no-install-recommends ca-certificates curl distro-info-data gpg gpgconf kmod libasn1-8-heimdal           \
#   libassuan0 libcurl4 libexpat1 libgssapi-krb5-2 libgssapi3-heimdal libhcrypto4-heimdal libheimbase1-heimdal                      \
#   libheimntlm0-heimdal libhx509-5-heimdal libk5crypto3 libkeyutils1 libkmod2 libkrb5-26-heimdal libkrb5-3 libkrb5support0         \
#   libldap-2.4-2 libldap-common libmpdec2 libnghttp2-14 libpsl5 libpython3-stdlib libpython3.6-minimal libpython3.6-stdlib         \
#   libreadline7 libroken18-heimdal librtmp1 libsasl2-2 libsasl2-modules-db libsqlite3-0 libssl1.1 libwind0-heimdal                 \
#   libyaml-0-2 linux-base linux-base-sgx mime-support openssl python3 python3-minimal python3.6 python3.6-minimal readline-common  

WORKDIR "/app"

LABEL maintainers="vinli-cn;fuxiuyin;joe-atzinger;Justin-Jin"
LABEL description="Azure Lustre CSI driver"

COPY "./_output/azurelustreplugin" "/app/azurelustreplugin"
COPY "./pkg/azurelustreplugin/entrypoint.sh" "/app/entrypoint.sh"

RUN chmod +x "/app/entrypoint.sh"

ENTRYPOINT ["/app/entrypoint.sh", "/app/azurelustreplugin"]
