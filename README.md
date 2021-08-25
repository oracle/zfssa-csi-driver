# About zfssa-csi-driver 

This plugin supports Oracle ZFS Storage Appliance
as a backend for block storage (iSCSI volumes) and file storage (NFS).

| CSI Plugin Version | Supported CSI Versions | Supported Kubernetes Versions | Persistence | Supported Access Modes | Dynamic Provisioning | Raw Block Support |
| --- | --- | --- | --- | --- | --- | --- |
| v0.5.x | v1.0+ | v1.17.X+ | Persistent | Read/Write Once (for Block) | Yes | Yes |

## Requirements

* Kubernetes v1.17 or above. 
* A Container runtime implementing the Kubernetes Container Runtime Interface. This plugin was tested with CRI-O v1.17.
* An Oracle ZFS Storage Appliance running Appliance Kit Version 8.8 or above. This plugin may work with previous
versions but it is not tested with them. It is possible to use this
driver with the [Oracle ZFS Storage Simulator](https://www.oracle.com/downloads/server-storage/sun-unified-storage-sun-simulator-downloads.html)
* Access to both a management path and a data path for the target Oracle
ZFS Storage Appiance (or simulator). The management and data path
can be the same address.
* A suitable container image build environment. The Makefile currently uses docker
but with minor updates to release-tools/build.make, podman should also be usable.
* An account for use with [container-registry.oracle.com](https://container-registry.oracle.com/) image registry.

## Unsupported Functionality
ZFS Storage Appliance CSI driver does not support the following functionality:
* Volume Cloning

## Building

Use and enhance the Makefile in the root directory and release-tools/build.make.

Build the driver:
```
make build
```
Depending on your golang installation, there may be dependencies identified by the build, install
these and retry the build.

Prior to building the container image, docker login to container-registry.oracle.com so the
parent image container-registry.oracle.com/os/oraclelinux:7-slim can be retrieved. There may
be license terms to accept at the web entry to the container registry:
[container-registry.oracle.com](https://container-registry.oracle.com/).

Once you are logged in you can make the container with the following:
```
make container
```
Tag and push the resulting container image to a container registry.

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

## Contributing

See [CONTRIBUTING](./CONTRIBUTING.md) for details.
