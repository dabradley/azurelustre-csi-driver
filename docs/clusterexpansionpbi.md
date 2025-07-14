Here’s a detailed and comprehensive **DevOps Product Backlog Item (PBI)** description for integrating the upcoming **AMLFS cluster expansion** feature into the **Azurelustre CSI driver**, enabling volume expansion capabilities.

---

## **🔧 Title**
**Enable AMLFS Cluster Expansion Support in Azurelustre CSI Driver**

---

## **🎯 Objective**
To extend the Azurelustre CSI driver to support dynamic volume expansion by integrating the forthcoming AMLFS cluster expansion feature, leveraging the existing AMLFS SDK support already present in the driver.

---

## **📚 Background**
The Azurelustre CSI driver currently supports core AMLFS create / delete functionality through the SDK. However, with the upcoming release of AMLFS cluster expansion capabilities, the driver can be enhanced to:
- Detect and respond to volume expansion requests.
- Interface with the AMLFS SDK to trigger and validate cluster expansion.
- Ensure seamless integration with Kubernetes volume expansion workflows.

This enhancement will allow users to dynamically scale their AMLFS-backed volumes without downtime, aligning with modern elastic storage expectations.

---

## **📋 Detailed Requirements**

### **Driver Enhancements**
- Implement support for the `ControllerExpandVolume` gRPC call in the CSI driver.
- Integrate AMLFS SDK calls to:
  - Query current cluster size.
  - Trigger expansion operations.
    - If new API call, provide necessary values
    - If expansion is done through `storageCapacityTiB` update call, ensure no other values are changed.
  - Validate post-expansion state.

### **Kubernetes Compatibility**
- Ensure compatibility with Kubernetes volume expansion workflows.
- Update `CSIDriver` object to advertise `volumeExpansion: true`.

### **Error Handling & Logging**
- Implement robust error handling for expansion failures.
- Add detailed logging for expansion requests, SDK interactions, and outcomes.

### **Testing & Validation**
- Unit tests for new expansion logic.
- Integration tests using a mock AMLFS environment.
- End-to-end tests in a Kubernetes cluster with AMLFS backend.

### **Documentation**
- Update README and deployment guides to reflect new expansion capabilities.
- Provide usage examples and known limitations.

---

## **✅ Acceptance Criteria**
- [ ] Azurelustre CSI driver supports `ControllerExpandVolume` and passes CSI conformance tests.
- [ ] AMLFS cluster expansion is triggered and validated via SDK.
- [ ] Kubernetes PVCs backed by AMLFS can be expanded dynamically.
- [ ] All new code paths are covered by unit and integration tests.
- [ ] Documentation is updated and reviewed.

---

## **🔗 Dependencies**
- Release of AMLFS cluster expansion feature and corresponding SDK updates (is it a new endpoint or just allowing `storageCapacityTiB` in update call?)
- Kubernetes version ≥ 1.16 (for volume expansion support).

---


## 🛠️ **Implementation Notes: AMLFS Cluster Expansion in Azurelustre CSI Driver**

### 1. **CSI Driver Interface Enhancements**
- **Implement `ControllerExpandVolume` RPC** in the CSI controller server:
  - This is the core CSI method that Kubernetes invokes when a PVC expansion is requested.
  - Ensure the method checks for the `VolumeCapability.AccessMode` and validates the request.

- **Update `CSIDriver` object**:
  - Set `volumeExpansion: true` in the `CSIDriver` CRD to advertise expansion support to Kubernetes.

---

### 2. **AMLFS SDK Integration**
- **Extend SDK usage**:
  - Use the AMLFS SDK to:
    - Query current cluster size.
    - Trigger expansion using the new cluster expansion API.
    - Poll or verify the expansion status post-operation.

- **Error handling**:
  - Handle SDK errors gracefully and return appropriate gRPC status codes (e.g., `InvalidArgument`, `Internal`, `Unavailable`).

---

### 3. **Driver Configuration**
- **Add new config flags or environment variables**:
  - For enabling/disabling expansion support.
  - For specifying expansion timeout or retry intervals.

- **Update Helm charts or deployment YAMLs** to reflect new configuration options.

---

### 4. **Testing Strategy**
- **Unit Tests**:
  - Mock AMLFS SDK responses for expansion success/failure.
  - Validate `ControllerExpandVolume` logic under various scenarios.

- **Integration Tests**:
  - Use a test AMLFS cluster to simulate real expansion.
  - Validate PVC expansion from Kubernetes using `kubectl patch` or `kubectl edit`.

- **End-to-End Tests**:
  - Automate PVC expansion scenarios in CI pipelines.
  - Include rollback or failure recovery tests.

---

### 5. **Logging and Observability**
- **Add structured logs**:
  - Log expansion requests, parameters, and outcomes.
  - Include correlation IDs for traceability.

- **Expose metrics**:
  - Number of expansion requests.
  - Success/failure rates.
  - Average expansion duration.

---

### 6. **Documentation**
- Update:
  - `README.md` with feature overview and usage instructions.
  - `examples/` with YAMLs demonstrating PVC expansion.
  - Release notes to highlight the new capability and any breaking changes.

