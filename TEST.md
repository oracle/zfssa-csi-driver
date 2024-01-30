# Oracle ZFS Storage Appliance CSI Plugin Testing

There are two distinct paths in the plugin
* Filesystem (Mount)
* Block (iSCSI)

This write-up discusses various techniques for testing the plugin.

Note that some of the paths require the plug-in to be deployed (unit tests) while others are more for quick iterative development without
a cluster being available.

Refer to the README.md file for information about deploying the plug-in.

## Test Driver Locally

Create a local build
```
make build
```

You will need the following environment variables configured (these emulate the secrets setup). Note that
the driver *must* be run as root (or with root-equivalence for iscsiadmin utility). The csc tool can be run
from any process space as it only interacts with the socket that the driver is waiting on.

```
ZFSSA_TARGET=(target zfssa head)
ZFSSA_USER=(user for target)
ZFSSA_PASSWORD=(password for target)
ZFSSA_POOL=(pool to create resources in)
ZFSSA_PROJECT=(project to create resources in)
HOST_IP=(IP address of host where running)
POD_IP=(IP of pod)
NODE_NAME=(name of node)
CSI_ENDPOINT=tcp://127.0.0.1:10000
```

Now run the driver as root:

```
sudo su -
export CSI_ENDPOINT=tcp://127.0.0.1:10000;<other exports>;./bin/zfssa-csi-driver --endpoint tcp://127.0.0.1:10000 --nodeid MyCSINode
Building Provider
ERROR: logging before flag.Parse: I1108 15:34:55.472389    6622 service.go:63] Driver: zfssa-csi-driver version: 0.0.0
Using stored configuration {       }
ERROR: logging before flag.Parse: I1108 15:34:55.472558    6622 controller.go:42] NewControllerServer Implementation
ERROR: logging before flag.Parse: I1108 15:34:55.472570    6622 identity.go:15] NewIdentityServer Implementation
ERROR: logging before flag.Parse: I1108 15:34:55.472581    6622 node.go:24] NewNodeServer Implementation
Running gRPC
INFO[0000] identity service registered                  
INFO[0000] controller service registered                
INFO[0000] node service registered                      
INFO[0000] serving                                       endpoint="tcp://127.0.0.1:10000"

```

Test the block driver. Note the interactions of using the full volume id that is returned from each command.
```
./csc controller create-volume --cap MULTI_NODE_MULTI_WRITER,block --req-bytes 1073741824 --params pool=dedup1,project=pmonday,initiatorGroup=paulGroup,targetGroup=paulTargetGroup,blockSize=8192 --endpoint tcp://127.0.0.1:10000 pmonday5
./csc controller delete-volume  --endpoint tcp://127.0.0.1:10000 /iscsi/aie-7330a-h1/pmonday3/dedup1/local/pmonday/pmonday3
```

