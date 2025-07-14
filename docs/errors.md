Based on the Azure Lustre CSI Driver documentation and internal discussions, here's a comprehensive list of potential errors users may encounter when creating an AMLFS cluster with dynamic provisioning of PVCs, along with detailed diagnostics and fixes.

---

## ⚠️ Common Errors in AMLFS Dynamic Provisioning

---

### **Error 1: `ResourceNotFound`**

- **Description:**  
  The requested resource (e.g., Managed Identity or Key Vault) does not exist in the specified resource group.

- **Diagnosis:**
  - Check the error message for the missing resource name and resource group.
  - Confirm whether the resource was deleted or never created.
  - Example from rollout logs:
    ```
    Error: Code: ResourceNotFound
    Message: The Resource 'Microsoft.ManagedIdentity/userAssignedIdentities/amlfs-clients-esrp-ame' under resource group 'pmc-cert-kv-rg' was not found.
    ```

- **Fix:**
  - Ensure the resource exists in the correct resource group.
  - If recently deleted, purge it before reuse.
  - Use the Azure CLI to verify:
    ```bash
    az identity show --name amlfs-clients-esrp-ame --resource-group pmc-cert-kv-rg
    ```

---

### **Error 2: `VaultAlreadyExists`**

- **Description:**  
  A Key Vault with the same name already exists globally or is in a soft-deleted state.

- **Diagnosis:**
  - Occurs during deployment when trying to create a Key Vault with a name that’s already taken.
  - Example:
    ```
    Error: Code: VaultAlreadyExists
    Message: The vault name 'kv-esrp-ame' is already in use.
    ```

- **Fix:**
  - Choose a globally unique vault name.
  - If the vault was deleted, purge it:
    ```bash
    az keyvault purge --name kv-esrp-ame
    ```

---

### **Error 3: `DeploymentFailed`**

- **Description:**  
  A nested deployment failed due to one or more resource creation issues.

- **Diagnosis:**
  - Review the inner error messages for specific causes.
  - Example:
    ```
    Error: Code: DeploymentFailed
    Message: At least one resource deployment operation failed.
    ```

- **Fix:**
  - Use the Azure Resource Explorer or CLI to inspect deployment operations:
    ```bash
    az deployment group show --name getMSI --resource-group pmc-cert-kv-rg
    ```

---

### **Error 4: `PVC Stuck in Pending`**

- **Description:**  
  PersistentVolumeClaim remains in `Pending` state due to provisioning failure.

- **Diagnosis:**
  - Use `kubectl describe pvc <name>` to inspect events.
  - Look for errors like:
    ```
    failed to provision volume with StorageClass "amlfs-dynamic": rpc error: code = Internal desc = failed to create AMLFS cluster
    ```

- **Fix:**
  - Ensure the CSI driver is correctly installed and running:
    ```bash
    kubectl get pods -n kube-system | grep azurelustre
    ```
  - Check logs:
    ```bash
    kubectl logs <csi-azurelustre-controller-pod> -n kube-system
    ```

---

### **Error 5: `Insufficient Permissions`**

- **Description:**  
  The AKS cluster lacks permissions to create AMLFS resources.

- **Diagnosis:**
  - Check for `AuthorizationFailed` or `AccessDenied` errors in logs.
  - Confirm that the AKS-managed identity has the required roles.

- **Fix:**
  - Assign the necessary roles:
    ```bash
    az role assignment create --assignee <aks-managed-identity> --role "Contributor" --scope <subscription/resource-group>
    ```

---

### **Error 6: `Subnet Not Found or Misconfigured`**

- **Description:**  
  The subnet specified for AMLFS cluster creation is missing or misconfigured.

- **Diagnosis:**
  - Look for errors like:
    ```
    SubnetNotFound: The subnet 'subnet-name' could not be found in virtual network 'vnet-name'.
    ```

- **Fix:**
  - Ensure the subnet exists and is accessible.
  - Validate with:
    ```bash
    az network vnet subnet show --name <subnet-name> --vnet-name <vnet-name> --resource-group <rg>
    ```

---

### **Error 7: `Driver Not Installed or Misconfigured`**

- **Description:**  
  The CSI driver is not installed or not functioning properly.

- **Diagnosis:**
  - Pods for the CSI driver are not running or crash-looping.
  - Use:
    ```bash
    kubectl get csidriver
    kubectl get pods -n kube-system | grep azurelustre
    ```

