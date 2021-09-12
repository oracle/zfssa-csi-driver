# Installation of Oracle ZFS Storage Appliance CSI Plugin
This document reviews how to install and make use of the driver.

## Requirements
Ensure the following information and requirements can be met prior to installation.

* The following ZFS Storage Appliance information (see your ZFFSA device administrator):

    * The name or the IP address of the appliance. If it's a name, it must be DNS resolvable.
	  When the appliance is a clustered system, the connection for management operations is tied
      to a head in driver deployment. You will see different driver behaviors in takeover/failback
      scenarios with the target storage appliance depending on the management interface settings and
      if it remains locked to the failed node or not.
    * A login access to your ZFSSA in the form of a user login and associated password. 
	  It is desirable to create a normal login user with required authorizations.
    * The appliance certificate for the REST endpoint is available.
    * The name of the appliance storage pool from which volumes will be provisioned.
    * The name of the project in the pool.
    * In secure mode, the driver supports only TLSv1.2 for HTTPS connection to ZFSSA. Make sure that
	  TLSv1.2 is enabled for HTTPS service on ZFSSA.
    
    The user on the appliance must have a minimum of the following authorizations (where pool and project are those
    that will be used in the storage class), root should not be used for provisioning.
     
    * Object: nas.<pool>.<project>.*
        * Permissions:
            * changeAccessProps
            * changeGeneralProps
            * changeProtocolProps
            * changeSpaceProps
            * changeUserQuota
            * clearLocks
            * clone
            * createShare
            * destroy
            * rollback
            * scheduleSnap
            * takeSnap
            * destroySnap
            * renameSnap

    The File system being exported must have 'Share Mode' set to 'Read/Write' in the section 'NFS' of the tab 'Protocol'
    of the file system (Under 'Shares').

    More than one pool/project are possible if there are storage classes that identify different
    pools and projects.

* The Kubernetes cluster namespace you must use (see your cluster administrator)
* Sidecar images

    Make sure you have access to the registry or registries containing these images from the worker nodes. The image pull
    policy (`imagePullPolicy`) is set to `IfNotPresent` in the deployment files. During the first deployment the
    Container Runtime will likely try to pull them. If your Container Runtime cannot access the images you will have to
    pull them manually before deployment. The required images are: 

    * node-driver-registar v2.7.0+.
    * external-attacher v4.1.0+.
    * external-provisioner v3.4.0+.
    * external-resizer v1.7.0+.
    * external-snapshotter v6.2.1+.

    The current deployment uses the sidecar images built by Oracle and available
    from the Oracle Container Registry (container-registry.oracle.com/olcne/).
    Refer to the [current deployment for more information](deploy/helm/k8s-1.25/values.yaml).
    
* Plugin image

    You can pull the plugin image from a registry that you know hosts it or you can generate it and store it in one of
    your registries. In any case, as for the sidecar images, the Container Runtime must have access to that registry.
    If not you will have to pull it manually before deployment. If you choose to generate the plugin yourself use
    version 1.21.0 or above of the Go compiler.

## Setup

This volume driver supports both NFS (filesystem) and iSCSI (block) volumes. Preparation for iSCSI, at this time, will
take some setup, please see the information below.

### iSCSI Environment

Install iSCSI client utilities on the Kubernetes worker nodes:

```bash
$ yum install iscsi-initiator-utils -y
```

Verify `iscsid` and `iscsi` are running after installation (systemctl status iscsid iscsi).

