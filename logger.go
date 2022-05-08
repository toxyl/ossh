package main

import (
	"fmt"
	"time"
	"strings"
)

func colorWrap(str string, color uint) string {
	return fmt.Sprintf("\033[38;5;%dm%s\033[0m", color, str)
}

func colorUser(user string) string {
	return colorWrap(user, colorGreen)
}

func colorHost(host string) string {
	return colorWrap(host, colorBrightYellow)
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
	// #005fff
	colorLightBlue = 27
	// #00af00
	colorOliveGreen = 34
	// #00ff00
	colorGreen = 46
	// #00ffff
	colorCyan = 51
	// #d70000
	colorDarkRed = 160
	// #ff0000
	colorRed = 196
	// #ff8700
	colorOrange = 208
	// #ffffaf
	colorBrightYellow = 229
	// #bcbcbc
	colorGray = 250
)

func Log(indicator rune, format string, a ...interface{}) {
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
	case ' ':
		prefix = colorWrap("[ ]", colorGray)
	}
	msg := fmt.Sprintf(prefix+" "+format, a...)

	fmt.Print(msg)
	if SrvUI != nil {
		SrvUI.PushLog(msg)
	}
}

func LogDefault(format string, a ...interface{}) {
	Log(' ', format, a...)
}

func LogDefaultLn(format string, a ...interface{}) {
	LogDefault(fmt.Sprintf("%s\n", format), a...)
}

func LogInfo(format string, a ...interface{}) {
	Log('i', format, a...)
}

func LogInfoLn(format string, a ...interface{}) {
	LogInfo(fmt.Sprintf("%s\n", format), a...)
}

func LogSuccess(format string, a ...interface{}) {
	Log('✓', format, a...)
}

func LogSuccessLn(format string, a ...interface{}) {
	LogSuccess(fmt.Sprintf("%s\n", format), a...)
}

func LogOK(format string, a ...interface{}) {
	Log('+', format, a...)
}

func LogOKLn(format string, a ...interface{}) {
	LogOK(fmt.Sprintf("%s\n", format), a...)
}

func LogNotOK(format string, a ...interface{}) {
	Log('-', format, a...)
}

func LogNotOKLn(format string, a ...interface{}) {
	LogNotOK(fmt.Sprintf("%s\n", format), a...)
}

func LogError(format string, a ...interface{}) {
	Log('x', format, a...)
}

func LogErrorLn(format string, a ...interface{}) {
	LogError(fmt.Sprintf("%s\n", format), a...)
}

func LogWarning(format string, a ...interface{}) {
	Log('!', format, a...)
}

func LogWarningLn(format string, a ...interface{}) {
	LogWarning(fmt.Sprintf("%s\n", format), a...)
}
