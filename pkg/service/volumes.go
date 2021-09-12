/*
 * Copyright (c) 2021, 2022, Oracle.
 * Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
 */

package service

import (
	"github.com/oracle/zfssa-csi-driver/pkg/utils"
	"github.com/oracle/zfssa-csi-driver/pkg/zfssarest"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net/http"
	"sync"
)

// This file contains the definition of the volume interface. A volume can be
// a block device or a file system mounted over a network protocol. Both types
// of volumes must satisfy the interface defined here (zVolumeInterface).
//
// Volume Access Control
// ---------------------
// Under the subtitle "Concurrency", the CSI specification states:
//
//		"In general the Cluster Orchestrator (CO) is responsible for ensuring
//		that there is no more than one call “in-flight” per volume at a given
//		time. However, in some circumstances, the CO MAY lose state (for
//		example when the CO crashes and restarts), and MAY issue multiple calls
//		simultaneously for the same volume. The plugin SHOULD handle this as
//		gracefully as possible."
//
// In order to handle this situation, methods of the ZFSSA driver defined in this
// file guarantee that only one request (or Go routine) has access to a volume at
// any time. Those methods are:
//
//		newVolume()
//		lookupVolume()
//		releaseVolume()
//
// The first 2 methods, if successful, return a volume and exclusive access to it
// to the caller. When the volume is not needed anymore, exclusive access must
// be relinquished by calling releaseVolume().
//
// Snapshot Access Control
// -----------------------
// The same semantics as volumes apply to snapshots. The snapshot methods are:
//
//		newSnapshot()
//		lookupSnapshot()
//		releaseSnapshot()
//
// As for volumes, the first 2 methods, if successful, return a snapshot and
// exclusive access to it to the caller. It also gives exclusive access to the
// volume source of the snapshot. When the snapshot is not needed any more,
// calling releaseSnapshot() relinquishes exclusive access to the snapshot and
// volume source.
//

type volumeState	int

const (
	stateCreating volumeState = iota
	stateCreated
	stateDeleted
)

// Interface that all ZFSSA type volumes (mount and block) must satisfy.
type zVolumeInterface interface {
	create(ctx context.Context, token *zfssarest.Token,
		req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error)
	delete(ctx context.Context, token *zfssarest.Token,
		) (*csi.DeleteVolumeResponse, error)
	controllerPublishVolume(ctx context.Context, token *zfssarest.Token,
		req *csi.ControllerPublishVolumeRequest, nodeName string) (*csi.ControllerPublishVolumeResponse, error)
	controllerUnpublishVolume(ctx context.Context, token *zfssarest.Token,
		req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error)
	validateVolumeCapabilities(ctx context.Context, token *zfssarest.Token,
		req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error)
	controllerExpandVolume(ctx context.Context, token *zfssarest.Token,
		req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error)
	nodeStageVolume(ctx context.Context, token *zfssarest.Token,
		req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error)
	nodeUnstageVolume(ctx context.Context, token *zfssarest.Token,
		req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error)
	nodePublishVolume(ctx context.Context, token *zfssarest.Token,
		req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error)
	nodeUnpublishVolume(ctx context.Context, token *zfssarest.Token,
		req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error)
	nodeGetVolumeStats(ctx context.Context, token *zfssarest.Token,
		req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error)
	cloneSnapshot(ctx context.Context, token *zfssarest.Token,
		req *csi.CreateVolumeRequest, zsnap *zSnapshot) (*csi.CreateVolumeResponse, error)
	clone(ctx context.Context, token *zfssarest.Token,
		req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error)
	getDetails(ctx context.Context, token *zfssarest.Token) (int, error)
	setInfo(volInfo interface{})
	getSnapshotsList(context.Context, *zfssarest.Token) ([]*csi.ListSnapshotsResponse_Entry, error)
	hold(ctx context.Context) volumeState
	release(ctx context.Context) (int32, volumeState)
	lock(ctx context.Context) volumeState
	unlock(ctx context.Context) (int32, volumeState)
	getState() volumeState
	getName() string
	getHref() string
	getVolumeID() *utils.VolumeId
	getCapacity() int64
	isBlock() bool
}


