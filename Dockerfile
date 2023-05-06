# Copyright (c) 2021 Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.
#
# This is the Dockerfile for Oracle ZFS Storage Appliance CSI Driver
#

FROM container-registry.oracle.com/os/oraclelinux:9-slim
LABEL maintainers="Oracle"
LABEL description="Oracle ZFS Storage Appliance CSI Driver for Kubernetes"

ARG var_proxy
ENV http_proxy=$var_proxy
ENV https_proxy=$var_proxy

# Add util-linux to get a new version of losetup.
RUN microdnf -y install iscsi-initiator-utils nfs-utils e2fsprogs xfsprogs && microdnf clean all

ENV http_proxy ""
ENV https_proxy ""

COPY ./bin/zfssa-csi-driver /zfssa-csi-driver
ENTRYPOINT ["/zfssa-csi-driver"]
