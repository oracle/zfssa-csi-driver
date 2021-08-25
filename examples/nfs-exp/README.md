# Introduction

This is an end-to-end example of using NFS filesystems and expanding the volume size
on a target Oracle ZFS Storage Appliance.

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
helm  install -f ../local-values/local-values.yaml zfssa-nfs-exp ./
```

Once deployed, verify each of the created entities using kubectl:

1. Display the storage class (SC)
    The command `kubectl get sc` should now return something similar to this:

    ```text
	NAME                       PROVISIONER        RECLAIMPOLICY   VOLUMEBINDINGMODE   ALLOWVOLUMEEXPANSION   AGE
	zfssa-nfs-exp-example-sc   zfssa-csi-driver   Delete          Immediate           true                   15s
    ```
2. Display the volume claim
    The command `kubectl get pvc` should now return something similar to this:
    ```text
	NAME                        STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS               AGE
	zfssa-nfs-exp-example-pvc   Bound    pvc-8325aaa0-bbe3-495b-abb0-0c43cc309624   10Gi       RWX            zfssa-nfs-exp-example-sc   108s
    ```
3. Display the pod mounting the volume

    The command `kubectl get pod` should now return something similar to this:
    ```text
	NAME                         READY   STATUS    RESTARTS   AGE
	zfssa-csi-nodeplugin-xmv96   2/2     Running   0          43m
	zfssa-csi-nodeplugin-z5tmm   2/2     Running   0          43m
	zfssa-csi-provisioner-0      4/4     Running   0          43m
	zfssa-nfs-exp-example-pod    1/1     Running   0          3m23s
    ```

## Writing data

Once the pod is deployed, for demo, start the following analytics in a worksheet on
the Oracle ZFS Storage Appliance that is hosting the target filesystems:

Exec into the pod and write some data to the NFS volume:
```text
kubectl exec -it zfssa-nfs-exp-example-pod -- /bin/sh

/ # cd /mnt
/mnt # df -h
Filesystem                Size      Used Available Use% Mounted on
overlay                  38.4G     15.0G     23.4G  39% /
tmpfs                    64.0M         0     64.0M   0% /dev
tmpfs                    14.6G         0     14.6G   0% /sys/fs/cgroup
shm                      64.0M         0     64.0M   0% /dev/shm
tmpfs                    14.6G      1.4G     13.2G   9% /tmp/resolv.conf
tmpfs                    14.6G      1.4G     13.2G   9% /etc/hostname
<ZFSSA_IP_ADDR>:/export/pvc-8325aaa0-bbe3-495b-abb0-0c43cc309624
                         10.0G         0     10.0G   0% /mnt
...
/mnt # dd if=/dev/zero of=/mnt/data count=1024 bs=1024
1024+0 records in
1024+0 records out
/mnt # df -h
Filesystem                Size      Used Available Use% Mounted on
overlay                  38.4G     15.0G     23.4G  39% /
tmpfs                    64.0M         0     64.0M   0% /dev
tmpfs                    14.6G         0     14.6G   0% /sys/fs/cgroup
shm                      64.0M         0     64.0M   0% /dev/shm
tmpfs                    14.6G      1.4G     13.2G   9% /tmp/resolv.conf
tmpfs                    14.6G      1.4G     13.2G   9% /etc/hostname
<ZFSSA_IP_ADDR>:/export/pvc-8325aaa0-bbe3-495b-abb0-0c43cc309624
                         10.0G      1.0M     10.0G   0% /mnt

/mnt # 
```

The analytics on the appliance should have seen the spikes as data was written.

## Expanding volume capacity

After verifying the initially requested capaicy of the NFS volume is provisioned and usable,
exercise expanding of the volume capacity by editing the deployed Persistent Volume Claim.

Copy ./templates/01-pvc.yaml to /tmp/nfs-exp-pvc.yaml and modify this yaml file for volume expansion, for example:
```text
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: zfssa-nfs-exp-example-pvc
spec:
  accessModes:
    - ReadWriteMany
  volumeMode: Filesystem
  resources:
    requests:
      storage: "20Gi"
  storageClassName: zfssa-nfs-exp-example-sc
```
Then, apply the updated PVC configuration by running 'kubectl apply -f /tmp/nfs-exp-pvc.yaml' command. Note that the command will return a warning message similar to the following:
```text
Warning: kubectl apply should be used on resource created by either kubectl create --save-config or kubectl apply
```

Alternatively, you can perform volume expansion on the fly using 'kubectl edit' command. 
```text
kubectl edit pvc/zfssa-nfs-exp-example-pvc

...
spec:
  accessModes:
  - ReadWriteMany
  resources:
    requests:
      storage: 10Gi
  storageClassName: zfssa-nfs-exp-example-sc
    volumeMode: Filesystem
  volumeName: pvc-27281fde-be45-436d-99a3-b45cddbc74d1
status:
  accessModes:
  - ReadWriteMany
  capacity:
    storage: 10Gi
  phase: Bound
...

Modify the capacity from 10Gi to 20Gi on both spec and status sectioins, then save and exit the edit mode.
```

The command `kubectl get pv,pvc` should now return something similar to this:
```text
kubectl get pv,pvc,sc
NAME                                                        CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM                               STORAGECLASS               REASON   AGE
persistentvolume/pvc-8325aaa0-bbe3-495b-abb0-0c43cc309624   20Gi       RWX            Delete           Bound    default/zfssa-nfs-exp-example-pvc   zfssa-nfs-exp-example-sc            129s

NAME                                              STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS               AGE
persistentvolumeclaim/zfssa-nfs-exp-example-pvc   Bound    pvc-8325aaa0-bbe3-495b-abb0-0c43cc309624   20Gi       RWX            zfssa-nfs-exp-example-sc   132s
```

Exec into the pod and Verify the size of the mounted NFS volume is expanded:
```text
kubectl exec -it zfssa-nfs-exp-example-pod -- /bin/sh

/ # cd /mnt
/mnt # df -h
Filesystem                Size      Used Available Use% Mounted on
overlay                  38.4G     15.0G     23.4G  39% /
tmpfs                    64.0M         0     64.0M   0% /dev
tmpfs                    14.6G         0     14.6G   0% /sys/fs/cgroup
shm                      64.0M         0     64.0M   0% /dev/shm
tmpfs                    14.6G      1.4G     13.2G   9% /tmp/resolv.conf
tmpfs                    14.6G      1.4G     13.2G   9% /etc/hostname
<ZFSSA_IP_ADDR>:/export/pvc-8325aaa0-bbe3-495b-abb0-0c43cc309624
                         20.0G      1.0M     20.0G   0% /mnt
...
```
