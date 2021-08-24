/*
 * Copyright (c) 2021, Oracle and/or its affiliates.
 * Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
 */

package zfssarest

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/oracle/zfssa-csi-driver/pkg/utils"
	"google.golang.org/grpc/codes"
	grpcStatus "google.golang.org/grpc/status"
)

const (
	DefaultBlockSize		 int = 8192
	DevicePathKey 			 string = "devicePath"
	MaskAll					 string = "com.sun.ms.vss.hg.maskAll"
)

type Lun struct {
	SpaceData		float64		`json:"space_data"`
	CanonicalName	string		`json:"canonical_name"`
	VolumeSize		float64		`json:"volsize"`
	VolumeBlockSize	int64		`json:"volblocksize"`
	Pool			string		`json:"pool"`
	Project			string		`json:"project"`
	Name			string		`json:"name"`
	Href			string		`json:"href"`
	AssignedNumber	[]int32		`json:"assignednumber"`
	InitiatorGroup	[]string	`json:"initiatorgroup"`
	TargetGroup		string		`json:"targetgroup"`
}

type LunJson struct {
	LUN Lun `json:"lun"`
}

type Luns struct {
	List 		[]Lun `json:"luns"`
}

type LunInitiatorGrps struct {
	InitiatorGroup []string `json:"initiatorgroup"`
}

type createLunRequest struct {
	Name			string		`json:"name"`
	Size			int64		`json:"volsize"`
	Blocksize		int			`json:"volblocksize"`
	TargetGroup		string		`json:"targetgroup"`
	Sparse  		bool		`json:"sparse"`
	InitiatorGroup	[]string	`json:"initiatorgroup"`
}

func CreateLUN(ctx context.Context, token *Token, lunName string, volSize int64, 
	parameters *map[string]string) (*utils.VolumeId, *Lun, int, error) {

	pool := (*parameters)["pool"]
	project := (*parameters)["project"]

	url := fmt.Sprintf(zLUNs, token.Name, pool, project)

	blockSizeString := (*parameters)["blockSize"]
	blockSize, err := strconv.Atoi(blockSizeString)
	if err != nil {
		utils.GetLogREST(ctx, 2).Println("Invalid block size, will use default",
			"requested_block_size", blockSizeString, "default_block_size", DefaultBlockSize)
		blockSize = DefaultBlockSize
	}

	volTypeString := (*parameters)["volumeType"]
	sparse := false
	if volTypeString == "thin" {
		sparse = true
	}

	reqBody := createLunRequest{
		Name:           lunName,
		Size:           volSize,
		Blocksize:      blockSize,
		TargetGroup: 	(*parameters)["targetGroup"],
		Sparse: 		sparse,
		InitiatorGroup:	[]string{MaskAll},
	}

	rspBody := &LunJson{}
	_, code, err := MakeRequest(ctx, token, "POST", url, reqBody, http.StatusCreated, rspBody)
	if err != nil {
		return nil, nil, code, err
	}

	volumeId := utils.NewVolumeId(utils.BlockVolume, token.Name, pool, project, lunName)

	return volumeId, &rspBody.LUN, code, nil
}

func GetLun(ctx context.Context, token *Token, pool, project, lun string) (*Lun, int, error) {

	url := fmt.Sprintf(zLUN, token.Name, pool, project, lun)

	rspJSON := &LunJson{}
	_, httpStatus, err := MakeRequest(ctx, token, "GET", url, nil, http.StatusOK, rspJSON)
	if err != nil {
		return nil, httpStatus, err
	}

	return &rspJSON.LUN, httpStatus, nil
}

// Returns the List of LUNs belonging to the pool and project passed in. To
// get a system wide List of LUNs, the pool AND the project must be 'nil'
func GetLuns(ctx context.Context, token *Token, pool, project string) ([]Lun, error) {

	var url string
	if pool != "" && project != "" {
		url = fmt.Sprintf(zLUNs, token.Name, pool, project)
	} else if pool == "" && project == "" {
		url = fmt.Sprintf(zAllLUNs, token.Name)
	} else {
		return nil, grpcStatus.Error(codes.InvalidArgument, "pool and project must be both nil or both not nil")
	}

	luns := new(Luns)
	luns.List = make([]Lun, 0)

	_, _, err := MakeRequest(ctx, token, "GET", url, nil, http.StatusOK, luns)
	if err != nil {
		return nil, err
	}

	return luns.List, nil
}

func DeleteLun(ctx context.Context, token *Token, pool, project, lun string) (bool, int, error) {

	url := fmt.Sprintf(zLUN, token.Name, pool, project, lun)

	_, httpStatus, err := MakeRequest(ctx, token, "DELETE", url, nil, http.StatusNoContent, nil)
	if err != nil {
		return false, httpStatus, err
	}

	if httpStatus != http.StatusNoContent {
		utils.GetLogREST(ctx, 5).Println("DeleteLun", "http status", httpStatus)
		if httpStatus >= 200 && httpStatus < 300 {
			return false, httpStatus, nil
		}
	}

	return true, httpStatus, nil
}

func GetInitiatorGroupList(ctx context.Context, token *Token, pool, project, 
	lun string) ([]string, error) {

	url := fmt.Sprintf(zLUN, token.Name, pool, project, lun)

	rspBody := &LunJson{}
	_, _, err := MakeRequest(ctx, token, "GET", url, nil, http.StatusOK, rspBody)
	if err != nil {
		return nil, err
	}
	utils.GetLogREST(ctx, 2).Printf("Retrieved the initiator list: {}", rspBody)

	return rspBody.LUN.InitiatorGroup, nil
}

func SetInitiatorGroupList(ctx context.Context, token *Token, pool, project, lun, 
	group string) (int, error) {

	url := fmt.Sprintf(zLUN, token.Name, pool, project, lun)

	reqBody := &LunInitiatorGrps{InitiatorGroup: []string{group}}
	utils.GetLogREST(ctx, 2).Printf("Setting up initiator list: {}", reqBody)
	_, code, err := MakeRequest(ctx, token, "PUT", url, reqBody, http.StatusAccepted, nil)
	return code, err
}

func (l *Luns) UnmarshalJSON(b []byte) error {
	return zfssaUnmarshalList(b, &l.List)
}

func CloneLunSnapshot(ctx context.Context, token *Token, hRef string, 
	parameters map[string]interface{}) (*Lun, int, error) {

	url := fmt.Sprintf(zAppliance + hRef + "/clone", token.Name)

	rspBody := new(LunJson)

	_, code, err := MakeRequest(ctx, token, "PUT", url, &parameters, http.StatusCreated, rspBody)
	if err != nil {
		return nil, code, err
	}

	return &rspBody.LUN, code, nil
}
