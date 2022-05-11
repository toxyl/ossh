package main

import (
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gliderlabs/ssh"
	"golang.org/x/term"
)

const (
	fakeShellInitialWidth  = 80
	fakeShellInitialHeight = 24
)

type FakeShell struct {
	session  ssh.Session
	terminal *term.Terminal
	writer   *SlowWriter
	created  time.Time
	stats    *FakeShellStats
	prompt   string

	cwd       string
	overlayFS *OverlayFS
}

func (fs *FakeShell) User() string {
	return fs.session.User()
}
func (fs *FakeShell) Host() string {
	return strings.Split(fs.session.RemoteAddr().String(), ":")[0]
}

func (fs *FakeShell) Close() {
	_, err := fs.terminal.Write([]byte(""))
	if err != nil {
		if err == io.EOF {
			fs.session.Close()
			return
		} else {
			panic(err)
		}
	}
	fs.session.Close()
}

func (fs *FakeShell) UpdatePrompt(path string) {
	fs.prompt = fmt.Sprintf("%s@%s:%s# ", fs.User(), Conf.HostName, path)
	fs.terminal.SetPrompt(fs.prompt)
}

func (fs *FakeShell) RecordExec(input, output string) {
	fs.stats.recording.AddInputEvent(fs.prompt + input)
	fs.writer.WriteLn(output)
	fs.stats.recording.AddOutputEvent(output)
}

func (fs *FakeShell) RecordWriteLn(output string) {
	fs.writer.WriteLn(output)
	fs.stats.recording.AddOutputEvent(output)
}

func (fs *FakeShell) RecordWrite(output string) {
	fs.writer.Write(output)
	// TODO do we need to record this seperately?
	fs.stats.recording.AddOutputEvent(output)
}

func (fs *FakeShell) Exec(line string) bool {
	fs.stats.AddCommandToHistory(line)

	pieces := strings.Split(line, " ")
	command := pieces[0]
	args := pieces[1:]

	rmt := strings.Split(fs.session.RemoteAddr().String(), ":")
	lcl := strings.Split(fs.session.LocalAddr().String(), ":")
	rmtH := rmt[0]
	lclH := lcl[0]
	rmtP := 22
	lclP := 22
	if i, err := strconv.Atoi(rmt[1]); err == nil {
		rmtP = i
	}
	if i, err := strconv.Atoi(lcl[1]); err == nil {
		lclP = i
	}

	data := struct {
		User      string
		IP        string
		IPLocal   string
		Port      int
		PortLocal int
		HostName  string
		InputRaw  string
		Command   string
		Arguments []string
	}{
		User:      fs.session.User(),
		IP:        rmtH,
		IPLocal:   lclH,
		Port:      rmtP,
		PortLocal: lclP,
		HostName:  Conf.HostName,
		InputRaw:  line,
		Command:   command,
		Arguments: args,
	}

	if SrvSync.HasNode(data.IP) {
		LogFakeShell.Warning("%s, what are you doing here? Go home!", colorConnID(data.User, data.IP, data.Port))
		return true
	}

	LogFakeShell.Debug(
		"%s runs %s %s",
		colorConnID(data.User, data.IP, data.Port),
		colorReason(command),
		colorWrap(strings.Join(args, " "), colorLightBlue),
	)

	// 1) make sure the client waits some time at least,
	//    the more input the more wait time, hehe
	dly := time.Duration(len(line) * int(Conf.InputDelay))
	time.Sleep(dly * time.Millisecond)

	// Ignore just pressing enter with whitespace
	if strings.TrimSpace(line) == "" {
		return false
	}

	// 3) check if command should exit immediately
	for _, cmd := range Conf.Commands.Exit {
		if strings.HasPrefix(line+"  ", cmd+" ") {
			fs.RecordExec(line, GeneratePseudoEmptyString(0)) // just to waste some more time ;)
			return true
		}
	}

	// 3) check if command matches a simple command
	for _, cmd := range Conf.Commands.Simple {
		if strings.HasPrefix(line+"  ", cmd[0]+" ") {
			fs.RecordExec(line, ParseTemplateFromString(cmd[1], data))
			return false
		}
	}

	// 4) check if command should return permission denied error
	for _, cmd := range Conf.Commands.PermissionDenied {
		if strings.HasPrefix(line+"  ", cmd+" ") {
			fs.RecordExec(line, ParseTemplateFromString("{{ .Command }}: permission denied", data))
			return false
		}
	}

	// 5) check if command should return disk i/o error
	for _, cmd := range Conf.Commands.DiskError {
		if strings.HasPrefix(line+"  ", cmd+" ") {
			fs.RecordExec(line, ParseTemplateFromString(GenerateGarbageString(1000)+"\nend_request: I/O error", data))
			return false
		}
	}

	// 6) check if command should return command not found error
	for _, cmd := range Conf.Commands.CommandNotFound {
		if strings.HasPrefix(line+"  ", cmd+" ") {
			fs.RecordExec(line, ParseTemplateFromString("{{ .Command }}: command not found", data))
			return false
		}
	}

	// 7) check if command should return file not found error
	for _, cmd := range Conf.Commands.FileNotFound {
		if strings.HasPrefix(line+"  ", cmd+" ") {
			fs.RecordExec(line, ParseTemplateFromString("\"{{ .Command }}\": No such file or directory (os error 2)", data))
			return false
		}
	}

	// 8) check if command should return not implemented error
	for _, cmd := range Conf.Commands.NotImplemented {
		if strings.HasPrefix(line+" ", cmd+" ") {
			fs.RecordExec(line, ParseTemplateFromString("{{ .Command }}: Function not implemented", data))
			return false
		}
	}

	// 9) check if command should return bullshit
	for _, cmd := range Conf.Commands.Bullshit {
		if strings.HasPrefix(line+" ", cmd+" ") {
			fs.RecordExec(line, GenerateGarbageString(1000))
			return false
		}
	}

	instr := strings.TrimSpace(line)
	instrCmd := strings.Split(instr, " ")[0]

	// 10) check if there is a go-implemented command for this
	if goCmd, found := CmdLookup[instrCmd]; found {
		return goCmd(fs, instr)
	}

	// 11) check if we have a template for the command
	fs.RecordExec(line, ParseTemplateToString(command, data))
	return false
}

