# Introduction

This is an end-to-end example of taking a snapshot of an NFS filesystem
volume on a target Oracle ZFS Storage Appliance and making use of it 
on another pod by creating (restoring) a volume from the snapshot.

Prior to running this example, the NFS environment must be set up properly
on both the Kubernetes worker nodes and the Oracle ZFS Storage Appliance.
Refer to the [INSTALLATION](../../INSTALLATION.md) instructions for details.

There is a helm deployment in this example that handles initial setup of a volume
and a snapshot class:
* [Create and use initial volume](./nfs-snapshot-creator)

Then a set of resources that have to applied in order, outside a helm deployment.
* [Create and use snapshot](./nfs-snapshot-user)

The values between the deployments have to be coordinated though a local values
file and edited in the resource files. Because the creation and usage of the snapshot
does not use helm, the resource descriptions have to be modified based on
the environment for the example. There are more variables in the example then
in others, read carefully.

## Configuration

Set up a local values files. It must contain the values that customize to the 
target appliance, but can contain others. The minimum set of values to
customize are:

* appliance:
  * pool: the pool to create shares in
  * project: the project to create shares in
  * nfsServer: the NFS data path IP address
* volSize: the size of the filesystem share to create

## Initial share creation

This step includes deploying a pod with an NFS volume attached using a regular 
storage class and a persistent volume claim. It also deploys a volume snapshot class
required to take snapshots of the persistent volume in a later section.

From the nfs-vsc directory, the command to create the initial volume and snapshot
looks similar to the following (depending on your environment). Remember it is
always useful to use 'helm template' prior to installing to ensure the setup will
be correct.

```text
helm install -f local-values/local-values.yaml zfssa-nfs-vsc ./nfs-snapshot-creator
```

Once deployed, verify each of the created entities using kubectl:

1. Display the storage class (SC)
    The command `kubectl get sc` should now return something similar to this:

    ```text
	NAME                      PROVISIONER        RECLAIMPOLICY   VOLUMEBINDINGMODE   ALLOWVOLUMEEXPANSION   AGE
	zfssa-nfs-vs-example-sc   zfssa-csi-driver   Delete          Immediate           false                  86s
    ```
2. Display the volume claim
    The command `kubectl get pvc` should now return something similar to this:
    ```text
	NAME                       STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS              AGE
	zfssa-nfs-vs-example-pvc   Bound    pvc-0c1e5351-dc1b-45a4-8f54-b28741d1003e   10Gi       RWX            zfssa-nfs-vs-example-sc   86s
    ```
3. Display the volume snapshot class
    The command `kubectl get volumesnapshotclass` should now return something similar to this:
    ```text
	NAME                       DRIVER             DELETIONPOLICY   AGE
	zfssa-nfs-vs-example-vsc   zfssa-csi-driver   Delete           86s
    ```
4. Display the pod mounting the volume

    The command `kubectl get pod` should now return something similar to this:
    ```text
	NAME                         READY   STATUS    RESTARTS   AGE
    snapshot-controller-0        1/1     Running   0          6d6h
    zfssa-csi-nodeplugin-dx2s4   2/2     Running   0          24m
    zfssa-csi-nodeplugin-q9h9w   2/2     Running   0          24m
    zfssa-csi-provisioner-0      4/4     Running   0          24m
    zfssa-nfs-vs-example-pod     1/1     Running   0          86s
    ```

## Writing data

Once the pod is deployed, verify the volume is mounted and can be written. 

```text
kubectl exec -it zfssa-nfs-vs-example-pod -- /bin/sh

/ # cd /mnt
/mnt # 
/mnt # date > timestamp.txt
/mnt # cat timestamp.txt 
Tue Jan 19 23:13:10 UTC 2021
```

## Creating snapshot 

Use configuration files in the nfs-snapshot-user directory with proper modifications 
for the rest of the example steps.

Create a snapshot of the volume by running the command below:

```text
kubectl apply -f nfs-snapshot-user/nfs-snapshot.yaml
```

Verify the volume snapshot is created and available by running the following command:

```text
kubectl get volumesnapshot
```