Test the driver to Publish a LUN. Note that the LUN number is the number of the "assignednumber" on the
LUN on the appliance. Also note that the flow is create -> controller publish -> node publish -> node unpublish -> controller unpublish
```bash
First publish to the controller
./csc controller publish --cap MULTI_NODE_MULTI_WRITER,block --vol-context targetPortal=10.80.44.165:3260,discoveryCHAPAuth=false,sessionCHAPAuth=false,portals=[],iscsiInterface=default --node-id worknode  /iscsi/aie-7330a-h1/pmonday5/dedup1/local/pmonday/pmonday5
"/iscsi/aie-7330a-h1/pmonday5/dedup1/local/pmonday/pmonday5"    "devicePath"="/dev/disk/by-path/ip-10.80.44.165:3260-iscsi-iqn.1986-03.com.sun:02:ab7b55fa-53ee-e5ab-98e1-fad3cc29ae57-lun-13"

Publish to the Node
./csc node publish -l debug --endpoint tcp://127.0.0.1:10000 --target-path /mnt/iscsi --pub-context "devicePath"="/dev/disk/by-path/ip-10.80.44.165:3260-iscsi-iqn.1986-03.com.sun:02:ab7b55fa-53ee-e5ab-98e1-fad3cc29ae57-lun-13"  --cap MULTI_NODE_MULTI_WRITER,block --vol-context targetPortal=10.80.44.165:3260,discoveryCHAPAuth=false,sessionCHAPAuth=false,portals=[],iscsiInterface=default /iscsi/aie-7330a-h1/pmonday3/dedup1/local/pmonday/pmonday5
DEBU[0000] assigned the root context                    
DEBU[0000] mounting volume                               request="{/iscsi/aie-7330a-h1/pmonday3/dedup1/local/pmonday/pmonday5 map[devicePath:/dev/disk/by-path/ip-10.80.44.165:3260-iscsi-iqn.1986-03.com.sun:02:ab7b55fa-53ee-e5ab-98e1-fad3cc29ae57-lun-13]  /mnt/iscsi block:<> access_mode:<mode:MULTI_NODE_MULTI_WRITER >  false map[] map[discoveryCHAPAuth:false iscsiInterface:default portals:[] sessionCHAPAuth:false targetPortal:10.80.44.165:3260] {} [] 0}"
DEBU[0000] parsed endpoint info                          addr="127.0.0.1:10000" proto=tcp timeout=1m0s
/iscsi/aie-7330a-h1/pmonday3/dedup1/local/pmonday/pmonday5

Now unpublish from the node
./csc node unpublish -l debug --endpoint tcp://127.0.0.1:10000 --target-path /mnt/iscsi  /iscsi/aie-7330a-h1/pmonday3/dedup1/local/pmonday/pmonday5
DEBU[0000] assigned the root context                    
DEBU[0000] mounting volume                               request="{/iscsi/aie-7330a-h1/pmonday3/dedup1/local/pmonday/pmonday5 /mnt/iscsi {} [] 0}"
DEBU[0000] parsed endpoint info                          addr="127.0.0.1:10000" proto=tcp timeout=1m0s
/iscsi/aie-7330a-h1/pmonday3/dedup1/local/pmonday/pmonday5

Now unpublish from the controller (this is not working yet)
./csc controller unpublish -l debug --endpoint tcp://127.0.0.1:10000 /iscsi/aie-7330a-h1/pmonday3/dedup1/local/pmonday/pmonday5
DEBU[0000] assigned the root context                    
DEBU[0000] unpublishing volume                           request="{/iscsi/aie-7330a-h1/pmonday3/dedup1/local/pmonday/pmonday5  map[] {} [] 0}"
DEBU[0000] parsed endpoint info                          addr="127.0.0.1:10000" proto=tcp timeout=1m0s
Need to just detach the device here

```
If everything looks OK, push the driver to a container registry
```
make push
```

Now deploy the driver in a Kubernetes Cluster
```bash
Working on instructions
```

Test the driver to Create a file system
```
./csc controller create --cap MULTI_NODE_MULTI_WRITER,mount,nfs,uid=500,gid=500 --req-bytes 107374182400 --params node=zs32-01,pool=p0,project=default coucou
./csc controller delete /nfs/10.80.222.176/coucou/p0/local/default/coucou
```

## Unit Test the Driver

To run the unit tests, you must have compiled csi-sanity from the 
[csi-test project](https://github.com/kubernetes-csi/csi-test). Once
compiled, scp the file to the node that is functioning as a controller
in a Kubernetes Cluster with the Oracle ZFS Storage CSI Driver deployed and running.

There is documentation on the test process available at the Kubernetes
[testing of CSI drivers page](https://kubernetes.io/blog/2020/01/08/testing-of-csi-drivers/).

At least do a quick sanity test (create a PVC and remove it) prior to running.
This will shake out simple problems like authentication.

Create a test parameters file that makes sense in your environment, like this:
```yaml
volumeType: thin
targetGroup: csi-data-path-target
blockSize: "8192"
pool: <pool-name>
project: <project>
targetPortal: "<portal-ip-address>:3260"
nfsServer: "<nfs-server>"
```

Replace the variables above
* <pool-name> - the name of the pool to use on the target Oracle ZFS Storage Appliance
* <project> - the name of the project to use on the target Oracle ZFS Storage Appliance
* <portal-ip-address> - the IP address of the iSCSI target portal on the Oracle ZFS Storage Appliance
* <nfs-path> - the IP address of the NFS server on the Oracle ZFS Storage Appliance used to mount filesystems

Then run the tests
```
./csi-sanity --csi.endpoint=/var/lib/kubelet/plugins/com.oracle.zfssabs/csi.sock -csi.testvolumeparameters=./test-volume-parameters.yaml
```

There should only be one known failure due to volume name length limitations
on the Oracle ZFS Storage Appliance, it looks like this:
```
[Fail] Controller Service [Controller Server] CreateVolume [It] should not fail when creating volume with maximum-length name
```

A few additional notes on options for csi-sanity:

* -csi.mountdir needs be a location on a node that is accessible by zfssa-csi-nodeplugin (but not created yet)
* --ginkgo.focus and --ginkgo.skip can be used to specify the test cases to be executed or skipped. Eg, --ginkgo.focus NodeUnpublish or --ginkgo.skip '[Ss]napshot'.
* --ginkgo.fail-fast: stop running after a failure occurs.
* --ginkgo.v: verbose output
* --ginkgo.seed 1: do not randomize the execution of test suite. 
