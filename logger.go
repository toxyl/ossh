package main

import "fmt"

func colorWrap(str string, color uint) string {
	return fmt.Sprintf("\033[38;5;%dm%s\033[0m", color, str)
}

func Log(indicator rune, format string, a ...interface{}) {
	prefix := "[ ]"
	switch indicator {
	case 'i':
		prefix = colorWrap("[i]", 27)
	case '+':
		prefix = colorWrap("[+]", 34)
	case '✓':
		prefix = colorWrap("[✓]", 46)
	case '-':
		prefix = colorWrap("[-]", 160)
	case 'x':
		prefix = colorWrap("[x]", 196)
	case '!':
		prefix = colorWrap("[!]", 208)
	case ' ':
		prefix = colorWrap("[ ]", 250)
	}
	fmt.Printf(prefix+" "+format, a...)
}
