/*
 * Copyright (c) 2021, Oracle and/or its affiliates.
 * Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
 */

package zfssarest

import (
	"context"
	"fmt"
	"net/http"
)

type Target struct {
	Alias				string
	Auth				string
	HRef				string
	Interfaces			[]string
	IQN					string
	TargetChapSecret	string
	TargetChapUser		string
}

type targetJSON struct {
	Target Target `json:"Target"`
}

type TargetGroup struct {
	Name		string
	Targets		[]string `json:"targets"`
}

type targetGroupJSON struct {
	Group 	TargetGroup `json:"group"`
}

func GetTargetGroup(ctx context.Context, token *Token, protocol, groupName string) (*TargetGroup, error) {

	url := fmt.Sprintf(zTargetGroup, token.Name, protocol, groupName)

	rspBody := &targetGroupJSON{}
	_, _, err := MakeRequest(ctx, token, "GET", url, nil, http.StatusOK, rspBody)
	if err != nil {
		return nil, err
	}

	return &rspBody.Group, nil
}
