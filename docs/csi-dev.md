# Azure azurelustre Storage CSI driver development guide

## Make targets

| Target | Description |
| --- | --- |
| `make azurelustre` | Full rebuild of the binary (`-a` flag) |
| `make quicklustre` | Incremental build (faster for iteration) |
| `make container` | Full build + per-flavor docker images. Common CSI driver testing target. |
| `make docker-build` | Build per-flavor docker images only. Run after the Go binary already exists, e.g. `make quicklustre docker-build`. |
| `make build-push-latest` | Build the binary + per-flavor images + push with `latest` tag |
| `make build-push-latest-commit` | Build + push both `latest` and commit-tagged images |
| `make verify` | Run all checks (lint, vet, unit tests, etc.) |
| `make unit-test` | Unit tests only |
| `make sanity-test-local` | Sanity tests without Lustre hardware or Azure credentials |
| `make e2e-test` | E2E tests (requires `LUSTRE_FS_NAME`, `LUSTRE_MGS_IP`) |
| `make helm-chart-packages` | Repackage Helm charts + update index |
| `make clean` | Remove build artifacts |

## Clone repo and build locally

- Clone repo

```sh
mkdir -p $GOPATH/src/sigs.k8s.io
git clone https://github.com/kubernetes-sigs/azurelustre-csi-driver $GOPATH/src/sigs.k8s.io/azurelustre-csi-driver
```

- Build azurelustre Storage CSI driver

```sh
cd $GOPATH/src/sigs.k8s.io/azurelustre-csi-driver
make azurelustre
```

For faster iteration during development, use the quick build which skips the
`-a` (rebuild-all) flag and only recompiles changed packages:

```sh
make quicklustre
```

> **Note:** The build defaults to `CGO_ENABLED=1` and requires a C compiler
> (`gcc`) to be installed.

- Run verification before sending PR

```sh
make verify
```

- Update the Helm chart index after changing charts

If you modify any chart templates or values, repackage the charts and regenerate
the index:

```sh
make helm-chart-packages
```

Or equivalently:

```sh
hack/update-helm-chart-packages.sh
```

The `make verify` step will fail if the packages or index are out of date.

- Build container image locally for testing

Set up a personal ACR if you don't have one (one-time):

```sh
az group create --name <alias>-csi-infra --location <region> --subscription <subscription>
az acr create --name <alias>csiacr --resource-group <alias>-csi-infra --sku Basic --tags owner=<alias>
```

Log in before pushing:

```sh
az acr login --name <alias>csiacr
```

Build and push images:

```sh
REGISTRY="<alias>csiacr.azurecr.io" make build-push-latest
```

This pushes flavor-suffixed tags (e.g., `latest-jammy`, `latest-noble`), not just an
unsuffixed `:latest`.

