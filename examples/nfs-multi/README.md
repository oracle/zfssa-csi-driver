# Introduction

This is an end-to-end example of using NFS filesystems on a target
Oracle ZFS Storage Appliance. It creates several PVCs and optionally
creates a pod to consume them.

This example also illustrates the use of namespaces with PVCs and pods.
Be aware that PVCs and pods will be created in the user defined namespace
not in the default namespace as in other examples.

Prior to running this example, the NFS environment must be set up properly
on both the Kubernetes worker nodes and the Oracle ZFS Storage Appliance.
Refer to the [INSTALLATION](../../INSTALLATION.md) instructions for details.

## Configuration

Set up a local values files. It must contain the values that customize to the 
target appliance, but can container others. The minimum set of values to
customize are:

* appliance:
  * targetGroup: the target group that contains data path interfaces on the target appliance
  * pool: the pool to create shares in
  * project: the project to create shares in
  * targetPortal: the target iSCSI portal on the appliance
  * nfsServer: the NFS data path IP address
* volSize: the size of the filesystem share to create

## Deployment

Assuming there is a set of values in the local-values directory, deploy using Helm 3:

```
helm  install -f ../local-values/local-values.yaml zfssa-nfs-multi ./
```

## Check pod mounts

If you enabled the use of the test pod, exec into it and check the NFS volumes:

```
kubectl exec -n zfssa-nfs-multi -it zfssa-nfs-multi-example-pod -- /bin/sh
/ # cd /mnt
/mnt # ls
ssec0     ssec1     ssec2     ssg       ssp-many
```
