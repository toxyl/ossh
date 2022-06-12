package main

import (
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

func (fss *FakeShellStats) ToPayload() *Payload {
	pl := strings.Join(fss.CommandHistory, "\n")
	p := NewPayload()
	p.Set(pl)
	p.payload = fss.recording.String()
	return p
}
