/*
 * Copyright (c) 2021, 2022, Oracle.
 * Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
 */

package service

import (
	"github.com/oracle/zfssa-csi-driver/pkg/utils"
	"github.com/oracle/zfssa-csi-driver/pkg/zfssarest"
	"fmt"
	"os"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	// nodeCaps represents the capability of node service.
	nodeCaps = []csi.NodeServiceCapability_RPC_Type{
		csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
//		csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
		csi.NodeServiceCapability_RPC_UNKNOWN,
	}
)

func NewZFSSANodeServer(zd *ZFSSADriver) *csi.NodeServer {
	zd.NodeMounter = newNodeMounter()
	var ns csi.NodeServer = zd
	return &ns
}

func (zd *ZFSSADriver) NodeStageVolume(ctx context.Context,	req *csi.NodeStageVolumeRequest) (
	*csi.NodeStageVolumeResponse, error) {

	utils.GetLogNODE(ctx, 5).Println("NodeStageVolume", "request", req)

	// The request validity of the request is checked
	VolumeID := req.GetVolumeId()
	if len(VolumeID) == 0 {
		utils.GetLogNODE(ctx, 2).Println("VolumeID not provided, will return")
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	targetPath := req.GetStagingTargetPath()
	if len(targetPath) == 0 {
		utils.GetLogNODE(ctx, 2).Println("Target path not provided, will return")
		return nil, status.Error(codes.InvalidArgument, "Target path not provided")
	}

	reqCaps := req.GetVolumeCapability()
	if reqCaps == nil {
		utils.GetLogNODE(ctx, 2).Println("Capability not provided, will return")
		return nil, status.Error(codes.InvalidArgument, "Capability not provided")
	}

	// Not staging for either block or mount for now.
	return &csi.NodeStageVolumeResponse{}, nil
}

func (zd *ZFSSADriver) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (
	*csi.NodeUnstageVolumeResponse, error) {

	utils.GetLogNODE(ctx, 5).Println("NodeUnStageVolume", "request", req)

	VolumeID := req.GetVolumeId()
	if len(VolumeID) == 0 {
		utils.GetLogNODE(ctx, 2).Println("VolumeID not provided, will return")
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	target := req.GetStagingTargetPath()
	if len(target) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Staging target not provided")
	}

	// Check if target directory is a mount point. GetDeviceNameFromMount
	// given a mnt point, finds the device from /proc/mounts
	// returns the device name, reference count, and error code
	dev, refCount, err := zd.NodeMounter.GetDeviceName(target)
	if err != nil {
		msg := fmt.Sprintf("failed to check if volume is mounted: %v", err)
		return nil, status.Error(codes.Internal, msg)
	}

	// From the spec: If the volume corresponding to the volume_id
	// is not staged to the staging_target_path, the Plugin MUST
	// reply 0 OK.
	if refCount == 0 {
		utils.GetLogNODE(ctx, 3).Println("NodeUnstageVolume: target not mounted", "target", target)
		return &csi.NodeUnstageVolumeResponse{}, nil
	}

	if refCount > 1 {
		utils.GetLogNODE(ctx, 2).Println("NodeUnstageVolume: found references to device mounted at target path",
			"references", refCount, "device", dev, "target", target)
	}

	utils.GetLogNODE(ctx, 5).Println("NodeUnstageVolume: unmounting target", "target", target)
	err = zd.NodeMounter.Unmount(target)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Cannot unmount staging target %q: %v", target, err)
	}
	
	notMnt, mntErr := zd.NodeMounter.IsLikelyNotMountPoint(target)
	if mntErr != nil {
		utils.GetLogNODE(ctx, 2).Println("Cannot determine staging target path",
			"staging_target_path", target, "error", err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}

	if notMnt {
		if err := os.Remove(target); err != nil {
			utils.GetLogNODE(ctx, 2).Println("Cannot delete staging target path",
				"staging_target_path", target, "error", err.Error())
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (zd *ZFSSADriver) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (
	*csi.NodePublishVolumeResponse, error) {

	utils.GetLogNODE(ctx, 5).Println("NodePublishVolume", "request", req)

	VolumeID := req.GetVolumeId()
	if len(VolumeID) == 0 {
		utils.GetLogNODE(ctx, 2).Println("VolumeID not provided, will return")
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	zVolumeId, err := utils.VolumeIdFromString(VolumeID)
	if err != nil {
		utils.GetLogNODE(ctx, 2).Println("NodePublishVolume Volume ID was invalid",
			"volume_id", req.GetVolumeId(), "error", err.Error())
		// NOTE: by spec, we should return success since there is nothing to delete
		return nil, status.Error(codes.InvalidArgument, "Volume ID invalid")
	}

	source := req.GetStagingTargetPath()
	if len(source) == 0 {
		utils.GetLogNODE(ctx, 2).Println("Staging target path not provided, will return")
		return nil, status.Error(codes.InvalidArgument, "Staging target not provided")
	}

	target := req.GetTargetPath()
	if len(target) == 0 {
		utils.GetLogNODE(ctx, 2).Println("Target path not provided, will return")
		return nil, status.Error(codes.InvalidArgument, "Target path not provided")
	}

	utils.GetLogNODE(ctx, 2).Printf("NodePublishVolume: stagingTarget=%s, target=%s", source, target)

	volCap := req.GetVolumeCapability()
	if volCap == nil {
		utils.GetLogNODE(ctx, 2).Println("Volume Capabilities path not provided, will return")
		return nil, status.Error(codes.InvalidArgument, "Volume capability not provided")
	}

	// The account to be used for this operation is determined.
	user, password, err := zd.getUserLogin(ctx, req.Secrets)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "Invalid credentials")
	}
	token := zfssarest.LookUpToken(ctx, user, password)

	var mountOptions []string
	if req.GetReadonly() {
		mountOptions = append(mountOptions, "ro")
	}

	if req.GetVolumeCapability().GetBlock() != nil {
		mountOptions = append(mountOptions, "bind")
		return zd.nodePublishBlockVolume(ctx, token, req, zVolumeId, mountOptions)
	}

	switch mode := volCap.GetAccessType().(type) {
	case *csi.VolumeCapability_Block:
		mountOptions = append(mountOptions, "bind")
		return zd.nodePublishBlockVolume(ctx, token, req, zVolumeId, mountOptions)
	case *csi.VolumeCapability_Mount:
		return zd.nodePublishFileSystem(ctx, token, req, zVolumeId, mountOptions, mode)
	default:
		utils.GetLogNODE(ctx, 2).Println("Publish does not support Access Type", "access_type",
			volCap.GetAccessType())
		return nil, status.Error(codes.InvalidArgument, "Invalid access type")
	}
}

func (zd *ZFSSADriver) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (
	*csi.NodeUnpublishVolumeResponse, error) {

	utils.GetLogNODE(ctx, 5).Println("NodeUnpublishVolume", "request", req)

	targetPath := req.GetTargetPath()
	if len(targetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path not provided")
	}

	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		utils.GetLogNODE(ctx, 2).Println("VolumeID not provided, will return")
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	zVolumeId, err := utils.VolumeIdFromString(volumeID)
	if err != nil {
		utils.GetLogNODE(ctx, 2).Println("Cannot unpublish volume",
			"volume_id", req.GetVolumeId(), "error", err.Error())
		return nil, err
	}

	user, password, err := zd.getUserLogin(ctx, nil)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "Invalid credentials")
	}
	token := zfssarest.LookUpToken(ctx, user, password)
	if zVolumeId.IsBlock() {
		return zd.nodeUnpublishBlockVolume(ctx, token, req, zVolumeId)
	} else {
		return zd.nodeUnpublishFilesystemVolume(token, ctx, req, zVolumeId)
	}
}

func (zd *ZFSSADriver) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (
	*csi.NodeGetVolumeStatsResponse, error) {

	utils.GetLogNODE(ctx, 5).Println("NodeGetVolumeStats", "request", req)

	return nil, status.Error(codes.Unimplemented, "")
}

func (zd *ZFSSADriver) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (
	*csi.NodeExpandVolumeResponse, error) {

	utils.GetLogNODE(ctx, 5).Println("NodeExpandVolume", "request", req)

	return nil, status.Error(codes.Unimplemented, "")
}

func (zd *ZFSSADriver) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (
	*csi.NodeGetCapabilitiesResponse, error) {

	utils.GetLogNODE(ctx, 5).Println("NodeGetCapabilities", "request", req)

	var caps []*csi.NodeServiceCapability
	for _, capacity := range nodeCaps {
		c := &csi.NodeServiceCapability{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{
					Type: capacity,
				},
			},
		}
		caps = append(caps, c)
	}
	return &csi.NodeGetCapabilitiesResponse{Capabilities: caps}, nil

}

func (zd *ZFSSADriver) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (
	*csi.NodeGetInfoResponse, error) {

	utils.GetLogNODE(ctx, 2).Println("NodeGetInfo", "request", req)

	return &csi.NodeGetInfoResponse{
		NodeId: zd.config.NodeName,
	}, nil
}
