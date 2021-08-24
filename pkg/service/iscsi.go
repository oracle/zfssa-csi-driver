/*
 * Copyright (c) 2021, Oracle and/or its affiliates.
 * Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
 */

package service

import (
	"github.com/oracle/zfssa-csi-driver/pkg/utils"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/container-storage-interface/spec/lib/go/csi"
	iscsi_lib "github.com/kubernetes-csi/csi-lib-iscsi/iscsi"
	"k8s.io/utils/mount"
	"k8s.io/kubernetes/pkg/volume/util"
	"os"
	"os/exec"
	"path"
	"strings"
)

// A subset of the iscsiadm
type IscsiAdmReturnValues int32

const(
	ISCSI_SUCCESS IscsiAdmReturnValues = 0
	ISCSI_ERR_SESS_NOT_FOUND = 2
	ISCSI_ERR_TRANS_TIMEOUT = 8
	ISCSI_ERR_ISCSID_NOTCONN = 20
	ISCSI_ERR_NO_OBJS_FOUND = 21
)

func GetISCSIInfo(ctx context.Context, vid *utils.VolumeId, req *csi.NodePublishVolumeRequest, targetIqn string,
	assignedLunNumber int32) (*iscsiDisk, error) {

	volName := vid.Name
	tp := req.GetVolumeContext()["targetPortal"]
	iqn := targetIqn

	if tp == "" || iqn == "" {
		return nil, fmt.Errorf("iSCSI target information is missing (portal=%v), (iqn=%v)", tp, iqn)
	}

	portalList := req.GetVolumeContext()["portals"]
	if portalList == "" {
		portalList = "[]"
	}

	utils.GetLogCTRL(ctx, 5).Println("getISCSIInfo", "portal_list", portalList)
	secretParams := req.GetVolumeContext()["secret"]

	utils.GetLogCTRL(ctx, 5).Println("getISCSIInfo", "secret_params", secretParams)
	secret := parseSecret(secretParams)
	sessionSecret, err := parseSessionSecret(secret)
	if err != nil {
		return nil, err
	}

	discoverySecret, err := parseDiscoverySecret(secret)
	if err != nil {
		return nil, err
	}

	utils.GetLogCTRL(ctx, 5).Println("portalMounter", "tp", tp)
	portal := portalMounter(tp)
	var bkportal []string
	bkportal = append(bkportal, portal)

	portals := []string{}
	if err := json.Unmarshal([]byte(portalList), &portals); err != nil {
		return nil, err
	}

	for _, portal := range portals {
		bkportal = append(bkportal, portalMounter(string(portal)))
	}

	utils.GetLogCTRL(ctx, 5).Println("Built bkportal", "bkportal", bkportal)
	iface := req.GetVolumeContext()["iscsiInterface"]
	initiatorName := req.GetVolumeContext()["initiatorName"]
	chapDiscovery := false
	if req.GetVolumeContext()["discoveryCHAPAuth"] == "true" {
		chapDiscovery = true
	}

	chapSession := false
	if req.GetVolumeContext()["sessionCHAPAuth"] == "true" {
		chapSession = true
	}

	utils.GetLogCTRL(ctx, 5).Println("Final values", "iface", iface, "initiatorName", initiatorName)
	i := iscsiDisk{
		VolName: volName,
		Portals:         bkportal,
		Iqn: 			 iqn,
		lun:             assignedLunNumber,
		Iface:           iface,
		chapDiscovery:   chapDiscovery,
		chapSession:     chapSession,
		secret:          secret,
		sessionSecret:   sessionSecret,
		discoverySecret: discoverySecret,
		InitiatorName:   initiatorName,
	}
	return &i, nil
}

