## CSI driver troubleshooting guide

&nbsp;

### Case#1: volume create/delete issue

&nbsp;

- Symptoms
  - PVC can't go into Bound status
  - User workload pod can't go into a Running status

&nbsp;

- Locate csi driver pod

```console
$ kubectl get po -o wide -n kube-system -l app=csi-azurelustre-controller
```

<pre>
NAME                                              READY   STATUS    RESTARTS   AGE     IP             NODE
csi-azurelustre-controller-56bfddd689-dh5tk       3/3     Running   0          35s     10.240.0.19    k8s-agentpool-22533604-0
csi-azurelustre-controller-56bfddd689-sl4ll       3/3     Running   0          35s     10.240.0.23    k8s-agentpool-22533604-1
</pre>

&nbsp;

- Get csi driver logs

```console
$ kubectl logs csi-azurelustre-controller-56bfddd689-dh5tk -c azurelustre -n kube-system > csi-lustre-controller.log
```

> note:
>
> - add --previous to retrieve logs from a previous running container
>
> - there could be multiple controller pods, logs can be taken from all of them simultaneously
>
> ```console
> $ kubectl logs -n kube-system -l app=csi-azurelustre-controller -c azurelustre --tail=-1 --prefix 
> ```
>
> - retrieve logs with `follow` (realtime) mode
>
> ```console
> $ kubectl logs deploy/csi-azurelustre-controller -c azurelustre -f -n kube-system
> ```

&nbsp;

### Case#2: volume mount/unmount issue

- Locate csi driver pod and find out the pod does the actual volume mount/unmount operation

```console
$ kubectl get po -o wide -n kube-system -l app=csi-azurelustre-node
```

<pre>
NAME                           READY   STATUS    RESTARTS   AGE     IP             NODE
csi-azurelustre-node-9ds7f     3/3     Running   0          7m4s    10.240.0.35    k8s-agentpool-22533604-1
csi-azurelustre-node-dr4s4     3/3     Running   0          7m4s    10.240.0.4     k8s-agentpool-22533604-0
</pre>

&nbsp;

- Get csi driver logs

```console
$ kubectl logs csi-azurelustre-node-9ds7f -c azurelustre -n kube-system > csi-azurelustre-node.log
```

> note: to watch logs in realtime from multiple `csi-azurelustre-node` DaemonSet pods simultaneously, run the command:
>
> ```console
> $ kubectl logs daemonset/csi-azurelustre-node -c azurelustre -n kube-system -f
> ```

&nbsp;

- Check Lustre mounts inside driver

```console
$ kubectl exec -it csi-azurelustre-node-9ds7f -n kube-system -c azurelustre -- mount | grep lustre
```

<pre>
172.18.8.12@tcp:/lustrefs on /var/lib/kubelet/pods/6632349a-05fd-466f-bc8a-8946617089ce/volumes/kubernetes.io~csi/pvc-841498d9-fa63-418c-8cc7-d94ec27f2ee2/mount type lustre (rw,flock,lazystatfs,encrypt)
172.18.8.12@tcp:/lustrefs on /var/lib/kubelet/pods/6632349a-05fd-466f-bc8a-8946617089ce/volumes/kubernetes.io~csi/pvc-841498d9-fa63-418c-8cc7-d94ec27f2ee2/mount type lustre (rw,flock,lazystatfs,encrypt)
</pre>

&nbsp;
&nbsp;

### Update driver version quickly by editing driver deployment directly

&nbsp;

- Update controller deployment

```console
$ kubectl edit deployment csi-azurelustre-controller -n kube-system
```

&nbsp;

- Update daemonset deployment

```console
$ kubectl edit ds csi-azurelustre-node -n kube-system
```

&nbsp;

- Change lustre CSI docker image config

```console
image: mcr.microsoft.com/k8s/csi/azurelustre-csi:v0.1.0
imagePullPolicy: Always
```

&nbsp;
&nbsp;

### Get azure lustre driver version

