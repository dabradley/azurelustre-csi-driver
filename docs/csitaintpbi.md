# Summary

Hello :) We're a historically AWS shop, currently going multi-cloud and starting to run our workloads on Azure. So far the Azure Lustre CSI Driver has been working great!

One issue we ran into, is scheduling pods onto a node where the CSI failed to install. For context, the failure was due to these errors: 

```
Unpacking amlfs-lustre-client-2.15.4-42-gd6d405d:amd64 (5.15.0-1084-azure) ...
Errors were encountered while processing:
/tmp/apt-dpkg-install-5LAgDm/00-lustre-client-2.15.4-42-gd6d405d_1_amd64.deb
needrestart is being skipped since dpkg has failed
Reading package lists...
E: Sub-process /usr/bin/dpkg returned an error code (1)
```

This was a one-off issue, and not an issue in itself, the problem was pods attempting to schedule onto this node, and hanging until manually fixed.

How we've managed this in AWS, is a startup taint the CSI driver removes upon successful initialization. Here are the docs for reference:

https://github.com/kubernetes-sigs/aws-fsx-csi-driver/blob/master/docs/install.md#configure-node-startup-taint

This ensures we don't schedule onto broken nodes, which get replaced by the autoscaler auto-magically :)

# Proposal / Feature Request

The proposal is to add equivalent logic to the Azure Lustre CSI Driver. 

Users would taint nodes at startup with something like:

```
azurelustre.csi.azure.com/agent-not-ready:NoExecute
```

This taint would be removed only upon successful init of the CSI Driver.

# Summary

I'd be keen to know if something like this has been discussed, and if the community/maintainers are interested!

