/*
 * Copyright (c) 2021, Oracle and/or its affiliates.
 * Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
 */

package zfssarest

import (
	"github.com/oracle/zfssa-csi-driver/pkg/utils"
	"context"
	"fmt"
	"net/http"
)

type Snapshot struct {
	Name			string	`json:"name"`
	NumClones		int		`json:"numclones"`
	Creation		string	`json:"creation"`
	Collection		string	`json:"collection"`
	Project			string	`json:"project"`
	CanonicalName	string	`json:"canonical_name"`
	SpaceUnique		int64	`json:"space_unique"`
	SpaceData		int64	`json:"space_data"`
	Type			string	`json:"type"`
	ID				string	`json:"id"`
	Pool			string	`json:"pool"`
	Href			string	`json:"href"`
}

type snapshotJSON struct {
	Snapshot	Snapshot	`json:"snapshot"`
}

type snapshots struct {
	List		[]Snapshot	`json:"snapshots"`
}

type Dependent struct {
	Project		string	`json:"project"`
	HRef		string	`json:"href"`
	Share		string	`json:"share"`
}

type dependents struct {
	List		[]Dependent	`json:"dependents"`
}

// Issues a request to the appliance to create a snapshot. The source of the snapshot, LUN
// or filesystem, is determine by the HREF passed in.
func CreateSnapshot(ctx context.Context, token *Token, href, name string) (*Snapshot, int, error) {

	url := fmt.Sprintf(zAppliance + href + "/snapshots", token.Name)

	reqBody := make(map[string]interface{})
	reqBody["name"] = name
	rspBody := new(snapshotJSON)

	_, code, err := MakeRequest(ctx, token, "POST", url, &reqBody, http.StatusCreated, rspBody)
	if err != nil {
		return nil, code, err
	}

	return &rspBody.Snapshot, code, nil
}

// Issues a request to the appliance to delete a snapshot. The source of the snapshot, LUN
// or filesystem, is determine by the HREF passed in.
func DeleteSnapshot(ctx context.Context, token *Token, href string) (
	bool, int, error) {

	url := fmt.Sprintf(zAppliance + href, token.Name)

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

// Issues a request to the appliance asking for the detailed information of a snapshot.
func GetSnapshot(ctx context.Context, token *Token, href, name string) (*Snapshot, int, error) {

	url := fmt.Sprintf(zAppliance + href + "/snapshots/%s", token.Name, name)

	rspJSON := &snapshotJSON{}
	_, httpStatus, err := MakeRequest(ctx, token, "GET", url, nil, http.StatusOK, rspJSON)
	if err != nil {
		return nil, httpStatus, err
	}

	return &rspJSON.Snapshot, httpStatus, nil
}

// Issues a request to the appliance asking for a volume's snapshot list.
func GetSnapshots(ctx context.Context, token *Token, href string) ([]Snapshot, error) {

	var url string

	if len(href) > 0  {
		url = fmt.Sprintf(zAppliance + href + "/snapshots", token.Name)
	} else {
		url = fmt.Sprintf(zAllSnapshots, token.Name)
	}

	snapshots := new(snapshots)
	snapshots.List = make([]Snapshot, 0)

	_, _, err := MakeRequest(ctx, token, "GET", url, nil, http.StatusOK, snapshots)
	if err != nil {
		return nil, err
	}

	return snapshots.List, nil
}

// Returns the clones depending on a snapshot.
func GetSnapshotDependents(ctx context.Context, token *Token, href string) (*[]Dependent, error) {

	url := fmt.Sprintf(zAppliance + href + "/dependents", token.Name)

	dependents := new(dependents)
	dependents.List = make([]Dependent, 0)

	_, _, err := MakeRequest(ctx, token, "GET", url, nil, http.StatusOK, dependents)
	if err != nil {
		return nil, err
	}

	return &dependents.List, nil
}

func (l *snapshots) UnmarshalJSON(b []byte) error {
	return zfssaUnmarshalList(b, &l.List)
}

func (l *dependents) UnmarshalJSON(b []byte) error {
	return zfssaUnmarshalList(b, &l.List)
}
