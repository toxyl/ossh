package main

var Server *OSSHServer
var WebServer *WebinterfaceServer

func main() {
	initConfig()
	WebServer = NewWebinterfaceServer(Conf.Webinterface.Host, int(Conf.Webinterface.Port), Conf.Webinterface.CertFile, Conf.Webinterface.KeyFile)
	go func() {
		WebServer.Start()
	}()
	Server = NewOSSHServer()
	Server.Start()
}
