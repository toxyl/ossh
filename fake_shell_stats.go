package main

import (
	"fmt"
	"strings"
)

type FakeShellStats struct {
	Host             string
	User             string
	CommandsExecuted uint
	CommandHistory   []string
	recording        *ASCIICastV2
}

func (fss *FakeShellStats) AddCommandToHistory(cmd string) {
	fss.CommandHistory = append(fss.CommandHistory, cmd)
	fss.CommandsExecuted++
}

func (fss *FakeShellStats) ToSha1() string {
	pl := strings.Join(fss.CommandHistory, "\n")
	return StringToSha1(pl)
}

func (fss *FakeShellStats) ToPayload() *Payload {
	pl := strings.Join(fss.CommandHistory, "\n")
	p := NewPayload()
	p.Set(pl)
	p.payload = fss.recording.String()
	return p
}

func (fss *FakeShellStats) SaveCapture() {
	pl := fss.ToPayload()
	f := fmt.Sprintf("%s/ocap-%s-%s.cast", Conf.PathCaptures, fss.Host, pl.hash)

	if !FileExists(f) {
		err := fss.recording.Save(f)
		if err == nil {
			LogFakeShell.Success("Capture saved: %s", colorFile(f))
		}
	}
	SrvOSSH.Loot.AddPayload(pl.hash)
	pl.Save()
}
