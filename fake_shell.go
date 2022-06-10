package main

import (
	"fmt"
	"io"
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

// WriteBinary writes a binary value.
// Unlike the Record* functions it does not record this in the session capture.
func (fs *FakeShell) WriteBinary(val int) {
	fs.writer.Write(string(val))
}

// WriteBinary writes a binary value and sends it.
// Unlike the Record* functions it does not record this in the session capture.
func (fs *FakeShell) WriteBinaryLn(val int) {
	fs.writer.WriteLn(string(val))
}

// ReadBytes reads and returns a byte array with the given number of bytes from the SSH session.
func (fs *FakeShell) ReadBytes(numBytes int) ([]byte, error) {
	b := make([]byte, numBytes)
	_, err := fs.session.Read(b)
	return b, err
}

// ReadString reads and returns a string with the given length from the SSH session.
func (fs *FakeShell) ReadString(length int) (string, error) {
	b, err := fs.ReadBytes(length)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ReadBytesUntil reads from the SSH session until it encounters the separator byte.
// The separator byte will be discarded and the read bytes will be returned as byte array.
func (fs *FakeShell) ReadBytesUntil(sep byte) ([]byte, error) {
	bytes := []byte{}
	for {
		b, err := fs.ReadBytes(1)
		if err != nil {
			return nil, err
		}
		if b[0] != sep {
			bytes = append(bytes, b[0])
		} else {
			break
		}
	}
	return bytes, nil
}

func (fs *FakeShell) Exec(line string, s *Session, iSeq, lSeq int) bool {
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
	cmd := fmt.Sprintf("%s %s", colorReason(command), colorWrap(strings.Join(args, " "), colorLightBlue))
	if lSeq > 1 {
		cmd = fmt.Sprintf("(%s/%s) %s", colorInt(iSeq), colorInt(lSeq), cmd)
	}
	LogFakeShell.Info("%s @ %s: %s", colorConnID(data.User, data.IP, data.Port), colorDuration(uint(s.ActiveFor().Seconds())), cmd)
	s.lock.Lock()
	s.UpdateActivity()
	s.lock.Unlock()
	defer func() {
		s.lock.Lock()
		s.UpdateActivity()
		s.lock.Unlock()
	}()

	if !s.Whitelisted {
		// 1) make sure the client waits some time at least,
		//    the more input the more wait time, hehe
		dly := time.Duration(len(line) * int(Conf.InputDelay))
		time.Sleep(dly * time.Millisecond)
	}

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

func (fs *FakeShell) HandleInput(s *Session) {
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
		for i, ln := range lines {
			if fs.Exec(ln, s, i+1, len(lines)) {
				mustExit = true
				break
			}
		}
		if mustExit {
			break
		}
	}
}

func (fs *FakeShell) Process(s *Session) *FakeShellStats {
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
		for i, cmd := range commands {
			if fs.Exec(cmd, s, i+1, len(commands)) {
				break
			}
		}
	} else {
		fs.HandleInput(s)
	}
	fs.Close()
	return fs.stats
}

func NewFakeShell(s ssh.Session, overlay *OverlayFS, useSlowWriter bool) *FakeShell {
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
	if !useSlowWriter {
		fs.writer.ratelimit = 10000 // set ridicuously high to effectively disable rate limit
	}
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
