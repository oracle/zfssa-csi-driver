/*
 * Copyright (c) 2021, Oracle and/or its affiliates.
 * Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
 */

package service

import (
	"github.com/oracle/zfssa-csi-driver/pkg/utils"
	"github.com/oracle/zfssa-csi-driver/pkg/zfssarest"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"strconv"
)

var (
	// the current controller service accessModes supported
	controllerCaps = []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		csi.ControllerServiceCapability_RPC_LIST_VOLUMES,
		csi.ControllerServiceCapability_RPC_GET_CAPACITY,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
	}
)

func newZFSSAControllerServer(zd *ZFSSADriver) *csi.ControllerServer {
	var cs csi.ControllerServer = zd
	return &cs
}

func (zd *ZFSSADriver) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (
	*csi.CreateVolumeResponse, error) {

	utils.GetLogCTRL(ctx, 5).Println("CreateVolume", "request", protosanitizer.StripSecrets(req))

	// Token retrieved
	user, password, err := zd.getUserLogin(ctx, req.Secrets)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "Invalid credentials")
	}
	token := zfssarest.LookUpToken(user, password)

	// Validate the parameters
	if err := validateCreateVolumeReq(ctx, token, req); err != nil {
		return nil, err
	}

	parameters := req.GetParameters()
	pool := parameters["pool"]
	project := parameters["project"]
	zvol, err := zd.newVolume(ctx, pool, project,
		req.GetName(), isBlock(req.GetVolumeCapabilities()))
	if err != nil {
		return nil, err
	}
	defer 	zd.releaseVolume(ctx, zvol)

	if volumeContentSource := req.GetVolumeContentSource(); volumeContentSource != nil {
		if snapshot := volumeContentSource.GetSnapshot(); snapshot != nil {
			zsnap, err := zd.lookupSnapshot(ctx, token, snapshot.GetSnapshotId())
			if err != nil {
				return nil, err
			}
			defer 	zd.releaseSnapshot(ctx, zsnap)
			return zvol.cloneSnapshot(ctx, token, req, zsnap)
		}
		return nil, status.Error(codes.InvalidArgument, "Only snapshots are supported as content source")
	} else {
		return zvol.create(ctx, token, req)
	}
}

// Retrieve the volume size from the request (if not available, use a default)
func getVolumeSize(capRange *csi.CapacityRange) int64 {
	volSizeBytes := DefaultVolumeSizeBytes
	if capRange != nil {
		if capRange.RequiredBytes > 0 {
			volSizeBytes = capRange.RequiredBytes
		} else if capRange.LimitBytes > 0 && capRange.LimitBytes < volSizeBytes {
			volSizeBytes = capRange.LimitBytes
		}
	}
	return volSizeBytes
}

// Check whether the access mode of the volume to create is "block" or "filesystem"
//
//		true	block access mode
//		false	filesystem access mode
//
func isBlock(capabilities []*csi.VolumeCapability) bool {
	for _, capacity := range capabilities {
		if capacity.GetBlock() == nil {
			return false
		}
	}
	return true
}

// Validates as much of the "create volume request" as possible
//
func validateCreateVolumeReq(ctx context.Context, token *zfssarest.Token, req *csi.CreateVolumeRequest) error {

	log5 := utils.GetLogCTRL(ctx, 5)

	log5.Println("validateCreateVolumeReq started")

	// check the request object is populated
	if req == nil {
		return status.Errorf(codes.InvalidArgument, "request must not be nil")
	}

	reqCaps := req.GetVolumeCapabilities()
	if len(reqCaps) == 0 {
		return status.Errorf(codes.InvalidArgument, "no accessModes provided")
	}

	// check that the name is populated
	if req.GetName() == "" {
		return status.Error(codes.InvalidArgument, "name must be supplied")
	}

	// check as much of the ZFSSA pieces as we can up front, this will cache target information
	//	in a volatile cache, but in the long run, with many storage classes, this may save us
	//	quite a few trips to the appliance. Note that different storage classes may have
	//	different parameters
	parameters := req.GetParameters()
	poolName, ok := parameters["pool"]
	if !ok || len(poolName) < 1 || !utils.IsResourceNameValid(poolName) {
		utils.GetLogCTRL(ctx, 3).Println("pool name is invalid", poolName)
		return status.Errorf(codes.InvalidArgument, "pool name is invalid (%s)", poolName)
	}

	projectName, ok := parameters["project"]
	if !ok || len(projectName) < 1 || !utils.IsResourceNameValid(projectName) {
		utils.GetLogCTRL(ctx, 3).Println("project name is invalid", projectName)
		return status.Errorf(codes.InvalidArgument, "project name is invalid (%s)", projectName)
	}

	pool, err := zfssarest.GetPool(ctx, token, poolName)
	if err != nil {
		return err
	}

	if pool.Status != "online" && pool.Status != "degraded" {
		log5.Println("Pool not ready",  "State", pool.Status)
		return status.Errorf(codes.InvalidArgument, "pool %s in an error state (%s)", poolName, pool.Status)
	}

	_, err = zfssarest.GetProject(ctx, token, poolName, projectName)
	if err != nil {
		return err
	}

	// If this is a block request, the storage class must have the target group set and it must be on the target
	if isBlock(reqCaps) {
		err = validateCreateBlockVolumeReq(ctx, token, req)
	} else {
		err = validateCreateFilesystemVolumeReq(ctx, req)
	}

	return err
}

