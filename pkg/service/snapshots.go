/*
 * Copyright (c) 2021, Oracle and/or its affiliates.
 * Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
 */

package service

import (
	"github.com/oracle/zfssa-csi-driver/pkg/utils"
	"github.com/oracle/zfssa-csi-driver/pkg/zfssarest"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/protobuf/ptypes/timestamp"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net/http"
	"sync/atomic"
)

// Mount volume or BLock volume snapshot.
type zSnapshot struct {
	bolt			*utils.Bolt
	refcount		int32
	state			volumeState
	href			string
	zvol			zVolumeInterface
	id				*utils.SnapshotId
	numClones		int
	spaceUnique		int64
	spaceData		int64
	timeStamp		*timestamp.Timestamp
}

func newSnapshot(sid *utils.SnapshotId, zvol zVolumeInterface) *zSnapshot {
	zsnap := new(zSnapshot)
	zsnap.bolt = utils.NewBolt()
	zsnap.zvol = zvol
	zsnap.id = sid
	zsnap.state = stateCreating
	return zsnap
}

func (zsnap *zSnapshot) create(ctx context.Context, token *zfssarest.Token) (
	*csi.CreateSnapshotResponse, error) {

	utils.GetLogCTRL(ctx, 5).Println("zsnap.create")

	snapinfo, httpStatus, err := zfssarest.CreateSnapshot(ctx, token, zsnap.zvol.getHref(), zsnap.id.Name)
	if err != nil {
		if httpStatus != http.StatusConflict {
			zsnap.state = stateDeleted
			return nil, err
		}
		// The creation failed because the source file system already has a snapshot
		// with the same name.
		if zsnap.getState() == stateCreated {
			snapinfo, _, err := zfssarest.GetSnapshot(ctx, token, zsnap.zvol.getHref(), zsnap.id.Name)
			if err != nil {
				return nil, err
			}
			err = zsnap.setInfo(snapinfo)
			if err != nil {
				zsnap.state = stateDeleted
				return nil, err
			}
		}
	} else {
		err = zsnap.setInfo(snapinfo)
		if err != nil {
			zsnap.state = stateDeleted
			return nil, err
		}
	}

	return &csi.CreateSnapshotResponse {
		Snapshot: &csi.Snapshot{
			SizeBytes: zsnap.spaceData,
			SnapshotId: zsnap.id.String(),
			SourceVolumeId: zsnap.id.VolumeId.String(),
			CreationTime: zsnap.timeStamp,
			ReadyToUse: true,
		}}, nil
}

func (zsnap *zSnapshot) delete(ctx context.Context, token *zfssarest.Token) (
	*csi.DeleteSnapshotResponse, error) {

	utils.GetLogCTRL(ctx, 5).Println("zsnap.delete")

	// Update the snapshot information.
	httpStatus, err := zsnap.refreshDetails(ctx, token)
	if err != nil {
		if httpStatus != http.StatusNotFound {
			return nil, err
		}
		zsnap.state = stateDeleted
		return &csi.DeleteSnapshotResponse{}, nil
	}

	if zsnap.getNumClones() > 0 {
		return nil, status.Errorf(codes.FailedPrecondition, "Snapshot has (%d) dependents", zsnap.numClones)
	}

	_, httpStatus, err = zfssarest.DeleteSnapshot(ctx, token, zsnap.href)
	if err != nil && httpStatus != http.StatusNotFound {
		return nil, err
	}

	zsnap.state = stateDeleted
	return &csi.DeleteSnapshotResponse{}, nil
}


func (zsnap *zSnapshot) getDetails(ctx context.Context, token *zfssarest.Token) (int, error) {

	utils.GetLogCTRL(ctx, 5).Println("zsnap.getDetails")

	snapinfo, httpStatus, err := zfssarest.GetSnapshot(ctx, token, zsnap.zvol.getHref(), zsnap.id.Name)
	if err != nil {
		return httpStatus, err
	}
	zsnap.timeStamp, err = utils.DateToUnix(snapinfo.Creation)
	if err != nil {
		return httpStatus, err
	}
	zsnap.numClones = snapinfo.NumClones
	zsnap.spaceData = snapinfo.SpaceData
	zsnap.spaceUnique = snapinfo.SpaceUnique
	zsnap.href = snapinfo.Href
	zsnap.state = stateCreated
	return httpStatus, nil
}

