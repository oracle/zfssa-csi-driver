# Introduction

This is an end-to-end example of taking a snapshot of a block volume (iSCSI Lun) 
on a target Oracle ZFS Storage Appliance and making use of it
on another pod by creating (restoring) a volume from the snapshot.

Prior to running this example, the iSCSI environment must be set up properly
on both the Kubernetes worker nodes and the Oracle ZFS Storage Appliance.
Refer to the [INSTALLATION](../../INSTALLATION.md) instructions for details.

## Configuration

Set up a local values files. It must contain the values that customize to the 
target appliance, but can contain others. The minimum set of values to
customize are:

* appliance:
  * pool: the pool to create shares in
  * project: the project to create shares in
  * targetPortal: the target iSCSI portal on the appliance
  * targetGroup: the target iSCSI group to use on the appliance
* volSize: the size of the iSCSI LUN share to create

## Enabling Volume Snapshot Feature (Only for Kubernetes v1.17 - v1.19)

The Kubernetes Volume Snapshot feature became GA in Kubernetes v1.20. In order to use
this feature in Kubernetes pre-v1.20, it MUST be enabled prior to deploying ZS CSI Driver. 
To enable the feature on Kubernetes pre-v1.20, follow the instructions on 
[INSTALLATION](../../INSTALLATION.md).

## Deployment

This step includes deploying a pod with a block volume attached using a regular 
storage class and a persistent volume claim. It also deploys a volume snapshot class
required to take snapshots of the persistent volume.

Assuming there is a set of values in the local-values directory, deploy using Helm 3:

```text
helm ../install -f local-values/local-values.yaml zfssa-block-vsc ./
```

Once deployed, verify each of the created entities using kubectl:

1. Display the storage class (SC)
    The command `kubectl get sc` should now return something similar to this:

    ```text
	NAME                        PROVISIONER        RECLAIMPOLICY   VOLUMEBINDINGMODE   ALLOWVOLUMEEXPANSION   AGE
	zfssa-block-vs-example-sc   zfssa-csi-driver   Delete          Immediate           false                  86s
    ```
2. Display the volume claim
    The command `kubectl get pvc` should now return something similar to this:
    ```text
	NAME                         STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS                AGE
	zfssa-block-vs-example-pvc   Bound    pvc-477804b4-e592-4039-a77c-a1c99a1e537b   10Gi       RWO            zfssa-block-vs-example-sc   62s
    ```
3. Display the volume snapshot class
    The command `kubectl get volumesnapshotclass` should now return something similar to this:
    ```text
	NAME                         DRIVER             DELETIONPOLICY   AGE
	zfssa-block-vs-example-vsc   zfssa-csi-driver   Delete           100s
    ```
4. Display the pod mounting the volume

    The command `kubectl get pod` should now return something similar to this:
    ```text
    NAME                         READY   STATUS    RESTARTS   AGE
    snapshot-controller-0        1/1     Running   0          14d
    zfssa-block-vs-example-pod   1/1     Running   0          2m11s
    zfssa-csi-nodeplugin-7kj5m   2/2     Running   0          3m11s
    zfssa-csi-nodeplugin-rgfzf   2/2     Running   0          3m11s
    zfssa-csi-provisioner-0      4/4     Running   0          3m11s
    ```

## Writing data

Once the pod is deployed, verify the block volume is mounted and can be written. 

```text
kubectl exec -it zfssa-block-vs-example-pod -- /bin/sh

/ # cd /dev
/dev # 
/dev # date > block
/dev # dd if=block bs=64 count=1
Wed Jan 27 22:06:36 UTC 2021
1+0 records in
1+0 records out
/dev #
```
Alternatively, `cat /dev/block` followed by `CTRL-C` can be used to view the timestamp written on th /dev/block device file.

## Creating snapshot 

Use configuration files in examples/block-snapshot directory with proper modifications 
for the rest of the example steps.

Create a snapshot of the volume by running the command below:

