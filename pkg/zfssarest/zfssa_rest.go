/*
 * Copyright (c) 2021, Oracle and/or its affiliates.
 * Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
 */

package zfssarest

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"github.com/oracle/zfssa-csi-driver/pkg/utils"
	"google.golang.org/grpc/codes"
	grpcStatus "google.golang.org/grpc/status"
)

// Use RESTapi v2 as it returns scriptable and consistent values
const (
	zAppliance					string = "https://%s:215"
	zServices					= zAppliance + "/api/access/v2"
	zStorage					= zAppliance + "/api/storage/v2"
	zSan						= zAppliance + "/api/san/v2"
	zPools						= zStorage + "/pools"
	zPool						= zPools + "/%s"
	zAllProjects				= zStorage + "/projects"
	zProjects					= zPool + "/projects"
	zProject					= zProjects + "/%s"
	zAllFilesystems				= zStorage + "/filesystems"
	zFilesystems				= zProject + "/filesystems"
	zFilesystem					= zFilesystems + "/%s"
	zAllLUNs					= zStorage + "/luns"
	zLUNs						= zProject + "/luns"
	zLUN						= zLUNs + "/%s"
	zAllSnapshots				= zStorage + "/snapshots"
	zSnapshots					= zProject + "/snapshots"
	zSnapshot					= zSnapshots + "/%s"
	zFilesystemSnapshots		= zFilesystem + "/snapshots"
	zFilesystemSnapshot			= zFilesystemSnapshots + "/%s"
	zCloneFilesystemSnapshot	= zFilesystemSnapshot + "/clone"
	zLUNSnapshots				= zLUN + "/snapshots"
	zLUNSnapshot				= zLUNSnapshots + "/%s"
	zFilesystemDependents		= zFilesystemSnapshot + "/dependents"
	zLUNDependents				= zLUNSnapshot + "/dependents"
	zTargetGroups				= zSan + "/%s/target-groups"
	zTargetGroup				= zTargetGroups + "/%s"
	zProperties					= zAppliance + "/api/storage/v2/schema"
	zProperty					= zProperties + "/%s"
)

// State of a ZFSSA token
const (
	zfssaTokenInvalid = iota
	zfssaTokenCreating
	zfssaTokenValid
)

type Token struct {
	Name			string
	cv				*sync.Cond
	mtx				sync.Mutex
	user			string
	password		string
	state			int
	xAuthSession	string
	xAuthName		string
}

type tokenList struct {
	mtx				sync.Mutex
	list			map[string]*Token
}

type faultInfo struct {
	Message			string `json:"message"`
	Code			int `json:"code"`
	Name			string `json:"Name"`
}

type faultResponse struct {
	Fault faultInfo `json:"fault"`
}

var	httpTransport	= http.Transport{TLSClientConfig: &tls.Config{}}
var httpClient		= &http.Client{Transport: &httpTransport}
var zServicesURL	string
var zName 			string
var tokens 			tokenList

// Initializes the ZFSSA REST API interface
//
func InitREST(name string, certs []byte, secure bool) error {

	if secure {
		// set TLSv1.2 for the minimum version of supporting TLS
		httpTransport.TLSClientConfig.MinVersion = tls.VersionTLS12

		// Get the SystemCertPool, continue with an empty pool on error
		httpTransport.TLSClientConfig.RootCAs, _ = x509.SystemCertPool()
		if httpTransport.TLSClientConfig.RootCAs == nil {
			httpTransport.TLSClientConfig.RootCAs = x509.NewCertPool()
		}

		if ok := httpTransport.TLSClientConfig.RootCAs.AppendCertsFromPEM(certs); !ok {
			return errors.New("failed to append the certificate")
		}
	}

	httpTransport.TLSClientConfig.InsecureSkipVerify = !secure
	httpTransport.MaxConnsPerHost = 16
	httpTransport.MaxIdleConnsPerHost = 16
	httpTransport.IdleConnTimeout = 30 * time.Second

	tokens.list = make(map[string]*Token)
	zServicesURL = fmt.Sprintf(zServices, name)
	zName = name

	return nil
}

// Looks up a token context based on the user name passed in. If one doesn't exist
// yet, it is created.
func LookUpToken(user, password string) *Token {

	tokens.mtx.Lock()
	if token, ok := tokens.list[user]; ok {
		tokens.mtx.Unlock()
		return token
	}

	token := new(Token)
	token.Name = zName
	token.user = user
	token.password = password
	token.state = zfssaTokenInvalid
	token.xAuthName = ""
	token.xAuthSession = ""
	token.cv = sync.NewCond(&token.mtx)

	tokens.list[user] = token
	tokens.mtx.Unlock()
	return token
}

