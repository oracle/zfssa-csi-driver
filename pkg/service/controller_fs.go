/*
 * Copyright (c) 2021, 2024, Oracle and/or its affiliates.
 * Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
 */

package service

import (
	"context"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	"github.com/oracle/zfssa-csi-driver/pkg/utils"
	"github.com/oracle/zfssa-csi-driver/pkg/zfssarest"
	context2 "golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net/http"
	"sync/atomic"
)

var (
	filesystemAccessModes = []csi.VolumeCapability_AccessMode{
		{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
		{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER},
		{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY},
		{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY},
		{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY},
	}
)

// ZFSSA mount volume
type zFilesystem struct {
	bolt        *utils.Bolt
	refcount    int32
	state       volumeState
	href        string
	id          *utils.VolumeId
	capacity    int64
	accessModes []csi.VolumeCapability_AccessMode
	source      *csi.VolumeContentSource
	mountpoint  string
}

// Creates a new filesysyem structure. If no information is provided (fsinfo is nil), this
// method cannot fail. If information is provided, it will fail if it cannot create a volume ID
func newFilesystem(vid *utils.VolumeId) *zFilesystem {
	fs := new(zFilesystem)
	fs.id = vid
	fs.bolt = utils.NewBolt()
	fs.state = stateCreating
	return fs
}

func (fs *zFilesystem) create(ctx context.Context, token *zfssarest.Token,
	req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {

	utils.GetLogCTRL(ctx, 5).Println("fs.create")

	capacityRange := req.GetCapacityRange()
	capabilities := req.GetVolumeCapabilities()

	if _, ok := req.Parameters["restrictChown"]; !ok {
		utils.GetLogCTRL(ctx, 5).Println("Adding restrictChown to CreateFilesystem req parameters")
		req.Parameters["restrictChown"] = "false"
	}

	if _, ok := req.Parameters["shareNFS"]; !ok {
		utils.GetLogCTRL(ctx, 5).Println("Adding shareNFS to CreateFilesystem req parameters")
		req.Parameters["shareNFS"] = "on"
	}

	fsinfo, httpStatus, err := zfssarest.CreateFilesystem(ctx, token,
		req.GetName(), getVolumeSize(capacityRange), &req.Parameters)
	if err != nil {
		if httpStatus != http.StatusConflict {
			fs.state = stateDeleted
			return nil, err
		}

		utils.GetLogCTRL(ctx, 5).Println("Filesystem already exits")
		// The creation failed because the appliance already has a file system
		// with the same name. We get the information from the appliance, update
		// the file system context and check its compatibility with the request.
		if fs.state == stateCreated {
			fsinfo, _, err = zfssarest.GetFilesystem(ctx, token,
				req.Parameters["pool"], req.Parameters["project"], req.GetName())
			if err != nil {
				return nil, err
			}
			fs.setInfo(fsinfo)
			// pass mountpoint as a volume context value to use for nfs mount to the pod
			req.Parameters["mountpoint"] = fs.mountpoint
		}

		// The volume has already been created. The compatibility of the
		// capacity range and accessModes is checked.
		if !compareCapacityRange(capacityRange, fs.capacity) {
			return nil,
				status.Errorf(codes.AlreadyExists,
					"Volume (%s) is already on target (%s),"+
						" capacity range incompatible (%v), requested (%v/%v)",
					fs.id.Name, fs.id.Zfssa, fs.capacity,
					capacityRange.RequiredBytes, capacityRange.LimitBytes)
		}
		if !compareCapabilities(capabilities, fs.accessModes, false) {
			return nil,
				status.Errorf(codes.AlreadyExists,
					"Volume (%s) is already on target (%s), accessModes are incompatible",
					fs.id.Name, fs.id.Zfssa)
		}
	} else {
		fs.setInfo(fsinfo)
		// pass mountpoint as a volume context value to use for nfs mount to the pod
		req.Parameters["mountpoint"] = fs.mountpoint
	}

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      fs.id.String(),
			CapacityBytes: fs.capacity,
			VolumeContext: req.Parameters}}, nil
}