To build for ARM64 (noble only — jammy doesn't support ARM64):

```sh
sudo apt install gcc-aarch64-linux-gnu                         # one-time: install cross-compiler
docker run --privileged --rm tonistiigi/binfmt --install arm64 # one-time: enable arm64 emulation for Docker
REGISTRY="<alias>csiacr.azurecr.io" make build-push-latest ARCH=arm64
```

> **Note:** The `azurelustre-csi-integration` repository on team ACRs (e.g.,
> `tip5csiacr`) is reserved for CI builds. Don't push to it manually.

Optionally, set up a purge task to avoid storage costs from old images:

```sh
az acr task create --name purge-old-images \
    --registry <alias>csiacr --resource-group <alias>-csi-infra \
    --cmd "acr purge --filter 'azurelustre-csi:.*' --ago 30d --untagged" \
    --schedule "0 4 * * 0" --context /dev/null
```

Each container build produces two images — one for each Ubuntu flavor:
`-jammy` (Ubuntu 22.04) and `-noble` (Ubuntu 24.04). Choose the one matching
your cluster's node OS when deploying.

For faster iteration, use the quick variant which does an incremental Go
build before the docker step:

```sh
REGISTRY="<alias>csiacr.azurecr.io" make quicklustre push-latest
```

To cross-build for a different architecture:

```sh
ARCH=arm64 REGISTRY="<alias>csiacr.azurecr.io" make build-push-latest
```

**Key Makefile variable overrides:**

| Variable | Default | Purpose |
| --- | --- | --- |
| `REGISTRY` | `azurelustre.azurecr.io` | Container registry to push to |
| `IMAGE_VERSION` | `latest` | Image version tag |
| `LATEST_TAG` | `latest` | The "latest" alias tag (e.g., `my-feature`) |
| `ARCH` | `amd64` | Target architecture (`amd64`, `arm64`) |

Note: This builds images from the local Dockerfile for development and testing.
Production images are built through DALEC (see below).

- Run the Kubernetes external storage e2e tests

The `e2e-test` target installs the driver from the local Helm chart, runs the
e2e test suite, and cleans up afterwards. You need a running cluster with
`kubectl` configured and a Lustre filesystem accessible from the cluster:

```sh
make e2e-test LUSTRE_FS_NAME=<fs-name> LUSTRE_MGS_IP=<mgs-ip>
```

Or run the script directly for more options (custom image, skip install/cleanup, etc.):

```sh
hack/e2e-test.sh --lustre-fs-name <fs-name> --lustre-mgs-ip <mgs-ip> --help
```

- Run unit tests

`make verify` runs linters, vet, and unit tests together. If you only want to
run the unit tests (e.g., iterating on a specific test without waiting for
linters), run them directly:

```sh
make unit-test
```

- Test locally without Lustre hardware or Azure credentials

The sanity test suite exercises the CSI driver's gRPC endpoints with both
mock-mount and mock-dynamic-provisioning enabled, so it requires neither a
real Lustre filesystem nor an `azure.json` credential file:

```sh
make sanity-test-local
```

This is the fastest way to validate driver behavior during development.

## DALEC image builds

Production images are built through [DALEC](https://github.com/Azure/dalec-build-defs),
not from the Dockerfiles in this repo. The Dockerfiles
(`pkg/azurelustreplugin/Dockerfile`) are only for local development and testing;
released images come from hand-authored DALEC specs under
`specs/kubernetes-csi-azurelustre/` in the dalec-build-defs repo — one spec per
version, per OS flavor, with no template/matrix generation for this project.
Each spec pins a `COMMIT` from this repo and builds with `make azurelustre-dalec`.

For local iteration, build and run the image from the Dockerfile with
`make container`. The DALEC images are produced during the release process — see
[RELEASE.md](../RELEASE.md) for the full release and tagging flow.

## Advanced: manual testing with csc

For most local testing, `make sanity-test-local` (described above) is the
fastest option. For step-by-step debugging of individual CSI RPCs, you can
use the `csc` CLI tool instead.

> **Note:** The `csc` tool comes from [rexray/gocsi](https://github.com/rexray/gocsi)
> which is archived but still installable.

- Install csc

```sh
go install github.com/rexray/gocsi/csc@latest
```

- Setup variables

```sh
readonly volname="testvolume-$(date +%s)"
readonly cap="MULTI_NODE_MULTI_WRITER,mount,,,"
readonly target_path="/tmp/lustre-pv"
readonly endpoint="tcp://127.0.0.1:10000"

readonly lustre_fs_name="<your-lustre-fs-name>"
readonly lustre_fs_ip="<your-mgs-ip>"
```

- Start CSI driver locally

The driver requires an Azure cloud config file (`azure.json`) to initialize.
Without it, the driver will fatally exit unless mock dynamic provisioning is
enabled. Copy the file from a Kubernetes node or create one with your
subscription details:

```sh
export AZURE_CONFIG_FILE=/etc/kubernetes/azure.json
./_output/azurelustreplugin --endpoint $endpoint --nodeid CSINode -v=5 &
```

Alternatively, to skip the cloud config requirement and use mock dynamic
provisioning:

```sh
./_output/azurelustreplugin --endpoint $endpoint --nodeid CSINode \
    --enable-azurelustre-mock-mount --enable-azurelustre-mock-dyn-prov -v=5 &
```

- Exercise CSI RPCs

```sh
# Get plugin info
csc identity plugin-info --endpoint $endpoint

# Create a volume
csc controller new --endpoint $endpoint --cap $cap \
    --req-bytes 2147483648 \
    --params "fs-name=$lustre_fs_name,mgs-ip-address=$lustre_fs_ip" $volname

# Publish (mount) the volume
mkdir -p $target_path
csc node publish --endpoint $endpoint --cap $cap \
    --target-path $target_path \
    --vol-context "fs-name=$lustre_fs_name,mgs-ip-address=$lustre_fs_ip" $volname

# Unpublish (unmount) the volume
csc node unpublish --endpoint $endpoint --target-path $target_path $volname

# Delete the volume
csc controller del --endpoint $endpoint $volname

# Validate volume capabilities
csc controller validate-volume-capabilities --endpoint $endpoint --cap $cap $volname

# Get node info
csc node get-info --endpoint $endpoint
```
