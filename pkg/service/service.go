/*
 * Copyright (c) 2021, Oracle and/or its affiliates.
 * Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
 */

package service

import (
	"github.com/oracle/zfssa-csi-driver/pkg/utils"
	"github.com/oracle/zfssa-csi-driver/pkg/zfssarest"
	"errors"
	"fmt"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	// Default Log Level
	DefaultLogLevel = "3"
	DefaultCertPath = "/mnt/certs/zfssa.crt"
	DefaultCredPath = "/mnt/zfssa/zfssa.yaml"
)

type ZFSSADriver struct {
	name        string
	nodeID      string
	version     string
	endpoint    string
	config      config
	NodeMounter Mounter
	vCache      volumeHashTable
	sCache      snapshotHashTable
	ns          *csi.NodeServer
	cs          *csi.ControllerServer
	is          *csi.IdentityServer
}

type config struct {
	Appliance	string
	User		string
	endpoint	string
	HostIp		string
	NodeName	string
	PodIp		string
	Secure		bool
	logLevel	string
	Certificate	[]byte
	CertLocation string
	CredLocation string
}

// The structured data in the ZFSSA credentials file
type ZfssaCredentials struct {
	Username string `yaml:username`
	Password string `yaml:password`
}

type accessType int

// NonBlocking server

type nonBlockingGRPCServer struct {
	wg		sync.WaitGroup
	server	*grpc.Server
}

const (
	// Helpful size constants
	Kib    int64 = 1024
	Mib    int64 = Kib * 1024
	Gib    int64 = Mib * 1024
	Gib100 int64 = Gib * 100
	Tib    int64 = Gib * 1024
	Tib100 int64 = Tib * 100

	DefaultVolumeSizeBytes	int64 = 50 * Gib

	mountAccess accessType = iota
	blockAccess
)

const (
	UsernamePattern string = `^[a-zA-Z][a-zA-Z0-9_\-\.]*$`
	UsernameLength int = 255
)

type ZfssaBlockVolume struct {
	VolName       string     `json:"volName"`
	VolID         string     `json:"volID"`
	VolSize       int64      `json:"volSize"`
	VolPath       string     `json:"volPath"`
	VolAccessType accessType `json:"volAccessType"`
}

// Creates and returns a new ZFSSA driver structure.
func NewZFSSADriver(driverName, version string) (*ZFSSADriver, error) {

	zd := new(ZFSSADriver)

	zd.name = driverName
	zd.version = version
	err := getConfig(zd)
	if err != nil {
		return nil, err
	}

	zd.vCache.vHash = make(map[string]zVolumeInterface)
	zd.sCache.sHash = make(map[string]*zSnapshot)

	utils.InitLogs(zd.config.logLevel, zd.name, version, zd.config.NodeName)

	err = zfssarest.InitREST(zd.config.Appliance, zd.config.Certificate, zd.config.Secure)
	if err != nil {
		return nil, err
	}

	err = InitClusterInterface()
	if err != nil {
		return nil, err
	}

	zd.is = newZFSSAIdentityServer(zd)
	zd.cs = newZFSSAControllerServer(zd)
	zd.ns = NewZFSSANodeServer(zd)

	return zd, nil
}