// This method must be called when the possibility of the volume not existing yet exists.
// The following 3 scenarios are possible:
//
//	* The volume doesn't exists yet or it exists in the appliance but is not in the
//	  volume cache yet. Either way, a structure representing the volume is created
//	  in the stateCreating state, stored in the cache and a reference returned to the
//	  caller.
//	* A structure representing the volume already exists in the cache and is in the
//	  stateCreated state. A reference is returned to the caller.
//	* A structure representing the volume already exists in the cache and is NOT in
//	  the stateCreated state. This means the CO probably lost state and submitted multiple
//	  simultaneous requests for this volume. In this case an error is returned.
//
func (zd *ZFSSADriver) newVolume(ctx context.Context, pool, project, name string,
	block bool) (zVolumeInterface, error) {

	var vid *utils.VolumeId
	var zvolNew zVolumeInterface
	if block {
		vid = utils.NewVolumeId(utils.BlockVolume, zd.config.Appliance, pool, project, name)
		zvolNew = newLUN(vid)
	} else {
		vid = utils.NewVolumeId(utils.MountVolume, zd.config.Appliance, pool, project, name)
		zvolNew = newFilesystem(vid)
	}

	zd.vCache.Lock(ctx)
	zvol := zd.vCache.lookup(ctx, name)
	if zvol != nil {
		// Volume already known.
		utils.GetLogCTRL(ctx, 5).Println("zd.newVolume", "request", )
		zvol.hold(ctx)
		zd.vCache.Unlock(ctx)
		if zvol.lock(ctx) != stateCreated {
			zd.releaseVolume(ctx, zvol)
			return nil, status.Errorf(codes.Aborted, "volume busy (%s)", vid.String())
		}
		return zvol, nil
	}

	zd.vCache.add(ctx, name, zvolNew)
	zvolNew.hold(ctx)
	zvolNew.lock(ctx)
	zd.vCache.Unlock(ctx)
	return zvolNew, nil
}

// This method must be called in every CSI method receiving a request with a volume ID.
// The list of known volumes is scanned. If no volume with a matching ID is found, the
// appliance is queried. If the appliance knows of the volume, the local list is updated
// and the volume is returned to the caller. When the volume returned is not needed
// anymore, the method releaseVolume() must be called.
func (zd *ZFSSADriver) lookupVolume(ctx context.Context, token *zfssarest.Token,
	volumeId string) (zVolumeInterface, error) {

	vid, err := utils.VolumeIdFromString(volumeId)
	if err != nil {
		return nil, err
	}

	// Check first in the list of volumes if the volume is already known.
	zd.vCache.RLock(ctx)

	zvol := zd.vCache.lookup(ctx, vid.Name)
	if zvol != nil {
		zvol.hold(ctx)
		zd.vCache.RUnlock(ctx)
		if zvol.lock(ctx) != stateCreated {
			zd.releaseVolume(ctx, zvol)
			return nil, status.Errorf(codes.Aborted, "volume busy (%s)", volumeId)
		}
		return zvol, nil
	}

	zd.vCache.RUnlock(ctx)

	// Create a context for the new volume. The new context will act as a place holder
	// for the name passed in.
	zvol, err = zd.newVolume(ctx, vid.Pool, vid.Project, vid.Name, vid.Type == utils.BlockVolume)
	if err != nil {
		return nil, err
	}

	switch zvol.getState() {
	case stateCreating:	// We check with the appliance.
		httpStatus, err := zvol.getDetails(ctx, token)
		if err != nil {
			zd.releaseVolume(ctx, zvol)
			if httpStatus == http.StatusNotFound {
				return nil, status.Error(codes.NotFound, "Volume (%s) not found")
			}
			return nil, err
		}
		return zvol, nil
	case stateCreated:	// Another Goroutine beat us to it.
		return zvol, nil
	default:
		zd.releaseVolume(ctx, zvol)
		return nil, status.Error(codes.NotFound, "Volume (%s) not found")
	}
}

