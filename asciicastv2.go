package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// {"version": 2, "width": 80, "height": 24, "timestamp": 1504467315, "title": "Demo", "env": {"TERM": "xterm-256color", "SHELL": "/bin/zsh"}}
// color format: #rrggbb

type ASCIICastV2Theme struct {
	FG      string `json:"fg"`      // text color
	BG      string `json:"bg"`      // background color
	Palette string `json:"palette"` // list of 8 or 16 colors, separated by colon character
}

type ASCIICastV2Header struct {
	Version       int               `json:"version"`                   // (required) must be 2
	Width         int               `json:"width"`                     // (required) in columns
	Height        int               `json:"height"`                    // (required) in rows
	Timestamp     int               `json:"timestamp,omitempty"`       // (optional) unix epoch
	Duration      float64           `json:"duration,omitempty"`        // (optional) in seconds
	IdleTimeLimit float64           `json:"idle_time_limit,omitempty"` // (optional) in seconds
	Command       string            `json:"command,omitempty"`         // (optional) name of the command that was recorded
	Title         string            `json:"title,omitempty"`           // (optional) name of the asciicast
	Env           map[string]string `json:"env,omitempty"`             // (optional) key-value pair
	Theme         ASCIICastV2Theme  `json:"theme,omitempty"`           // (optional) color scheme of recorded terminal
}

func (ac2h *ASCIICastV2Header) String() string {
	json, err := json.Marshal(ac2h)
	if err != nil {
		LogASCIICastV2.Error("Could not marshal ASCIICastV2Header: %s", colorError(err))
		return ""
	}

	return string(json)
}

type ASCIICastV2Event struct {
	Time float64
	Type string // either "o" (stdout) or "i" (stdin)
	Data string // UTF-8 encoded JSON string
}

func (ac2e *ASCIICastV2Event) String() string {
	if ac2e.Type != "o" && ac2e.Type != "i" {
		LogASCIICastV2.Error(
			"Could not convert ASCIICastV2Event to string, type '%s' is unknown.",
			colorWrap(ac2e.Type, colorOrange),
		)
		return ""
	}

	json, err := json.Marshal([]any{
		ac2e.Time,
		ac2e.Type,
		ac2e.Data,
	})
	if err != nil {
		LogASCIICastV2.Error("Could not marshal ASCIICastV2Event data: %s", colorError(err))
		return ""
	}

	return string(json)
}

type ASCIICastV2 struct {
	Header      ASCIICastV2Header
	EventStream []ASCIICastV2Event
}

func (ac2 *ASCIICastV2) addEventRaw(eventtype, data string, time float64) {
	ac2.EventStream = append(ac2.EventStream, ASCIICastV2Event{
		Time: time,
		Type: eventtype,
		Data: data,
	})
}

func (ac2 *ASCIICastV2) addEvent(eventtype, data string) {
	timeStart := time.Unix(int64(ac2.Header.Timestamp), 0)
	timeNow := time.Now()
	timeSinceStart := timeNow.Sub(timeStart)
	secondsSinceStart := timeSinceStart.Seconds()

	ac2.Header.Duration = secondsSinceStart
	ac2.addEventRaw(eventtype, data, secondsSinceStart)
}

func (ac2 *ASCIICastV2) AddInputEvent(data string) {
	// asciinema records an event for every character of input
	// and stores one input event _and_ one output event for it.
	// the input event sequence is concluded with a \r and
	// the output event sequence with \r\n\u001b[?2004l\r.
	// for simplicity (no need to emulate typing, maybe later?)
	// we make only two events from that
	ac2.addEvent("i", fmt.Sprintf("%s\r", data))
	ac2.addEvent("o", fmt.Sprintf("%s\r\n\u001b[?2004l\r", data))
}

func (ac2 *ASCIICastV2) AddOutputEvent(data string) {
	ac2.addEvent("o", fmt.Sprintf("%s\r\n\u001b[?2004l\r", data))
}

func (ac2 *ASCIICastV2) String() string {
	output := []string{ac2.Header.String()}
	for _, e := range ac2.EventStream {
		output = append(output, e.String())
	}
	return strings.Join(output, "\n")
}

func (ac2 *ASCIICastV2) Save(file string) error {
	data := ac2.String()
	err := os.WriteFile(file, []byte(data), 0744)
	if err != nil {
		return err
	}
	return nil
}

func (ac2 *ASCIICastV2) Load(file string) {
	data, err := os.ReadFile(file)
	if err != nil {
		LogASCIICastV2.Error(
			"Could not load ASCIICastV2 from file '%s': %s",
			colorFile(file),
			colorError(err),
		)
		return
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > 0 {
		meta := lines[0]
		lines = lines[1:]
		err = json.Unmarshal([]byte(meta), &ac2.Header)
		if err != nil {
			LogASCIICastV2.Error(
				"Could not unmarshal ASCIICastV2Header from file '%s': %s",
				colorFile(file),
				colorError(err),
			)
			return
		}
		ac2.EventStream = []ASCIICastV2Event{}
		for _, line := range lines {
			var ed []any
			err := json.Unmarshal([]byte(line), &ed)

			if err != nil {
				LogASCIICastV2.Error(
					"Could not unmarshal ASCIICastV2Event from file '%s': %s (input was: '%s')",
					colorFile(file),
					colorError(err),
					colorHighlight(line),
				)
				continue
			}

			ac2.addEventRaw(ed[1].(string), ed[2].(string), ed[0].(float64))
		}
	}
}

func NewASCIICastV2(width int, height int) *ASCIICastV2 {
	ac2 := &ASCIICastV2{
		Header: ASCIICastV2Header{
			Version:   2,
			Width:     width,
			Height:    height,
			Timestamp: int(time.Now().Unix()),
			Duration:  0,
		},
		EventStream: []ASCIICastV2Event{},
	}
	return ac2
}

func OpenASCIICastV2(file string) *ASCIICastV2 {
	ac2 := NewASCIICastV2(fakeShellInitialWidth, fakeShellInitialHeight)
	ac2.Load(file)
	return ac2
}
