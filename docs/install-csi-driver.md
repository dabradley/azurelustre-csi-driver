# Install Azure Lustre CSI driver on a kubernetes cluster

This document explains how to install Azure Lustre CSI driver on a kubernetes cluster.

## Instructions for current production release

### Install with kubectl

- Option 1: Remote install

    ```shell
    curl -skSL https://raw.githubusercontent.com/kubernetes-sigs/azurelustre-csi-driver/main/deploy/install-driver.sh | bash -s main
    ```

- Option 2: Local install

    ```shell
    git clone https://github.com/kubernetes-sigs/azurelustre-csi-driver.git
    cd azurelustre-csi-driver
    ./deploy/install-driver.sh
    ```

- check pods status:

    ```shell
    $ kubectl get -n kube-system pod -l app=csi-azurelustre-controller

    NAME                                         READY    STATUS    RESTARTS   AGE
    csi-azurelustre-controller-778bf84cc5-4vrth   3/3     Running   0          30s
    csi-azurelustre-controller-778bf84cc5-5zqhl   3/3     Running   0          30s

    $ kubectl get -n kube-system pod -l app=csi-azurelustre-node

    NAME                        READY    STATUS    RESTARTS   AGE
    csi-azurelustre-node-7lw2n   3/3     Running   0          30s
    csi-azurelustre-node-drlq2   3/3     Running   0          30s
    csi-azurelustre-node-g6sfx   3/3     Running   0          30s
    ```

## Verifying CSI Driver Readiness for Lustre Operations

Before mounting Azure Lustre filesystems, it is important to verify that the CSI driver nodes are fully initialized and ready for Lustre operations. The driver includes enhanced LNet validation that performs comprehensive readiness checks:

- Load required kernel modules (lnet, lustre)
- Configure LNet networking with valid Network Identifiers (NIDs)
- Verify LNet self-ping functionality
- Validate all network interfaces are operational
- Complete all initialization steps

### Enhanced Readiness Validation

The CSI driver deployment includes automated **exec-based readiness probes** for accurate readiness detection:

- **Readiness & Startup Probes**: `/app/readinessProbe.sh` - Exec-based validation with comprehensive LNet checking
- **Liveness Probe**: `/healthz` (Port 29763) - HTTP endpoint for basic container health

#### Verification Steps

1. **Check pod readiness status:**

   ```shell
   kubectl get -n kube-system pod -l app=csi-azurelustre-node -o wide
   ```

   All node pods should show `READY` status as `3/3` and `STATUS` as `Running`.

2. **Verify probe configuration:**

   ```shell
   kubectl describe -n kube-system pod -l app=csi-azurelustre-node
   ```

   Look for exec-based readiness and startup probe configuration and check that no recent probe failures appear in the Events section.

3. **Monitor validation logs:**

   ```shell
   kubectl logs -n kube-system -l app=csi-azurelustre-node -c azurelustre --tail=20
   ```

   Look for CSI driver startup and successful GRPC operation logs indicating driver initialization is complete.

> **Note**: If you encounter readiness or initialization issues, see the [CSI Driver Troubleshooting Guide](csi-debug.md#enhanced-lnet-validation-troubleshooting) for detailed debugging steps.

**Important**: The enhanced validation ensures the driver reports ready only when LNet is fully functional for Lustre operations. Wait for all CSI driver node pods to pass enhanced readiness checks before creating PersistentVolumes or mounting Lustre filesystems.

## Startup Taints

When the CSI driver starts on each node, it automatically removes the following taint if present:

- **Taint Key**: `azurelustre.csi.azure.com/agent-not-ready`
- **Taint Effect**: `NoSchedule`

This ensures that:

1. **Node Readiness**: Pods requiring Azure Lustre storage are only scheduled to nodes where the CSI driver is fully initialized
2. **Lustre Client Ready**: The node has successfully loaded Lustre kernel modules and networking components

### Configuring Startup Taint Behavior

The startup taint functionality is enabled by default but can be configured during installation:

- **Default Behavior**: Startup taint removal is **enabled** by default
- **Disable Taint Removal**: To disable, set `--remove-not-ready-taint=false` in the driver deployment

For most AKS users, the default behavior provides optimal pod scheduling and should not be changed

## Custom Entrypoint (Advanced)

The CSI driver supports overriding the built-in entrypoint script via a Kubernetes ConfigMap. This is intended as a **troubleshooting/debugging feature** for use when suggested by Microsoft support, or for customers with custom initialization requirements (e.g., non-standard networking setups).

### How It Works

The container uses a wrapper script (`start.sh`) that checks for a custom entrypoint at `/app/custom-entrypoint/entrypoint.sh`. If found, it runs the custom version; otherwise it falls back to the built-in entrypoint. The custom entrypoint is mounted from an optional ConfigMap (`csi-azurelustre-entrypoint`) into the **node DaemonSet pods only** — the controller deployment is not affected.

### Installing with a Custom Entrypoint

Pass `--custom-entrypoint <file>` to the install script:

```shell
./deploy/install-driver.sh --custom-entrypoint ./my-entrypoint.sh local
```

This creates a ConfigMap from the provided file and restarts the node DaemonSet pods to use it.

### Reverting to the Built-in Entrypoint

Run the install script without the `--custom-entrypoint` flag:

```shell
./deploy/install-driver.sh local
```

This deletes the ConfigMap and restarts the node pods to use the built-in entrypoint. **The custom entrypoint is not sticky** — each install must explicitly request it.

### Important Notes

- The custom entrypoint replaces the **entire** built-in entrypoint, including Lustre client installation logic. Your custom script is responsible for any required setup before launching the CSI driver binary.
- A good starting point for a custom entrypoint is the built-in script at `pkg/azurelustreplugin/entrypoint.sh`.
- **Security note:** the custom entrypoint is stored in the `csi-azurelustre-entrypoint` ConfigMap in `kube-system` and is executed by a privileged container. Treat this as a code-injection path: tightly restrict RBAC for creating or updating this ConfigMap, and only use custom entrypoints in trusted/admin scenarios.
- If you edit the ConfigMap directly (e.g., `kubectl edit configmap csi-azurelustre-entrypoint -n kube-system`), you must manually restart the node DaemonSets for changes to take effect: `kubectl rollout restart daemonset csi-azurelustre-node-jammy csi-azurelustre-node-noble -n kube-system`
- The uninstall script automatically cleans up the ConfigMap if it exists.