// Releases the volume reference and exclusive access to the volume.
func (zd *ZFSSADriver) releaseVolume(ctx context.Context, zvol zVolumeInterface) {
	zd.vCache.RLock(ctx)
	refCount, state := zvol.release(ctx)
	if refCount == 0 && state != stateCreated {
		zd.vCache.RUnlock(ctx)
		zd.vCache.Lock(ctx)
		refCount, state = zvol.unlock(ctx)
		if refCount == 0 && state != stateCreated {
			zd.vCache.delete(ctx, zvol.getName())
		}
		zd.vCache.Unlock(ctx)
	} else {
		zvol.unlock(ctx)
		zd.vCache.RUnlock(ctx)
	}
	utils.GetLogCTRL(ctx, 5).Printf(" zd.releaseVolume is done")
}

// If a snapshot with the passed in name already exists, it is returned. If it doesn't exist,
// a new snapshot structure is created and returned. This method could fail or reasons:
//
//		1)	A snapshot with the passed in name already exists but the volume source
//			is not the volume source passed in.
//		2)	A snapshot with the passed in name already exists but is not in the stateCreated
//			state (or stable state). As for volumes, This would mean the CO lost state and
//			issued simultaneous requests for the same snapshot.
//
//	If the call is successful, that caller has exclusive access to the snapshot and its volume
//	source. When the snapshot returned is not needed anymore, the method releaseSnapshot()
//	must be called.
func (zd *ZFSSADriver) newSnapshot(ctx context.Context, token *zfssarest.Token,
	name, sourceId string) (*zSnapshot, error) {

	zvol, err := zd.lookupVolume(ctx, token, sourceId)
	if err != nil {
		return nil, err
	}

	sid := utils.NewSnapshotId(zvol.getVolumeID(), name)

	zd.sCache.Lock(ctx)

	zsnap := zd.sCache.lookup(ctx, name)
	if zsnap == nil {
		// Snapshot doesn't exist or is unknown.
		zsnap := newSnapshot(sid, zvol)
		_ = zsnap.hold(ctx)
		zd.sCache.add(ctx, name, zsnap)
		zd.sCache.Unlock(ctx)
		zsnap.lock(ctx)
		return zsnap, nil
	}

	// We don't have exclusive access to the snapshot yet but it's safe to access
	// here the field id of the snapshot.
	if zsnap.getStringSourceId() != sourceId {
		zd.sCache.Unlock(ctx)
		zd.releaseVolume(ctx, zvol)
		return nil, status.Errorf(codes.AlreadyExists,
			"snapshot (%s) already exists with different source", name)
	}

	zsnap.hold(ctx)
	zd.sCache.Unlock(ctx)
	if zsnap.lock(ctx) != stateCreated {
		zd.releaseSnapshot(ctx, zsnap)
		return nil, status.Errorf(codes.Aborted, "snapshot (%s) is busy", name)
	}

	return zsnap, nil
}

