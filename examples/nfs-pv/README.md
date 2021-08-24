# Introduction

This is an end-to-end example of using an existing filesystem share on a target
Oracle ZFS Storage Appliance.

Prior to running this example, the NFS environment must be set up properly
on both the Kubernetes worker nodes and the Oracle ZFS Storage Appliance.
Refer to the [INSTALLATION](../../INSTALLATION.md) instructions for details.

This flow to use an existing volume is:
* create a persistent volume (PV) object
* allocate it to a persistent volume claim (PVC)
* use the PVC from a pod

The following must be set up:
* the volume handle must be a fully formed volume id
* there must be volume attributes defined as part of the persistent volume

In this example, the volume handle is constructed via values in the helm
chart. The only new attribute necessary is the name of the volume on the
target appliance. The remaining is assembled from the information that is still
in the local-values.yaml file (appliance name, pool, project, etc...).

The resulting VolumeHandle appears similar to the following, with the values
in ```<>``` filled in from the helm variables:

```
    volumeHandle: /nfs/<appliance name>/<volume name>/<pool name>/local/<project name>/<volume name>
```
From the above, note that the volumeHandle is in the form of an ID with the components:
* 'nfs' - denoting an exported NFS share
* 'appliance name' - this is the management path of the ZFSSA target appliance
* 'volume name' - the name of the share on the appliance
* 'pool name' - the pool on the target appliance
* 'local' - denotes that the pool is owned by the head
* 'project' - the project that the share is in

In the volume attributes, nfsServer must be defined.

Once created, a persistent volume claim can be made for this share and used in a pod.

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
* applianceName: the existing appliance name (this is the management path)
* pvExistingFilesystemName: the name of the filesystem share on the target appliance
* volMountPoint: the mount point on the target appliance of the filesystem share 
* volSize: the size of the filesystem share 

On the target appliance, ensure that the filesystem share is exported via NFS.

## Deployment

Assuming there is a set of values in the local-values directory, deploy using Helm 3:

```
helm  install -f ../local-values/local-values.yaml zfssa-nfs-existing ./
```

Once deployed, verify each of the created entities using kubectl:

```
kubectl get sc
kubectl get pvc
kubectl get pod
```

## Writing data

Once the pod is deployed, for demo, start the following analytics in a worksheet on
the Oracle ZFS Storage Appliance that is hosting the target filesystems:

Exec into the pod and write some data to the block volume:
```yaml
kubectl exec -it zfssa-fs-existing-pod  -- /bin/sh
/ # cd /mnt
/mnt # ls
/mnt # echo "hello world" > demo.txt
/mnt # 
```

The analytics on the appliance should have seen the spikes as data was written.