func (fs *zFilesystem) cloneSnapshot(ctx context.Context, token *zfssarest.Token,
	req *csi.CreateVolumeRequest, zsnap *zSnapshot) (*csi.CreateVolumeResponse, error) {

	utils.GetLogCTRL(ctx, 5).Println("fs.cloneSnapshot")

	parameters := make(map[string]interface{})
	parameters["project"] = req.Parameters["project"]
	parameters["share"] = req.GetName()

	fsinfo, _, err := zfssarest.CloneFileSystemSnapshot(ctx, token, zsnap.getHref(), parameters)
	if err != nil {
		return nil, err
	}

	fs.setInfo(fsinfo)
	// pass mountpoint as a volume context value to use for nfs mount to the pod
	req.Parameters["mountpoint"] = fs.mountpoint

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      fs.id.String(),
			CapacityBytes: fs.capacity,
			VolumeContext: req.GetParameters(),
			ContentSource: req.GetVolumeContentSource(),
		}}, nil
}

func (fs *zFilesystem) delete(ctx context.Context, token *zfssarest.Token) (*csi.DeleteVolumeResponse, error) {

	utils.GetLogCTRL(ctx, 5).Println("fs.delete")

	// Check first if the filesystem has snapshots.
	snaplist, err := zfssarest.GetSnapshots(ctx, token, fs.href)
	if err != nil {
		return nil, err
	}

	if len(snaplist) > 0 {
		return nil, status.Errorf(codes.FailedPrecondition, "filesysytem (%s) has snapshots", fs.id.String())
	}

	_, httpStatus, err := zfssarest.DeleteFilesystem(ctx, token, fs.href)
	if err != nil && httpStatus != http.StatusNotFound {
		return nil, err
	}

	fs.state = stateDeleted
	return &csi.DeleteVolumeResponse{}, nil
}

func (lun *zFilesystem) cloneVolume(ctx context.Context, token *zfssarest.Token,
	req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	utils.GetLogCTRL(ctx, 5).Println("cloneVolume", "request", protosanitizer.StripSecrets(req))

	// Create a snapshot to base the clone on

	// Clone the snapshot to the volume

	utils.GetLogCTRL(ctx, 5).Println("fs.cloneVolume")
	return nil, status.Error(codes.Unimplemented, "Filesystem cloneVolume not implemented yet")
}

// Publishes a file system. In this case there's nothing to do.
func (fs *zFilesystem) controllerPublishVolume(ctx context.Context, token *zfssarest.Token,
	req *csi.ControllerPublishVolumeRequest, nodeName string) (*csi.ControllerPublishVolumeResponse, error) {

	// Note: the volume context of the volume provisioned from an existing share does not have the mountpoint.
	// Use the share (corresponding to volumeAttributes.share of PV configuration) to define the mountpoint.

	return &csi.ControllerPublishVolumeResponse{}, nil
}