// Looks up a snapshot using a volume ID (in string form). If successful, the caller gets
// exclusive access to the returned snapshot and its volume source. This method could fail
// for the following reasons:
//
//		1)	The source volume cannot be found locally or in the appliance.
//		2)	The snapshot exists but is in an unstable state. This would mean the
//			CO lost state and issued multiple simultaneous requests for the same
//			snapshot.
//		3)	There's an inconsistency between what the appliance thinks the volume
//			source is and what the existing snapshot says it is (a panic is issued).
//		4)	The snapshot cannot be found locally or in the appliance.
func (zd *ZFSSADriver) lookupSnapshot(ctx context.Context, token *zfssarest.Token,
	snapshotId string) (*zSnapshot, error) {

	var zsnap *zSnapshot
	var zvol zVolumeInterface
	var err error

	// Break up the string into its components
	sid, err := utils.SnapshotIdFromString(snapshotId)
	if err != nil {
		utils.GetLogCTRL(ctx, 2).Println("Snapshot ID was invalid", "snapshot_id", snapshotId)
		return nil, status.Errorf(codes.NotFound, "Unknown snapshot (%s)", snapshotId)
	}

	// Get first exclusive access to the volume source.
	zvol, err = zd.lookupVolume(ctx, token, sid.GetVolumeId().String())
	if err != nil {
		return nil, err
	}

	zd.sCache.RLock(ctx)
	zsnap = zd.sCache.lookup(ctx, sid.Name)
	if zsnap != nil {
		if zsnap.getSourceVolume() != zvol {
			// This is a serious problem. It means the volume source found using
			// the snapshot id is not the same as the volume source already is the
			// snapshot structure.
			panic("snapshot id and volume source inconsistent")
		}
		zsnap.hold(ctx)
		zd.sCache.RUnlock(ctx)
		if zsnap.lock(ctx) != stateCreated {
			zd.releaseSnapshot(ctx, zsnap)
			err = status.Errorf(codes.Aborted, "snapshot (%s) is busy", snapshotId)
			zsnap = nil
		} else {
			err = nil
		}
		return zsnap, err
	}
	zd.sCache.RUnlock(ctx)
	zd.releaseVolume(ctx, zvol)

	// Query the appliance.
	zsnap, err = zd.newSnapshot(ctx, token, sid.Name, sid.VolumeId.String())
	if err != nil {
		return nil, err
	}

	switch zsnap.getState() {
	case stateCreating:	// We check with the appliance.
		_, err = zsnap.getDetails(ctx, token)
		if err != nil {
			zd.releaseSnapshot(ctx, zsnap)
			return nil, err
		}
		return zsnap, nil
	case stateCreated:	// Another Goroutine beat us to it.
		return zsnap, nil
	default:
		zd.releaseSnapshot(ctx, zsnap)
		return nil, err
	}
}

// Releases the snapshot reference and exclusive access to the snapshot. Holding
// a snapshot reference and having exclusive access to it means the caller also have
// a reference to the source volume and exclusive access to it. The volume source
// is also released.
func (zd *ZFSSADriver) releaseSnapshot(ctx context.Context, zsnap *zSnapshot) {
	zvol := zsnap.getSourceVolume()
	zd.sCache.RLock(ctx)
	refCount, state := zsnap.release(ctx)
	if refCount == 0 && state != stateCreated {
		zd.sCache.RUnlock(ctx)
		zd.sCache.Lock(ctx)
		refCount, state = zsnap.unlock(ctx)
		if refCount == 0 && state != stateCreated {
			zd.sCache.delete(ctx, zsnap.getName())
		}
		zd.sCache.Unlock(ctx)
	} else {
		zd.sCache.RUnlock(ctx)
		zsnap.unlock(ctx)
	}
	zd.releaseVolume(ctx, zvol)
}

// Asks the appliance for the list of LUNs and filesystems, updates the local list of
// volumes and returns a list in CSI format.
func (zd *ZFSSADriver) getVolumesList(ctx context.Context) ([]*csi.ListVolumesResponse_Entry, error) {

	err := zd.updateVolumeList(ctx)
	if err != nil {
		return nil, err
	}

	zd.vCache.RLock(ctx)
	entries := make([]*csi.ListVolumesResponse_Entry, len(zd.sCache.sHash))
	for _, zvol := range zd.vCache.vHash {
		entry := new(csi.ListVolumesResponse_Entry)
		entry.Volume = &csi.Volume{
			VolumeId:		zvol.getVolumeID().String(),
			CapacityBytes:	zvol.getCapacity(),
		}
		entries = append(entries, entry)
	}
	zd.vCache.RUnlock(ctx)

	return entries, nil
}