func GetNodeISCSIInfo(vid *utils.VolumeId, req *csi.NodePublishVolumeRequest, targetIqn string, assignedLunNumber int32) (
	*iscsiDisk, error) {

	volName := vid.Name
	tp := req.GetVolumeContext()["targetPortal"]
	iqn := targetIqn
	if tp == "" || iqn == "" {
		return nil, fmt.Errorf("iSCSI target information is missing")
	}

	portalList := req.GetVolumeContext()["portals"]
	if portalList == "" {
		portalList = "[]"
	}
	secretParams := req.GetVolumeContext()["secret"]
	secret := parseSecret(secretParams)
	sessionSecret, err := parseSessionSecret(secret)
	if err != nil {
		return nil, err
	}

	discoverySecret, err := parseDiscoverySecret(secret)
	if err != nil {
		return nil, err
	}

	// For ZFSSA, the portal should also contain the assigned number
	portal := portalMounter(tp)
	var bkportal []string
	bkportal = append(bkportal, portal)

	portals := []string{}
	if err := json.Unmarshal([]byte(portalList), &portals); err != nil {
		return nil, err
	}

	for _, portal := range portals {
		bkportal = append(bkportal, portalMounter(string(portal)))
	}

	iface := req.GetVolumeContext()["iscsiInterface"]
	initiatorName := req.GetVolumeContext()["initiatorName"]
	chapDiscovery := false
	if req.GetVolumeContext()["discoveryCHAPAuth"] == "true" {
		chapDiscovery = true
	}

	chapSession := false
	if req.GetVolumeContext()["sessionCHAPAuth"] == "true" {
		chapSession = true
	}

	i := iscsiDisk{
		VolName: volName,
		Portals:         bkportal,
		Iqn: iqn,
		lun:             assignedLunNumber,
		Iface:           iface,
		chapDiscovery:   chapDiscovery,
		chapSession:     chapSession,
		secret:          secret,
		sessionSecret:   sessionSecret,
		discoverySecret: discoverySecret,
		InitiatorName:   initiatorName,
	}
	return &i, nil
}

func buildISCSIConnector(iscsiInfo *iscsiDisk) *iscsi_lib.Connector {
	c := iscsi_lib.Connector{
		VolumeName:    iscsiInfo.VolName,
		TargetIqn:     iscsiInfo.Iqn,
		TargetPortals: iscsiInfo.Portals,
		Lun:		   iscsiInfo.lun,
		Multipath:     len(iscsiInfo.Portals) > 1,
	}

	if iscsiInfo.sessionSecret != (iscsi_lib.Secrets{}) {
		c.SessionSecrets = iscsiInfo.sessionSecret
		if iscsiInfo.discoverySecret != (iscsi_lib.Secrets{}) {
			c.DiscoverySecrets = iscsiInfo.discoverySecret
		}
	}

	return &c
}

func GetISCSIDiskMounter(iscsiInfo *iscsiDisk, readOnly bool, fsType string, mountOptions []string,
	targetPath string) *iscsiDiskMounter {

	return &iscsiDiskMounter{
		iscsiDisk:    iscsiInfo,
		fsType:       fsType,
		readOnly:     readOnly,
		mountOptions: mountOptions,
		mounter:      &mount.SafeFormatAndMount{Interface: mount.New("")},
		targetPath:   targetPath,
		deviceUtil:   util.NewDeviceHandler(util.NewIOHandler()),
		connector:    buildISCSIConnector(iscsiInfo),
	}
}

func GetISCSIDiskUnmounter(volumeId *utils.VolumeId) *iscsiDiskUnmounter {
	volName := volumeId.Name
	return &iscsiDiskUnmounter{
		iscsiDisk: &iscsiDisk{
			VolName: volName,
		},
		mounter: mount.New(""),
	}
}

func portalMounter(portal string) string {
	if !strings.Contains(portal, ":") {
		portal = portal + ":3260"
	}
	return portal
}

func parseSecret(secretParams string) map[string]string {
	var secret map[string]string
	if err := json.Unmarshal([]byte(secretParams), &secret); err != nil {
		return nil
	}
	return secret
}