// Unpublishes a file system. In this case there's nothing to do.
func (fs *zFilesystem) controllerUnpublishVolume(ctx context.Context, token *zfssarest.Token,
	req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	utils.GetLogCTRL(ctx, 5).Println("fs.controllerUnpublishVolume")

	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (fs *zFilesystem) validateVolumeCapabilities(ctx context.Context, token *zfssarest.Token,
	req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {

	if areFilesystemVolumeCapsValid(req.VolumeCapabilities) {
		return &csi.ValidateVolumeCapabilitiesResponse{
			Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
				VolumeCapabilities: req.VolumeCapabilities,
			},
			Message: "",
		}, nil
	} else {
		return &csi.ValidateVolumeCapabilitiesResponse{
			Message: "One or more volume accessModes failed",
		}, nil
	}
}

func (fs *zFilesystem) controllerExpandVolume(ctx context.Context, token *zfssarest.Token,
	req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {

	utils.GetLogCTRL(ctx, 5).Println("fs.controllerExpandVolume")

	reqCapacity := req.GetCapacityRange().RequiredBytes
	if fs.capacity >= reqCapacity {
		return &csi.ControllerExpandVolumeResponse{
			CapacityBytes:         fs.capacity,
			NodeExpansionRequired: false,
		}, nil
	}

	parameters := make(map[string]interface{})
	parameters["quota"] = reqCapacity
	parameters["reservation"] = reqCapacity
	fsinfo, _, err := zfssarest.ModifyFilesystem(ctx, token, fs.href, &parameters)
	if err != nil {
		return nil, err
	}
	fs.capacity = fsinfo.Quota

	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         fsinfo.Quota,
		NodeExpansionRequired: false,
	}, nil
}

func (fs *zFilesystem) nodeStageVolume(ctx context.Context, token *zfssarest.Token,
	req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return nil, nil
}

func (fs *zFilesystem) nodeUnstageVolume(ctx context.Context, token *zfssarest.Token,
	req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return nil, nil
}

func (fs *zFilesystem) nodePublishVolume(ctx context.Context, token *zfssarest.Token,
	req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	return nil, nil
}

func (fs *zFilesystem) nodeUnpublishVolume(ctx context.Context, token *zfssarest.Token,
	req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	return nil, nil
}

func (fs *zFilesystem) nodeGetVolumeStats(ctx context.Context, token *zfssarest.Token,
	req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, nil
}

func (fs *zFilesystem) getDetails(ctx context2.Context, token *zfssarest.Token) (int, error) {
	fsinfo, httpStatus, err := zfssarest.GetFilesystem(ctx, token, fs.id.Pool, fs.id.Project, fs.id.Name)
	if err != nil {
		return httpStatus, err
	}
	fs.setInfo(fsinfo)
	return httpStatus, nil
}

func (fs *zFilesystem) getSnapshotsList(ctx context.Context, token *zfssarest.Token) (
	[]*csi.ListSnapshotsResponse_Entry, error) {

	snapList, err := zfssarest.GetSnapshots(ctx, token, fs.href)
	if err != nil {
		return nil, err
	}
	return zfssaSnapshotList2csiSnapshotList(ctx, token.Name, snapList), nil
}

func (fs *zFilesystem) getState() volumeState        { return fs.state }
func (fs *zFilesystem) getName() string              { return fs.id.Name }
func (fs *zFilesystem) getHref() string              { return fs.href }
func (fs *zFilesystem) getVolumeID() *utils.VolumeId { return fs.id }
func (fs *zFilesystem) getCapacity() int64           { return fs.capacity }
func (fs *zFilesystem) isBlock() bool                { return false }

func (fs *zFilesystem) setInfo(volInfo interface{}) {

	switch fsinfo := volInfo.(type) {
	case *zfssarest.Filesystem:
		fs.capacity = fsinfo.Quota
		fs.mountpoint = fsinfo.MountPoint
		fs.href = fsinfo.Href
		if fsinfo.ReadOnly {
			fs.accessModes = filesystemAccessModes[2:len(filesystemAccessModes)]
		} else {
			fs.accessModes = filesystemAccessModes[0:len(filesystemAccessModes)]
		}
		fs.state = stateCreated
	default:
		panic("fs.setInfo called with wrong type")
	}
}

// Waits until the file system is available and, when it is, returns with its current state.
func (fs *zFilesystem) hold(ctx context.Context) volumeState {
	utils.GetLogCTRL(ctx, 5).Printf("%s held", fs.id.String())
	atomic.AddInt32(&fs.refcount, 1)
	return fs.state
}

func (fs *zFilesystem) lock(ctx context.Context) volumeState {
	utils.GetLogCTRL(ctx, 5).Printf("locking %s", fs.id.String())
	fs.bolt.Lock(ctx)
	utils.GetLogCTRL(ctx, 5).Printf("%s is locked", fs.id.String())
	return fs.state
}

func (fs *zFilesystem) unlock(ctx context.Context) (int32, volumeState) {
	fs.bolt.Unlock(ctx)
	utils.GetLogCTRL(ctx, 5).Printf("%s is unlocked", fs.id.String())
	return fs.refcount, fs.state
}

// Releases the file system and returns its current reference count.
func (fs *zFilesystem) release(ctx context.Context) (int32, volumeState) {
	utils.GetLogCTRL(ctx, 5).Printf("%s released", fs.id.String())
	return atomic.AddInt32(&fs.refcount, -1), fs.state
}

// Validates the filesystem specific parameters of the create request.
func validateCreateFilesystemVolumeReq(ctx context.Context, req *csi.CreateVolumeRequest) error {

	reqCaps := req.GetVolumeCapabilities()
	if !areFilesystemVolumeCapsValid(reqCaps) {
		return status.Error(codes.InvalidArgument, "invalid volume accessModes")
	}
	return nil
}

// Checks whether the capability list is all supported.
func areFilesystemVolumeCapsValid(volCaps []*csi.VolumeCapability) bool {

	hasSupport := func(cap *csi.VolumeCapability) bool {
		for _, c := range filesystemAccessModes {
			if c.GetMode() == cap.AccessMode.GetMode() {
				return true
			}
		}
		return false
	}

	foundAll := true
	for _, c := range volCaps {
		if !hasSupport(c) {
			foundAll = false
			break
		}
	}

	return foundAll
}
