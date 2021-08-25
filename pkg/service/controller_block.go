/*
 * Copyright (c) 2021, Oracle and/or its affiliates.
 * Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
 */

package service

import (
	"github.com/oracle/zfssa-csi-driver/pkg/utils"
	"github.com/oracle/zfssa-csi-driver/pkg/zfssarest"
	"context"
	"fmt"
	"github.com/container-storage-interface/spec/lib/go/csi"
	context2 "golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net/http"
	"sync/atomic"
)

// ZFSSA block volume
type zLUN struct {
	bolt			*utils.Bolt
	refcount		int32
	state			volumeState
	href			string
	id				*utils.VolumeId
	capacity		int64
	accessModes		[]csi.VolumeCapability_AccessMode
	source			*csi.VolumeContentSource
	initiatorgroup	[]string
	targetgroup		string``
}

var (
	// access modes supported by block volumes.
	blockVolumeCaps = []csi.VolumeCapability_AccessMode {
		{ Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER },
	}
)

// Creates a new LUN structure. If no information is provided (luninfo is nil), this
// method cannot fail. If information is provided, it will fail if it cannot create
// a volume ID.
func newLUN(vid *utils.VolumeId) *zLUN {
	lun := new(zLUN)
	lun.id = vid
	lun.bolt = utils.NewBolt()
	lun.state = stateCreating
	return lun
}

func (lun *zLUN) create(ctx context.Context, token *zfssarest.Token,
	req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {

	utils.GetLogCTRL(ctx, 5).Println("lun.create")

	capacityRange := req.GetCapacityRange()
	capabilities := req.GetVolumeCapabilities()

	_, luninfo, httpStatus, err := zfssarest.CreateLUN(ctx, token, 
							req.GetName(), getVolumeSize(capacityRange), &req.Parameters)
	if err != nil {
		if httpStatus != http.StatusConflict {
			lun.state = stateDeleted
			return nil, err
		}

		utils.GetLogCTRL(ctx, 5).Println("LUN already exits")
		// The creation failed because the appliance already has a LUN 
		// with the same name. We get the information from the appliance, 
		// update the LUN context and check its compatibility with the request.
		if lun.state == stateCreated {
			luninfo, _, err := zfssarest.GetLun(ctx, token, 
            				req.Parameters["pool"], req.Parameters["project"], req.GetName())
			if err != nil {
				return nil, err
			}
			lun.setInfo(luninfo)
		}
			
		// The LUN has already been created. The compatibility of the
		// capacity range and accessModes is checked.
		if !compareCapacityRange(capacityRange, lun.capacity) {
			return nil, 
				   status.Errorf(codes.AlreadyExists, 
						"Volume (%s) is already on target (%s),"+ 
						" capacity range incompatible (%v), requested (%v/%v)",
						lun.id.Name, lun.id.Zfssa, lun.capacity,
						capacityRange.RequiredBytes, capacityRange.LimitBytes)
		}
		if !compareCapabilities(capabilities, lun.accessModes, true) {
			return nil, 
				   status.Errorf(codes.AlreadyExists, 
				   		"Volume (%s) is already on target (%s), accessModes are incompatible",
						lun.id.Name, lun.id.Zfssa)
		}
	} else {
		lun.setInfo(luninfo)
	}

	utils.GetLogCTRL(ctx, 5).Printf(
		"LUN created: name=%s, target=%s, assigned_number=%d", 
		luninfo.CanonicalName, luninfo.TargetGroup, luninfo.AssignedNumber[0])

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:		lun.id.String(),
			CapacityBytes:	lun.capacity,
			VolumeContext:	req.GetParameters()}}, nil
}

func (lun *zLUN) cloneSnapshot(ctx context.Context, token *zfssarest.Token,
	req *csi.CreateVolumeRequest, zsnap *zSnapshot) (*csi.CreateVolumeResponse, error) {

	utils.GetLogCTRL(ctx, 5).Println("lun.cloneSnapshot")

	parameters := make(map[string]interface{})
	parameters["project"] = req.Parameters["project"]
	parameters["share"] = req.GetName()
	parameters["initiatorgroup"] = []string{zfssarest.MaskAll}

	luninfo, _, err := zfssarest.CloneLunSnapshot(ctx, token, zsnap.getHref(), parameters)
	if err != nil {
		return nil, err
	}

	lun.setInfo(luninfo)

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      lun.id.String(),
			CapacityBytes: lun.capacity,
			VolumeContext: req.GetParameters(),
			ContentSource: req.GetVolumeContentSource(),
		}}, nil
}