func (zsnap *zSnapshot) refreshDetails(ctx context.Context, token *zfssarest.Token) (int, error) {
	snapinfo, httpStatus, err := zfssarest.GetSnapshot(ctx, token, zsnap.zvol.getHref(), zsnap.id.Name)
	if err == nil {
		zsnap.numClones = snapinfo.NumClones
		zsnap.spaceData = snapinfo.SpaceData
		zsnap.spaceUnique = snapinfo.SpaceUnique
	}
	return httpStatus, err
}

// Populate the snapshot structure with the information provided
func (zsnap *zSnapshot) setInfo(snapinfo *zfssarest.Snapshot) error {
	var err error
	zsnap.timeStamp, err = utils.DateToUnix(snapinfo.Creation)
	if err != nil {
		return err
	}
	zsnap.numClones = snapinfo.NumClones
	zsnap.spaceData = snapinfo.SpaceData
	zsnap.spaceUnique = snapinfo.SpaceUnique
	zsnap.href = snapinfo.Href
	zsnap.state = stateCreated
	return nil
}

// Waits until the file system is available and, when it is, returns with its current state.
func (zsnap *zSnapshot) hold(ctx context.Context) volumeState {
	atomic.AddInt32(&zsnap.refcount, 1)
	return zsnap.state
}

func (zsnap *zSnapshot) lock(ctx context.Context) volumeState {
	zsnap.bolt.Lock(ctx)
	return zsnap.state
}

func (zsnap *zSnapshot) unlock(ctx context.Context) (int32, volumeState){
	zsnap.bolt.Unlock(ctx)
	return zsnap.refcount, zsnap.state
}

// Releases the file system and returns its current reference count.
func (zsnap *zSnapshot) release(ctx context.Context) (int32, volumeState) {
	return atomic.AddInt32(&zsnap.refcount, -1), zsnap.state
}

func (zsnap *zSnapshot) isBlock() bool { return zsnap.zvol.isBlock() }
func (zsnap *zSnapshot) getSourceVolume() zVolumeInterface { return zsnap.zvol }
func (zsnap *zSnapshot) getState() volumeState { return zsnap.state }
func (zsnap *zSnapshot) getName() string { return zsnap.id.Name }
func (zsnap *zSnapshot) getStringId() string { return zsnap.id.String() }
func (zsnap *zSnapshot) getStringSourceId() string { return zsnap.id.VolumeId.String() }
func (zsnap *zSnapshot) getHref() string { return zsnap.href }
func (zsnap *zSnapshot) getSize() int64 { return zsnap.spaceData }
func (zsnap *zSnapshot) getCreationTime() *timestamp.Timestamp { return zsnap.timeStamp }
func (zsnap *zSnapshot) getNumClones() int { return zsnap.numClones }

func zfssaSnapshotList2csiSnapshotList(ctx context.Context, zfssa string,
	snapList []zfssarest.Snapshot) []*csi.ListSnapshotsResponse_Entry {

	utils.GetLogCTRL(ctx, 5).Println("zfssaSnapshotList2csiSnapshotList")

	entries := make([]*csi.ListSnapshotsResponse_Entry, 0, len(snapList))

	for _, snapInfo := range snapList {
		sid, err := utils.SnapshotIdStringFromHref(zfssa, snapInfo.Href)
		if err != nil {
			continue
		}
		vid, err := utils.VolumeIdStringFromHref(zfssa, snapInfo.Href)
		if err != nil {
			continue
		}
		creationTime, err := utils.DateToUnix(snapInfo.Creation)
		if err != nil {
			continue
		}
		entry := new(csi.ListSnapshotsResponse_Entry)
		entry.Snapshot = &csi.Snapshot{
			SnapshotId: sid,
			SizeBytes: snapInfo.SpaceData,
			SourceVolumeId: vid,
			CreationTime: creationTime,
			ReadyToUse: true,
		}
		entries = append(entries, entry)
	}
	return entries
}
