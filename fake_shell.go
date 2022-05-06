package main

import (
	"encoding/json"
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
	fakeShellInitialHeight = 40
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

	if Server.KnownNodes.Has(data.IP) {
		instr := strings.TrimSpace(line)
		instrCmd := strings.Split(instr, " ")[0]

		switch instrCmd {
		case "check":
			ss, err := Server.KnownNodes.Get(data.IP)
			if err != nil {
				LogError("Sync with %s failed: %s\n",
					colorWrap(data.IP, colorBrightYellow),
					colorWrap(err.Error(), colorCyan),
				)
				return true
			}

			_ = executeSSHCommand(ss.Host, ss.Port, ss.User, ss.Password, fmt.Sprintf("sync %s", Server.Loot.Fingerprint()))
			fs.writer.WriteLnUnlimited("Sync complete.")
			return true
		case "sync":
			hash := strings.Split(line, " ")[1]
			if Server.Loot.Fingerprint() != hash {
				node, err := Server.KnownNodes.Get(data.IP)
				if err != nil {
					LogError("Sync with %s failed: %s\n",
						colorWrap(data.IP, colorBrightYellow),
						colorWrap(err.Error(), colorCyan),
					)
					return true
				}
				clientData := executeSSHCommand(node.Host, node.Port, node.User, node.Password, "get-data")
				cd := LootJSON{}
				err = json.Unmarshal([]byte(clientData), &cd)
				if err != nil {
					LogError("Sync with %s failed, could not unmarshal remote data: %s\n",
						colorWrap(data.IP, colorBrightYellow),
						colorWrap(err.Error(), colorCyan),
					)
					return true
				}
				ch, cu, cp, cf := 0, 0, 0, 0
				for _, host := range cd.Hosts {
					if !Server.Loot.HasHost(host) {
						Server.Loot.AddHost(host)
						ch++
					}
				}
				for _, user := range cd.Users {
					if !Server.Loot.HasUser(user) {
						Server.Loot.AddUser(user)
						cu++
					}
				}
				for _, password := range cd.Passwords {
					if !Server.Loot.HasPassword(password) {
						Server.Loot.AddPassword(password)
						cp++
					}
				}
				for _, fingerprint := range cd.Fingerprints {
					if !Server.Loot.HasFingerprint(fingerprint) {
						Server.Loot.AddFingerprint(fingerprint)
						cf++
					}

				}
				if ch > 0 || cu > 0 || cp > 0 || cf > 0 {
					LogInfoLn("[sync] Added %s host(s), %s user name(s), %s password(s) and %s fingerprint(s) from %s",
						colorWrap(fmt.Sprint(ch), colorBrightYellow),
						colorWrap(fmt.Sprint(cu), colorBrightYellow),
						colorWrap(fmt.Sprint(cp), colorBrightYellow),
						colorWrap(fmt.Sprint(cf), colorBrightYellow),
						colorWrap(data.IP, colorBrightYellow),
					)
				}
			}
			return true
		case "get-data":
			fs.writer.WriteLnUnlimited(Server.Loot.JSON())
			return true
		case "get-payload":
			hash := strings.Split(line, " ")[1]
			payload, err := Server.Loot.payloads.Get(hash)
			if err != nil {
				return true
			}
			pl := ""
			if payload.payload != "" {
				pl = payload.EncodeToString()
			}
			fs.writer.WriteLnUnlimited(pl)
			return true
		default:
			LogError("[sync] Command unknown: %s\n", colorWrap(instrCmd, colorBrightYellow))
			fs.writer.WriteLnUnlimited("Illegal sync command")
			return true
		}
	}

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
			fs.RecordExec(line, "^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@") // just to waste some more time ;)
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
			fs.RecordExec(line, ParseTemplateFromString("end_request: I/O error", data))
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
			fs.RecordExec(line, ParseTemplateFromString("{{ .Command }}: No such file or directory", data))
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

	instr := strings.TrimSpace(line)
	instrCmd := strings.Split(instr, " ")[0]

	// 9) check if there is a go-implemented command for this
	if goCmd, found := CmdLookup[instrCmd]; found {
		return goCmd(fs, instr)
	}

	// 10) check if we have a template for the command
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
		for _, cmd := range commands {
			if fs.Exec(cmd) {
				break
			}
		}
	} else {
		fs.HandleInput()
	}
	fs.Close()
	fs.stats.TimeSpent = uint(time.Now().Unix()) - uint(fs.created.Unix())
	return fs.stats
}

func NewFakeShell(s ssh.Session, overlay *OverlayFS) *FakeShell {
	fs := &FakeShell{
		session:  s,
		terminal: nil,
		writer:   nil,
		created:  time.Now(),
		stats: &FakeShellStats{
			TimeSpent:        0,
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
			overlay.Mkdir("/home", 700)
		}

		if !overlay.DirExists("/home/" + s.User()) {
			overlay.Mkdir("/home/"+s.User(), 700)
		}
	}
	fs.cwd = "/home/" + s.User()

	fs.UpdatePrompt("~")
	return fs
}