// Gets the configuration and sanity checks it. Several environment variables values
// are retrieved:
//
//	ZFSSA_TARGET	The name or IP address of the appliance.
//	NODE_NAME		The name of the node on which the container is running.
//	NODE_ID			The ID of the node on which the container is running.
//	CSI_ENDPOINT	Unix socket the CSI driver will be listening on.
//	ZFSSA_INSECURE	Boolean specifying whether an appliance certificate is not required.
//	ZFSSA_CERT		Path to the certificate file (defaults to "/mnt/certs/zfssa.crt")
//	ZFSSA_CRED		Path to the credential file (defaults to "/mnt/zfssa/zfssa.yaml")
//	HOST_IP			IP address of the node.
//	POD_IP			IP address of the pod.
//	LOG_LEVEL		Log level to apply.
//
// Verifies the credentials are in the ZFSSA_CRED yaml file, does not verify their
// correctness.
func getConfig(zd *ZFSSADriver) error {
	// Validate the ZFSSA credentials are available
	credfile := strings.TrimSpace(getEnvFallback("ZFSSA_CRED", DefaultCredPath))
	if len(credfile) == 0 {
		return errors.New(fmt.Sprintf("a ZFSSA credentials file location is required, current value: <%s>",
			credfile))
	}
	zd.config.CredLocation = credfile
	_, err := os.Stat(credfile)
	if os.IsNotExist(err) {
		return errors.New(fmt.Sprintf("the ZFSSA credentials file is not present at location: <%s>",
			credfile))
	}

	// Get the user from the credentials file, this can be stored in the config file without a problem
	zd.config.User, err = zd.GetUsernameFromCred()
	if err != nil {
		return errors.New(fmt.Sprintf("Cannot get ZFSSA username: %s", err))
	}

	appliance := getEnvFallback("ZFSSA_TARGET", "")
	zd.config.Appliance = strings.TrimSpace(appliance)
	if zd.config.Appliance == "not-set" {
		return errors.New("appliance name required")
	}

	zd.config.NodeName = getEnvFallback("NODE_NAME", "")
	if zd.config.NodeName == "" {
		return errors.New("node name required")
	}

	zd.config.endpoint = getEnvFallback("CSI_ENDPOINT", "")
	if zd.config.endpoint == ""	{
		return errors.New("endpoint is required")
	} else {
		if !strings.HasPrefix(zd.config.endpoint, "unix://") {
			return errors.New("endpoint is invalid")
		}
		s := strings.SplitN(zd.config.endpoint, "://", 2)
		zd.config.endpoint = "/" + s[1]
		err := os.RemoveAll(zd.config.endpoint)
		if err != nil && !os.IsNotExist(err) {
			return errors.New("failed to remove endpoint path")
		}
	}

	switch strings.ToLower(strings.TrimSpace(getEnvFallback("ZFSSA_INSECURE", "False"))) {
	case "true":	zd.config.Secure = false
	case "false":	zd.config.Secure = true
	default:
		return errors.New("ZFSSA_INSECURE value is invalid")
	}

	if zd.config.Secure {
		certfile := strings.TrimSpace(getEnvFallback("ZFSSA_CERT", DefaultCertPath))
		if len(certfile) == 0 {
			return errors.New("a certificate is required")
		}
		_, err := os.Stat(certfile)
		if os.IsNotExist(err) {
			return errors.New("certificate does not exits")
		}
		zd.config.Certificate, err = ioutil.ReadFile(certfile)
		if err != nil {
			return errors.New("failed to read certificate")
		}
	}

	zd.config.HostIp = getEnvFallback("HOST_IP", "0.0.0.0")
	zd.config.PodIp = getEnvFallback("POD_IP", "0.0.0.0")
	zd.config.logLevel = getEnvFallback("LOG_LEVEL", DefaultLogLevel)
	_, err = strconv.Atoi(zd.config.logLevel)
	if err != nil {
		return errors.New("invalid debug level")
	}
	return nil
}

// Starts the CSI driver. This includes registering the different servers (Identity, Controller and Node) with
// the CSI framework and starting listening on the UNIX socket.

var sigList = []os.Signal {
	syscall.SIGTERM,
	syscall.SIGHUP,
	syscall.SIGINT,
	syscall.SIGQUIT,
}

// Retrieves just the username from a credential file (zd.config.CredLocation)
func (zd *ZFSSADriver) GetUsernameFromCred() (string, error) {
	yamlData, err := ioutil.ReadFile(zd.config.CredLocation)
	if err != nil {
		return "", errors.New(fmt.Sprintf("the ZFSSA credentials file <%s> could not be read: <%s>",
			zd.config.CredLocation, err))
	}

	var yamlConfig ZfssaCredentials
	err = yaml.Unmarshal(yamlData, &yamlConfig)
	if err != nil {
		return "", errors.New(fmt.Sprintf("the ZFSSA credentials file <%s> could not be parsed: <%s>",
			zd.config.CredLocation, err))
	}

	if !isUsernameValid(yamlConfig.Username) {
		return "", errors.New(fmt.Sprintf("ZFSSA username is invalid: <%s>", yamlConfig.Username))
	}

	return yamlConfig.Username, nil
}

