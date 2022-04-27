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

type FakeShell struct {
	session  ssh.Session
	terminal *term.Terminal
	log      []string
	writer   *SlowWriter
	created  time.Time
	stats    *FakeShellStats
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
	fs.terminal.SetPrompt(fmt.Sprintf("%s@%s:%s# ", fs.User(), Conf.HostName, path))
}

func (fs *FakeShell) Exec(line string) bool {
	fs.stats.CommandHistory = append(fs.stats.CommandHistory, line)
	fs.stats.CommandsExecuted++

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

	if Server.syncClients[data.IP] {
		instr := strings.TrimSpace(line)
		instrCmd := strings.Split(instr, " ")[0]

		switch instrCmd {
		case "check":
			ss, err := Server.getSyncSecrets(data.IP)
			if err != nil {
				Log('x', "Sync with %s failed: %s\n",
					colorWrap(data.IP, 229),
					colorWrap(err.Error(), 51),
				)
				return true
			}

			_ = executeSSHCommand(ss.Host, ss.Port, ss.User, ss.Password, fmt.Sprintf("sync %s", Server.statsHash()))
			fs.writer.WriteLnUnlimited("Sync complete.")
			return true
		case "sync":
			hash := strings.Split(line, " ")[1]
			if Server.statsHash() != hash {
				ss, err := Server.getSyncSecrets(data.IP)
				if err != nil {
					Log('x', "Sync with %s failed: %s\n",
						colorWrap(data.IP, 229),
						colorWrap(err.Error(), 51),
					)
					return true
				}
				clientData := executeSSHCommand(ss.Host, ss.Port, ss.User, ss.Password, "get-data")
				cd := StatsJSON{}
				err = json.Unmarshal([]byte(clientData), &cd)
				if err != nil {
					Log('x', "Sync with %s failed, could not unmarshal remote data: %s\n",
						colorWrap(data.IP, 229),
						colorWrap(err.Error(), 51),
					)
					return true
				}
				ch, cu, cp, cf := 0, 0, 0, 0
				for _, host := range cd.Hosts {
					if !Server.hasHost(host) {
						Server.addHost(host)
						ch++
					}
				}
				for _, user := range cd.Users {
					if !Server.hasUser(user) {
						Server.addUser(user)
						cu++
					}
				}
				for _, password := range cd.Passwords {
					if !Server.hasPassword(password) {
						Server.addPassword(password)
						cp++
					}
				}
				for _, fingerprint := range cd.Fingerprints {
					if !Server.hasFingerprint(fingerprint) {
						Server.addFingerprint(fingerprint)
						cf++
					}
				}
				if ch > 0 || cu > 0 || cp > 0 || cf > 0 {
					Log('i', "[sync] Added %s host(s), %s user name(s), %s password(s) and %s fingerprint(s) from %s\n",
						colorWrap(fmt.Sprint(ch), 229),
						colorWrap(fmt.Sprint(cu), 229),
						colorWrap(fmt.Sprint(cp), 229),
						colorWrap(fmt.Sprint(cf), 229),
						colorWrap(data.IP, 229),
					)
				}
			}
			return true
		case "get-data":
			fs.writer.WriteLnUnlimited(Server.statsJSON())
			return true
		default:
			Log('x', "[sync] Command unknown: %s\n", colorWrap(instrCmd, 229))
			fs.writer.WriteLnUnlimited("Illegal sync command")
			return true
		}
	}

	// 1) check if it's an admin command
	if strings.TrimSpace(line) == "my-little-pony" { // = stats
		fs.writer.WriteLnUnlimited(ParseTemplateFromString(`
Hosts:        {{ .CntHosts }}
Users:        {{ .CntUsers }}
Passwords:    {{ .CntPasswords }}
Fingerprints: {{ .CntFingerprints }}
Time wasted:  {{ .TimeWasted }}
`, struct {
			CntHosts        int
			CntPasswords    int
			CntUsers        int
			CntFingerprints int
			TimeWasted      string
		}{
			CntHosts:        len(Server.Stats.Hosts),
			CntPasswords:    len(Server.Stats.Passwords),
			CntUsers:        len(Server.Stats.Users),
			CntFingerprints: len(Server.Stats.Fingerprints),
			TimeWasted:      time.Duration(Server.Stats.TimeWasted * int(time.Second)).String(),
		}))
		return true
	}

	// 2) make sure the client waits some time at least,
	//    the more input the more wait time, hehe
	dly := time.Duration(len(line) * int(Conf.InputDelay))
	time.Sleep(dly * time.Millisecond)

	// 3) check if command should exit immediately
	for _, cmd := range Conf.Commands.Exit {
		if strings.HasPrefix(line+"  ", cmd+" ") {
			fs.writer.WriteLn("^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@^@") // just to waste some more time ;)
			return true
		}
	}

	// 4) check if command matches a simple command
	for _, cmd := range Conf.Commands.Simple {
		if strings.HasPrefix(line+"  ", cmd[0]+" ") {
			fs.writer.WriteLn(ParseTemplateFromString(cmd[1], data))
			return false
		}
	}

	// 5) check if command should return permission denied error
	for _, cmd := range Conf.Commands.PermissionDenied {
		if strings.HasPrefix(line+"  ", cmd+" ") {
			fs.writer.WriteLn(ParseTemplateFromString("{{ .Command }}: permission denied", data))
			return false
		}
	}

	// 6) check if command should return disk i/o error
	for _, cmd := range Conf.Commands.DiskError {
		if strings.HasPrefix(line+"  ", cmd+" ") {
			fs.writer.WriteLn(ParseTemplateFromString("end_request: I/O error", data))
			return false
		}
	}

	// 7) check if command should return command not found error
	for _, cmd := range Conf.Commands.CommandNotFound {
		if strings.HasPrefix(line+"  ", cmd+" ") {
			fs.writer.WriteLn(ParseTemplateFromString("{{ .Command }}: command not found", data))
			return false
		}
	}

	// 8) check if command should return file not found error
	for _, cmd := range Conf.Commands.FileNotFound {
		if strings.HasPrefix(line+"  ", cmd+" ") {
			fs.writer.WriteLn(ParseTemplateFromString("{{ .Command }}: No such file or directory", data))
			return false
		}
	}

	// 9) check if command should return not implemented error
	for _, cmd := range Conf.Commands.NotImplemented {
		if strings.HasPrefix(line+" ", cmd+" ") {
			fs.writer.WriteLn(ParseTemplateFromString("{{ .Command }}: Function not implemented", data))
			return false
		}
	}

	// 10) check if we have a template for the command
	fs.writer.WriteLn(ParseTemplateToString(command, data))
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

func NewFakeShell(s ssh.Session) *FakeShell {
	fs := &FakeShell{
		session:  s,
		terminal: nil,
		log:      []string{},
		writer:   nil,
		created:  time.Now(),
		stats: &FakeShellStats{
			TimeSpent:        0,
			CommandsExecuted: 0,
			CommandHistory:   []string{},
			Host:             "",
			User:             s.User(),
		},
	}
	fs.terminal = term.NewTerminal(s, "")
	fs.writer = NewSlowWriter(fs.terminal)
	fs.stats.Host = fs.Host()
	fs.UpdatePrompt("~")
	return fs
}