Wait until the READYTOUSE of the snapshot becomes true before moving on to the next steps. 
It is important to use the RESTORESIZE value of the volume snapshot just created when specifying
the storage capacity of a persistent volume claim to provision a persistent volume using this 
snapshot. For example, the storage capacity in nfs-snapshot-user/nfs-pvc-from-snapshot.yaml.

Optionally, verify the volume snapshot exists on the Oracle ZFS Storage Appliance. The snapshot name
on the Oracle ZFS Storage Appliance should have the volume snapshot UID as the suffix.

## Creating persistent volume claim 

Create a persistent volume claim to provision a volume from the snapshot by running
the command below. Be aware that the persistent volume provisioned by this persistent volume claim
is not expandable. Create a new storage class with allowVolumeExpansion: true and use it when 
specifying the persistent volume claim.

```text
kubectl apply -f nfs-snapshot-user/nfs-pvc-from-snapshot.yaml
```

Verify the persistent volume claim is created and a volume is provisioned by running the following command:

```text
kubectl get pv,pvc
```

The command `kubectl get pv,pvc` should return something similar to this:
```text
NAME                                                        CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM                              STORAGECLASS              REASON   AGE
persistentvolume/pvc-0c1e5351-dc1b-45a4-8f54-b28741d1003e   10Gi       RWX            Delete           Bound    default/zfssa-nfs-vs-example-pvc   zfssa-nfs-vs-example-sc            34m
persistentvolume/pvc-59d8d447-302d-4438-a751-7271fbbe8238   10Gi       RWO            Delete           Bound    default/zfssa-nfs-vs-restore-pvc   zfssa-nfs-vs-example-sc            112s

NAME                                             STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS              AGE
persistentvolumeclaim/zfssa-nfs-vs-example-pvc   Bound    pvc-0c1e5351-dc1b-45a4-8f54-b28741d1003e   10Gi       RWX            zfssa-nfs-vs-example-sc   34m
persistentvolumeclaim/zfssa-nfs-vs-restore-pvc   Bound    pvc-59d8d447-302d-4438-a751-7271fbbe8238   10Gi       RWO            zfssa-nfs-vs-example-sc   116s
```

Optionally, verify the new volume exists on the Oracle ZFS Storage Appliance. Notice that the new
volume is a clone off the snapshot taken from the original volume.

## Creating pod using restored volume

Create a pod with the persistent volume claim created from the above step by running the command below:

```text
kubectl apply -f nfs-snapshot-user/nfs-pod-restored-volume.yaml
```

The command `kubectl get pod` should now return something similar to this:
```text
NAME                         READY   STATUS    RESTARTS   AGE
snapshot-controller-0        1/1     Running   0          6d7h
zfssa-csi-nodeplugin-dx2s4   2/2     Running   0          68m
zfssa-csi-nodeplugin-q9h9w   2/2     Running   0          68m
zfssa-csi-provisioner-0      4/4     Running   0          68m
zfssa-nfs-vs-example-pod     1/1     Running   0          46m
zfssa-nfs-vs-restore-pod     1/1     Running   0          37s
```

Verify the new volume has the contents of the original volume at the point in time 
when the snapsnot was taken.

```text
kubectl exec -it zfssa-nfs-vs-restore-pod -- /bin/sh

/ # cd /mnt
/mnt # 
/mnt # cat timestamp.txt 
Tue Jan 19 23:13:10 UTC 2021
```

## Deleting pod, persistent volume claim and volume snapshot

To delete the pod, persistent volume claim and volume snapshot created from the above steps,
run the following commands below. Wait until the resources being deleted disappear from
the list that `kubectl get ...` command displays before running the next command.

```text
kubectl delete -f nfs-snapshot-user/nfs-pod-restored-volume.yaml
kubectl delete -f nfs-snapshot-user/nfs-pvc-from-snapshot.yaml
kubectl delete -f nfs-snapshot-user/nfs-snapshot.yaml
```

Once the clones and snapshots are deleted, uninstall the initial helm deployment:
```text
helm uninstall zfssa-nfs-vsc
```