func (lun *zLUN) delete(ctx context.Context, token *zfssarest.Token) (*csi.DeleteVolumeResponse, error) {

	utils.GetLogCTRL(ctx, 5).Println("lun.delete")

	if lun.state == stateCreated {
		_, httpStatus, err := zfssarest.DeleteLun(ctx, token, lun.id.Pool, lun.id.Project, lun.id.Name)
		if err != nil && httpStatus != http.StatusNotFound {
			return nil, err
		}

		lun.state = stateDeleted
	}

	return &csi.DeleteVolumeResponse{}, nil
}

func (lun *zLUN) controllerPublishVolume(ctx context.Context, token *zfssarest.Token,
	req *csi.ControllerPublishVolumeRequest, nodeName string) (*csi.ControllerPublishVolumeResponse, error) {

	utils.GetLogCTRL(ctx, 5).Println("lun.controllerPublishVolume")

	pool := lun.id.Pool
	project := lun.id.Project
	name := lun.id.Name

	list, err := zfssarest.GetInitiatorGroupList(ctx, token, pool, project, name)
	if err != nil {
		// Log something
		return nil, err
	}

	// When the driver creates a LUN or clones a Lun from a snapshot of another Lun, 
	// it masks the intiator group of the Lun using zfssarest.MaskAll value. 
	// When the driver unpublishes the Lun, it also masks the initiator group.
	// This block is to test if the Lun to publish was created or unpublished 
	// by the driver. Publishing a Lun with unmasked initiator group fails
	// to avoid mistakenly publishing a Lun that may be in use by other entity. 
	utils.GetLogCTRL(ctx, 5).Printf("Volume to publish: %s:%s", lun.id, list[0])
	if len(list) != 1 || list[0] != zfssarest.MaskAll {
		var msg string
		if len(list) > 0 {
			msg = fmt.Sprintf("Volume (%s:%s) may already be published", lun.id, list[0])
		} else {
			msg = fmt.Sprintf("Volume (%s) did not return an initiator group list", lun.id)
		}
		return nil, status.Error(codes.FailedPrecondition, msg)
	}

    // Reset the masked initiator group with one named by the current node name.
	// There must be initiator groups on ZFSSA defined by the node names.
	_, err = zfssarest.SetInitiatorGroupList(ctx, token, pool, project, name, nodeName)
	if err != nil {
		// Log something
		return nil, err
	}

	return &csi.ControllerPublishVolumeResponse{}, nil
}

