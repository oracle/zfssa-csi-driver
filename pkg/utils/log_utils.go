/*
 * Copyright (c) 2021, Oracle and/or its affiliates.
 * Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
 */

package utils

import (
	"flag"
	"fmt"
	"golang.org/x/net/context"
	"io/ioutil"
	"k8s.io/klog/v2"
	"log"
	"os"
	"strconv"
	"sync/atomic"
)

const (
	CSID int = iota // CSI Driver
	CTRL            // Controller Service
	NODE            // Node Service
	IDTY            // Identity Service
	REST            // REST interface
	UTIL            // Utilities that may be controller or node
	SENTINEL
)

const MAX_LEVEL int = 5

type zLogger struct {
	prefix string
	logger [MAX_LEVEL]*log.Logger
}

// Type of the key being used to add the array of loggers to the context.
type zLoggersKey string

var (
	reqCounter    uint64
	logLevelStr   string
	logLevel      int
	loggersPrefix [SENTINEL]string
	loggersTable  [MAX_LEVEL]*log.Logger
	loggersKey    zLoggersKey = "zloggers"
	loggerNOP     *log.Logger
)

// Log service initialization. The original loggers are created with a prefix identifying the
// service (service, controller, node, indentifier, REST interface and utility), the node,
// the driver and its version. These loggers will be clone for each request.
func InitLogs(level, driverName, version, nodeID string) {

	klog.InitFlags(nil)

	logLevelStr = level
	logLevel, _ = strconv.Atoi(level)
	reqCounter = 0

	_ = flag.Set("logtostderr", "true")
	_ = flag.Set("v", logLevelStr)

	loggersPrefix[CSID] = nodeID + "/" + driverName + "/" + version + "\n\t"
	loggersPrefix[CTRL] = nodeID + "/" + driverName + "/" + version + "\n\t"
	loggersPrefix[NODE] = nodeID + "/" + driverName + "/" + version + "\n\t"
	loggersPrefix[IDTY] = nodeID + "/" + driverName + "/" + version + "\n\t"
	loggersPrefix[REST] = nodeID + "/" + driverName + "/" + version + "\n\t"
	loggersPrefix[UTIL] = nodeID + "/" + driverName + "/" + version + "\n\t"

	for i := 0; i < MAX_LEVEL; i++ {
		loggersTable[i] = log.New(os.Stdout, loggersPrefix[CSID]+"***  ",
			log.Lmsgprefix|log.Ldate|log.Ltime|log.Lshortfile)
	}

	loggerNOP = log.New(ioutil.Discard, "NOP logger", log.LstdFlags)
}

// Creates a new context by duplicating the context passed in and adding a array of loggers
// with a value added. The value is unique and is generated in this function. That value will
// be systematically displayed each time any of the loggers in the array are called.
func GetNewContext(ctx context.Context) context.Context {

	loggers := new([SENTINEL]zLogger)
	reqNum := fmt.Sprintf("%d", atomic.AddUint64(&reqCounter, 1))

	for i := 0; i < SENTINEL; i++ {
		loggers[i].prefix = loggersPrefix[i] + reqNum + "  "
		for j := 0; j < MAX_LEVEL; j++ {
			loggers[i].logger[j] = nil
		}
	}

	return context.WithValue(ctx, loggersKey, loggers)
}

// Return the appropriate logger based on the service and level provided.
func getLogger(ctx context.Context, sel int, level int) *log.Logger {

	if level > logLevel {
		return loggerNOP
	}

	level--

	if ctx != nil {
		loggers := ctx.Value(loggersKey).(*[SENTINEL]zLogger)
		if loggers == nil {
			panic("context without loggers")
		}
		if loggers[sel].logger[level] == nil {
			// No logger has yet been created for this level
			loggers[sel].logger[level] = log.New(os.Stdout, loggers[sel].prefix,
				log.Lmsgprefix|log.Ldate|log.Ltime|log.Lshortfile)
		}
		return loggers[sel].logger[level]
	}

	return loggersTable[level]
}

// Public function returning the appropriate logger.
func GetLogCSID(ctx context.Context, level int) *log.Logger { return getLogger(ctx, CSID, level) }
func GetLogCTRL(ctx context.Context, level int) *log.Logger { return getLogger(ctx, CTRL, level) }
func GetLogNODE(ctx context.Context, level int) *log.Logger { return getLogger(ctx, NODE, level) }
func GetLogIDTY(ctx context.Context, level int) *log.Logger { return getLogger(ctx, IDTY, level) }
func GetLogREST(ctx context.Context, level int) *log.Logger { return getLogger(ctx, REST, level) }
func GetLogUTIL(ctx context.Context, level int) *log.Logger { return getLogger(ctx, UTIL, level) }