func (zd *ZFSSADriver) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (
	*csi.DeleteVolumeResponse, error) {

	utils.GetLogCTRL(ctx, 5).Println("DeleteVolume",
		"request", protosanitizer.StripSecrets(req), "context", ctx)

	log2 := utils.GetLogCTRL(ctx, 2)

	// The account to be used for this operation is determined.
	user, password, err := zd.getUserLogin(ctx, req.Secrets)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "Invalid credentials")
	}
	token := zfssarest.LookUpToken(user, password)

	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		log2.Println("VolumeID not provided, will return")
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	zvol, err := zd.lookupVolume(ctx, token, volumeID)
	if err != nil {
		if status.Convert(err).Code() == codes.NotFound {
			log2.Println("Volume already removed", "volume_id", req.GetVolumeId())
			return &csi.DeleteVolumeResponse{}, nil
		} else {
			log2.Println("Cannot delete volume", "volume_id", req.GetVolumeId(), "error", err.Error())
			return nil, err
		}
	}

	defer zd.releaseVolume(ctx, zvol)

	entries, err := zvol.getSnapshotsList(ctx, token)
	if err != nil {
		return nil, err
	}
	if len(entries) > 0 {
		return nil, status.Errorf(codes.FailedPrecondition, "Volume (%s) has snapshots", volumeID)
	}
	return zvol.delete(ctx, token)
}

func (zd *ZFSSADriver) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (
	*csi.ControllerPublishVolumeResponse, error) {

	utils.GetLogCTRL(ctx, 5).Println("ControllerPublishVolume",
		"request", protosanitizer.StripSecrets(req), "volume_context",
		req.GetVolumeContext(), "volume_capability", req.GetVolumeCapability())

	log2 := utils.GetLogCTRL(ctx, 2)

	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		log2.Println("Volume ID not provided, will return")
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	nodeID := req.GetNodeId()
	if len(nodeID) == 0 {
		log2.Println("Node ID not provided, will return")
		return nil, status.Error(codes.InvalidArgument, "Node ID not provided")
	}

	capability := req.GetVolumeCapability()
	if capability == nil {
		log2.Println("Capability not provided, will return")
		return nil, status.Error(codes.InvalidArgument, "Capability not provided")
	}

	nodeName, err := GetNodeName(nodeID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "Node (%s) was not found: %v", req.NodeId, err)
	}

	// The account to be used for this operation is determined.
	user, password, err := zd.getUserLogin(ctx, req.Secrets)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "Invalid credentials")
	}
	token := zfssarest.LookUpToken(user, password)

	zvol, err := zd.lookupVolume(ctx, token, volumeID)
	if err != nil {
		log2.Println("Volume ID unknown", "volume_id", volumeID, "error", err.Error())
		return nil, err
	}
	defer zd.releaseVolume(ctx, zvol)

	return zvol.controllerPublishVolume(ctx, token, req, nodeName)
}

