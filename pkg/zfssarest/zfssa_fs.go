/*
 * Copyright (c) 2021, Oracle and/or its affiliates.
 * Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
 */

package zfssarest

import (
	"github.com/oracle/zfssa-csi-driver/pkg/utils"
	"context"
	"fmt"
	"strconv"
	"google.golang.org/grpc/codes"
	grpcStatus "google.golang.org/grpc/status"
	"net/http"
)

type Filesystem struct {
	MountPoint			string	`json:"mountpoint"`
	CreationTime		string	`json:"creation"`
	RootUser			string	`json:"root_user"`
	RootGroup			string	`json:"root_group"`
	RootPermissions		string	`json:"root_permissions"`
	RestrictChown		bool	`json:"rstchown"`
	ShareNFS			string	`json:"sharenfs"`
	ShareSMB			string	`json:"sharesmb"`
	SpaceData			int64	`json:"space_data"`
	CanonicalName		string	`json:"canonical_name"`
	RecordSize			int64	`json:"recordsize"`
	SpaceAvailable		int64	`json:"space_available"`
	Quota				int64	`json:"quota"`
	UTF8Only			bool	`json:"utf8only"`
	MaxBlockSize		int64	`json:"maxblocksize"`
	Atime				bool	`json:"atime"`
	ReadOnly			bool	`json:"readonly"`
	Pool				string	`json:"pool"`
	Name				string	`json:"name"`
	SpaceTotal			int64	`json:"space_total"`
	SpaceUnused			int64	`json:"space_unused_res"`
	Project				string	`json:"project"`
	Href				string	`json:"href"`
}

type filesystemJSON struct {
	FileSystem	Filesystem `json:"filesystem"`
}

type filesystems struct {
	List 		[]Filesystem `json:"filesystems"`
}

// This variable provides a mapping between the name of the parameters used in the storage
// class yaml file and the name of equivalent parameters of the appliance.
var yml2fsProperty = map[string]string {
	"rootUser":"root_user",
	"rootGroup":"root_group",
	"rootPermissions":"root_permissions",
	"shareNFS":"sharenfs",
	"restrictChown":"rstchown",
}

func CreateFilesystem(ctx context.Context, token *Token, fsname string, volSize int64, 
	parameters *map[string]string) (*Filesystem, int, error) {

	pool := (*parameters)["pool"]
	project := (*parameters)["project"]
	url := fmt.Sprintf(zFilesystems, token.Name, pool, project)
	reqBody := buildFilesystemReq(ctx, fsname, volSize, parameters)
	rspBody := new(filesystemJSON)

	_, code, err := MakeRequest(ctx, token, "POST", url, reqBody, http.StatusCreated, rspBody)
	if err != nil {
		return nil, code, err
	}

	utils.GetLogCTRL(ctx, 5).Println("CreateFilesystem succeeded")  // ***

	return &rspBody.FileSystem, code, nil
}

// Builds the body of the "create file system" request
func buildFilesystemReq(ctx context.Context, name string, volSize int64,
	parameters *map[string]string) *map[string]interface{} {

	fsReq := make(map[string]interface{})

	fsReq["name"] = name
	fsReq["quota"] = volSize
	fsReq["reservation"] = volSize

	for key, param := range *parameters {
		fsProp, ok := yml2fsProperty[key]
		if ok {
			switch fsProp {
			case "rstchown":
				val, err := strconv.ParseBool(param)
				if err != nil {
					utils.GetLogREST(ctx, 2).Println("Invalid restrict chown, will use default: true",
					"rstchown", param)
					val = true
				}
				fsReq[fsProp] = val
			default:
				fsReq[fsProp] = param
			}
			utils.GetLogREST(ctx, 5).Println("BuildFSRequest", "key", key, "value", param)
		}
	}

	return &fsReq
}

func GetFilesystem(ctx context.Context, token *Token, pool, project, filesystem string) (
	*Filesystem, int, error) {

	url := fmt.Sprintf(zFilesystem, token.Name, pool, project, filesystem)

	rspJSON := &filesystemJSON{}
	_, httpStatus, err := MakeRequest(ctx, token, "GET", url, nil, http.StatusOK, rspJSON)
	if err != nil {
		return nil, httpStatus, err
	}

	return &rspJSON.FileSystem, httpStatus, nil
}

func ModifyFilesystem(ctx context.Context, token *Token, href string, 
	parameters *map[string]interface{}) (*Filesystem, int, error) {

	url := fmt.Sprintf(zAppliance + href, token.Name)

	rspJSON := &filesystemJSON{}
	_, httpStatus, err := MakeRequest(ctx, token, "PUT", url, parameters, http.StatusAccepted, rspJSON)
	if err != nil {
		return nil, httpStatus, err
	}

	return &rspJSON.FileSystem, httpStatus, nil
}

func DeleteFilesystem(ctx context.Context, token *Token, hRef string) (bool, int, error) {

	utils.GetLogREST(ctx, 5).Println("DeleteFilesystem", "appliance", token.Name, "Filesystem", hRef)

	url := fmt.Sprintf(zAppliance + hRef, token.Name)

	_, httpStatus, err := MakeRequest(ctx, token, "DELETE", url, nil, http.StatusNoContent, nil)
	if err != nil {
		return false, httpStatus, err
	}

	if httpStatus != http.StatusNoContent {
		utils.GetLogREST(ctx, 5).Println("DeleteFilesystem", "http status", httpStatus)
		if httpStatus >= 200 && httpStatus < 300 {
			return false, httpStatus, nil
		}
	}

	return true, httpStatus, nil
}

// Returns the List of filesystems associated with the pool and project passed in. To
// get a system wide List of file systems, the pool AND the project must be 'nil'
func GetFilesystems(ctx context.Context, token *Token, pool, project string) ([]Filesystem, error) {

	var url string
	if pool != "" && project != "" {
		url = fmt.Sprintf(zFilesystems, token.Name, pool, project)
	} else if pool == "" && project == "" {
		url = fmt.Sprintf(zAllFilesystems, token.Name)
	} else {
		return nil, grpcStatus.Error(codes.InvalidArgument, "pool and project must be both nil or both not nil")
	}

	filesystems := new(filesystems)
	filesystems.List = make([]Filesystem, 0)

	_, _, err := MakeRequest(ctx, token, "GET", url, nil, http.StatusOK, filesystems)
	if err != nil {
		return nil, err
	}

	return filesystems.List, nil
}

func (l *filesystems) UnmarshalJSON(b []byte) error {
	return zfssaUnmarshalList(b, &l.List)
}

func CloneFileSystemSnapshot(ctx context.Context, token *Token, hRef string, 
	parameters map[string]interface{}) (*Filesystem, int, error) {

	url := fmt.Sprintf(zAppliance + hRef + "/clone", token.Name)

	rspBody := new(filesystemJSON)

	_, code, err := MakeRequest(ctx, token, "PUT", url, &parameters, http.StatusCreated, rspBody)
	if err != nil {
		return nil, code, err
	}

	return &rspBody.FileSystem, code, nil
}
