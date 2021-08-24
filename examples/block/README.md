# Introduction

This is an end-to-end example of using iSCSI block devices on a target
Oracle ZFS Storage Appliance.

Prior to running this example, the iSCSI environment must be set up properly
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
  * targetGroup: the target iSCSI group to use on the appliance
  * nfsServer: the NFS data path IP address
* volSize: the size of the block volume (iSCSI LUN) to create

## Deployment

Assuming there is a set of values in the local-values directory, deploy using Helm 3:

```
helm  install -f ../local-values/local-values.yaml zfssa-block ./
```

Once deployed, verify each of the created entities using kubectl:

```
kubectl get sc
kubectl get pvc
kubectl get pod
```

## Writing data

Once the pod is deployed, for demo, start the following analytics in a worksheet on
the Oracle ZFS Storage Appliance that is hosting the target LUNs:

* Protocol -> iSCSI bytes broken down by initiator
* Protocol -> iSCSI bytes broken down by target
* Protocol -> iSCSI bytes broken down by LUN

Exec into the pod and write some data to the block volume:
```yaml
kubectl exec -it zfssa-block-example-pod -- /bin/sh
/ # cd /dev
/dev # ls
block            fd               mqueue           ptmx             random           stderr           stdout           tty              zero
core             full             null             pts              shm              stdin            termination-log  urandom
/dev # dd if=/dev/zero of=/dev/block count=1024 bs=1024
1024+0 records in
1024+0 records out
/dev # 
```

The analytics on the appliance should have seen the spikes as data was written.
