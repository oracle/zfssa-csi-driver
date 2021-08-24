# Introduction

This is an end-to-end example of using NFS filesystems on a target
Oracle ZFS Storage Appliance.

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

Check out the parameters section of the storage class configuration file (storage-class.yaml) 
to see all supporting properties. Refer to NFS Protocol page of Oracle ZFS Storage Appliance
Administration Guide how to defind the values properly.

## Deployment

Assuming there is a set of values in the local-values directory, deploy using Helm 3:

```
helm  install -f local-values/local-values.yaml zfssa-nfs ./nfs
```

Once deployed, verify each of the created entities using kubectl:

1. Display the storage class (SC)
    The command `kubectl get sc` should now return something similar to this:

    ```text
    NAME               PROVISIONER        RECLAIMPOLICY   VOLUMEBINDINGMODE   ALLOWVOLUMEEXPANSION   AGE
    zfssa-csi-nfs-sc   zfssa-csi-driver   Delete          Immediate           false                  2m9s
    ```
2. Display the volume
    The command `kubectl get pvc` should now return something similar to this:
    ```text
    NAME                STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS       AGE
    zfssa-csi-nfs-pvc   Bound    pvc-808d9bd7-cbb0-47a7-b400-b144248f1818   10Gi       RWX            zfssa-csi-nfs-sc   8s
    ```
3. Display the pod mounting the volume

    The command `kubectl get all` should now return something similar to this:
    ```text
    NAME                             READY   STATUS    RESTARTS   AGE
    pod/zfssa-csi-nodeplugin-lpts9   2/2     Running   0          25m
    pod/zfssa-csi-nodeplugin-vdb44   2/2     Running   0          25m
    pod/zfssa-csi-provisioner-0      2/2     Running   0          23m
    pod/zfssa-nfs-example-pod        1/1     Running   0          12s

    NAME                                  DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
    daemonset.apps/zfssa-csi-nodeplugin   2         2         2       2            2           <none>          25m

    NAME                                     READY   AGE
    statefulset.apps/zfssa-csi-provisioner   1/1     23m

    ```

## Writing data

Once the pod is deployed, for demo, start the following analytics in a worksheet on
the Oracle ZFS Storage Appliance that is hosting the target filesystems:

Exec into the pod and write some data to the block volume:
```yaml
kubectl exec -it zfssa-nfs-example-pod -- /bin/sh
/ # cd /mnt
/mnt # ls
/mnt # echo "hello world" > demo.txt
/mnt # 
```

The analytics on the appliance should have seen the spikes as data was written.
