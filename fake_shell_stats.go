package main

type FakeShellStats struct {
	Host             string
	User             string
	TimeSpent        uint
	CommandsExecuted uint
	CommandHistory   []string
}