func (fs *FakeShell) HandleInput() {
	for {
		line, err := fs.terminal.ReadLine()
		if err != nil {
			if err == io.EOF {
				break
			} else {
				panic(err)
			}
		}

		// execute all rewriters
		for _, rw := range Conf.Commands.Rewriters {
			re := regexp.MustCompile(rw[0])
			if re.MatchString(line) {
				line = re.ReplaceAllString(line, rw[1])
			}
		}

		lines := strings.Split(line, "\n")
		mustExit := false
		for _, ln := range lines {
			if fs.Exec(ln) {
				mustExit = true
				break
			}
		}
		if mustExit {
			break
		}
	}
}

func (fs *FakeShell) Process() *FakeShellStats {
	if fs.session.RawCommand() != "" {
		// this means the client passed a command along (e.g. with -t/-tt param),
		// let's run it and then close the connection.
		raw := fs.session.RawCommand()

		// execute all rewriters
		for _, rw := range Conf.Commands.Rewriters {
			re := regexp.MustCompile(rw[0])
			if re.MatchString(raw) {
				raw = re.ReplaceAllString(raw, rw[1])
			}
		}

		commands := strings.Split(raw, "\n")
		host, port, _ := net.SplitHostPort(fs.session.RemoteAddr().String())
		LogFakeShell.Info(
			"%s wants to run %s commands",
			colorConnID(fs.User(), host, StringToInt(port, 0)),
			colorInt(len(commands)),
		)
		for _, cmd := range commands {
			if fs.Exec(cmd) {
				break
			}
		}
	} else {
		fs.HandleInput()
	}
	fs.Close()
	return fs.stats
}

func NewFakeShell(s ssh.Session, overlay *OverlayFS) *FakeShell {
	fs := &FakeShell{
		session:  s,
		terminal: nil,
		writer:   nil,
		created:  time.Now(),
		stats: &FakeShellStats{
			CommandsExecuted: 0,
			CommandHistory:   []string{},
			Host:             "",
			User:             s.User(),
			recording:        NewASCIICastV2(fakeShellInitialWidth, fakeShellInitialHeight),
		},
		overlayFS: overlay,
	}

	fs.terminal = term.NewTerminal(s, "")
	fs.writer = NewSlowWriter(fs.terminal)
	fs.stats.Host = fs.Host()

	if overlay != nil {
		if !overlay.DirExists("/home") {
			_ = overlay.Mkdir("/home", 700)
		}

		if !overlay.DirExists("/home/" + s.User()) {
			_ = overlay.Mkdir("/home/"+s.User(), 700)
		}
	}
	fs.cwd = "/home/" + s.User()

	fs.UpdatePrompt("~")
	return fs
}