func (zd *ZFSSADriver) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (
	*csi.ControllerUnpublishVolumeResponse, error) {

	utils.GetLogCTRL(ctx, 5).Println("ControllerUnpublishVolume",
		"request", protosanitizer.StripSecrets(req))

	log2 := utils.GetLogCTRL(ctx, 2)

	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		log2.Println("Volume ID not provided, will return")
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	// The account to be used for this operation is determined.
	user, password, err := zd.getUserLogin(ctx, req.Secrets)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "Invalid credentials")
	}
	token := zfssarest.LookUpToken(user, password)

	zvol, err := zd.lookupVolume(ctx, token, volumeID)
	if err != nil {
		if status.Convert(err).Code() == codes.NotFound {
			log2.Println("Volume already removed", "volume_id", req.GetVolumeId())
			return &csi.ControllerUnpublishVolumeResponse{}, nil
		} else {
			log2.Println("Cannot unpublish volume", "volume_id", req.GetVolumeId(), "error", err.Error())
			return nil, err
		}
	}
	defer zd.releaseVolume(ctx, zvol)

	return zvol.controllerUnpublishVolume(ctx, token, req)
}

func (zd *ZFSSADriver) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (
	*csi.ValidateVolumeCapabilitiesResponse, error) {

	log2 := utils.GetLogCTRL(ctx, 2)
	log2.Println("validateVolumeCapabilities", "request", protosanitizer.StripSecrets(req))

	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "no volume ID provided")
	}

	reqCaps := req.GetVolumeCapabilities()
	if len(reqCaps) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "no accessModes provided")
	}

	user, password, err := zd.getUserLogin(ctx, req.Secrets)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "Invalid credentials")
	}
	token := zfssarest.LookUpToken(user, password)

	zvol, err := zd.lookupVolume(ctx, token, volumeID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "Volume (%s) was not found: %v", volumeID)
	}
	defer zd.releaseVolume(ctx, zvol)

	return zvol.validateVolumeCapabilities(ctx, token, req)
}

func (zd *ZFSSADriver) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (
	*csi.ListVolumesResponse, error) {

	utils.GetLogCTRL(ctx, 5).Println("ListVolumes", "request", protosanitizer.StripSecrets(req))

	var startIndex int
	if len(req.GetStartingToken()) > 0 {
		var err error
		startIndex, err = strconv.Atoi(req.GetStartingToken())
		if err != nil {
			return nil, status.Errorf(codes.Aborted, "invalid starting_token value")
		}
	} else {
		startIndex = 0
	}

	var maxIndex int
	maxEntries := int(req.GetMaxEntries())
	if maxEntries < 0 {
		return nil, status.Errorf(codes.InvalidArgument, "invalid max_entries value")
	} else if maxEntries > 0 {
		maxIndex = startIndex + maxEntries
	} else {
		maxIndex = (1 << 31) - 1
	}

	entries, err := zd.getVolumesList(ctx)
	if err != nil {
		return nil, err
	}

	// The starting index and the maxIndex have to be adjusted based on
	// the results of the query.
	var nextToken string

	if startIndex >= len(entries) {
		// An empty list is returned.
		nextToken = "0"
		entries = []*csi.ListVolumesResponse_Entry{}
	} else if maxIndex >= len(entries) {
		// All entries from startIndex are returned.
		nextToken = "0"
		entries = entries[startIndex:]
	} else {
		nextToken = strconv.Itoa(maxIndex)
		entries = entries[startIndex:maxIndex]
	}

	rsp := &csi.ListVolumesResponse{
		NextToken: nextToken,
		Entries: entries,
	}

	return rsp, nil
}

func (zd *ZFSSADriver) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (
	*csi.GetCapacityResponse, error) {

	utils.GetLogCTRL(ctx,5).Println("GetCapacity", "request", protosanitizer.StripSecrets(req))

	reqCaps := req.GetVolumeCapabilities()
	if len(reqCaps) > 0 {
		// Providing accessModes is optional, but if provided they must be supported.
		var capsValid bool
		if isBlock(reqCaps) {
			capsValid = areBlockVolumeCapsValid(reqCaps)
		} else {
			capsValid = areFilesystemVolumeCapsValid(reqCaps)
		}

		if !capsValid {
			return nil, status.Error(codes.InvalidArgument, "invalid volume accessModes")
		}
	}

	var availableCapacity int64
	user, password, err := zd.getUserLogin(ctx, nil)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "Invalid credentials")
	}
	token := zfssarest.LookUpToken(user, password)

	parameters := req.GetParameters()
	projectName, ok := parameters["project"]
	if !ok || len(projectName) == 0 {
		// No project name provided the capacity returned will be the capacity
		// of the pool if a pool is provided.
		poolName, ok := parameters["pool"]
		if !ok || len(poolName) == 0 {
			// No pool name provided. In this case the sum of the space
			// available in each pool is returned.
			pools, err := zfssarest.GetPools(ctx, token)
			if err != nil {
				return nil, err
			}
			for _, pool := range *pools {
				availableCapacity += pool.Usage.Available
			}
		} else {
			// A pool name was provided. The space available in the pool is returned.
			pool, err := zfssarest.GetPool(ctx, token, poolName)
			if err != nil {
				return nil, err
			}
			availableCapacity = pool.Usage.Available
		}
	} else {
		// A project name was provided. In this case a pool name is required. If
		// no pool name was provided, the request is failed.
		poolName, ok := parameters["pool"]
		if !ok || len(poolName) == 0 {
			return nil, status.Error(codes.InvalidArgument, "a pool name is required")
		}
		project, err := zfssarest.GetProject(ctx, token, poolName, projectName)
		if err != nil {
			return nil, err
		}
		availableCapacity = project.SpaceAvailable
	}
	
	return &csi.GetCapacityResponse{AvailableCapacity: availableCapacity}, nil
}

