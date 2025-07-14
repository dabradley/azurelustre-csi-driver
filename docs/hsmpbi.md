Here is a comprehensive and structured DevOps Product Backlog Item (PBI) description for integrating HSM (Hierarchical Storage Management) support into the Azurelustre CSI driver for AMLFS (Azure Managed Lustre File System). This PBI is tailored for your development team and reflects the latest internal and external documentation, including your own authored materials[1](https://microsoft-my.sharepoint.com/personal/davidbradley_microsoft_com/_layouts/15/Doc.aspx?sourcedoc=%7B8DD77189-76C5-4D36-A6DE-DC7A9F01EB4C%7D&file=DevOps%20K8S%20with%20AMLFS.pptx&action=edit&mobileredirect=true&DefaultItemOpen=1)[2](https://microsoft.sharepoint.com/teams/ManagedLustreinAzure/_layouts/15/Doc.aspx?sourcedoc=%7B2A82F6FB-8E14-4A47-866A-853B836E2992%7D&file=Azure%20Managed%20Lustre%20Review%20v2.docx&action=default&mobileredirect=true&DefaultItemOpen=1)[3](https://microsoft.sharepoint.com/teams/ManagedLustreinAzure/_layouts/15/Doc.aspx?sourcedoc=%7BB2305565-F8FB-422B-9C9E-60876B2884FB%7D&file=Wayve-AMLFS-Presentation.pptx&action=edit&mobileredirect=true&DefaultItemOpen=1)[4](https://teams.microsoft.com/l/meeting/details?eventId=AAMkADMwMDhhMjNmLWU5MjMtNDc4ZS1hZWU1LWYxZjMzNDFjYjJkZgBGAAAAAABXK0wXd9Z6SZkYkoWDVia8BwA3DemjisnwT6r6EJihGM3zAAAAAAENAAA3DemjisnwT6r6EJihGM3zAARgbponAAA%3d)[5](https://eng.ms/docs/cloud-ai-platform/azure-edge-platform-aep/aep-edge/avere/avere-azure-storage-cache/laaso-common-documentation/servicedocs/teamdocs/design/hsm_docs/importer)[6](https://techcommunity.microsoft.com/blog/azurehighperformancecomputingblog/azure-managed-lustre-with-automatic-synchronisation-to-azure-blob-storage/3997202)[7](https://learn.microsoft.com/en-us/azure/azure-managed-lustre/use-csi-driver-kubernetes).

---

# **DevOps PBI: Add HSM Support for AMLFS in Azurelustre CSI Driver**

## **Objective**
Enable full support for AMLFS HSM features within the Azurelustre CSI driver to allow Kubernetes workloads to leverage hierarchical storage capabilities. This includes metadata-aware archiving and retrieval from Azure Blob, ensuring cost-effective, scalable, and performant storage for AKS-based applications.

---

## **Technical Details**

### **Background**
AMLFS integrates Lustre’s HSM functionality to offload cold data to Azure Blob Storage. Currently, AMLFS supports:
- Auto-export of data to Blob (excluding metadata changes like renames or permission updates).
- Manual or creation-time import of data from Blob into AMLFS.
- Support for LRS/ZRS/GRS and Hot/Cold/Cool tiers[3](https://microsoft.sharepoint.com/teams/ManagedLustreinAzure/_layouts/15/Doc.aspx?sourcedoc=%7BB2305565-F8FB-422B-9C9E-60876B2884FB%7D&file=Wayve-AMLFS-Presentation.pptx&action=edit&mobileredirect=true&DefaultItemOpen=1).

The CSI driver currently supports static provisioning and is being extended to support dynamic provisioning and advanced features like HSM[1](https://microsoft-my.sharepoint.com/personal/davidbradley_microsoft_com/_layouts/15/Doc.aspx?sourcedoc=%7B8DD77189-76C5-4D36-A6DE-DC7A9F01EB4C%7D&file=DevOps%20K8S%20with%20AMLFS.pptx&action=edit&mobileredirect=true&DefaultItemOpen=1).

### **HSM Architecture**
The HSM system uses:
- **Robinhood policy engine** to monitor Lustre changelogs.
- **lhsmd** (Lustre HSM daemon) to manage archive/release tasks.
- **lustremetasync** to synchronize metadata changes.
- **Azure Blob** as the archive backend[6](https://techcommunity.microsoft.com/blog/azurehighperformancecomputingblog/azure-managed-lustre-with-automatic-synchronisation-to-azure-blob-storage/3997202).

### **CSI Driver Role**
The CSI driver must:
- Detect and respect HSM state (e.g., released, archived).
- Trigger metadata-aware import jobs when mounting volumes.
- Optionally expose HSM state to workloads via annotations or mount options.
- Handle errors gracefully when accessing released files.

---

## **Implementation Steps**

### **1. CSI Driver Enhancements**
- Extend the CSI node plugin to detect HSM-released files and trigger import jobs.
- Integrate with AMLFS Importer APIs or CLI tools to initiate metadata-aware imports[5](https://eng.ms/docs/cloud-ai-platform/azure-edge-platform-aep/aep-edge/avere/avere-azure-storage-cache/laaso-common-documentation/servicedocs/teamdocs/design/hsm_docs/importer).
- Add support for mount options like `--hsm-aware` or `--auto-import`.

### **2. Metadata Synchronization**
- Ensure CSI driver can read metadata from Blob for released files.
- Use Robinhood and lustremetasync logs to verify metadata consistency.

### **3. Archive Task Management**
- Implement hooks to trigger archive tasks post-write or based on policy.
- Optionally expose archive status via Kubernetes volume annotations.

### **4. Testing and Validation**
- Use Trivy scanner and Go version updates as part of the build pipeline[4](https://teams.microsoft.com/l/meeting/details?eventId=AAMkADMwMDhhMjNmLWU5MjMtNDc4ZS1hZWU1LWYxZjMzNDFjYjJkZgBGAAAAAABXK0wXd9Z6SZkYkoWDVia8BwA3DemjisnwT6r6EJihGM3zAAAAAAENAAA3DemjisnwT6r6EJihGM3zAARgbponAAA%3d).
- Validate with AKS Ubuntu node pools and dynamic provisioning scenarios[1](https://microsoft-my.sharepoint.com/personal/davidbradley_microsoft_com/_layouts/15/Doc.aspx?sourcedoc=%7B8DD77189-76C5-4D36-A6DE-DC7A9F01EB4C%7D&file=DevOps%20K8S%20with%20AMLFS.pptx&action=edit&mobileredirect=true&DefaultItemOpen=1).
- Test with Robinhood-configured thresholds (e.g., 85% OST usage, 30-minute last-modified)[6](https://techcommunity.microsoft.com/blog/azurehighperformancecomputingblog/azure-managed-lustre-with-automatic-synchronisation-to-azure-blob-storage/3997202).

---

## **Potential Challenges**

### **1. Metadata Coherency**
- Importing metadata post-deployment can lead to race conditions (e.g., file created before import completes)[5](https://eng.ms/docs/cloud-ai-platform/azure-edge-platform-aep/aep-edge/avere/avere-azure-storage-cache/laaso-common-documentation/servicedocs/teamdocs/design/hsm_docs/importer).
- Solution: Delay workload access until import completes or implement locking.

### **2. Performance Bottlenecks**
- Archive/release bandwidth is limited by VM network capacity.
- Solution: Allow multi-VM deployment of HSM copytools and tune Robinhood rate limits.

### **3. Blob Storage Constraints**
- HNS (Hierarchical Namespace) is not supported.
- Solution: Use flat namespace and encode directory structure in metadata.

### **4. Kubernetes Compatibility**
- Only Ubuntu node pools are currently supported.
- Solution: Document limitations and plan for Azure Linux support[8](https://microsoft-my.sharepoint.com/personal/jebearer_microsoft_com/_layouts/15/Doc.aspx?sourcedoc=%7B49A9E428-74A5-43FC-AEC7-BAB007C9EBED%7D&file=AMLFS%20Clients%20Overview.pptx&action=edit&mobileredirect=true&DefaultItemOpen=1).

---

## **Key Considerations for Integration**

- **Security**: Ensure proper IAM roles for Blob access via Managed Identity.
- **Scalability**: Design for multi-tenant isolation using dynamic provisioning[9](https://microsoft-my.sharepoint.com/personal/davidbradley_microsoft_com/_layouts/15/Doc.aspx?sourcedoc=%7BE2C11A26-EACC-41AD-A0E3-C95A8BD64262%7D&file=Dynamic%20Provisioning.pptx&action=edit&mobileredirect=true&DefaultItemOpen=1).
- **Observability**: Integrate with Azure Monitor and expose HSM metrics.
- **Documentation**: Update AKS deployment guides and CSI driver README with HSM usage instructions[7](https://learn.microsoft.com/en-us/azure/azure-managed-lustre/use-csi-driver-kubernetes).

---

Here’s a draft of **Acceptance Criteria** and **Test Cases** for the DevOps PBI: *Add HSM Support for AMLFS in Azurelustre CSI Driver*. These are structured to ensure clarity, traceability, and testability for your development and QA teams.

---

## ✅ Acceptance Criteria

### **1. HSM-Aware Mounting**
- **Given** a volume provisioned from AMLFS with HSM enabled,  
  **When** the CSI driver mounts the volume,  
  **Then** it must detect and handle released files by triggering metadata-aware import jobs.

### **2. Metadata Visibility**
- **Given** a released file in AMLFS,  
  **When** the volume is mounted,  
  **Then** the file’s metadata (e.g., name, size, permissions) must be visible to the workload.

### **3. Archive Trigger Support**
- **Given** a file written to AMLFS,  
  **When** the Robinhood policy threshold is met,  
  **Then** the file must be archived to Azure Blob and marked as released.

### **4. Import Job Completion**
- **Given** a released file is accessed by a workload,  
  **When** the CSI driver triggers an import,  
  **Then** the file must be restored and accessible within a defined timeout (e.g., 30 seconds).

### **5. Error Handling**
- **Given** a file is not yet restored,  
  **When** a workload attempts to read it,  
  **Then** the CSI driver must return a clear error or retry until the import completes.

### **6. Observability**
- **Given** HSM operations are performed,  
  **When** monitoring tools are queried,  
  **Then** metrics such as archive/import counts and latencies must be available via Azure Monitor.

### **7. Compatibility**
- **Given** the CSI driver is deployed on AKS Ubuntu node pools,  
  **When** HSM features are used,  
  **Then** the driver must function without kernel or OS-level errors.

---

## 🧪 Test Cases

### **TC1: Mount Volume with Released Files**
- **Setup**: Create AMLFS volume with HSM enabled. Archive a file.
- **Action**: Mount volume via CSI driver.
- **Expected**: Metadata of released file is visible; file is not accessible until import completes.

### **TC2: Trigger Import on Access**
- **Setup**: Archive a file and release it.
- **Action**: Access file from a pod.
- **Expected**: Import job is triggered; file becomes accessible within timeout.

### **TC3: Archive via Robinhood Policy**
- **Setup**: Write large files to AMLFS.
- **Action**: Wait for Robinhood to archive based on OST usage.
- **Expected**: Files are archived and marked as released.

### **TC4: Error on Premature Access**
- **Setup**: Archive and release a file.
- **Action**: Access file before import completes.
- **Expected**: CSI driver returns retryable error or delays access until import finishes.

### **TC5: Observability Metrics**
- **Setup**: Perform archive and import operations.
- **Action**: Query Azure Monitor.
- **Expected**: Metrics for HSM operations are visible and accurate.

### **TC6: Compatibility with Ubuntu Node Pools**
- **Setup**: Deploy CSI driver on AKS Ubuntu nodes.
- **Action**: Perform full HSM lifecycle (write → archive → release → import).
- **Expected**: No kernel panics, mount errors, or CSI plugin crashes.

---