* Create an initiator group on the Oracle ZFS Storage Appliance *per worker node name*. For example, if
your worker node name is `pmonday-olcne-worker-0`, then there should be an initiator group named `pmonday-olcne-worker-0`
on the target appliance with the IQN of the worker node. The initiator can be determined by looking at
`/etc/iscsi/initiatorname.iscsi`.
* Create one or more targets and target groups on the interface that you intend to use for iSCSI traffic.
* CHAP is not supported at this time.
* Cloud instances often have duplicate IQNs, these MUST be regenerated and unique or connection storms
happen ([Instructions](https://www.thegeekdiary.com/how-to-modify-the-iscsi-initiator-id-in-linux/)).
* There are cases where fresh instances do not start the iscsi service properly with the following,
modify the iscsi.service to remove the ConditionDirectoryNotEmpty temporarily
```
Condition: start condition failed at Wed 2020-10-28 18:37:35 GMT; 1 day 4h ago
           ConditionDirectoryNotEmpty=/var/lib/iscsi/nodes was not met
```
* iSCSI may get timeouts in particular networking conditions. Review the following web pages for possible
solutions. The first involves
[modifying sysctl](https://www.thegeekdiary.com/iscsiadm-discovery-timeout-with-two-or-more-network-interfaces-in-centos-rhel/),
the second involves changing the 
[replacement timeout](https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/6/html/storage_administration_guide/iscsi-replacement-timeout)
for iSCSI.

* There is a condition where a 'uefi' target creates noise in iscsi discovery, this is noticeable in the
 iscsid output (systemctl status iscsid). This issue appears to be in Oracle Linux 7 in a virtualized
 environment:
```
● iscsid.service - Open-iSCSI
   Loaded: loaded (/usr/lib/systemd/system/iscsid.service; disabled; vendor preset: disabled)
   Active: active (running) since Wed 2020-10-28 17:30:17 GMT; 1 day 23h ago
     Docs: man:iscsid(8)
           man:iscsiuio(8)
           man:iscsiadm(8)
 Main PID: 1632 (iscsid)
   Status: "Ready to process requests"
    Tasks: 1
   Memory: 6.4M
   CGroup: /system.slice/iscsid.service
           └─1632 /sbin/iscsid -f -d2

Oct 30 16:23:02 pbm-kube-0-w1 iscsid[1632]: iscsid: disconnecting conn 0x56483ca0f050, fd 7
Oct 30 16:23:02 pbm-kube-0-w1 iscsid[1632]: iscsid: connecting to 169.254.0.2:3260
Oct 30 16:23:02 pbm-kube-0-w1 iscsid[1632]: iscsid: connect to 169.254.0.2:3260 failed (Connection refused)
Oct 30 16:23:02 pbm-kube-0-w1 iscsid[1632]: iscsid: deleting a scheduled/waiting thread!
Oct 30 16:23:03 pbm-kube-0-w1 iscsid[1632]: iscsid: Poll was woken by an alarm
Oct 30 16:23:03 pbm-kube-0-w1 iscsid[1632]: iscsid: re-opening session -1 (reopen_cnt 55046)
Oct 30 16:23:03 pbm-kube-0-w1 iscsid[1632]: iscsid: disconnecting conn 0x56483cb55e60, fd 9
Oct 30 16:23:03 pbm-kube-0-w1 iscsid[1632]: iscsid: connecting to 169.254.0.2:3260
Oct 30 16:23:03 pbm-kube-0-w1 iscsid[1632]: iscsid: connect to 169.254.0.2:3260 failed (Connection refused)
Oct 30 16:23:03 pbm-kube-0-w1 iscsid[1632]: iscsid: deleting a scheduled/waiting thread!
```

### NFS Environment

Ensure that:
 
* All worker nodes have the NFS packages installed for their Operating System:
 
  ```bash
  $ yum install nfs-utils -y
  ```
* All worker nodes are running the daemon `rpc.statd`

### Enabling Kubernetes Volume Snapshot Feature (Only for Kubernetes v1.17 - v1.19)

The Kubernetes Volume Snapshot feature became GA in Kubernetes v1.20. In order to use
this feature in Kubernetes pre-v1.20, it MUST be enabled prior to deploying ZS CSI Driver. 
To enable the feature on Kubernetes pre-v1.20, deploy API extensions, associated configurations,
and a snapshot controller by running the following command in deploy directory:

```text
kubectl apply -R -f k8s-1.17/snapshot-controller
```

This command will report creation of resources and configuratios as follows:

```text
customresourcedefinition.apiextensions.k8s.io/volumesnapshotclasses.snapshot.storage.k8s.io created
customresourcedefinition.apiextensions.k8s.io/volumesnapshotcontents.snapshot.storage.k8s.io created
customresourcedefinition.apiextensions.k8s.io/volumesnapshots.snapshot.storage.k8s.io created
serviceaccount/snapshot-controller created
clusterrole.rbac.authorization.k8s.io/snapshot-controller-runner created
clusterrolebinding.rbac.authorization.k8s.io/snapshot-controller-role created
role.rbac.authorization.k8s.io/snapshot-controller-leaderelection created
rolebinding.rbac.authorization.k8s.io/snapshot-controller-leaderelection created
statefulset.apps/snapshot-controller created
```

The details of them can be viewed using kubectl get <resource-type> command. Note that the command
above deploys a snapshot-controler in the default namespace by default. The command
`kubectl get all` should present something similar to this:

```text
NAME                        READY   STATUS    RESTARTS   AGE
pod/snapshot-controller-0   1/1     Running   0          5h22m
...
NAME                                   READY   AGE
statefulset.apps/snapshot-controller   1/1     5h22m
```

### CSI Volume Plugin Deployment from Helm

A sample Helm chart is available in the deploy/helm directory, this method can be used for 
simpler deployment than the section below.

Create a local-values.yaml file that, at a minimum, sets the values for the
zfssaInformation section. Depending on your environment, the image block may
also need updates if the identified repositories cannot be reached.

The secrets must be encoded. There are many ways to Base64 strings and files,
this technique would encode a user name of 'demo' for use in the values file on
a Mac with the base64 tool installed:

```bash
echo -n 'demo' | base64
```
The following example shows how to get the server certificate of ZFSSA and encode it:
```bash
openssl s_client -connect <zfssa>:215 2>/dev/null </dev/null | sed -ne '/-BEGIN CERTIFICATE-/,/-END CERTIFICATE-/p' | base64
```

Deploy the driver using Helm 3:

```bash
helm install -f local-values/local-values.yaml zfssa-csi ./k8s-1.17
```

When all pods are running, move to verification.

### CSI Volume Plugin Deployment from YAML

To deploy the plugin using YAML files, follow the steps listed below.
They assume you are installing at least version 0.4.0 of the plugin
on a cluster running version 1.17 of Kubernetes, and you are using Kubernetes secrets to provide the appliance login
access information and certificate. They also use generic information described below. When following these steps change
the values to your own values.         

| Information | Value |
| --- | --- |
| Appliance name or IP address | _myappliance_ |
| Appliance login user | _mylogin_ |
| Appliance password | _mypassword_ |
| Appliance file certificate | _mycertfile_ |
| Appliance storage pool | _mypool_ |
| Appliance storage project | _myproject_ |
| Cluster namespace | _myspace_ |

1. The driver requires a file (zfssa.yaml) mounted as a volume to /mnt/zfssa. The volume should be an
   in memory volume and the file should
   be provided by a secure secret service that shares the secret via a sidecar, such as a Hashicorp Vault
   agent that interacts with vault via role-based access controls.
   ```yaml
   username: <text>
   password: <text>
   ```
    For development only, other mechanisms can be used to create and share the secret with the container.

    *Warning* Do not store your credentials in source code control (such as this project). For production
    environments use a secure secret store that encrypts at rest and can provide credentials through role
    based access controls (refer to Kubernetes documentation). Do not use root user in production environments,
    a purpose-built user will provide better audit controls and isolation for the driver.

2. Create the Kubernetes secret containing the certificate chain for the appliance and make it available to the
   driver in a mounted volume (/mnt/certs) and file name of zfssa.crt. While a certificate chain is a public
   document, it is typically also provided by a volume mounted from a secret provider to protect the chain
   of trust and bind it to the instance.
   
   To create a Kubernetes-secret from the certificate chain:
    
   ```text
   kubectl create secret generic oracle.zfssa.csi.node.myappliance.certs -n myspace --from-file=./mycertfile
   ```
   For development only, it is possible to run without the appliance chain of trust, see the options for
   the driver.
   
3. Update the deployment files.

    * __zfssa-csi-plugin.yaml__
    
        In the `DaemonSet` section make the following modifications:

        * in the container _node-driver-registar_ subsection
            * set `image` to the appropriate container image.
        * in the container _zfssabs_ subsection
            * set `image` for the container _zfssabs_ to the appropriate container image.
            * in the `env` subsection
                 * under `ZFSSA_TARGET` set `valueFrom.secretKeyRef.name` to _oracle.zfssa.csi.node.myappliance_
                 * under `ZFSSA_INSECURE` set `value` to _False_ (if you choose to **SECURE** the communication
                   with the appliance otherwise let it set to _True_)
    * in the volume subsection, (skip if you set `ZFSSA_INSECURE` to _True_)  
        * under `cert`, **if you want communication with the appliance to be secure**
            * set `secret.secretName` to _oracle.zfssa.csi.node.myappliance.certs_
            * set `secret.secretName.items.key` to _mycertfile_
            * set `secret.secretName.items.path` to _mycertfile_
        
    * __zfssa-csi-provisioner.yaml__
    
        In the `StatefulSet` section make the following modifications:

        * set `image` for the container _zfssa-csi-provisioner_ to the appropriate image.
        * set `image` for the container _zfssa-csi-attacher_ to the appropriate container image.

4. Deploy the plugin running the following commands:

    ```text
    kubectl apply -n myspace -f ./zfssa-csi-rbac.yaml
    kubectl apply -n myspace -f ./zfssa-csi-plugin.yaml
    kubectl apply -n myspace -f ./zfssa-csi-provisioner.yaml
    ```
    At this point the command `kubectl get all -n myspace` should return something similar to this:
    ```text
    NAME                             READY   STATUS    RESTARTS   AGE
    pod/zfssa-csi-nodeplugin-lpts9   2/2     Running   0          3m22s
    pod/zfssa-csi-nodeplugin-vdb44   2/2     Running   0          3m22s
    pod/zfssa-csi-provisioner-0      2/2     Running   0          72s

    NAME                                  DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
    daemonset.apps/zfssa-csi-nodeplugin   2         2         2       2            2           <none>          3m16s

    NAME                                     READY   AGE
    statefulset.apps/zfssa-csi-provisioner   1/1     72s
    ```

###Deployment Example Using an NFS Share

Refer to the [NFS EXAMPLE README](./examples/nfs/README.md) file for details.

###Deployment Example Using a Block Volume

Refer to the [BLOCK EXAMPLE README](./examples/block/README.md) file for details.