```console
$ kubectl exec -it csi-azurelustre-node-9ds7f -n kube-system -c azurelustre -- /bin/bash -c "./azurelustreplugin -version"
```

<pre>
Build Date: "2022-05-11T10:25:15Z"
Compiler: gc
Driver Name: azurelustre.csi.azure.com
Driver Version: v0.1.0
Git Commit: 43017c96b7cecaa09bc05ce9fad3fb9860a4c0ce
Go Version: go1.18.1
Platform: linux/amd64
</pre>

&nbsp;
&nbsp;

### Collect logs for Lustre CSI Driver Product Team for further investigation

&nbsp;

- get utility from /utils/azurelustre_log.sh, run it and share output lustre.logs with us
  
```console
$ chmod +x ./azurelustre_log.sh
$ ./azurelustre_log.sh > lustre.logs 2>&1
```

&nbsp;

### Case#3: dynamic provisioning issue (AMLFS cluster creation failure)

&nbsp;

- Symptoms
  - PVC remains in Pending status for extended periods (more than 15-20 minutes)
  - Dynamic provisioning StorageClass is configured but AMLFS cluster is not created
  - PVC events show provisioning errors or timeouts

&nbsp;

- Check PVC status and events

```console
$ kubectl describe pvc <pvc-name>
```

Look for events such as:
- `waiting for a volume to be created`
- `failed to provision volume`
- `error creating AMLFS cluster`

&nbsp;

- Check controller logs for dynamic provisioning errors

```console
$ kubectl logs -n kube-system -l app=csi-azurelustre-controller -c azurelustre --tail=100 | grep -i "dynamic\|provision\|amlfs\|create"
```

Common error patterns to look for:
- Authentication/authorization errors
- Quota exceeded errors
- Network/subnet configuration issues
- Invalid StorageClass parameters

&nbsp;

- Verify StorageClass configuration

```console
$ kubectl get storageclass <storageclass-name> -o yaml
```

Check for:
- Correct provisioner: `azurelustre.csi.azure.com`
- Valid SKU name, zone, and maintenance window parameters
- Proper network configuration (vnet-name, subnet-name, etc.)
- Resource group and location settings

&nbsp;

- Check Azure subscription quotas and limits

```console
# Check if you've reached the AMLFS cluster limit in your subscription
$ kubectl logs -n kube-system -l app=csi-azurelustre-controller -c azurelustre --tail=200 | grep -i "quota\|limit\|insufficient"
```

&nbsp;

- Verify Azure permissions for the kubelet identity

The kubelet identity needs the following permissions:
- `Microsoft.StorageCache/amlFilesystems/read`
- `Microsoft.StorageCache/amlFilesystems/write`
- `Microsoft.StorageCache/amlFilesystems/delete`
- `Microsoft.Network/virtualNetworks/subnets/read`
- `Microsoft.Network/virtualNetworks/subnets/join/action`

Check for permission errors in the controller logs:
```console
$ kubectl logs -n kube-system -l app=csi-azurelustre-controller -c azurelustre --tail=200 | grep -i "forbidden\|unauthorized\|permission"
```

&nbsp;

- Monitor AMLFS cluster creation progress in Azure portal

1. Navigate to Azure portal → Resource Groups
2. Look for the resource group specified in StorageClass (or AKS infrastructure RG if not specified)
3. Check if AMLFS cluster resource is being created
4. Review Activity Log for any deployment failures

&nbsp;

- Check for network connectivity issues

```console
# Verify the specified virtual network and subnet exist and are accessible
$ kubectl logs -n kube-system -l app=csi-azurelustre-controller -c azurelustre --tail=200 | grep -i "network\|subnet\|vnet"
```

Common network issues:
- Virtual network or subnet doesn't exist
- Insufficient IP addresses in the subnet
- Network security group blocking traffic
- Missing virtual network peering

&nbsp;

- Force retry dynamic provisioning

If the issue is transient, you can delete and recreate the PVC:

```console
$ kubectl delete pvc <pvc-name>
$ kubectl apply -f <pvc-file>.yaml
```
