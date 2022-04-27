package main

var Server *OSSHServer

func main() {
	initConfig()
	Server = NewOSSHServer()
	Server.Start()
}