- **Fix:**
  - Reinstall the driver using Helm or YAML manifests.
  - Follow the [official setup guide](https://github.com/kubernetes-sigs/azurelustre-csi-driver)[1](https://github.com/kubernetes-sigs/azurelustre-csi-driver).

---

### **Error 8: `Mount Failure`**

- **Description:**  
  The AMLFS volume fails to mount inside the pod.

- **Diagnosis:**
  - Pod events show:
    ```
    MountVolume.MountDevice failed for volume "amlfs-vol" : mount failed: exit status 32
    ```

- **Fix:**
  - Ensure the AMLFS client is installed on the node.
  - Check node logs:
    ```bash
    journalctl -u kubelet
    ```

---

Would you like this list exported to a Markdown file or integrated into a troubleshooting guide for your team?
















When a PersistentVolumeClaim (PVC) is stuck in the "Pending" state while using the Azure Lustre CSI driver for dynamic provisioning in AKS, it typically indicates a failure in the volume provisioning workflow. Based on internal diagnostics, meeting discussions, and external documentation, here’s a comprehensive guide to diagnosing and resolving this issue.

---

## 🛠️ Troubleshooting PVC Stuck in Pending State (Azure Lustre CSI)

---

### **1. Check PVC Events for Provisioning Errors**

- **Diagnosis:**
  Use the following command to inspect the PVC:
  ```bash
  kubectl describe pvc <pvc-name> -n <namespace>
  ```
  Look for events like:
  ```
  Warning  ProvisioningFailed  azurelustre.csi.azure.com [...] failed to provision volume with StorageClass "sc.azurelustre.csi.azure.com": rpc error: code = InvalidArgument desc = Invalid parameter(s) [...]
  ```

- **Fix:**
  - Ensure the StorageClass parameters are valid.
  - Double-check the `csi.storage.k8s.io/*` annotations.
  - Validate that the CSI driver is correctly installed and running.

---

### **2. Validate CSI Driver Deployment**

- **Diagnosis:**
  Check if the CSI controller and node pods are running:
  ```bash
  kubectl get pods -n kube-system | grep azurelustre
  ```

- **Fix:**
  - If pods are crash-looping, inspect logs:
    ```bash
    kubectl logs <pod-name> -n kube-system
    ```
  - Reinstall or upgrade the CSI driver if needed.

---

### **3. Confirm AKS Permissions and Identity Setup**

- **Diagnosis:**
  The AKS cluster’s managed identity must have sufficient permissions to create AMLFS resources.

- **Fix:**
  Assign the required roles:
  ```bash
  az role assignment create --assignee <aks-managed-identity> --role "Contributor" --scope <resource-group-or-subscription>
  ```

---

### **4. Check for Throttling or Quota Issues**

- **Diagnosis:**
  Azure may throttle requests if limits are exceeded. Look for 429 errors in logs:
  ```
  Status=429 Code="TooManyRequests"
  ```

- **Fix:**
  - Reduce the frequency of provisioning operations.
  - Request quota increases via Azure Support.
  - Reference: [Azure Throttling Limits](https://aka.ms/srpthrottlinglimits)[1](https://dev.azure.com/msazure/b32aa71e-8ed2-41b2-9d77-5bc261222004/_wiki/wikis/4482edfe-a7ee-4522-9ec5-48442a1579d5?pagePath=%2FAzure%20Confidential%20Ledger%2FTroubleshooting%20Guides%2FInfrastructure%2FAKS%2FTroubleshooting%20AKS%20storage%20CSI%20errors%20PVC%20stuck%20in%20Pending%20state).

---

### **5. Validate AMLFS Cluster Creation**

- **Diagnosis:**
  If AMLFS cluster creation fails silently, the PVC will remain pending.

- **Fix:**
  - Check the AMLFS resource group for failed deployments.
  - Use Azure Resource Explorer or CLI:
    ```bash
    az resource list --resource-group <rg-name> --output table
    ```

---

### **6. StorageClass Misconfiguration**

- **Diagnosis:**
  Incorrect or missing parameters in the StorageClass can prevent volume provisioning.

- **Fix:**
  - Review the StorageClass definition:
    ```yaml
    provisioner: azurelustre.csi.azure.com
    parameters:
      fsName: <amlfs-name>
      resourceGroup: <rg-name>
      subnetId: <subnet-resource-id>
    ```
  - Ensure all required fields are present and correct.

---

### **7. Node Scheduling Constraints**

- **Diagnosis:**
  Pods may not be scheduled if nodes lack access to the AMLFS mount or if PVCs are unbound.

- **Fix:**
  - Check pod events:
    ```bash
    kubectl describe pod <pod-name>
    ```
  - Ensure nodes are in the correct subnet and have Lustre client installed.

---

### **8. Version Compatibility Issues**

- **Diagnosis:**
  Upgrading AKS or the CSI driver may introduce incompatibilities.

- **Fix:**
  - Validate compatibility matrix in the [Azure Lustre CSI GitHub repo](https://github.com/kubernetes-sigs/azurelustre-csi-driver)[2](https://learn.microsoft.com/en-us/azure/azure-managed-lustre/use-csi-driver-kubernetes).
  - Roll back to a known working version if needed.

---

### **9. AMLFS Quota or Region Limitations**

- **Diagnosis:**
  AMLFS provisioning may fail due to quota limits or unsupported regions.

- **Fix:**
  - Check Azure subscription quotas.
  - Ensure AMLFS is supported in the target region.

---

### **10. Debugging with Logs and Integration Tests**

- **Diagnosis:**
  Use integration test scripts and logs to simulate and validate provisioning.

- **Fix:**
  - Run integration tests from the CSI repo:
    ```bash
    ./test/integration_dalec/setup_integration_test_dalec.sh
    ```
  - Review logs and patch behavior in the Dalec build system[3](https://teams.microsoft.com/l/meeting/details?eventId=AAMkADMwMDhhMjNmLWU5MjMtNDc4ZS1hZWU1LWYxZjMzNDFjYjJkZgBGAAAAAABXK0wXd9Z6SZkYkoWDVia8BwA3DemjisnwT6r6EJihGM3zAAAAAAENAAA3DemjisnwT6r6EJihGM3zAARoPQEpAAA%3d).

---

Would you like a YAML validation checklist or a script to automate some of these diagnostics?