```text
kubectl apply -f ../block-snapshot/block-snapshot.yaml
```

Verify the volume snapshot is created and available by running the following command:

```text
kubectl get volumesnapshot
```

Wait until the READYTOUSE of the snapshot becomes true before moving on to the next steps.
It is important to use the RESTORESIZE value of the volume snapshot just created when specifying
the storage capacity of a persistent volume claim to provision a persistent volume using this
snapshot. For example, the storage capacity in ../block-snapshot/block-pvc-from-snapshot.yaml

Optionally, verify the volume snapshot exists on ZFS Storage Appliance. The snapshot name
on ZFS Storage Appliance should have the volume snapshot UID as the suffix.

## Creating persistent volume claim 

Create a persistent volume claim to provision a volume from the snapshot by running
the command below. Be aware that the persistent volume provisioned by this persistent volume claim
is not expandable. Create a new storage class with allowVolumeExpansion: true and use it when
specifying the persistent volume claim.

```text
kubectl apply -f ../block-snapshot/block-pvc-from-snapshot.yaml
```

Verify the persistent volume claim is created and a volume is provisioned by running the following command:

```text
kubectl get pv,pvc
```

The command `kubectl get pv,pvc` should return something similar to this:
```text
NAME                                                        CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM                                STORAGECLASS                REASON   AGE
persistentvolume/pvc-477804b4-e592-4039-a77c-a1c99a1e537b   10Gi       RWO            Delete           Bound    default/zfssa-block-vs-example-pvc   zfssa-block-vs-example-sc            13m
persistentvolume/pvc-91f949f6-5d77-4183-bab5-adfdb1452a90   10Gi       RWO            Delete           Bound    default/zfssa-block-vs-restore-pvc   zfssa-block-vs-example-sc            11s

NAME                                               STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS                AGE
persistentvolumeclaim/zfssa-block-vs-example-pvc   Bound    pvc-477804b4-e592-4039-a77c-a1c99a1e537b   10Gi       RWO            zfssa-block-vs-example-sc   13m
persistentvolumeclaim/zfssa-block-vs-restore-pvc   Bound    pvc-91f949f6-5d77-4183-bab5-adfdb1452a90   10Gi       RWO            zfssa-block-vs-example-sc   16s
```

Optionally, verify the new volume exists on ZFS Storage Appliance. Notice that the new
volume is a clone off the snapshot taken from the original volume.

## Creating pod using restored volume

Create a pod with the persistent volume claim created from the above step by running the command below:

```text
kubectl apply -f ../block-snapshot/block-pod-restored-volume.yaml
```

The command `kubectl get pod` should now return something similar to this:
```text
NAME                         READY   STATUS    RESTARTS   AGE
snapshot-controller-0        1/1     Running   0          14d
zfssa-block-vs-example-pod   1/1     Running   0          15m
zfssa-block-vs-restore-pod   1/1     Running   0          21s
zfssa-csi-nodeplugin-7kj5m   2/2     Running   0          16m
zfssa-csi-nodeplugin-rgfzf   2/2     Running   0          16m
zfssa-csi-provisioner-0      4/4     Running   0          16m
```

Verify the new volume has the contents of the original volume at the point in time 
when the snapsnot was taken.

```text
kubectl exec -it zfssa-block-vs-restore-pod -- /bin/sh

/ # cd /dev
/dev # dd if=block bs=64 count=1
Wed Jan 27 22:06:36 UTC 2021
1+0 records in
1+0 records out
/dev # 
```

## Deleting pod, persistent volume claim and volume snapshot

To delete the pod, persistent volume claim and volume snapshot created from the above steps,
run the following commands below. Wait until the resources being deleted disappear from
the list that `kubectl get ...` command displays before running the next command.

```text
kubectl delete -f ../block-snapshot/block-pod-restored-volume.yaml
kubectl delete -f ../block-snapshot/block-pvc-from-snapshot.yaml
kubectl delete -f ../block-snapshot/block-snapshot.yaml
```
