package main

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/toxyl/glog"
	"github.com/toxyl/gutils"
	"github.com/toxyl/ossh/utils"
	"golang.org/x/term"
)

const (
	fakeShellInitialWidth  = 80
	fakeShellInitialHeight = 24
)

type FakeShell struct {
	session   *ssh.Session
	terminal  *term.Terminal
	writer    *utils.SlowWriter
	created   time.Time
	stats     *FakeShellStats
	prompt    string
	cwd       string
	logger    *glog.Logger
	overlayFS *OverlayFS
}

func (fs *FakeShell) SetOverlayFS(ofs *OverlayFS) {
	fs.overlayFS = ofs
}

func (fs *FakeShell) User() string {
	return (*fs.session).User()
}
func (fs *FakeShell) Host() string {
	return gutils.ExtractHostFromAddr((*fs.session).RemoteAddr())
}

func (fs *FakeShell) Close() {
	_, err := fs.terminal.Write([]byte(""))
	if err != nil {
		if err == io.EOF {
			(*fs.session).Close()
			return
		} else {
			panic(err)
		}
	}
	(*fs.session).Close()
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
	_, err := (*fs.session).Read(b)
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

// ReadBytesUntilEOF reads from the SSH session until it encounters an error or EOF.
// It will not read more bytes than the given maxBytes.
// The read bytes will be returned as byte array.
func (fs *FakeShell) ReadBytesUntilEOF(maxBytes int) ([]byte, error) {
	bytes := []byte{}
	i := 0
	for {
		if i >= maxBytes {
			break
		}
		b, err := fs.ReadBytes(1)
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, err
		}
		bytes = append(bytes, b[0])
		i++
	}
	return bytes, nil
}

func (fs *FakeShell) Exec(line string, s *Session, iSeq, lSeq int) bool {
	fs.stats.AddCommandToHistory(line)

	line = string(regexEnvVarPrefixes.ReplaceAll([]byte(line), []byte("$1")))

	pieces := strings.Split(line, " ")
	command := pieces[0]
	args := pieces[1:]

	rmtH, rmtP := gutils.SplitHostPortFromAddr((*fs.session).RemoteAddr())
	lclH, lclP := gutils.SplitHostPortFromAddr((*fs.session).LocalAddr())

	cmd := fmt.Sprintf("%s %s", glog.Reason(command), glog.Wrap(strings.Join(args, " "), glog.LightBlue))
	if lSeq > 1 {
		cmd = fmt.Sprintf("(%s/%s) %s", glog.Int(iSeq), glog.Int(lSeq), cmd)
	}
	s.UpdateActivity()
	fs.logger.Info("%s: %s", s.LogID(), cmd)
	defer func() {
		s.UpdateActivity()
	}()

	if !s.Whitelisted {
		// 1) make sure the client waits some time at least,
		//    the more input the more wait time, hehe
		dly := len(line) * int(Conf.InputDelay)
		gutils.RandomSleep(dly, dly*2, time.Millisecond)
	}

	// Ignore just pressing enter with whitespace
	if strings.TrimSpace(line) == "" {
		return false
	}

	// 3) check if command should exit immediately
	for _, cmd := range Conf.Commands.Exit {
		if strings.HasPrefix(line+"  ", cmd+" ") {
			fs.RecordExec(line, gutils.GeneratePseudoEmptyString(0)) // just to waste some more time ;)
			return true
		}
	}

	SrvMetrics.IncrementExecutedCommands()

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
		User:      s.User,
		IP:        rmtH,
		IPLocal:   lclH,
		Port:      rmtP,
		PortLocal: lclP,
		HostName:  Conf.HostName,
		InputRaw:  line,
		Command:   command,
		Arguments: args,
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
			fs.RecordExec(line, ParseTemplateFromString(gutils.GenerateGarbageString(1000)+"\nend_request: I/O error", data))
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
			fs.RecordExec(line, gutils.GenerateGarbageString(1000))
			return false
		}
	}

	instr := strings.TrimSpace(line)
	instrCmd := strings.Split(instr, " ")[0]

	// 10) check if there is a go-implemented command for this
	if goCmd, found := CmdLookup[instrCmd]; found {
		SrvOSSH.initOverlayFS(fs, s)
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
	if (*fs.session).RawCommand() != "" {
		// this means the client passed a command along (e.g. with -t/-tt param),
		// let's run it and then close the connection.
		raw := (*fs.session).RawCommand()

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

func NewFakeShell(s *Session) *FakeShell {
	fs := &FakeShell{
		session:  s.SSHSession,
		terminal: nil,
		writer:   nil,
		created:  time.Now(),
		stats: &FakeShellStats{
			CommandsExecuted: 0,
			CommandHistory:   []string{},
			Host:             "",
			User:             (*s.SSHSession).User(),
			recording:        utils.NewASCIICastV2(fakeShellInitialWidth, fakeShellInitialHeight),
		},
		overlayFS: nil,
		logger:    glog.NewLogger("Fake Shell", glog.OliveGreen, Conf.Debug.FakeShell, false, false, logMessageHandler),
	}

	fs.terminal = term.NewTerminal(*s.SSHSession, "")
	fs.writer = utils.NewSlowWriter(Conf.Ratelimit, fs.terminal)
	if s.Whitelisted {
		fs.writer.SetRatelimit(10000) // set ridicuously high to effectively disable rate limit
	}
	fs.stats.Host = fs.Host()
	fs.cwd = "/home/" + (*s.SSHSession).User()
	fs.UpdatePrompt("~")
	fs.logger.Debug("%s: Fake shell ready, current working directory: %s", s.LogID(), glog.File(fs.cwd))
	return fs
}