func parseSessionSecret(secretParams map[string]string) (iscsi_lib.Secrets, error) {
	var ok bool
	secret := iscsi_lib.Secrets{}

	if len(secretParams) == 0 {
		return secret, nil
	}

	if secret.UserName, ok = secretParams["node.session.auth.username"]; !ok {
		return iscsi_lib.Secrets{}, fmt.Errorf("node.session.auth.username not found in secret")
	}
	if secret.Password, ok = secretParams["node.session.auth.password"]; !ok {
		return iscsi_lib.Secrets{}, fmt.Errorf("node.session.auth.password not found in secret")
	}
	if secret.UserNameIn, ok = secretParams["node.session.auth.username_in"]; !ok {
		return iscsi_lib.Secrets{}, fmt.Errorf("node.session.auth.username_in not found in secret")
	}
	if secret.PasswordIn, ok = secretParams["node.session.auth.password_in"]; !ok {
		return iscsi_lib.Secrets{}, fmt.Errorf("node.session.auth.password_in not found in secret")
	}

	secret.SecretsType = "chap"
	return secret, nil
}

func parseDiscoverySecret(secretParams map[string]string) (iscsi_lib.Secrets, error) {
	var ok bool
	secret := iscsi_lib.Secrets{}

	if len(secretParams) == 0 {
		return secret, nil
	}

	if secret.UserName, ok = secretParams["node.sendtargets.auth.username"]; !ok {
		return iscsi_lib.Secrets{}, fmt.Errorf("node.sendtargets.auth.username not found in secret")
	}
	if secret.Password, ok = secretParams["node.sendtargets.auth.password"]; !ok {
		return iscsi_lib.Secrets{}, fmt.Errorf("node.sendtargets.auth.password not found in secret")
	}
	if secret.UserNameIn, ok = secretParams["node.sendtargets.auth.username_in"]; !ok {
		return iscsi_lib.Secrets{}, fmt.Errorf("node.sendtargets.auth.username_in not found in secret")
	}
	if secret.PasswordIn, ok = secretParams["node.sendtargets.auth.password_in"]; !ok {
		return iscsi_lib.Secrets{}, fmt.Errorf("node.sendtargets.auth.password_in not found in secret")
	}

	secret.SecretsType = "chap"
	return secret, nil
}

type iscsiDisk struct {
	Portals         []string
	Iqn             string
	lun             int32
	Iface           string
	chapDiscovery   bool
	chapSession     bool
	secret          map[string]string
	sessionSecret   iscsi_lib.Secrets
	discoverySecret iscsi_lib.Secrets
	InitiatorName   string
	VolName         string
}

type iscsiDiskMounter struct {
	*iscsiDisk
	readOnly     bool
	fsType       string
	mountOptions []string
	mounter      *mount.SafeFormatAndMount
	deviceUtil   util.DeviceUtil
	targetPath   string
	connector    *iscsi_lib.Connector
}

type iscsiDiskUnmounter struct {
	*iscsiDisk
	mounter mount.Interface
}


type ISCSIUtil struct{}

func (util *ISCSIUtil) Rescan (ctx context.Context) (string, error) {
	cmd := exec.Command("iscsiadm", "-m", "session", "--rescan")
	var stdout bytes.Buffer
	var iscsiadmError error
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout
	defer stdout.Reset()

	// we're using Start and Wait because we want to grab exit codes
	err := cmd.Start()
	if err != nil {
		// Check if this is simply no sessions found (rc 21)
		exitCode := err.(*exec.ExitError).ExitCode()
		if exitCode == ISCSI_ERR_NO_OBJS_FOUND {
			// No error, just no objects found
			utils.GetLogUTIL(ctx, 4).Println("iscsiadm: no sessions, will continue (start path)")
		} else {
			formattedOutput := strings.Replace(string(stdout.Bytes()), "\n", "", -1)
			iscsiadmError = fmt.Errorf("iscsiadm error: %s (%s)", formattedOutput, err.Error())
		}
	    return string(stdout.Bytes()), iscsiadmError
	} 
    
	err = cmd.Wait()
	if err != nil {
		exitCode := err.(*exec.ExitError).ExitCode()
		if exitCode == ISCSI_ERR_NO_OBJS_FOUND {
			// No error, just no objects found
			utils.GetLogUTIL(ctx, 4).Println("iscsiadm: no sessions, will continue (wait path)")
		} else {
			formattedOutput := strings.Replace(string(stdout.Bytes()), "\n", "", -1)
			iscsiadmError = fmt.Errorf("iscsiadm error: %s (%s)", formattedOutput, err.Error())
		}
	}
	return string(stdout.Bytes()), iscsiadmError
}

