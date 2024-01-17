/*
 * Copyright (c) 2021, 2024, Oracle and/or its affiliates.
 * Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
 */

package service

import (
	"fmt"
	"os"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/oracle/zfssa-csi-driver/pkg/utils"
	"github.com/oracle/zfssa-csi-driver/pkg/zfssarest"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (zd *ZFSSADriver) nodePublishFileSystem(ctx context.Context, token *zfssarest.Token,
	req *csi.NodePublishVolumeRequest, vid *utils.VolumeId, mountOptions []string,
	mode *csi.VolumeCapability_Mount) (*csi.NodePublishVolumeResponse, error) {

	targetPath := req.GetTargetPath()
	notMnt, err := zd.NodeMounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(targetPath, 0750); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			notMnt = true
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if !notMnt {
		return &csi.NodePublishVolumeResponse{}, nil
	}

	s := req.GetVolumeContext()["nfsServer"]
	ep, found := req.GetVolumeContext()["mountpoint"]
	if !found {
		// The volume context of the volume provisioned from an existing share does not have the mountpoint.
		// Use the share (corresponding to volumeAttributes.share of PV configuration) to get the mountpoint.
		ep = req.GetVolumeContext()["share"]
	}

	source := fmt.Sprintf("%s:%s", s, ep)
	utils.GetLogNODE(ctx, 5).Println("nodePublishFileSystem", "mount_point", source)

	err = zd.NodeMounter.Mount(source, targetPath, "nfs", mountOptions)
	if err != nil {
		if os.IsPermission(err) {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
		if strings.Contains(err.Error(), "invalid argument") {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (zd *ZFSSADriver) nodeUnpublishFilesystemVolume(token *zfssarest.Token,
	ctx context.Context, req *csi.NodeUnpublishVolumeRequest, vid *utils.VolumeId) (
	*csi.NodeUnpublishVolumeResponse, error) {

	utils.GetLogNODE(ctx, 5).Println("nodeUnpublishFileSystem", "request", req)

	targetPath := req.GetTargetPath()
	if len(targetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path not provided")
	}

	if _, pathErr := os.Stat(targetPath); os.IsNotExist(pathErr) {
		//targetPath doesn't exist; nothing to do
		utils.GetLogNODE(ctx, 2).Println("nodeUnpublishFilesystemVolume targetPath doesn't exist", targetPath)
		return &csi.NodeUnpublishVolumeResponse{}, nil
	}
	err := zd.NodeMounter.Unmount(targetPath)
	if err != nil {
		utils.GetLogNODE(ctx, 2).Println("Cannot unmount volume",
			"volume_id", req.GetVolumeId(), "error", err.Error())
		if !strings.Contains(err.Error(), "not mounted") {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	notMnt, mntErr := zd.NodeMounter.IsLikelyNotMountPoint(targetPath)
	if mntErr != nil {
		utils.GetLogNODE(ctx, 2).Println("Cannot determine target path",
			"target_path", targetPath, "error", err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}

	if notMnt {
		if err := os.Remove(targetPath); err != nil {
			utils.GetLogNODE(ctx, 2).Println("Cannot delete target path",
				"target_path", targetPath, "error", err.Error())
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}