// Retrieves the list of LUNs and filesystems from the appliance and updates
// the local list.
func (zd *ZFSSADriver) updateVolumeList(ctx context.Context) error {

	fsChan := make(chan error)
	lunChan := make(chan error)
	go zd.updateFilesystemList(ctx, fsChan)
	go zd.updateLunList(ctx, lunChan)
	errfs := <- fsChan
	errlun := <- lunChan

	if errfs != nil {
		return errfs
	} else if errlun != nil {
		return errlun
	}

	return nil
}

// Asks the appliance for the list of filesystems and updates the local list of volumes.
func (zd *ZFSSADriver) updateFilesystemList(ctx context.Context, out chan<- error) {

	utils.GetLogCTRL(ctx, 5).Println("zd.updateFilesystemList")

	user, password, err := zd.getUserLogin(ctx, nil)
	if err != nil {
		out <- err
	}
	token := zfssarest.LookUpToken(ctx, user, password)
	fsList, err := zfssarest.GetFilesystems(ctx, token, "", "")
	if err != nil {
		utils.GetLogCTRL(ctx, 2).Println("zd.updateFilesystemList failed", "error", err.Error())
	} else {
		for _, fsInfo := range fsList {
			zvol, err := zd.newVolume(ctx, fsInfo.Pool, fsInfo.Project, fsInfo.Name, false)
			if err != nil {
				continue
			}
			zvol.setInfo(&fsInfo)
			zd.releaseVolume(ctx, zvol)
		}
	}
	out <- err
}

// Asks the appliance for the list of LUNs and updates the local list of volumes.
func (zd *ZFSSADriver) updateLunList(ctx context.Context, out chan<- error) {

	utils.GetLogCTRL(ctx, 5).Println("zd.updateLunList")

	user, password, err := zd.getUserLogin(ctx, nil)
	if err != nil {
		out <- err
	}
	token := zfssarest.LookUpToken(ctx, user, password)

	lunList, err := zfssarest.GetLuns(ctx, token, "", "")
	if err != nil {
		utils.GetLogCTRL(ctx, 2).Println("zd.updateLunList failed", "error", err.Error())
	} else {
		for _, lunInfo := range lunList {
			zvol, err := zd.newVolume(ctx, lunInfo.Pool, lunInfo.Project, lunInfo.Name, true)
			if err != nil {
				continue
			}
			zvol.setInfo(&lunInfo)
			zd.releaseVolume(ctx, zvol)
		}
	}
	out <- err
}

// Asks the appliance for the list of its snapshots and returns it in CSI format. The
// local list of snapshots is updated in the process.
func (zd *ZFSSADriver) getSnapshotList(ctx context.Context) ([]*csi.ListSnapshotsResponse_Entry, error) {

	utils.GetLogCTRL(ctx, 5).Println("zd.getSnapshotList")

	err := zd.updateSnapshotList(ctx)
	if err != nil {
		return nil, err
	}

	zd.sCache.RLock(ctx)
	entries := make([]*csi.ListSnapshotsResponse_Entry, 0, len(zd.sCache.sHash))
	for _, zsnap := range zd.sCache.sHash {
		entry := new(csi.ListSnapshotsResponse_Entry)
		entry.Snapshot = &csi.Snapshot {
			SizeBytes: zsnap.getSize(),
			SnapshotId: zsnap.getStringId(),
			SourceVolumeId: zsnap.getStringSourceId(),
			CreationTime: zsnap.getCreationTime(),
			ReadyToUse: zsnap.getState() == stateCreated,
		}
		entries = append(entries, entry)
	}
	zd.sCache.RUnlock(ctx)

	return entries, nil
}