func (zd *ZFSSADriver) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (
	*csi.ControllerGetCapabilitiesResponse, error) {

	utils.GetLogCTRL(ctx,5).Println("ControllerGetCapabilities",
		"request", protosanitizer.StripSecrets(req))

	var caps []*csi.ControllerServiceCapability
	for _, capacity := range controllerCaps {
		c := &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: capacity,
				},
			},
		}
		caps = append(caps, c)
	}
	return &csi.ControllerGetCapabilitiesResponse{Capabilities: caps}, nil
}

func (zd *ZFSSADriver) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (
	*csi.CreateSnapshotResponse, error) {

	utils.GetLogCTRL(ctx, 5).Println("CreateSnapshot", "request", protosanitizer.StripSecrets(req))

	sourceId := req.GetSourceVolumeId()
	snapName := req.GetName()
	if len(snapName) == 0 || len(sourceId) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Source or snapshot ID missing")
	}

	user, password, err := zd.getUserLogin(ctx, req.Secrets)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "Invalid credentials")
	}
	token := zfssarest.LookUpToken(user, password)

	zsnap, err := zd.newSnapshot(ctx, token, snapName, sourceId)
	if err != nil {
		return nil, err
	}
	defer zd.releaseSnapshot(ctx, zsnap)

	return zsnap.create(ctx, token)
}

func (zd *ZFSSADriver) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (
	*csi.DeleteSnapshotResponse, error) {

	utils.GetLogCTRL(ctx, 5).Println("DeleteSnapshot",	"request", protosanitizer.StripSecrets(req))

	if len(req.GetSnapshotId()) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "no snapshot ID provided")
	}

	log2 := utils.GetLogCTRL(ctx, 2)

	// Retrieve Token
	user, password, err := zd.getUserLogin(ctx, req.Secrets)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "Invalid credentials")
	}
	token := zfssarest.LookUpToken(user, password)

	// Get exclusive access to the snapshot.
	zsnap, err := zd.lookupSnapshot(ctx, token, req.SnapshotId)
	if err != nil {
		return &csi.DeleteSnapshotResponse{}, nil
	}
	if err != nil {
		if status.Convert(err).Code() == codes.NotFound {
			log2.Println("Snapshot already removed", "snapshot_id", req.GetSnapshotId())
			return &csi.DeleteSnapshotResponse{}, nil
		} else {
			log2.Println("Cannot delete snapshot", "snapshot_id", req.GetSnapshotId(), "error", err.Error())
			return nil, err
		}
	}
	defer zd.releaseSnapshot(ctx, zsnap)

	return zsnap.delete(ctx, token)
}

