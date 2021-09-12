/*
 * Copyright (c) 2021, 2024, Oracle and/or its affiliates.
 * Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
 */

package utils

import (
	"fmt"
	"github.com/golang/protobuf/ptypes/timestamp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	VolumeMinComponents = 2
	VolumeIdLen         = 6
	VolumeHandleLen     = 8
	SnapshotIdLen       = 7
	VolumeHrefLen       = 10
	SnapshotHrefLen     = 12
)

const (
	BlockVolume string = "lun"
	MountVolume        = "mnt"
)

const (
	ResourceNamePattern string = `^[a-zA-Z0-9_\-\.\:]+$`
	ResourceNameLength  int    = 64
)

// Volume ID
// ---------
// This structure is what identifies a volume (lun or filesystem).
type VolumeId struct {
	Type    string
	Zfssa   string
	Pool    string
	Project string
	Name    string
}

func NewVolumeId(vType, zfssaName, pool, project, name string) *VolumeId {
	return &VolumeId{
		Type:    vType,
		Zfssa:   zfssaName,
		Pool:    pool,
		Project: project,
		Name:    name,
	}
}

func VolumeIdStringFromHref(zfssa, hRef string) (string, error) {
	result := strings.Split(hRef, "/")
	if len(result) < VolumeHrefLen {
		return "", status.Errorf(codes.NotFound,
			"Volume ID (%s) contains insufficient components (%d)", hRef, VolumeHrefLen)
	}

	var vType string
	switch result[8] {
	case "filesystems":
		vType = MountVolume
	case "luns":
		vType = BlockVolume
	default:
		return "", status.Errorf(codes.NotFound, "Invalid snapshot href (%s)", hRef)
	}

	return fmt.Sprintf("/%v/%v/%v/%v/%v",
		vType,
		zfssa,
		result[5],
		result[7],
		result[9]), nil
}

func VolumeIdFromString(volumeId string) (*VolumeId, error) {
	result := strings.Split(volumeId, "/")

	if len(result) < VolumeMinComponents {
		return nil, status.Errorf(codes.InvalidArgument,
			"Volume ID/Handle (%s) contains insufficient components to continue handling (%d)",
			volumeId, VolumeMinComponents)
	}

	var pool, project, share string
	switch result[1] {
	case "nfs", "iscsi":
		if len(result) < VolumeHandleLen {
			return nil, status.Errorf(codes.NotFound,
				"Volume ID/Handle (%s) contains insufficient components (%d)", volumeId, VolumeHandleLen)
		}
		pool = result[4]
		project = result[6]
		share = result[7]
	default:
		if len(result) < VolumeIdLen {
			return nil, status.Errorf(codes.NotFound,
				"Volume ID (%s) contains insufficient components (%d)", volumeId, VolumeIdLen)
		}
		pool = result[3]
		project = result[4]
		share = result[5]
	}

	if !IsResourceNameValid(pool) {
		return nil, status.Errorf(codes.InvalidArgument, "pool name is invalid (%s)", pool)
	}

	if !IsResourceNameValid(project) {
		return nil, status.Errorf(codes.InvalidArgument, "project name is invalid (%s)", project)
	}

	if !IsResourceNameValid(share) {
		return nil, status.Errorf(codes.InvalidArgument, "share name is invalid (%s)", share)
	}

	return &VolumeId{
		Type:    result[1],
		Zfssa:   result[2],
		Pool:    pool,
		Project: project,
		Name:    share,
	}, nil
}

func (zvi *VolumeId) String() string {
	return fmt.Sprintf("/%v/%v/%v/%v/%v", zvi.Type, zvi.Zfssa, zvi.Pool, zvi.Project, zvi.Name)
}

func (zvi *VolumeId) IsBlock() bool {
	switch zvi.Type {
	case BlockVolume:
		return true
	case MountVolume:
		return false
	}
	return false
}

// Snapshot ID
// -----------
// This structure is what identifies a volume (lun or filesystem).
type SnapshotId struct {
	VolumeId *VolumeId
	Name     string
}

