/*
 * Copyright (c) 2021, Oracle and/or its affiliates.
 * Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
*/

package main

import (
	"github.com/oracle/zfssa-csi-driver/pkg/service"
	"flag"
	"fmt"
	"os"
)

var (
	driverName		= flag.String("drivername", "zfssa-csi-driver", "name of the driver")
	// Provided by the build process
	version			= "0.0.0"
)

func main() {

	zd, err := service.NewZFSSADriver(*driverName, version)
	if err != nil {
		fmt.Print(err)
	} else {
		zd.Run()
	}
	os.Exit(1)
}
