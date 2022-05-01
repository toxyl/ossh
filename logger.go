package main

import "fmt"

func colorWrap(str string, color uint) string {
	return fmt.Sprintf("\033[38;5;%dm%s\033[0m", color, str)
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
	fmt.Printf(prefix+" "+format, a...)
}

func LogDefault(format string, a ...interface{}) {
	Log(' ', format, a...)
}

func LogInfo(format string, a ...interface{}) {
	Log('i', format, a...)
}

func LogSuccess(format string, a ...interface{}) {
	Log('✓', format, a...)
}

func LogOK(format string, a ...interface{}) {
	Log('+', format, a...)
}

func LogNotOK(format string, a ...interface{}) {
	Log('-', format, a...)
}

func LogError(format string, a ...interface{}) {
	Log('x', format, a...)
}

func LogWarning(format string, a ...interface{}) {
	Log('!', format, a...)
}
