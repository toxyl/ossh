package main

import (
	"fmt"
	"strings"
	"time"
)

func colorConnID(user, host string, port int) string {
	whitelisted := isIPWhitelisted(host)
	host = colorHost(host)
	if whitelisted {
		host = fmt.Sprintf("(whitelisted) %s", host)
	}

	if user == "" {
		return fmt.Sprintf("%s:%s", host, colorPort(port))
	}
	return fmt.Sprintf("%s:%s > %s", host, colorPort(port), colorUser(user))
}

func colorWrap(str string, color uint) string {
	return fmt.Sprintf("\033[38;5;%dm%s\033[0m", color, str)
}

func colorUser(user string) string {
	return colorWrap(user, colorGreen)
}

func colorPort(port int) string {
	// 94 - 231 (137 total)
	return colorWrap(fmt.Sprint(port), uint(94.0+137.0*(float64(port)/65535.0)))
}

func colorHost(host string) string {
	parts := strings.Split(host, ".")
	pt := 0.0
	for _, p := range parts {
		f, _ := GetFloat(p)
		pt += f
	}
	// 88 - 231 (143 total)
	return colorWrap(host, uint(88.0+143.0*(pt/4.0/255.0)))
}

func colorPassword(password string) string {
	return colorWrap(password, colorGreen)
}

func colorError(err error) string {
	return colorWrap(err.Error(), colorOrange)
}

func colorReason(reason string) string {
	return colorWrap(reason, colorOrange)
}

func colorFile(file string) string {
	return colorWrap(file, colorLightBlue)
}

func colorHighlight(message string) string {
	return colorWrap(message, colorCyan)
}

func colorDuration(seconds uint) string {
	return colorWrap(time.Duration(seconds*uint(time.Second)).String(), colorCyan)
}

func colorInt(n int) string {
	return colorWrap(fmt.Sprintf("%d", n), colorCyan)
}

const (
	colorDarkBlue     = 17
	colorBlue         = 21
	colorDarkGreen    = 22
	colorLightBlue    = 27
	colorOliveGreen   = 34
	colorGreen        = 46
	colorCyan         = 51
	colorPurple       = 53
	colorDarkOrange   = 130
	colorDarkYellow   = 142
	colorLime         = 154
	colorDarkRed      = 160
	colorRed          = 196
	colorPink         = 201
	colorOrange       = 208
	colorYellow       = 220
	colorBrightYellow = 229
	colorDarkGray     = 234
	colorMediumGray   = 240
	colorGray         = 250
)

type Logger struct {
	ID    string
	color uint
	debug bool
}

func (ssl *Logger) write(indicator rune, format string, a ...interface{}) {
	prefix := "[ ]"
	switch indicator {
	case 'i':
		prefix = colorWrap("[i]", colorLightBlue)
	case '+':
		prefix = colorWrap("[+]", colorOliveGreen)
	case '✓':
		prefix = colorWrap("[✓]", colorGreen)
	case '-':
		prefix = colorWrap("[-]", colorDarkRed)
	case 'x':
		prefix = colorWrap("[x]", colorRed)
	case '!':
		prefix = colorWrap("[!]", colorOrange)
	case 'd':
		prefix = colorWrap("[D]", colorOrange)
	case ' ':
		prefix = colorWrap("[ ]", colorGray)
	}
	msg := fmt.Sprintf(prefix+" "+format+"\n", a...)

	fmt.Print(msg)
	if SrvUI != nil {
		SrvUI.PushLog(msg)
	}
}

func (ssl *Logger) prependFormat(format string) string {
	return fmt.Sprintf("%s: %s\n", colorWrap(fmt.Sprintf("%-16s", ssl.ID), ssl.color), format)
}

func (ssl *Logger) Default(format string, a ...interface{}) {
	ssl.write(' ', ssl.prependFormat(format), a...)
}

func (ssl *Logger) Info(format string, a ...interface{}) {
	ssl.write('i', ssl.prependFormat(format), a...)
}

func (ssl *Logger) Success(format string, a ...interface{}) {
	ssl.write('✓', ssl.prependFormat(format), a...)
}

func (ssl *Logger) OK(format string, a ...interface{}) {
	ssl.write('+', ssl.prependFormat(format), a...)
}

func (ssl *Logger) NotOK(format string, a ...interface{}) {
	ssl.write('-', ssl.prependFormat(format), a...)
}

func (ssl *Logger) Error(format string, a ...interface{}) {
	ssl.write('x', ssl.prependFormat(format), a...)
}

func (ssl *Logger) Warning(format string, a ...interface{}) {
	ssl.write('!', ssl.prependFormat(format), a...)
}

func (ssl *Logger) Debug(format string, a ...interface{}) {
	if !ssl.debug {
		return
	}
	ssl.write('d', ssl.prependFormat(format), a...)
}

func (ssl *Logger) EnableDebug() {
	ssl.debug = true
}

func NewLogger(id string, color uint) *Logger {
	return &Logger{
		ID:    id,
		color: color,
		debug: false,
	}
}

var (
	LogGlobal        = NewLogger("Global", colorGray)
	LogASCIICastV2   = NewLogger("ASCIICast v2", colorBrightYellow)
	LogFakeShell     = NewLogger("Fake Shell", colorOliveGreen)
	LogOverlayFS     = NewLogger("Overlay FS", colorLightBlue)
	LogPayloads      = NewLogger("Payloads", colorDarkYellow)
	LogOSSHServer    = NewLogger("oSSH Server", colorLime)
	LogSessions      = NewLogger("Sessions", colorDarkOrange)
	LogSyncClient    = NewLogger("Sync Client", colorBlue)
	LogSyncCommands  = NewLogger("Sync Commands", colorDarkGreen)
	LogSyncServer    = NewLogger("Sync Server", colorDarkRed)
	LogHTMLTemplater = NewLogger("HTML Templater", colorMediumGray)
	LogTextTemplater = NewLogger("Text Templater", colorMediumGray)
	LogUIServer      = NewLogger("UI Server", colorCyan)
)
