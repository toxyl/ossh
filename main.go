package main

import (
	"os"
	"time"
)

var SrvOSSH *OSSHServer
var SrvUI *UIServer
var SrvSync *SyncServer
var SrvMetrics *MetricsServer

var startTime time.Time

func main() {
	args := os.Args

	if len(args) == 2 {
		cfgFile = args[1]
	}

	startTime = time.Now()
	initConfig()
	SrvMetrics = NewMetricsServer()
	SrvOSSH = NewOSSHServer()
	SrvUI = NewUIServer()
	SrvSync = NewSyncServer()
	SrvMetrics.Start()
	SrvSync.Start()
	SrvUI.Start()
	SrvOSSH.Start()
}

func uptime() time.Duration {
	return time.Since(startTime)
}
