# Release Process

The Azure Lustre CSI driver is released on an as-needed basis. Production images
are built through [DALEC](https://github.com/Azure/dalec-build-defs) (the Azure
Container Upstream build system), which provides FIPS compliance, supply-chain
security, and publishing to the Microsoft Container Registry (MCR). Images not
built through DALEC cannot be released.

## Overview

The DALEC spec pins a specific commit SHA from this repo, so the images build
from that SHA directly — no tag is required for the build. **Tagging the release
and merging to `main` are the final steps**, done only after the MCR images are
validated, so the tag and `main` (what users install from) always point at a
known-good build.

```text
development -> version bump -> merge PR (this commit is the release SHA)
      -> DALEC spec PR pinned to that SHA
      -> validate staging images -> merge DALEC PR -> images published to MCR
      -> validate MCR images
      -> tag the SHA + merge development to main + GitHub Release   (last)
      -> reset development to latest
```

## Step 1 — Version bump and chart update (this repo)

Do this first, so the deploy manifests and charts are ready before the DALEC
image exists.

- **Makefile**: set `IMAGE_VERSION ?= vX.Y.Z`.
- **`deploy/csi-azurelustre-node-<flavor>.yaml`** (one per flavor; `make
  print-all-flavors` lists them): bump the image tag `...:latest-<flavor>` ->
  `...:vX.Y.Z-<flavor>` and the `app.kubernetes.io/version` labels `latest` ->
  `vX.Y.Z`.
- **Charts**: copy `charts/latest/azurelustre-csi-driver` to
  `charts/vX.Y.Z/azurelustre-csi-driver`; in the copy set `Chart.yaml`
  `version`/`appVersion` and `values.yaml` `image.tag` to `vX.Y.Z`; package it
  (`helm package charts/vX.Y.Z/azurelustre-csi-driver -d charts/vX.Y.Z/`) and
  regenerate the index (`hack/update-helm-chart-index.sh`). Leave
  `charts/latest/` at its development values (`v0.0.0` / `latest`).
- **README.md**: add a version-table row for `vX.Y.Z` (image tags and per-flavor
  Lustre client versions from `charts/latest/azurelustre-csi-driver/values.yaml`).
- Run `make verify` plus the e2e (`make e2e-test`) and sanity
  (`make sanity-test-local`) tests, then open a PR and merge to `development`.
  That merged commit on `development` is the **release SHA** used everywhere
  below.

## Step 2 — DALEC image build

DALEC specs for this driver are **hand-authored, one file per version per OS
flavor**, under `specs/kubernetes-csi-azurelustre/` in the dalec-build-defs repo
— there is no template/matrix generation for this project.

- Copy the previous release's specs to `azurelustre-csi-<version>-<flavor>.yml`
  (e.g. `azurelustre-csi-0.5.0-jammy.yml`, `-noble.yml`). The spec filename and
  its `version:` field use **no `v` prefix**, even though the image tag does
  (`vX.Y.Z-<flavor>`).
- Set each spec's `COMMIT` arg to the release SHA. The build step is
  `make azurelustre-dalec`.
- Open a draft PR; its build publishes staging images to
  `upstream.azurecr.io/oss/v2/kubernetes-csi/azurelustre-csi:vX.Y.Z-<flavor>`.
  Validate them with the CSI image-validation and dynamic-provisioning
  pipelines, then merge — the final images publish to MCR at
  `mcr.microsoft.com/oss/v2/kubernetes-csi/azurelustre-csi:vX.Y.Z-<flavor>`.

## Step 3 — Validate MCR images

Re-run the CSI image-validation and dynamic-provisioning pipelines against the
published MCR images
(`mcr.microsoft.com/oss/v2/kubernetes-csi/azurelustre-csi:vX.Y.Z-<flavor>`) to
confirm they match what was validated from staging.

## Step 4 — Tag, merge to main, and publish the release

These are the final, irreversible steps — do them only after the MCR images are
validated.

1. Tag the release SHA on `development`:

   ```console
   git tag -a vX.Y.Z -m "Release vX.Y.Z" <release-sha>
   git push origin vX.Y.Z
   ```

2. Merge `development` into `main` (this is what users install from). Confirm the
   merge carries the versioned chart directory (`charts/vX.Y.Z/`) with MCR image
   tags, and no `development` work beyond the release SHA.
3. Cut the [GitHub Release](https://github.com/kubernetes-sigs/azurelustre-csi-driver/releases)
   on the tag:

   ```console
   gh release create vX.Y.Z --title "vX.Y.Z" --generate-notes
   ```

## Step 5 — Update public docs

Review the [Microsoft Learn Azure Managed Lustre docs](https://learn.microsoft.com/azure/azure-managed-lustre/)
(source in [azure-stack-docs-pr](https://github.com/MicrosoftDocs/azure-stack-docs-pr)
under `azure-managed-lustre/`) for version-specific install or usage statements,
e.g. `use-csi-driver-kubernetes.md`.

## Step 6 — Reset development

Revert the Step 1 mutations on `development`: `IMAGE_VERSION ?= latest`, deploy
image tags/labels back to `latest`, remove `charts/vX.Y.Z/`, and regenerate the
index. Leave the released README row as the historical record. Run `make verify`
and commit.
