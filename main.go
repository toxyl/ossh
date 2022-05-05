package main

var Server *OSSHServer
var WebServer *UIServer

func main() {
	initConfig()
	Server = NewOSSHServer()
	WebServer = NewUIServer()
	go func() {
		WebServer.Start()
	}()
	Server.Start()
}
