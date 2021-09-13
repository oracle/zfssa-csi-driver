# Introduction

This is an end-to-end example of using filesystem 'volume clone' as the foundation
for PVCs, allowing clients to use the same base filesystem for
different volumes.

Clones on an Oracle ZFS Storage Appliance start from a snapshot of the
share, the snapshot is then cloned and made available. A snapshot takes
no space as long as the origin filesystem is unchanged.

More information about snapshots and clones on the Oracle ZFS Storage
Appliance can be found in the
[Oracle® ZFS Storage Appliance Administration Guide,](https://docs.oracle.com/cd/F13758_01/html/F13769/gprif.html)

This example comes in two parts:

* The [setup-volume](./setup-volume) helm chart starts a container with
  an attached PVC. Exec into the pod and store data onto the volume or
  use `kubectl cp` to copy files into the pod. Then the pod is deleted
  using `kubectl delete`.

* The [clone-volume](./clone-volume) helm chart starts two pods that
  use the existing PVC as the foundation for their volumes through the
  use of the Oracle ZFS Storage Appliance Snapshot/Clone operation.

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
* volSize: the size of the filesystem share to create, this can be
  changed for the clone-template example as long as the size is greater
  than or equal to the original PVC size.

Check out the parameters section of the storage class configuration file (storage-class.yaml) 
to see all supporting properties. Refer to NFS Protocol page of Oracle ZFS Storage Appliance
Administration Guide how to defind the values properly.

## Deploy a pod with a volume

Assuming there is a set of values in the local-values directory, deploy using Helm 3:

```
helm  install -f local-values/local-values.yaml setup-volume ./setup-volume
```

Once deployed, verify each of the created entities using kubectl:

1. Display the storage class (SC)
    The command `kubectl get sc` should now return something similar to this:

    ```text
    NAME                                PROVISIONER        RECLAIMPOLICY   VOLUMEBINDINGMODE   ALLOWVOLUMEEXPANSION   AGE
    zfssa-nfs-volume-clone-example-sc   zfssa-csi-driver   Delete          Immediate           false                  63s
    ```

2. Display the volume
    The command `kubectl get pvc` should return something similar to this:
    ```text
    NAME                           STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS                        AGE
    zfssa-nfs-volume-example-pvc   Bound    pvc-c7ac4970-8ae1-4dc8-ba7f-0e37a35fb39d   50Gi       RWX            zfssa-nfs-volume-clone-example-sc   7s
    ```
3. Display the pod mounting the volume

    The command `kubectl get pods` should now return something similar to this:
    ```text
    NAME                           READY   STATUS    RESTARTS   AGE
    zfssa-csi-nodeplugin-ph8qr     2/2     Running   0          8m32s
    zfssa-csi-nodeplugin-wzgpq     2/2     Running   0          8m32s
    zfssa-csi-provisioner-0        4/4     Running   0          8m32s
    zfssa-nfs-volume-example-pod   1/1     Running   0          2m21s
    ```

## Write data to the volume

Once the pod is deployed, write data to the volume in the pod:
```yaml
kubectl exec -it pod/zfssa-nfs-volume-example-pod -- /bin/sh
/ # cd /mnt
/mnt # ls
/mnt # echo "hello world" > demo.txt
/mnt # 
```

Write as much data to the volume as you would like.

## Remove the pod

For this step, do *not* use `helm uninstall` yet. Instead, delete the
example pod: `kubectl delete pod/zfssa-nfs-volume-example-pod`.

This leaves the PVC intact and bound to the share on the
Oracle ZFS Storage Appliance.

## Deploy pods using clones of the original volume

```
helm  install -f local-values/local-values.yaml clone-volume ./clone-volume
```

Once deployed, there should be two new PVCs (clones of the original) and two
new pods with the cloned volume mounted. Without writing, the cloned filesystems
will have the same data as the original filesystem. The filesystems are different
and will diverge with writes.

2. Display the volumes
   The command `kubectl get pvc` should now return something similar to this:
    ```text
    NAME                STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS       AGE
    zfssa-csi-nfs-pvc   Bound    pvc-808d9bd7-cbb0-47a7-b400-b144248f1818   10Gi       RWX            zfssa-csi-nfs-sc   8s
zfssa-nfs-volume-clone-pvc-0
zfssa-nfs-volume-clone-pvc-1

    ```
3. Display the pods mounting the clones

   The command `kubectl get pods` should now return something similar to this:
    ```text
    NAME                             READY   STATUS    RESTARTS   AGE
    pod/zfssa-csi-nodeplugin-lpts9   2/2     Running   0          25m
    pod/zfssa-csi-nodeplugin-vdb44   2/2     Running   0          25m
    pod/zfssa-csi-provisioner-0      2/2     Running   0          23m
zfssa-nfs-volume-clone-pod-0
zfssa-nfs-volume-clone-pod-1

    ```

## Verify the data to the volumes

Once the pod is deployed, write data to the volume in the pod:
```yaml
kubectl exec -it zfssa-nfs-volume-clone-pod-0 -- /bin/sh
/ # cd /mnt
/mnt # ls
/mnt # echo "hello world" > demo.txt
/mnt # 
```

## Remove the example when complete

Helm will remove all of our pods and data:
```
helm uninstall setup-volume
helm uninstall clone volume
```