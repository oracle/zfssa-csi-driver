/*
 * Copyright (c) 2021, Oracle and/or its affiliates.
 * Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
 */

package service

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/oracle/zfssa-csi-driver/pkg/utils"
	"github.com/oracle/zfssa-csi-driver/pkg/zfssarest"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Nothing is done
func (zd *ZFSSADriver) NodeStageBlockVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (
	*csi.NodeStageVolumeResponse, error) {

	return &csi.NodeStageVolumeResponse{}, nil
}

func (zd *ZFSSADriver) NodeUnstageBlockVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (
	*csi.NodeUnstageVolumeResponse, error) {

	utils.GetLogNODE(ctx, 5).Println("NodeUnStageVolume", "request", req, "context", ctx)

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
		return nil, status.Errorf(codes.Internal, "Could not unmount target %q: %v", target, err)
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

// nodePublishBlockVolume is the worker for block volumes only, it is going to get the
// block device mounted to the target path so it can be moved to the container requesting it
func (zd *ZFSSADriver) nodePublishBlockVolume(ctx context.Context, token *zfssarest.Token,
	req *csi.NodePublishVolumeRequest, vid *utils.VolumeId, mountOptions []string) (
	*csi.NodePublishVolumeResponse, error) {

	target := req.GetTargetPath()

	utils.GetLogNODE(ctx, 5).Println("nodePublishBlockVolume", req)
	devicePath, err := attachBlockVolume(ctx, token, req, vid)
	if err != nil {
		return nil, err
	}
	utils.GetLogNODE(ctx, 5).Println("nodePublishBlockVolume", "devicePath", devicePath)

	_, err = zd.NodeMounter.ExistsPath(devicePath)
	if err != nil {
		return nil, err
	}

	globalMountPath := filepath.Dir(target)

	// create the global mount path if it is missing
	// Path in the form of /var/lib/kubelet/plugins/kubernetes.io/csi/volumeDevices/publish/{volumeName}
	utils.GetLogNODE(ctx, 5).Println("NodePublishVolume [block]", "globalMountPath", globalMountPath)

	// Check if the global mount path exists and create it if it does not
	if _, err := os.Stat(globalMountPath); os.IsNotExist(err) {
		err := os.Mkdir(globalMountPath, 0700)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not create dir %q: %v", globalMountPath, err)
		}
	} else if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not check if path exists %q: %v", globalMountPath, err)
	}

	utils.GetLogNODE(ctx, 5).Println("NodePublishVolume [block]: making target file", "target_file", target)

	// Create a new target file for the mount location
	err = zd.NodeMounter.MakeFile(target)
	if err != nil {
		if removeErr := os.Remove(target); removeErr != nil {
			return nil, status.Errorf(codes.Internal, "Could not remove mount target %q: %v", target, removeErr)
		}
		return nil, status.Errorf(codes.Internal, "Could not create file %q: %v", target, err)
	}

	utils.GetLogNODE(ctx, 5).Println("NodePublishVolume [block]: mounting block device",
		"device_path", devicePath, "target", target, "mount_options", mountOptions)
	if err := zd.NodeMounter.Mount(devicePath, target, "", mountOptions); err != nil {
		if removeErr := os.Remove(target); removeErr != nil {
			return nil, status.Errorf(codes.Internal, "Could not remove mount target %q: %v", target, removeErr)
		}
		return nil, status.Errorf(codes.Internal, "Could not mount %q at %q: %v", devicePath, target, err)
	}
	utils.GetLogNODE(ctx, 5).Println("NodePublishVolume [block]: mounted block device",
		"device_path", devicePath, "target", target)
	return &csi.NodePublishVolumeResponse{}, nil
}

// attachBlockVolume rescans the iSCSI session and attempts to attach the disk.
// This may actually belong in ControllerPublish (and was there for a while), but
// if it goes in the controller, then we have to have a way to remote the request
// to the proper node since the controller may not co-exist with the node where
// the device is actually needed for the work to be done
func attachBlockVolume(ctx context.Context, token *zfssarest.Token, req *csi.NodePublishVolumeRequest,
	vid *utils.VolumeId) (string, error) {

	lun := vid.Name
	pool := vid.Pool
	project := vid.Project

	lunInfo, _, err := zfssarest.GetLun(nil, token, pool, project, lun)
	if err != nil {
		return "", err
	}

	targetGroup := lunInfo.TargetGroup
	targetInfo, err := zfssarest.GetTargetGroup(nil, token, "iscsi", targetGroup)
	if err != nil {
		return "", err
	}

	iscsiInfo, err := GetISCSIInfo(ctx, vid, req, targetInfo.Targets[0], lunInfo.AssignedNumber[0])
	if err != nil {
		return "", status.Error(codes.Internal, err.Error())
	}

	utils.GetLogNODE(ctx, 5).Printf("attachBlockVolume: prepare mounting: %v", iscsiInfo)
	fsType := req.GetVolumeCapability().GetMount().GetFsType()
	mountOptions := req.GetVolumeCapability().GetMount().GetMountFlags()
	diskMounter := GetISCSIDiskMounter(iscsiInfo, false, fsType, mountOptions, "")

	utils.GetLogNODE(ctx, 5).Println("iSCSI Connector", "TargetPortals", diskMounter.connector.TargetPortals,
		"Lun", diskMounter.connector.Lun, "TargetIqn", diskMounter.connector.TargetIqn,
		"VolumeName", diskMounter.connector.VolumeName)

	util := &ISCSIUtil{}

	utils.GetLogNODE(ctx, 5).Println("attachBlockVolume: connecting disk", "diskMounter", diskMounter)
	devicePath, err := util.ConnectDisk(ctx, *diskMounter)
	if err != nil {
		utils.GetLogNODE(ctx, 5).Println("attachBlockVolume: failed connecting the disk: %s", err.Error())
		return "", status.Error(codes.Internal, err.Error())
	}

	utils.GetLogNODE(ctx, 5).Println("attachBlockVolume: attached at: %s", devicePath)
	return devicePath, nil
}

func (zd *ZFSSADriver) nodeUnpublishBlockVolume(ctx context.Context, token *zfssarest.Token,
	req *csi.NodeUnpublishVolumeRequest, zvid *utils.VolumeId) (*csi.NodeUnpublishVolumeResponse, error) {

	targetPath := req.GetTargetPath()
	if len(targetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path not provided")
	}

	diskUnmounter := GetISCSIDiskUnmounter(zvid)

	// Add decrement a node-local file reference count so we can keep track of when
	//	we can release the node's attach (RWO this reference count would only reach 1,
	//	RWM we may have many pods using the disk so we need to keep track)

	err := diskUnmounter.mounter.Unmount(targetPath)
	if err != nil {
		utils.GetLogNODE(ctx, 2).Println("Cannot unmount volume",
			"volume_id", req.GetVolumeId(), "error", err.Error())
		return nil, status.Error(codes.Internal, err.Error())
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
