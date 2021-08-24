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

type Project struct {
	Name			string	`json:"name"`
	Pool			string	`json:"pool"`
	SpaceAvailable	int64	`json:"space_available"`
}

type ProjectJSON struct {
	Project 	Project `json:"project"`
}

type projects struct {
	list 		[]Project `json:"projects"`
}

func GetProject(ctx context.Context, token *Token, pool string, project string) (*Project, error) {

	url := fmt.Sprintf(zProject, token.Name, pool, project)

	jsonData := &ProjectJSON{}
	_, _, err := MakeRequest(ctx, token, "GET", url, nil, http.StatusOK, jsonData)
	if err != nil {
		return nil, err
	}

	return &jsonData.Project, nil
}

// Returns the List of filesystems associated with the pool and project passed in. To
// get a system wide List of file systems, the pool must be 'nil'
func GetProjects(ctx context.Context, token *Token, pool string) ([]Project, error) {

	var url string
	if pool != "" {
		url = fmt.Sprintf(zProjects, token.Name, pool)
	} else {
		url = fmt.Sprintf(zAllProjects, token.Name)
	}

	projects := new(projects)
	projects.list = make([]Project, 0)

	_, _, err := MakeRequest(ctx, token, "GET", url, nil, http.StatusOK, projects)
	if err != nil {
		return nil, err
	}

	return projects.list, nil
}

func (l *projects) UnmarshalJSON(b []byte) error {
	return zfssaUnmarshalList(b, &l.list)
}