func (util *ISCSIUtil) ConnectDisk(ctx context.Context, b iscsiDiskMounter) (string, error) {
	utils.GetLogUTIL(ctx, 4).Println("ConnectDisk started")
	_, err := util.Rescan(ctx)
	if err != nil {
		utils.GetLogUTIL(ctx, 4).Println("iSCSI rescan error: %s", err.Error())
		return "", err
	}
	utils.GetLogUTIL(ctx, 4).Println("ConnectDisk will connect and get device path")
	devicePath, err := iscsi_lib.Connect(*b.connector)
	if err != nil {
		utils.GetLogUTIL(ctx, 4).Println("iscsi_lib connect error: %s", err.Error())
		return "", err
	}

	if devicePath == "" {
		utils.GetLogUTIL(ctx, 4).Println("iscsi_lib devicePath is empty, cannot continue")
		return "", fmt.Errorf("connect reported success, but no path returned")
	}
	utils.GetLogUTIL(ctx, 4).Println("ConnectDisk devicePath: %s", devicePath)
	return devicePath, nil
}

func (util *ISCSIUtil) AttachDisk(ctx context.Context, b iscsiDiskMounter, devicePath string) (string, error) {
	// Mount device
	if len(devicePath) == 0 {
		localDevicePath, err := util.ConnectDisk(ctx, b)
		if err != nil {
			utils.GetLogUTIL(ctx, 3).Println("ConnectDisk failure: %s", err.Error())
			return "", err
		}
		devicePath = localDevicePath
	}

	mntPath := b.targetPath
	notMnt, err := b.mounter.IsLikelyNotMountPoint(mntPath)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("heuristic determination of mount point failed: %v", err)
	}
	if !notMnt {
		utils.GetLogUTIL(ctx, 3).Println("iscsi: device already mounted", "mount_path", mntPath)
		return "", nil
	}

	if err := os.MkdirAll(mntPath, 0750); err != nil {
		return "", err
	}

	// Persist iscsi disk config to json file for DetachDisk path
	file := path.Join(mntPath, b.VolName+".json")
	err = iscsi_lib.PersistConnector(b.connector, file)
	if err != nil {
		return "", err
	}

	options := []string{"bind"}

	if b.readOnly {
		options = append(options, "ro")
	} else {
		options = append(options, "rw")
	}
	options = append(options, b.mountOptions...)

	utils.GetLogUTIL(ctx, 3).Println("Mounting disk at path: %s", mntPath)
	err = b.mounter.Mount(devicePath, mntPath, "", options)
	if err != nil {
		utils.GetLogUTIL(ctx, 3).Println("iscsi: failed to mount iscsi volume",
			"device_path", devicePath, "mount_path", mntPath, "error", err.Error())
		return "", err
	}

	return devicePath, err
}

func (util *ISCSIUtil) DetachDisk(ctx context.Context, c iscsiDiskUnmounter, targetPath string) error {
	_, cnt, err := mount.GetDeviceNameFromMount(c.mounter, targetPath)
	if err != nil {
		return err
	}

	if pathExists, pathErr := mount.PathExists(targetPath); pathErr != nil {
		return fmt.Errorf("Error checking if path exists: %v", pathErr)
	} else if !pathExists {
		utils.GetLogUTIL(ctx, 2).Println("Unmount skipped because path does not exist",
			"target_path", targetPath)
		return nil
	}
	if err = c.mounter.Unmount(targetPath); err != nil {
		utils.GetLogUTIL(ctx, 3).Println("iscsi detach disk: failed to unmount",
			"target_path", targetPath, "error", err.Error())
		return err
	}

	cnt--
	if cnt != 0 {
		return nil
	}

	// load iscsi disk config from json file
	file := path.Join(targetPath, c.iscsiDisk.VolName+".json")
	connector, err := iscsi_lib.GetConnectorFromFile(file)
	if err != nil {
		return err
	}

	err = iscsi_lib.Disconnect(connector.TargetIqn, connector.TargetPortals)
	if err := os.RemoveAll(targetPath); err != nil {
		return err
	}

	return nil
}