func (lun *zLUN) controllerUnpublishVolume(ctx context.Context, token *zfssarest.Token,
	req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {

	utils.GetLogCTRL(ctx, 5).Println("lun.controllerUnpublishVolume")

	pool := lun.id.Pool
	project := lun.id.Project
	name := lun.id.Name

	code, err := zfssarest.SetInitiatorGroupList(ctx, token, pool, project, name, zfssarest.MaskAll)
	if err != nil {
		utils.GetLogCTRL(ctx, 5).Println("Could not unpublish volume {}, code {}", lun, code)
		if code != 404 {
			return nil, err
		}
		utils.GetLogCTRL(ctx, 5).Println("Unpublish failed because LUN was deleted, return success")
	}

	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (lun *zLUN) validateVolumeCapabilities(ctx context.Context, token *zfssarest.Token,
	req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {

	utils.GetLogCTRL(ctx, 5).Println("lun.validateVolumeCapabilities")

	if areBlockVolumeCapsValid(req.VolumeCapabilities) {
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

func (lun *zLUN) controllerExpandVolume(ctx context.Context, token *zfssarest.Token,
	req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	return nil, status.Error(codes.OutOfRange, "Not allowed for block devices")
}

func (lun *zLUN) nodeStageVolume(ctx context.Context, token *zfssarest.Token,
	req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return nil, nil
}

func (lun *zLUN) nodeUnstageVolume(ctx context.Context, token *zfssarest.Token,
	req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return nil, nil
}

func (lun *zLUN) nodePublishVolume(ctx context.Context, token *zfssarest.Token,
	req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	return nil, nil
}

func (lun *zLUN) nodeUnpublishVolume(ctx context.Context, token *zfssarest.Token,
	req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	return nil, nil
}

func (lun *zLUN) nodeGetVolumeStats(ctx context.Context, token *zfssarest.Token,
	req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, nil
}

func (lun *zLUN) getDetails(ctx context2.Context, token *zfssarest.Token) (int, error) {
	lunInfo, httpStatus, err := zfssarest.GetLun(ctx, token, lun.id.Pool, lun.id.Project, lun.id.Name)
	if err != nil {
		return httpStatus, err
	}
	lun.setInfo(lunInfo)
	return httpStatus, nil
}

func (lun *zLUN) getSnapshotsList(ctx context.Context, token *zfssarest.Token) (
	[]*csi.ListSnapshotsResponse_Entry, error) {

	snapList, err := zfssarest.GetSnapshots(ctx, token, lun.href)
	if err != nil {
		return nil, err
	}

	return zfssaSnapshotList2csiSnapshotList(ctx, token.Name, snapList), nil
}

func (lun *zLUN) getState() volumeState { return lun.state }
func (lun *zLUN) getName() string { return lun.id.Name }
func (lun *zLUN) getHref() string { return lun.href }
func (lun *zLUN) getVolumeID() *utils.VolumeId { return lun.id }
func (lun *zLUN) getCapacity() int64 { return lun.capacity }
func (lun *zLUN) isBlock() bool { return true }

func (lun *zLUN) getSnapshots(ctx context.Context, token *zfssarest.Token) ([]zfssarest.Snapshot, error) {
	return  zfssarest.GetSnapshots(ctx, token, lun.href)
}

func (lun *zLUN) setInfo(volInfo interface{}) {
	switch luninfo := volInfo.(type) {
	case *zfssarest.Lun:
		lun.capacity = int64(luninfo.VolumeSize)
		lun.href = luninfo.Href
		lun.initiatorgroup = luninfo.InitiatorGroup
		lun.targetgroup = luninfo.TargetGroup
		lun.state = stateCreated
	default:
		panic("lun.setInfo called with wrong type")
	}
}

// Waits until the file system is available and, when it is, returns with its current state.
func (lun *zLUN) hold(ctx context.Context) volumeState {
	utils.GetLogCTRL(ctx, 5).Printf("holding lun (%s)", lun.id.Name)
	atomic.AddInt32(&lun.refcount, 1)
	return lun.state
}

// Releases the file system and returns its current reference count.
func (lun *zLUN) release(ctx context.Context) (int32, volumeState) {
	utils.GetLogCTRL(ctx, 5).Printf("releasing lun (%s)", lun.id.Name)
	return atomic.AddInt32(&lun.refcount, -1), lun.state
}

func (lun *zLUN) lock(ctx context.Context) volumeState {
	utils.GetLogCTRL(ctx, 5).Printf("locking %s", lun.id.String())
	lun.bolt.Lock(ctx)
	utils.GetLogCTRL(ctx, 5).Printf("%s is locked", lun.id.String())
	return lun.state
}

func (lun *zLUN) unlock(ctx context.Context) (int32, volumeState){
	lun.bolt.Unlock(ctx)
	utils.GetLogCTRL(ctx, 5).Printf("%s is unlocked", lun.id.String())
	return lun.refcount, lun.state
}

// Validates the block specific parameters of the create request.
func validateCreateBlockVolumeReq(ctx context.Context, token *zfssarest.Token, req *csi.CreateVolumeRequest) error {

	reqCaps := req.GetVolumeCapabilities()
	if !areBlockVolumeCapsValid(reqCaps) {
		return status.Error(codes.InvalidArgument, "invalid volume accessModes")
	}

	parameters := req.GetParameters()
	tg, ok := parameters["targetGroup"]
	if !ok || len(tg) < 1 {
		return status.Error(codes.InvalidArgument, "a valid ZFSSA target group is required ")
	}

	_, err := zfssarest.GetTargetGroup(ctx, token, "iscsi", tg)
	if err != nil {
		return err
	}

	return nil
}

// Checks whether the capability list is all supported.
func areBlockVolumeCapsValid(volCaps []*csi.VolumeCapability) bool {

	hasSupport := func(cap *csi.VolumeCapability) bool {
		for _, c := range blockVolumeCaps {
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