// Retrieves just the username from a credential file
func (zd *ZFSSADriver) GetPasswordFromCred() (string, error) {
	yamlData, err := ioutil.ReadFile(zd.config.CredLocation)
	if err != nil {
		return "", errors.New(fmt.Sprintf("the ZFSSA credentials file <%s> could not be read: <%s>",
			zd.config.CredLocation, err))
	}

	var yamlConfig ZfssaCredentials
	err = yaml.Unmarshal(yamlData, &yamlConfig)
	if err != nil {
		return "", errors.New(fmt.Sprintf("the ZFSSA credentials file <%s> could not be parsed: <%s>",
			zd.config.CredLocation, err))
	}

	return yamlConfig.Password, nil
}

func (zd *ZFSSADriver) Run() {
	// Refresh current information
	_ = zd.updateVolumeList(nil)
	_ = zd.updateSnapshotList(nil)

	// Create GRPC servers
	s := new(nonBlockingGRPCServer)

	sigChannel := make(chan os.Signal, 1)
	signal.Notify(sigChannel, sigList...)

	s.Start(zd.config.endpoint, *zd.is, *zd.cs, *zd.ns)
	s.Wait(sigChannel)
	s.Stop()
	_ = os.RemoveAll(zd.config.endpoint)
}

func (s *nonBlockingGRPCServer) Start(endpoint string,
	ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer) {

	s.wg.Add(1)
	go s.serve(endpoint, ids, cs, ns)
}

func (s *nonBlockingGRPCServer) Wait(ch chan os.Signal) {
	for sig := range ch {
		switch sig {
		case syscall.SIGTERM,
			syscall.SIGHUP,
			syscall.SIGINT,
			syscall.SIGQUIT:
			utils.GetLogCSID(nil, 5).Println("Termination signal received", "signal", sig)
			return
		default:
			utils.GetLogCSID(nil, 5).Println("Signal received", "signal", sig)
			continue
		}
	}
}

func (s *nonBlockingGRPCServer) Stop() {
	s.server.GracefulStop()
	s.wg.Add(-1)
}

func (s *nonBlockingGRPCServer) ForceStop() {
	s.server.Stop()
	s.wg.Add(-1)
}

func (s *nonBlockingGRPCServer) serve(endpoint string,
	ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer) {

	listener, err := net.Listen("unix", endpoint)
	if err != nil {
		utils.GetLogCSID(nil, 2).Println("Failed to listen", "error", err)
		return
	}

	opts := []grpc.ServerOption{ grpc.UnaryInterceptor(interceptorGRPC)	}

	server := grpc.NewServer(opts...)
	s.server = server

	csi.RegisterIdentityServer(server, ids)
	csi.RegisterControllerServer(server, cs)
	csi.RegisterNodeServer(server, ns)

	utils.GetLogCSID(nil, 5).Println("Listening for connections", "address", endpoint)

	err = server.Serve(listener)
	if err != nil {
		utils.GetLogCSID(nil, 2).Println("Serve returned with error", "error", err)
	}
}

// Interceptor measuring the response time of the requests.
func interceptorGRPC(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {

	// Get a new context with a list of loggers request specific.
	newContext := utils.GetNewContext(ctx)

	// Calls the handler
	utils.GetLogCSID(newContext, 4).Println("Request submitted", "method:", info.FullMethod)
	start := time.Now()
	rsp, err := handler(newContext, req)
	utils.GetLogCSID(newContext, 4).Println("Request completed", "method:", info.FullMethod,
		"duration:", time.Since(start), "error", err)

	return rsp, err
}

// A local GetEnv utility function
func getEnvFallback(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// validate username
func isUsernameValid(username string) bool {
	if len(username) == 0 || len(username) > UsernameLength {
		return false
	}

	var validUsername = regexp.MustCompile(UsernamePattern).MatchString
	return validUsername(username)
}
