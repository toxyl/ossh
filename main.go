package main

import "time"

var Server *OSSHServer
var WebServer *UIServer
var startTime time.Time

func main() {
	startTime = time.Now()
	initConfig()
	Server = NewOSSHServer()
	WebServer = NewUIServer()
	go func() {
		WebServer.Start()
	}()
	Server.Start()
}

func uptime() time.Duration {
	return time.Since(startTime)
}