// Requests the list of snapshots from the appliance and updates the local list. Only
// snapshots that can be identified as filesytem snapshots or lun snapshots are kept.
func (zd *ZFSSADriver) updateSnapshotList(ctx context.Context) error {

	utils.GetLogCTRL(ctx, 5).Println("zd.updateSnapshotList")

	user, password, err := zd.getUserLogin(ctx, nil)
	if err != nil {
		utils.GetLogCTRL(ctx, 2).Println("Authentication error", "error", err.Error())
		return err
	}

	token := zfssarest.LookUpToken(ctx, user, password)
	snapList, err := zfssarest.GetSnapshots(ctx, token, "")
	if err != nil {
		utils.GetLogCTRL(ctx, 2).Println("zd.updateSnapshotList failed", "error", err.Error())
		return err
	}

	for _, snapInfo := range snapList {
		sid, err := utils.SnapshotIdFromHref(token.Name, snapInfo.Href)
		if err != nil {
			continue
		}
		zsnap, err := zd.newSnapshot(ctx, token, snapInfo.Name, sid.VolumeId.String())
		if err != nil {
			continue
		}
		err = zsnap.setInfo(&snapInfo)
		if err != nil {
			continue
		}
		zd.releaseSnapshot(ctx, zsnap)
	}

	return nil
}

func compareCapacityRange(req *csi.CapacityRange, capacity int64) bool {
	if req != nil {
		if req.LimitBytes != 0 && req.LimitBytes < capacity {
			return false
		}
		if req.RequiredBytes != 0 && req.RequiredBytes > capacity {
			return false
		}
	}
	return true
}

func compareCapabilities(req []*csi.VolumeCapability, cur []csi.VolumeCapability_AccessMode, block bool) bool {

	hasSupport := func(cap *csi.VolumeCapability, caps []csi.VolumeCapability_AccessMode, block bool) bool {
		for _, c := range caps {
			if ((block && cap.GetBlock() != nil) || (!block && cap.GetBlock() == nil)) &&
				cap.GetAccessMode().Mode == c.Mode {
				return true
			}
		}
		return false
	}

	foundAll := true
	for _, c := range req {
		if !hasSupport(c, cur, block) {
			foundAll = false
			break
		}
	}
	return foundAll
}

type volumeHashTable struct {
	vMutex	sync.RWMutex
	vHash	map[string]zVolumeInterface
}

func (h *volumeHashTable) add(ctx context.Context, key string, zvol zVolumeInterface) {
	h.vHash[key] = zvol
}

func (h *volumeHashTable) delete(ctx context.Context, key string) {
	delete(h.vHash, key)
}

func (h *volumeHashTable) lookup(ctx context.Context, key string) zVolumeInterface {
	return h.vHash[key]
}

func (h *volumeHashTable) Lock(ctx context.Context) {
	h.vMutex.Lock()
}

func (h *volumeHashTable) Unlock(ctx context.Context) {
	h.vMutex.Unlock()
}

func (h *volumeHashTable) RLock(ctx context.Context) {
	h.vMutex.RLock()
}

func (h *volumeHashTable) RUnlock(ctx context.Context) {
	h.vMutex.RUnlock()
}

type snapshotHashTable struct {
	sMutex	sync.RWMutex
	sHash	map[string]*zSnapshot
}

func (h *snapshotHashTable) add(ctx context.Context, key string, zsnap *zSnapshot) {
	h.sHash[key] = zsnap
}

func (h *snapshotHashTable) delete(ctx context.Context, key string) {
	delete(h.sHash, key)
}

func (h *snapshotHashTable) lookup(ctx context.Context, key string) *zSnapshot {
	return h.sHash[key]
}

func (h *snapshotHashTable) Lock(ctx context.Context) {
	h.sMutex.Lock()
}

func (h *snapshotHashTable) Unlock(ctx context.Context) {
	h.sMutex.Unlock()
}

func (h *snapshotHashTable) RLock(ctx context.Context) {
	h.sMutex.RLock()
}

func (h *snapshotHashTable) RUnlock(ctx context.Context) {
	h.sMutex.RUnlock()
}