---
Here’s a sample stub for implementing the `ControllerExpandVolume` method in the Azurelustre CSI driver. This is written in Go, following the Kubernetes CSI spec and assuming the AMLFS SDK provides a method like `ExpandClusterVolume(volumeID, newSizeGiB)`.

```go
// ControllerExpandVolume handles volume expansion requests from Kubernetes
func (d *Driver) ControllerExpandVolume(
    ctx context.Context,
    req *csi.ControllerExpandVolumeRequest,
) (*csi.ControllerExpandVolumeResponse, error) {

    volumeID := req.GetVolumeId()
    capacityRange := req.GetCapacityRange()

    if volumeID == "" {
        return nil, status.Error(codes.InvalidArgument, "Volume ID is required")
    }

    if capacityRange == nil || capacityRange.GetRequiredBytes() == 0 {
        return nil, status.Error(codes.InvalidArgument, "Required capacity must be specified")
    }

    // Convert bytes to GiB
    requiredGiB := capacityRange.GetRequiredBytes() / (1024 * 1024 * 1024)

    // Log the expansion request
    klog.Infof("Received request to expand volume %s to %d GiB", volumeID, requiredGiB)

    // Call AMLFS SDK to perform the expansion
    err := d.amlfsClient.ExpandClusterVolume(volumeID, requiredGiB)
    if err != nil {
        klog.Errorf("Failed to expand volume %s: %v", volumeID, err)
        return nil, status.Errorf(codes.Internal, "Expansion failed: %v", err)
    }

    // Return success with the new capacity
    return &csi.ControllerExpandVolumeResponse{
        CapacityBytes: requiredGiB * 1024 * 1024 * 1024,
        NodeExpansionRequired: false, // Set to true if node-level expansion is needed
    }, nil
}
```

### 🔍 Key Notes:
- **Validation**: Ensures `volumeID` and `requiredBytes` are present.
- **Logging**: Uses `klog` for observability.
- **AMLFS SDK**: Assumes a method `ExpandClusterVolume(volumeID, sizeGiB)` exists.
- **Node Expansion**: Set `NodeExpansionRequired` to `true` if the filesystem on the node also needs resizing (e.g., ext4, xfs).

Here are the **Acceptance Criteria** and **Test Cases** for the AMLFS cluster expansion integration in the Azurelustre CSI driver:

---

## ✅ **Acceptance Criteria**

1. **CSI Volume Expansion Support**
   - The driver implements the `ControllerExpandVolume` RPC.
   - The `CSIDriver` object advertises `volumeExpansion: true`.

2. **AMLFS SDK Integration**
   - The driver successfully invokes the AMLFS SDK to expand the cluster volume.
   - Expansion requests are validated and confirmed via the SDK.

3. **Kubernetes Compatibility**
   - A PersistentVolumeClaim (PVC) backed by AMLFS can be expanded using standard Kubernetes workflows.
   - No manual intervention is required post-expansion.

4. **Error Handling**
   - The driver returns appropriate gRPC error codes for invalid or failed expansion requests.
   - Logs contain clear and actionable messages for debugging.

5. **Observability**
   - Expansion operations are logged with volume ID, requested size, and result.
   - Metrics are exposed for expansion attempts and outcomes.

6. **Testing Coverage**
   - Unit tests cover all new logic paths.
   - Integration tests validate end-to-end expansion behavior.
   - Tests simulate both successful and failed expansion scenarios.

7. **Documentation**
   - README and deployment guides are updated with expansion instructions.
   - Example manifests demonstrate PVC expansion.

---

## 🧪 **Test Cases**

### 🔹 Unit Tests
| Test Case | Description | Expected Result |
|-----------|-------------|-----------------|
| `TestControllerExpandVolume_ValidRequest` | Simulate a valid expansion request | Returns success with updated capacity |
| `TestControllerExpandVolume_InvalidVolumeID` | Pass empty volume ID | Returns `InvalidArgument` error |
| `TestControllerExpandVolume_InvalidCapacity` | Pass zero or negative capacity | Returns `InvalidArgument` error |
| `TestControllerExpandVolume_SDKFailure` | Simulate SDK failure during expansion | Returns `Internal` error |

### 🔹 Integration Tests
| Test Case | Description | Expected Result |
|-----------|-------------|-----------------|
| `ExpandPVC_Success` | Expand a PVC from 100Gi to 200Gi | PVC status reflects new size, no errors |
| `ExpandPVC_UnsupportedVolume` | Attempt expansion on non-AMLFS volume | Expansion is rejected gracefully |
| `ExpandPVC_Timeout` | Simulate SDK timeout | Expansion fails with retry logic triggered |

### 🔹 End-to-End Tests
| Test Case | Description | Expected Result |
|-----------|-------------|-----------------|
| `PVCExpansionWorkflow` | Create PVC → Expand → Validate mount | Filesystem reflects new size |
| `MultipleConcurrentExpansions` | Expand multiple PVCs simultaneously | All expansions complete successfully |
| `RollbackOnFailure` | Simulate partial expansion failure | System logs error, PVC remains stable |

---

Would you like me to generate YAML test manifests or CI pipeline steps for these test cases?