func (zd *ZFSSADriver) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (
	*csi.ListSnapshotsResponse, error) {

	utils.GetLogCTRL(ctx, 5).Println("ListSnapshots", "request", protosanitizer.StripSecrets(req))

	var startIndex int
	var err error
	if len(req.GetStartingToken()) > 0 {
		startIndex, err = strconv.Atoi(req.GetStartingToken())
		if err != nil {
			return nil, status.Errorf(codes.Aborted, "invalid starting_token value")
		}
	} else {
		startIndex = 0
	}

	var maxIndex int
	maxEntries := int(req.GetMaxEntries())
	if maxEntries < 0 {
		return nil, status.Errorf(codes.InvalidArgument, "invalid max_entries value")
	} else if maxEntries > 0 {
		maxIndex = startIndex + maxEntries
	} else {
		maxIndex = (1 << 31) - 1
	}

	// Retrieve Token
	user, password, err := zd.getUserLogin(ctx, req.Secrets)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "Invalid credentials")
	}
	token := zfssarest.LookUpToken(user, password)

	var entries []*csi.ListSnapshotsResponse_Entry

	snapshotId := req.GetSnapshotId()
	if len(snapshotId) > 0 {
		// Only this snapshot is requested.
		zsnap, err := zd.lookupSnapshot(ctx, token, snapshotId)
		if err == nil {
			entry := new(csi.ListSnapshotsResponse_Entry)
			entry.Snapshot = &csi.Snapshot{
				SnapshotId: zsnap.id.String(),
				SizeBytes: zsnap.getSize(),
				SourceVolumeId: zsnap.getStringSourceId(),
				CreationTime: zsnap.getCreationTime(),
				ReadyToUse: true,
			}
			zd.releaseSnapshot(ctx, zsnap)
			utils.GetLogCTRL(ctx, 5).Println("ListSnapshots with snapshot ID", "Snapshot", zsnap.getHref())
			entries = append(entries, entry)
		}
	} else if len(req.GetSourceVolumeId()) > 0 {
		// Only snapshots of this volume are requested.
		zvol, err := zd.lookupVolume(ctx, token, req.GetSourceVolumeId())
		if err == nil {
			entries, err = zvol.getSnapshotsList(ctx, token)
			if err != nil {
				entries = []*csi.ListSnapshotsResponse_Entry{}
				utils.GetLogCTRL(ctx, 5).Println("ListSnapshots with source ID", "Count", len(entries))
			}
			zd.releaseVolume(ctx, zvol)
		}
	} else {
		entries, err = zd.getSnapshotList(ctx)
		if err != nil {
			entries = []*csi.ListSnapshotsResponse_Entry{}
		}
		utils.GetLogCTRL(ctx, 5).Println("ListSnapshots All", "Count", len(entries))
	}

	// The starting index and the maxIndex have to be adjusted based on
	// the results of the query.
	var nextToken string

	if startIndex >= len(entries) {
		nextToken = "0"
		entries = []*csi.ListSnapshotsResponse_Entry{}
	} else if maxIndex >= len(entries) {
		nextToken = "0"
		entries = entries[startIndex:]
	} else {
		nextToken = strconv.Itoa(maxIndex)
		entries = entries[startIndex:maxIndex]
	}

	rsp := &csi.ListSnapshotsResponse{
		NextToken: nextToken,
		Entries: entries,
	}

	return rsp, nil
}

func (zd *ZFSSADriver) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (
	*csi.ControllerExpandVolumeResponse, error) {

	utils.GetLogCTRL(ctx, 5).Println("ControllerExpandVolume", "request", protosanitizer.StripSecrets(req))

	log2 := utils.GetLogCTRL(ctx, 2)

	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		log2.Println("Volume ID not provided, will return")
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	user, password, err := zd.getUserLogin(ctx, req.Secrets)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "Invalid credentials")
	}
	token := zfssarest.LookUpToken(user, password)

	zvol, err := zd.lookupVolume(ctx, token, volumeID)
	if err != nil {
		log2.Println("ControllerExpandVolume request failed, bad VolumeId",
			"volume_id", volumeID, "error", err.Error())
		return nil, err
	}
	defer zd.releaseVolume(ctx, zvol)

	return zvol.controllerExpandVolume(ctx, token, req)
}

// Check the secrets map (typically in a request context) for a change in the username
// and password or retrieve the username/password from the credentials file, the username
// and password should be scrubbed quickly after use and not remain in memory
func (zd *ZFSSADriver) getUserLogin(ctx context.Context, secrets map[string]string) (string, string, error) {
	if secrets != nil {
		user, ok := secrets["username"]
		if ok {
			password := secrets["password"]
			return user, password, nil
		}
	}

	username, err := zd.GetUsernameFromCred()
	if err != nil {
		utils.GetLogCTRL(ctx, 2).Println("ZFSSA username error:", err)
		username = "INVALID_USERNAME"
		return "", "", err
	}

	password, err := zd.GetPasswordFromCred()
	if err != nil {
		utils.GetLogCTRL(ctx, 2).Println("ZFSSA password error:", err)
		return "", "", err
	}

	return username, password, nil
}
