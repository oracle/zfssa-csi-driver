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

type Schema struct {
	Type		string	`json:"type"`
	Description string	`json:"description"`
	Property	string  `json:"property"`
	Href		string  `json:"href,omitempty"`
}

// This type is to help deserialize from a ZFSSA response, not used internally
type SchemaList struct {
	Schema 	[]Schema `json:"schema"`
}

type Property struct {
	Property Schema `json:"property"`
}

func CreateProperty(ctx context.Context, token *Token, s Schema) (*Schema, error) {

	utils.GetLogREST(ctx, 5).Println("CreateSchema", "schema", s, "target", token.Name)

	url := fmt.Sprintf(zProperties, token.Name)

	resultSchema := &Property{}
	_, _, err := MakeRequest(nil, token, "POST", url, s, http.StatusCreated, resultSchema)
	if err != nil {
		return nil, err
	}

	return &resultSchema.Property, nil
}

func GetProperty(ctx context.Context, token *Token, property string) (*Schema, error) {
	utils.GetLogREST(ctx, 5).Println("GetSchema", "property", property, "target", token.Name)
	url := fmt.Sprintf(zProperty, token.Name, property)

	resultSchema := &Property{}
	_, _, err := MakeRequest(nil, token, "GET", url, nil, http.StatusOK, resultSchema)
	if err != nil {
		return nil, err
	}

	return &resultSchema.Property, nil
}

func GetSchema(ctx context.Context, token *Token) (*SchemaList, error) {
	utils.GetLogREST(ctx, 5).Println("GetSchema")

	url := fmt.Sprintf(zProperties, token.Name)

	jsonData := &SchemaList{}
	_, _, err := MakeRequest(nil, token, "GET", url, nil, http.StatusOK, jsonData)
	if err != nil {
		return nil, err
	}

	return jsonData, nil
}
