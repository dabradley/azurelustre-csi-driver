# Azure Lustre CSI Driver for Kubernetes

[![Coverage Status](https://coveralls.io/repos/github/kubernetes-sigs/azurelustre-csi-driver/badge.svg?branch=main)](https://coveralls.io/github/kubernetes-sigs/azurelustre-csi-driver?branch=main)
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Fkubernetes-sigs%2Fazurelustre-csi-driver.svg?type=shield)](https://app.fossa.com/projects/git%2Bgithub.com%2Fkubernetes-sigs%2Fazurelustre-csi-driver?ref=badge_shield)

## About

This driver allows Kubernetes to access Azure Lustre file system.

- CSI plugin name: `azurelustre.csi.azure.com`
- Project status: under early development

&nbsp;

### Container Images & CSI Driver Versions

#### Latest Release
| Driver Version | Image | Lustre Client | Features |
|----------------|-------|---------------|----------|
| **v0.3.1** (Recommended) | `mcr.microsoft.com/oss/v2/kubernetes-csi/azurelustre-csi:v0.3.1` | 2.15.7 | Static + Dynamic Provisioning |

#### Installation
```bash
# Install latest stable release
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/azurelustre-csi-driver/v0.3.1/deploy/install-driver.sh
```

> **Note:** For AMLFS compatibility, ensure your Lustre client version is compatible with your AMLFS cluster version. See [client installation](https://learn.microsoft.com/en-us/azure/azure-managed-lustre/client-install) for details.

<details>
<summary>Previous Releases</summary>

| Version | Lustre Client | Notable Changes |
|---------|---------------|-----------------|
| v0.3.1  | 2.15.7 | Updated Lustre client to 2.15.7 |
| v0.3.0  | 2.15.5 | Added dynamic provisioning (preview) |
| v0.2.0  | 2.15.5 | Updated to v2 MCR path |
| v0.1.18 | 2.15.5 | Stability improvements |
| v0.1.17 | 2.15.5 | Updated Lustre client |
| v0.1.15 | 2.15.4 | Bug fixes |
| v0.1.14 | 2.15.3 | Performance improvements |
| v0.1.11 | 2.15.1 | Initial stable release |

See the [full release history](https://github.com/kubernetes-sigs/azurelustre-csi-driver/releases) for details.
</details>

&nbsp;

### Set up CSI driver on AKS cluster (only for AKS users)

- [Install CSI driver in AKS cluster](./docs/install-csi-driver.md)
- [Deploy workload with Static Provisioning](./docs/static-provisioning.md)
- [Deploy workload with Dynamic Provisioning](./docs/dynamic-provisioning.md)

&nbsp;

### Troubleshooting

- [CSI driver troubleshooting guide](./docs/csi-debug.md)

&nbsp;

### Support

- Please see our [support policy][support-policy]

&nbsp;

## Kubernetes Development

- Please refer to [development guide](./docs/csi-dev.md)

&nbsp;

### Links

- [Kubernetes CSI Documentation](https://kubernetes-csi.github.io/docs/)
- [CSI Drivers](https://github.com/kubernetes-csi/drivers)
- [Container Storage Interface (CSI) Specification](https://github.com/container-storage-interface/spec)

[support-policy]: support.md
