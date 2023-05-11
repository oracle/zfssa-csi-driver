/*
 * Copyright (c) 2023, Oracle and/or its affiliates.
 * Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
 */

package utils

import (
	"io/ioutil"
	"gopkg.in/yaml.v2"
	"errors"
	"fmt"
)

func GetValueFromYAML(yamlFilePath string, key string) (string, error) {
	yamlData, err := ioutil.ReadFile(yamlFilePath)
	if err != nil {
		return "", errors.New(fmt.Sprintf("the file <%s> could not be read from: <%s>",
			yamlFilePath, err))
	}

	// Unmarshal YAML into a map[string]interface{}
	yamlMap := make(map[string]interface{})
	err = yaml.Unmarshal(yamlData, &yamlMap)
	if err != nil {
		return "", errors.New(fmt.Sprintf("the file <%s> could not be parsed: <%s>",
			yamlFilePath, err))
	}

	// Get value from map using key
	value, ok := yamlMap[key]
	if !ok {
		return "", errors.New(fmt.Sprintf("key: <%s> could not be retrieved from <%s> : <%s>",
		     key, yamlFilePath, err))
	}
	// Convert value to string and return
	return fmt.Sprintf("%v", value), nil

}
