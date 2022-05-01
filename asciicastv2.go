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
type ASCIICastV2Header struct {
	Version       int               `json:"version"`         // (required) must be 2
	Width         int               `json:"width"`           // (required) in columns
	Height        int               `json:"height"`          // (required) in rows
	Timestamp     int               `json:"timestamp"`       // (optional) unix epoch
	Duration      float64           `json:"duration"`        // (optional) in seconds
	IdleTimeLimit float64           `json:"idle_time_limit"` // (optional) "This should be used by an asciicast player to reduce all terminal inactivity (delays between frames) to maximum of idle_time_limit value."
	Command       string            `json:"command"`         // (optional) name of the command that was recorded
	Title         string            `json:"title"`           // (optional) name of the asciicast
	Env           map[string]string `json:"env"`             // (optional) key-value pair
	Theme         struct {          // (optional) color scheme of recorded terminal
		FG      string `json:"fg"`      // normal text color
		BG      string `json:"bg"`      // normal background color
		Palette string `json:"palette"` // list of 8 or 16 colors, separated by colon character
	} `json:"theme"`
}

func (ac2h *ASCIICastV2Header) String() string {
	json, err := json.Marshal(ac2h)
	if err != nil {
		Log('x', "Could not marshal ASCIICastV2Header: %s\n", err.Error())
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
		Log(
			'x',
			"Could not convert ASCIICastV2Event to string, type '%s' is unknown.\n",
			colorWrap(ac2e.Type, colorOrange),
		)
		return ""
	}

	json, err := json.Marshal(ac2e.Data)
	if err != nil {
		Log('x', "Could not marshal ASCIICastV2Event data: %s\n", err.Error())
		return ""
	}

	return fmt.Sprintf("[%f, \"%s\", %s]", ac2e.Time, ac2e.Type, json)
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
	t := float64(int(time.Now().Unix()) - ac2.Header.Timestamp)
	ac2.Header.Duration = t
	ac2.addEventRaw(eventtype, data, t)
}

func (ac2 *ASCIICastV2) AddInputEvent(data string) {
	ac2.addEvent("i", data)
}

func (ac2 *ASCIICastV2) AddOutputEvent(data string) {
	ac2.addEvent("o", data)
}

func (ac2 *ASCIICastV2) String() string {
	output := []string{ac2.Header.String()}
	for _, e := range ac2.EventStream {
		output = append(output, e.String())
	}
	return strings.Join(output, "\n")
}

func (ac2 *ASCIICastV2) Save(file string) {
	data := ac2.String()
	err := os.WriteFile(file, []byte(data), 0755)
	if err != nil {
		Log(
			'x',
			"Could not save ASCIICastV2 to file '%s': %s\n",
			colorWrap(file, colorOrange),
			colorWrap(err.Error(), colorRed),
		)
	}
}

func (ac2 *ASCIICastV2) Load(file string) {
	data, err := os.ReadFile(file)
	if err != nil {
		Log(
			'x',
			"Could not load ASCIICastV2 from file '%s': %s\n",
			colorWrap(file, colorOrange),
			colorWrap(err.Error(), colorRed),
		)
		return
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > 0 {
		meta := lines[0]
		lines = lines[1:]
		err = json.Unmarshal([]byte(meta), &ac2.Header)
		if err != nil {
			Log(
				'x',
				"Could not unmarshal ASCIICastV2Header from file '%s': %s\n",
				colorWrap(file, colorOrange),
				colorWrap(err.Error(), colorRed),
			)
			return
		}
		ac2.EventStream = []ASCIICastV2Event{}
		for _, line := range lines {
			var ed []interface{}
			err := json.Unmarshal([]byte(line), &ed)

			if err != nil {
				Log(
					'x',
					"Could not unmarshal ASCIICastV2Event from file '%s': %s (input was: '%s')\n",
					colorWrap(file, colorOrange),
					colorWrap(err.Error(), colorRed),
					colorWrap(line, colorCyan),
				)
				continue
			}

			ac2.addEventRaw(ed[1].(string), ed[2].(string), ed[0].(float64))
		}
	}
}

func NewASCIICastV2(width int, height int, title, command string) *ASCIICastV2 {
	ac2 := &ASCIICastV2{
		Header: ASCIICastV2Header{
			Version:       2,
			Width:         width,
			Height:        height,
			Timestamp:     int(time.Now().Unix()),
			Duration:      0,
			IdleTimeLimit: 0, // TODO: not yet sure what a good default is
			Command:       command,
			Title:         title,
			Env:           map[string]string{},
			Theme: struct {
				FG      string "json:\"fg\""
				BG      string "json:\"bg\""
				Palette string "json:\"palette\""
			}{},
		},
		EventStream: []ASCIICastV2Event{},
	}
	return ac2
}
