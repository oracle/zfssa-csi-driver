/*
 * Copyright (c) 2021, Oracle and/or its affiliates.
 * Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
 */

package zfssarest

import (
	"github.com/oracle/zfssa-csi-driver/pkg/utils"
	"context"
	"fmt"
	"google.golang.org/grpc/codes"
	grpcStatus "google.golang.org/grpc/status"
	"net/http"
)

type Pool struct {
	Status 		string	`json:"status"`
	Name		string	`json:"name"`
	Usage	struct {
		Available				int64	`json:"available"`
		UsageSnapshots			int64	`json:"usage_snapshots"`
		UsageMetasize			int64	`json:"usage_metasize"`
		Used					int64	`json:"used"`
		UsageChildReservation	int64	`json:"usage_child_reservation"`
		UsageReplication		int64	`json:"usage_replication"`
		UsageReservation		int64	`json:"usage_reservation"`
		Free					int64	`json:"free"`
		UsageTotal				int64	`json:"usage_total"`
		UsageMetaused			int64	`json:"usage_metaused"`
		Total					int64	`json:"total"`
		UsageData				int64	`json:"usage_data"`
	} `json:"usage"`
	HRef		string	`json:"href"`
	ASN			string	`json:"asn"`
}

type poolJSON struct {
	Pool	Pool	`json:"pool"`
}

type pools struct {
	List	[]Pool `json:"pools"`
}

func GetPool(ctx context.Context, token *Token, name string) (*Pool, error) {

	// We retrieve the information from the ZFSSA
	url := fmt.Sprintf(zPool, token.Name, name)

	json := new(poolJSON)
	_, httpstatus, err := MakeRequest(ctx, token, "GET", url, nil, http.StatusOK, json)
	if err == nil && httpstatus == http.StatusOK {
		return &json.Pool, nil
	}

	if err != nil {
		utils.GetLogREST(ctx, 2).Println("Request for pool information failed with a local error",
			"url", url, "error", err.Error())
	} else {
		utils.GetLogREST(ctx, 2).Println("Request for pool information failed with a ZFSSA error",
			"url", url, "http status", httpstatus)
	}

	return nil, grpcStatus.Error(codes.NotFound,"Pool not found")
}

func GetPools(ctx context.Context, token *Token) (*[]Pool, error) {

	url := fmt.Sprintf(zPools, token.Name)

	zfssaPools := new(pools)
	zfssaPools.List = make([]Pool, 0)

	_, _, err := MakeRequest(ctx, token, "GET", url, nil, http.StatusOK, zfssaPools)
	if err != nil {
		return nil, err
	}

	return &zfssaPools.List, nil
}

func (l *pools) UnmarshalJSON(b []byte) error {
	return zfssaUnmarshalList(b, &l.List)
}