// Returns a token. If no token is available it attempts to create one. If a previous
// token is passed in, it assumes that the caller received a status 401 from the ZFSSA
// (probably because the token has expired). In that case this function will try to
// create another one or, if another thread is already in the process of creating one,
// it will wait until the creation has completed.
//
// The possible return values are:
//
//	Code			Message						X-Auth-Session
//
//	nil											Valid
//  codes.Internal	"Failure getting token"		""
//	codes.Internal	"Failure creating token"	""
//
//	In case of failure, the message logged will provide more information
//	as to where the problem occurred.
func getToken(ctx context.Context, token *Token, previous *string) (string, error) {

	token.mtx.Lock()
	for {
		switch token.state {
		case zfssaTokenInvalid:
			// No token available. We create one.
			token.state = zfssaTokenCreating
			token.mtx.Unlock()

			var err error
			token.xAuthSession, token.xAuthName, err = createToken(ctx, token)
			xAuthSession := token.xAuthSession

			token.mtx.Lock()
			if err != nil {
				token.state = zfssaTokenInvalid
			} else {
				token.state = zfssaTokenValid
			}
			token.cv.Broadcast()
			token.mtx.Unlock()
			return xAuthSession, err

		case zfssaTokenCreating:
			// Another thread is creating a token. We wait until it's done.
			token.cv.Wait()
			continue

		case zfssaTokenValid:
			// We can use the current token.
			if previous == nil || *previous != token.xAuthSession {
				xAuthSession := token.xAuthSession
				token.mtx.Unlock()
				return xAuthSession, nil
			}
			token.state = zfssaTokenInvalid
			continue

		default:
			panic(fmt.Sprintf("State of token is unknown %s, %d", token.user, token.state))
		}
	}
}

// Send an HTTP request to the ZFSSA to create a non-persistent token.
//
// A non-persistent token is specific to the cluster node on which the ID was
// created and is not synchronized between the cluster peers.
func createToken(ctx context.Context, token *Token) (string, string, error) {

	httpReq, err := http.NewRequest("POST", zServicesURL, bytes.NewBuffer(nil))
	if err != nil {
		utils.GetLogREST(ctx,2).Println("Could not build a request to create a token",
			"method", "POST", "url", zServicesURL, "error", err.Error())
		return "", "", grpcStatus.Error(codes.Internal, "Failure creating token")
	}

	httpReq.Header.Add("X-Auth-User", token.user)
	httpReq.Header.Add("X-Auth-Key", token.password)

	httpRsp, err := httpClient.Do(httpReq)
	if err != nil {
		utils.GetLogREST(ctx,2).Println("Token creation failed in Do",
			"url", zServicesURL, "error", err.Error())
		return "", "", grpcStatus.Error(codes.Internal, "Failure creating token")
	}

	defer httpRsp.Body.Close()

	if httpRsp.StatusCode != http.StatusCreated {
		utils.GetLogREST(ctx,2).Println("Token creation failed in ZFSSA",
			"url", zServicesURL, "StatusCode", httpRsp.StatusCode)
		return "", "", grpcStatus.Error(codes.Internal, "Failure creating token")
	}

	return httpRsp.Header.Get("X-Auth-Session"), httpRsp.Header.Get("X-Auth-Name"), nil
}

// Makes a request to a target appliance updating the token if needed.
func MakeRequest(ctx context.Context, token *Token, method, url string, reqbody interface{}, status int,
	rspbody interface{}) (interface{}, int, error) {

	rsp, code, err := makeRequest(ctx, token, method, url, reqbody, status, rspbody)
	if code == http.StatusUnauthorized && err == nil {
		rsp, code, err = makeRequest(ctx, token, method, url, reqbody, status, rspbody)
	}
	return rsp, code, err
}

