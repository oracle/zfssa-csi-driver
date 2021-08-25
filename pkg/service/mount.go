/*
 * Copyright (c) 2021, Oracle and/or its affiliates.
 * Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
 */

package service

import (
	"k8s.io/utils/mount"
	"os"
)

// Mounter is an interface for mount operations
type Mounter interface {
	mount.Interface
	GetDeviceName(mountPath string) (string, int, error)
	MakeFile(pathname string) error
	ExistsPath(pathname string) (bool, error)
}

type NodeMounter struct {
	mount.SafeFormatAndMount
}

func newNodeMounter() Mounter {
	return &NodeMounter{
		mount.SafeFormatAndMount{
			Interface: mount.New(""),
		},
	}
}

// Retrieve a device name from a mount point (this is a compatibility interface)
func (m *NodeMounter) GetDeviceName(mountPath string) (string, int, error) {
	return mount.GetDeviceNameFromMount(m, mountPath)
}

// Make a file at the pathname
func (mounter *NodeMounter) MakeFile(pathname string) error {
	f, err := os.OpenFile(pathname, os.O_CREATE, os.FileMode(0644))
	defer f.Close()

	if err != nil && !os.IsExist(err) {
		return err
	}

	return nil
}

// Check if a file exists
func (mount *NodeMounter) ExistsPath(pathname string) (bool, error) {
	// Check if the global mount path exists and create it if it does not
	exists := true
	_, err := os.Stat(pathname)

	if _, err := os.Stat(pathname); os.IsNotExist(err) {
		exists = false
	}

	return exists, err
}
