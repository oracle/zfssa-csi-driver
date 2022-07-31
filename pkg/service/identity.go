/*
 * Copyright (c) 2021, 2022, Oracle.
 * Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
 */

package service

import (
	"github.com/oracle/zfssa-csi-driver/pkg/utils"
	"github.com/oracle/zfssa-csi-driver/pkg/zfssarest"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/protobuf/ptypes/wrappers"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	grpcStatus "google.golang.org/grpc/status"
)

func newZFSSAIdentityServer(zd *ZFSSADriver) *csi.IdentityServer {
	var id csi.IdentityServer = zd
	return &id
}

func (zd *ZFSSADriver) GetPluginInfo(ctx context.Context, req *csi.GetPluginInfoRequest) (
	*csi.GetPluginInfoResponse, error) {

	utils.GetLogIDTY(ctx, 5).Println("GetPluginInfo")

	return &csi.GetPluginInfoResponse{
		Name:          zd.name,
		VendorVersion: zd.version,
	}, nil
}

func (zd *ZFSSADriver) GetPluginCapabilities(ctx context.Context, req *csi.GetPluginCapabilitiesRequest) (
	*csi.GetPluginCapabilitiesResponse, error) {

	utils.GetLogIDTY(ctx, 5).Println("GetPluginCapabilities")

	return &csi.GetPluginCapabilitiesResponse{
		Capabilities: []*csi.PluginCapability{
			{
				Type: &csi.PluginCapability_Service_{
					Service: &csi.PluginCapability_Service{
						Type: csi.PluginCapability_Service_CONTROLLER_SERVICE,
					},
				},
			},
			{
				Type: &csi.PluginCapability_VolumeExpansion_{
					VolumeExpansion: &csi.PluginCapability_VolumeExpansion{
						Type: csi.PluginCapability_VolumeExpansion_ONLINE,
					},
				},
			},
		},
	}, nil
}

// This is a readiness probe for the driver, it is for checking if proper drivers are
// loaded. Typical response to failure is a driver restart.
//
func (zd *ZFSSADriver) Probe(ctx context.Context, req *csi.ProbeRequest) (
	*csi.ProbeResponse, error) {

	utils.GetLogIDTY(ctx, 5).Println("Probe")

	// Check that the appliance is responsive, if it is not, we are on hold
	user, password, err := zd.getUserLogin(ctx, nil)
	if err != nil {
		return nil, grpcStatus.Error(codes.Unauthenticated, "Invalid credentials")
	}
	token := zfssarest.LookUpToken(ctx, user, password)
	_, err = zfssarest.GetServices(ctx, token)
	if err != nil {
		return &csi.ProbeResponse{
			Ready: &wrappers.BoolValue{Value: false},
		}, grpcStatus.Error(codes.FailedPrecondition, "Failure creating token")
	}

	return &csi.ProbeResponse{
		Ready: &wrappers.BoolValue{Value: true},
	}, nil
}