// Local function makes the actual request to the ZFSSA.
func makeRequest(ctx context.Context, token *Token, method, url string, reqbody interface{}, status int,
	rspbody interface{}) (interface{}, int, error) {

	utils.GetLogREST(ctx,5).Println("MakeRequest to ZFSSA",
		"method", method, "url", url, "body", reqbody)

	xAuthSession, err := getToken(ctx, token, nil)
	if err != nil {
		return nil, 0, err
	}

	reqjson, err := json.Marshal(reqbody)
	if err != nil {
		utils.GetLogREST(ctx,2).Println("json.Marshal call failed",
			"method", method, "url", url, "body", reqbody, "error", err.Error())
		return nil, 0, grpcStatus.Error(codes.Unknown, "json.Marshal call failed")
	}

	reqhttp, err := http.NewRequest(method, url, bytes.NewBuffer(reqjson))
	if err != nil {
		utils.GetLogREST(ctx,2).Println("http.NewRequest call failed",
			"method", method, "url", url, "body", reqbody, "error", err.Error())
		return nil, 0, grpcStatus.Error(codes.Unknown, "http.NewRequest call failed")
	}

	reqhttp.Header.Add("X-Auth-Session", xAuthSession)
	reqhttp.Header.Set("Content-Type", "application/json")
	reqhttp.Header.Set("Accept", "application/json")

	rsphttp, err := httpClient.Do(reqhttp)
	if err != nil {
		utils.GetLogREST(ctx,2).Println("client.do call failed",
			"method", method, "url", url, "error", err.Error())
		return nil, 0, grpcStatus.Error(codes.Unknown, "client.do call failed")
	}

	// when err is nil, response body is always non-nil
	defer rsphttp.Body.Close()

	//d := json.NewDecoder(rsphttp.Body)
	//err = d.Decode(rspbody)

	// read json http response
	rspjson, err := ioutil.ReadAll(rsphttp.Body)
	if err != nil {
		utils.GetLogREST(ctx,2).Println("ioutil.ReadAll call failed",
			"method", method, "url", url, "code", rsphttp.StatusCode,
			"status", rsphttp.Status, "error", err.Error())
		return nil, rsphttp.StatusCode, grpcStatus.Error(codes.Unknown,"ioutil.ReadAll call failed")
	}

	if rsphttp.StatusCode == status {
		if rspbody != nil {
			err = json.Unmarshal(rspjson, rspbody)
			if err != nil {
				utils.GetLogREST(ctx,2).Println("json.Unmarshal call failed",
					"\nmethod", method, "\nurl", url, "\ncode", rsphttp.StatusCode,
					"\nstatus", rsphttp.Status, "\nbody", rspjson, "\nerror", err)
				return nil, rsphttp.StatusCode, grpcStatus.Error(codes.Unknown, "json.Unmarshal call failed")
			}
		}
		utils.GetLogREST(ctx,5).Println("Successful response from ZFSSA",
			"method", method, "url", url, "result", rsphttp.StatusCode)
		return rspbody, rsphttp.StatusCode, nil
	}

	// We check here whether the token may have expired and renew it if needed.
	if rsphttp.StatusCode == http.StatusUnauthorized {
		_, err = getToken(ctx, token, &xAuthSession)
		return nil, http.StatusUnauthorized, err
	}

	// status code was not what the user expected, attempt to unpack
	utils.GetLogREST(ctx,2).Println("MakeRequest to ZFSSA resulted in an unexpected status",
		"method", method, "url", url, "expected", status, "code", rsphttp.StatusCode,
		"status", rsphttp.Status)

	failure := &faultResponse{}
	err = json.Unmarshal(rspjson, failure)
	var responseString string
	if err != nil {
		utils.GetLogREST(ctx,2).Println("Failure from ZFSSA could not be un-marshalled",
			"method", method, "url", url, "code", rsphttp.StatusCode,
			"status", rsphttp.Status, "body", rspjson)
		responseString = string(rspjson)
	} else {
		responseString = failure.Fault.Message
	}

	switch rsphttp.StatusCode {
	case http.StatusNotFound:
		err  = grpcStatus.Errorf(codes.NotFound, "Resource not found on target appliance: %s", responseString)
	default:
		err = grpcStatus.Errorf(codes.Unknown, "Unknown Error Occurred on target appliance: %s", responseString)
	}

	return nil, rsphttp.StatusCode, err
}

type services struct {
	List []Service `json:"services"`
}

type Service struct {
	Version		string	`json:"version"`
	Name		string	`json:"name"`
	URI			string	`json:"uri"`
}

func GetServices(ctx context.Context, token *Token) (*[]Service, error) {

	rspJSON := new(services)
	rspJSON.List = make([]Service, 0)
	_, _, err := MakeRequest(ctx, token, "GET", zServicesURL, nil, http.StatusOK, rspJSON)
	if err != nil {
		return nil, err
	}

	return &rspJSON.List, nil
}

// Unmarshalling of a "List" structure. This structure is the ZFSSA response to
// the http request:
//
//		GET /api/access/v1 HTTP/1.1
//		Host: zfs-storage.example.com
//		X-Auth-User: admin
//		X-Auth-Key: password
//
func (l *services) UnmarshalJSON(b []byte) error {
	return zfssaUnmarshalList(b, &l.List)
}

// Unmarshalling of a List sent by the ZFSSA
//
func zfssaUnmarshalList(b []byte, l interface{}) error {

	// 'b' starts and ends like this:
	// {List:[{...},...,{...}]}
	b = b[0:len(b) - 1]
	for i := 1; i < len(b); i++ {
		if b[i] == '[' {
			b = b[i:]
			break
		}
	}
	// Now 'b' starts and ends like this:
	// [{...},...,{...}]
	err := json.Unmarshal(b, l)
	if err != nil {
		return err
	}

	return nil
}
