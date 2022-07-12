package main

import (
	"fmt"

	"github.com/toxyl/glog"
)

func colorConnID(user, host string, port int) string {
	addr := glog.AddrHostPort(host, port, true)
	if user == "" {
		return addr
	}
	return fmt.Sprintf("%s > %s", addr, glog.Wrap(user, glog.Green))
}

func logMessageHandler(msg string) {
	fmt.Print(msg)
	if SrvUI != nil {
		SrvUI.PushLog(msg)
	}
}

var (
	LogGlobal        = glog.NewLogger("Global", glog.Gray, false, false, false, logMessageHandler)
	LogASCIICastV2   = glog.NewLogger("ASCIICast v2", glog.BrightYellow, false, false, false, logMessageHandler)
	LogFakeShell     = glog.NewLogger("Fake Shell", glog.OliveGreen, false, false, false, logMessageHandler)
	LogOverlayFS     = glog.NewLogger("Overlay FS", glog.LightBlue, false, false, false, logMessageHandler)
	LogOSSHServer    = glog.NewLogger("oSSH Server", glog.Lime, false, false, false, logMessageHandler)
	LogSessions      = glog.NewLogger("Sessions", glog.DarkOrange, false, false, false, logMessageHandler)
	LogSyncClient    = glog.NewLogger("Sync Client", glog.Blue, false, false, false, logMessageHandler)
	LogSyncCommands  = glog.NewLogger("Sync Commands", glog.DarkGreen, false, false, false, logMessageHandler)
	LogSyncServer    = glog.NewLogger("Sync Server", glog.DarkRed, false, false, false, logMessageHandler)
	LogTextTemplater = glog.NewLogger("Text Templater", glog.MediumGray, false, false, false, logMessageHandler)
	LogUIServer      = glog.NewLogger("UI Server", glog.Cyan, false, false, false, logMessageHandler)
)
