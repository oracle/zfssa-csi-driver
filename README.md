# About zfssa-csi-driver 

This plugin supports Oracle ZFS Storage Appliance
as a backend for block storage (iSCSI volumes) and file storage (NFS).

| CSI Plugin Version | Supported CSI Versions | Supported Kubernetes Versions | Persistence | Supported Access Modes | Dynamic Provisioning | Raw Block Support |
|--------------------|------------------------|-------------------------------| --- | --- | --- | --- |
| v1.1.0             | v1.0+                  | v1.20.X+                      | Persistent | Read/Write Once (for Block), ReadWriteMany (for File) | Yes | Yes |
| v1.8.0             | v1.8.0+                | v1.26.X+                      | Persistent | Read/Write Once (for Block), ReadWriteMany (for File) | Yes | Yes |

## Requirements

* Kubernetes v1.26 or above (Oracle Linux Cloud Native Environment 1.3)
* A Container runtime implementing the Kubernetes Container Runtime Interface (ex. CRI-O)
* An Oracle ZFS Storage Appliance running Appliance Kit Version 8.8 or above. This plugin may work with previous 
  versions but it is not tested with them. It is possible to use this 
  driver with the [Oracle ZFS Storage Simulator](https://www.oracle.com/downloads/server-storage/sun-unified-storage-sun-simulator-downloads.html)
* Access to both a management path and a data path for the target Oracle 
  ZFS Storage Appiance (or simulator). The management and data path 
  can be the same address.
* A suitable container image build environment (podman or docker are accounted
  for in the makefile)

## Unsupported Functionality
ZFS Storage Appliance CSI driver does not support the following functionality:
* Volume Cloning

## Building

Use and enhance the Makefile in the root directory and release-tools/build.make.

Build the driver:
```
make build
```
Depending on the golang installation, there may be dependencies identified by the build, install
these and retry the build.

The parent image for the container is container-registry.oracle.com/os/oraclelinux:7-slim, refer
to [container-registry.oracle.com](https://container-registry.oracle.com/) for more information.
The parent image can also be obtained from ghcr.io/oracle/oraclelinux and docker.io/library/oraclelinux.

The container build can use the "CONTAINER_PROXY" environment variable if the build
is being done from behind a firewall:
```
export DOCKER_PROXY=<proxy>
make container
```
Tag and push the resulting container image to a container registry available to the
Kubernetes cluster where it will be deployed or use the 'make push' option.

The push target depends on the branch or tag name:

* the branch must be prefixed with 'zfssa-' and can be pushed once
* a branch with a suffix of '-canary' will be a canary image and can be pushed
repeatedly

Specify the REPOSITORY_NAME on the make command (login prior to pushing):
```
make push REGISTRY_NAME=<your registry base>
```

## Installation

See [INSTALLATION](./INSTALLATION.md) for details.

## Testing

For information about testing the driver, see [TEST](./TEST.md).

## Examples

Example usage of this driver can be found in the ./examples
directory.

The examples below use the image _container-registry.oracle.com/os/oraclelinux:7-slim_
when they create a container where a persistent volume(s) is attached and mounted.

This set uses dynamic volume creation.
* [NFS](./examples/nfs/README.md) - illustrates NFS volume usage
from a simple container.
* [Block](./examples/block/README.md) - illustrates block volume
usage from a simple container.
* [NFS multi deployment](./examples/nfs-multi) - illustrates the use
of Helm to build several volumes and optionally build a pod to consume them.

This next set uses existing shares on the target appliance:
* [Existing NFS](./examples/nfs-pv/README.md) - illustrates NFS volume usage
from a simple container of an existing NFS filesystem share.
* [Existing Block](./examples/block-pv/README.md) - illustrates block volume
usage from a simple container of an existing iSCSI LUN.

This set exercises dynamic volume creation followed by expanding the volume capacity.
* [NFS Volume Expansion](./examples/nfs-exp/README.md) - illustrates an expansion of an NFS volume.

This set exercises dynamic volume creation (restoring from a volume snapshot) followed by creating a snapshot of the volume.
* [NFS Volume Snapshot](./examples/nfs-vsc/README.md) - illustrates a snapshot creation of an NFS volume.
* [Block Volume Snapshot](./examples/block-vsc/README.md) - illustrates a snapshot creation of a block volume.

## Help

Refer to the documentation links and examples for more information on
this driver.

## Security

Please consult the [security guide](./SECURITY.md) for our responsible security vulnerability disclosure process

## Contributing

See [CONTRIBUTING](./CONTRIBUTING.md) for details.

## License

Copyright (c) 2021 Oracle and/or its affiliates.

Released under the Universal Permissive License v1.0 as shown at
<https://oss.oracle.com/licenses/upl/>.