func NewSnapshotId(volumeId *VolumeId, snapshotName string) *SnapshotId {
	return &SnapshotId{
		VolumeId: volumeId,
		Name:     snapshotName,
	}
}

func SnapshotIdFromString(snapshotId string) (*SnapshotId, error) {

	result := strings.Split(snapshotId, "/")
	if len(result) < SnapshotIdLen {
		return nil, status.Errorf(codes.NotFound,
			"Volume ID (%s) contains insufficient components (%d)", snapshotId, SnapshotIdLen)
	}

	return &SnapshotId{
		VolumeId: &VolumeId{
			Type:    result[1],
			Zfssa:   result[2],
			Pool:    result[3],
			Project: result[4],
			Name:    result[5]},
		Name: result[6]}, nil
}

func SnapshotIdFromHref(zfssa, hRef string) (*SnapshotId, error) {
	result := strings.Split(hRef, "/")
	if len(result) < SnapshotHrefLen {
		return nil, status.Errorf(codes.NotFound,
			"Snapshot ID (%s) contains insufficient components (%d)", hRef, SnapshotHrefLen)
	}

	if result[10] != "snapshots" {
		return nil, status.Errorf(codes.NotFound, "Invalid snapshot href (%s)", hRef)
	}

	var vType string
	switch result[8] {
	case "filesystems":
		vType = MountVolume
	case "luns":
		vType = BlockVolume
	default:
		return nil, status.Errorf(codes.NotFound, "Invalid snapshot href (%s)", hRef)
	}

	return &SnapshotId{
		VolumeId: &VolumeId{
			Type:    vType,
			Zfssa:   zfssa,
			Pool:    result[5],
			Project: result[7],
			Name:    result[9]},
		Name: result[11]}, nil
}

func SnapshotIdStringFromHref(zfssa, hRef string) (string, error) {
	result := strings.Split(hRef, "/")
	if len(result) < SnapshotHrefLen {
		return "", status.Errorf(codes.NotFound,
			"Snapshot ID (%s) contains insufficient components (%d)", hRef, SnapshotHrefLen)
	}

	if result[10] != "snapshots" {
		return "", status.Errorf(codes.NotFound, "Invalid snapshot href (%s)", hRef)
	}

	var vType string
	switch result[8] {
	case "filesystems":
		vType = MountVolume
	case "luns":
		vType = BlockVolume
	default:
		return "", status.Errorf(codes.NotFound, "Invalid snapshot href (%s)", hRef)
	}

	return fmt.Sprintf("/%v/%v/%v/%v/%v/%v",
		vType,
		zfssa,
		result[5],
		result[7],
		result[9],
		result[11]), nil
}

func (zsi *SnapshotId) String() string {
	return fmt.Sprintf("/%v/%v/%v/%v/%v/%v",
		zsi.VolumeId.Type,
		zsi.VolumeId.Zfssa,
		zsi.VolumeId.Pool,
		zsi.VolumeId.Project,
		zsi.VolumeId.Name,
		zsi.Name)
}

func (zsi *SnapshotId) GetVolumeId() *VolumeId {
	return zsi.VolumeId
}

func DateToUnix(date string) (*timestamp.Timestamp, error) {
	year, err := strconv.Atoi(date[0:4])
	if err == nil {
		month, err := strconv.Atoi(date[5:7])
		if err == nil {
			day, err := strconv.Atoi(date[8:10])
			if err == nil {
				hour, err := strconv.Atoi(date[11:13])
				if err == nil {
					min, err := strconv.Atoi(date[14:16])
					if err == nil {
						sec, err := strconv.Atoi(date[17:19])
						if err == nil {
							seconds := time.Date(year, time.Month(month), day, hour, min, sec,
								0, time.Local).Unix()
							return &timestamp.Timestamp{Seconds: seconds, Nanos: 0}, nil
						}
					}
				}
			}
		}
	}
	return nil, err
}

func IsResourceNameValid(name string) bool {
	if len(name) > ResourceNameLength {
		return false
	}

	var validResourceName = regexp.MustCompile(ResourceNamePattern).MatchString
	return validResourceName(name)
}
