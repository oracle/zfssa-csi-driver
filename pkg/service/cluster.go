/*
 * Copyright (c) 2021, Oracle and/or its affiliates.
 * Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
 */

package service

import (
	"errors"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	clusterConfig	*rest.Config
	clientset		*kubernetes.Clientset
)

// Initializes the cluster interface.
//
func InitClusterInterface() error {

	var err error
	clusterConfig, err = rest.InClusterConfig()
	if err != nil {
		fmt.Print("not in cluster mode")
	} else {
		clientset, err = kubernetes.NewForConfig(clusterConfig)
		if err != nil {
			return errors.New("could not get Clientset for Kubernetes work")
		}
	}

	return nil
}

// Returns the node name based on the passed in node ID.
//
func GetNodeName(nodeID string) (string, error) {
	nodeInfo, err := clientset.CoreV1().Nodes().Get(nodeID, metav1.GetOptions{
		TypeMeta:        metav1.TypeMeta{
			Kind:       "",
			APIVersion: "",
		},
		ResourceVersion: "1",
	})

	if err != nil {
		return "", err
	} else {
		return nodeInfo.Name, nil
	}
}

// Returns the list of nodes in the form of a slice containing their name.
//
func GetNodeList() ([]string, error) {

	nodeList, err := clientset.CoreV1().Nodes().List(metav1.ListOptions{
		TypeMeta:        metav1.TypeMeta{
			Kind:       "",
			APIVersion: "",
		},
		ResourceVersion: "1",
	})

	if err != nil {
		return nil, err
	}

	var nodeNameList	[]string
	for _, node:= range nodeList.Items {
		nodeNameList = append(nodeNameList, node.Name)
	}

	return nodeNameList, nil
}
