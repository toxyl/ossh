package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gliderlabs/ssh"
)

type OSSHServer struct {
	Loot       *Loot
	Logins     *Logins
	Sessions   *Sessions
	TimeWasted int
	server     *ssh.Server
	fs         *OverlayFSManager
}

func (ossh *OSSHServer) statsJSONSimple() string {
	data := struct {
		Hosts            int     `json:"hosts"`
		Passwords        int     `json:"passwords"`
		Users            int     `json:"users"`
		Payloads         int     `json:"payloads"`
		Sessions         int     `json:"sessions"`
		AttemptedLogins  uint    `json:"logins_attempted"`
		SuccessfulLogins uint    `json:"logins_successful"`
		FailedLogins     uint    `json:"logins_failed"`
		TimeWasted       float64 `json:"time_wasted"`
		Uptime           float64 `json:"uptime"`
	}{
		Hosts:            ossh.Loot.CountHosts(),
		Passwords:        ossh.Loot.CountPasswords(),
		Users:            ossh.Loot.CountUsers(),
		Payloads:         ossh.Loot.CountPayloads(),
		Sessions:         ossh.Sessions.Count(),
		AttemptedLogins:  ossh.Logins.GetAttempts(),
		SuccessfulLogins: ossh.Logins.GetSuccesses(),
		FailedLogins:     ossh.Logins.GetFailures(),
		TimeWasted:       time.Duration(ossh.TimeWasted * int(time.Second)).Seconds(),
		Uptime:           uptime().Round(1 * time.Second).Seconds(),
	}
	json, err := json.Marshal(data)
	if err != nil {
		LogOSSHServer.Error("Could not marshal web interface stats data: %s", colorError(err))
		return ""
	}

	return string(json)
}

func (ossh *OSSHServer) loadDataFile(path, contentType string, fnAdd func(s string) bool) {
	if FileExists(path) {
		content, err := os.ReadFile(path)
		if err != nil {
			LogOSSHServer.Error("Failed to read %s file: %s", colorHighlight(contentType), colorError(err))
			return
		}
		items := strings.Split(string(content), "\n")

		LogOSSHServer.OK("Loading %s %s", colorInt(len(items)), contentType)
		for _, fp := range items {
			_ = fnAdd(fp)
		}
	}
}

func (ossh *OSSHServer) loadData() {
	ossh.loadDataFile(Conf.PathHosts, "hosts", ossh.Loot.AddHost)
	ossh.loadDataFile(Conf.PathUsers, "users", ossh.Loot.AddUser)
	ossh.loadDataFile(Conf.PathPasswords, "passwords", ossh.Loot.AddPassword)
	ossh.loadDataFile(Conf.PathPayloads, "payloads", ossh.Loot.AddPayload)
	LogOSSHServer.Debug("Loaded data files")
}

func (ossh *OSSHServer) saveDataFile(path, contentType string, lines []string) {
	data := strings.Join(lines, "\n") + "\n"
	err := os.WriteFile(path, []byte(data), 0644)
	if err != nil {
		LogOSSHServer.Error("Failed to write %s file: %s", colorHighlight(contentType), colorError(err))
	}
}

func (ossh *OSSHServer) SaveData() {
	ossh.saveDataFile(Conf.PathHosts, "hosts", ossh.Loot.GetHosts())
	ossh.saveDataFile(Conf.PathUsers, "users", ossh.Loot.GetUsers())
	ossh.saveDataFile(Conf.PathPasswords, "passwords", ossh.Loot.GetPasswords())
	ossh.saveDataFile(Conf.PathPayloads, "payloads", ossh.Loot.GetPayloads())
	LogOSSHServer.Debug("Saved data files")
}

func (ossh *OSSHServer) addLoginFailure(s *Session, reason string) {
	if s.Password == "" {
		s.Password = "(empty)"
	}

	if isIPWhitelisted(s.Host) {
		LogOSSHServer.NotOK(
			"%s failed to login: %s.",
			s.LogID(),
			colorReason(reason),
		)
		return // we don't want stats for whitelisted IPs
	}

	SrvSync.Broadcast(fmt.Sprintf("ADD-USER %s", s.User))
	SrvSync.Broadcast(fmt.Sprintf("ADD-PASSWORD %s", s.Password))
	SrvSync.Broadcast(fmt.Sprintf("ADD-HOST %s", s.Host))

	ossh.Loot.AddUser(s.User)
	ossh.Loot.AddPassword(s.Password)
	ossh.Loot.AddHost(s.Host)
	ossh.Logins.Get(s.Host).AddFailure()
	LogOSSHServer.NotOK(
		"%s failed to login with password %s: %s. (%d attempts; %d failed; %d success)",
		s.LogID(),
		colorPassword(s.Password),
		colorReason(reason),
		ossh.Logins.Get(s.Host).GetAttempts(),
		ossh.Logins.Get(s.Host).GetFailures(),
		ossh.Logins.Get(s.Host).GetSuccesses(),
	)
}

func (ossh *OSSHServer) addLoginSuccess(s *Session, reason string) {
	if s.Password == "" {
		s.Password = "(empty)"
	}

	if isIPWhitelisted(s.Host) {
		LogOSSHServer.OK(
			"%s logged in.",
			s.LogID(),
		)
		return // we don't want stats for whitelisted IPs
	}

	SrvSync.Broadcast(fmt.Sprintf("ADD-USER %s", s.User))
	SrvSync.Broadcast(fmt.Sprintf("ADD-PASSWORD %s", s.Password))
	SrvSync.Broadcast(fmt.Sprintf("ADD-HOST %s", s.Host))

	ossh.Loot.AddUser(s.User)
	ossh.Loot.AddPassword(s.Password)
	ossh.Loot.AddHost(s.Host)
	ossh.Logins.Get(s.Host).AddSuccess()
	LogOSSHServer.OK(
		"%s logged in with password %s: %s. (%d attempts; %d failed; %d success)",
		s.LogID(),
		colorPassword(s.Password),
		colorReason(reason),
		ossh.Logins.Get(s.Host).GetAttempts(),
		ossh.Logins.Get(s.Host).GetFailures(),
		ossh.Logins.Get(s.Host).GetSuccesses(),
	)
}

func (ossh *OSSHServer) GracefulCloseOnError(err error, s *Session, sess *ssh.Session, ofs *OverlayFS) {
	// TODO  graceful fallback?
	LogOSSHServer.Debug("Graceful close because %s.", colorError(err))
	if ofs != nil {
		ofs.Close()
	}
	(*sess).Close()
	if s != nil {
		ossh.Sessions.Remove(s.ID)
	}
}

func (ossh *OSSHServer) sessionHandler(sess ssh.Session) {
	// Catch panics, so a bug triggered in a SSH session doesn't crash the whole service
	defer func() {
		if err := recover(); err != nil {
			LogOSSHServer.Error("Fatal error: %s", colorReason(fmt.Sprint(err)))
		}
	}()
	s := ossh.Sessions.Create(sess.RemoteAddr().String()).SetSSHSession(&sess)
	if s == nil {
		ossh.GracefulCloseOnError(errors.New("Failed to create oSSH session."), nil, &sess, nil)
		return
	}

	// only create for bots, sync nodes don't need it
	overlayFS, err := ossh.fs.NewSession(s.Host)
	if err != nil {
		ossh.GracefulCloseOnError(err, s, s.SSHSession, overlayFS)
		return
	}

	err = overlayFS.Mount()
	if err != nil {
		ossh.GracefulCloseOnError(err, s, s.SSHSession, overlayFS)
		return
	}
	defer func() {
		overlayFS.Close()
	}()

	s.SetShell(NewFakeShell((*s.SSHSession), overlayFS))
	stats := s.Shell.Process()

	if !s.Whitelisted {
		LogOSSHServer.Success("%s spent %s running %s command(s)",
			s.LogID(),
			colorDuration(uint(s.Uptime().Seconds())),
			colorInt(int(stats.CommandsExecuted)),
		)
	} else {
		LogOSSHServer.Success("%s: %s",
			s.LogID(),
			colorReason("Elvis has left the building."),
		)
	}

	ossh.SaveData()

	if !s.Whitelisted {
		stats.SaveCapture()
	}

	ossh.Sessions.Remove(s.ID)
}

func (ossh *OSSHServer) localPortForwardingCallback(ctx ssh.Context, bindHost string, bindPort uint32) bool {
	s := ossh.Sessions.Create(ctx.RemoteAddr().String())
	LogOSSHServer.Warning("%s tried to locally port forward to %s:%s. Request denied!",
		s.LogID(),
		colorHost(bindHost),
		colorInt(int(bindPort)),
	)
	ossh.Sessions.Remove(s.ID)
	return false
}

func (ossh *OSSHServer) reversePortForwardingCallback(ctx ssh.Context, bindHost string, bindPort uint32) bool {
	s := ossh.Sessions.Create(ctx.RemoteAddr().String())
	LogOSSHServer.Warning("%s tried to reverse port forward to %s:%s. Request denied!",
		s.LogID(),
		colorHost(bindHost),
		colorInt(int(bindPort)),
	)
	ossh.Sessions.Remove(s.ID)
	return false
}

func (ossh *OSSHServer) ptyCallback(ctx ssh.Context, pty ssh.Pty) bool {
	s := ossh.Sessions.Create(ctx.RemoteAddr().String()).SetTerm(pty.Term)
	if s.Whitelisted {
		return true
	}
	LogOSSHServer.OK("%s started %s PTY session",
		s.LogID(),
		colorHighlight(s.Term),
	)
	return true
}

func (ossh *OSSHServer) sessionRequestCallback(sess ssh.Session, requestType string) bool {
	s := ossh.Sessions.Create(sess.RemoteAddr().String()).SetType(requestType)
	if s.Whitelisted {
		return true
	}
	LogOSSHServer.OK("%s requested %s session",
		s.LogID(),
		colorHighlight(s.Type),
	)
	return true
}

func (ossh *OSSHServer) connectionFailedCallback(conn net.Conn, err error) {
	s := ossh.Sessions.Create(conn.RemoteAddr().String())

	if err.Error() != "EOF" && err.Error() != "[ssh: no auth passed yet, permission denied]" {
		LogOSSHServer.Warning("%s's connection failed: %s",
			s.LogID(),
			colorError(err),
		)
	}
	ossh.Sessions.Remove(s.ID)
}

func (ossh *OSSHServer) authHandler(ctx ssh.Context, pwd string) bool {
	s := ossh.Sessions.Create(ctx.RemoteAddr().String())
	s.SetUser(ctx.User()).SetPassword(pwd)

	if s.Whitelisted {
		ossh.addLoginSuccess(s, "host is whitelisted")
		return true // I know you, have fun
	}

	if ossh.Loot.HasHost(s.Host) {
		ossh.addLoginSuccess(s, "host is back for more")
		return true // let's see what it wants
	}

	if ossh.Loot.HasUser(s.User) && ossh.Loot.HasPassword(s.Password) {
		ossh.addLoginFailure(s, "host does not have new credentials")
		ossh.Sessions.Remove(s.ID)
		return false // come back when you have something we don't know yet!
	}

	if ossh.Loot.HasUser(s.User) {
		ossh.addLoginSuccess(s, "host got the user name right")
		return true // ok, we'll take it
	}

	if ossh.Loot.HasPassword(s.Password) {
		ossh.addLoginSuccess(s, "host got the password right")
		return true // ok, we'll take it
	}

	// ok, the attacker has credentials we don't know yet, let's roll dice.
	if time.Now().Unix()%3 != 0 {
		ossh.addLoginFailure(s, "host lost a game of dice")
		ossh.Sessions.Remove(s.ID)
		return false // no luck, big boy, try again
	}

	ossh.addLoginSuccess(s, "host dodged all obstacles")
	return true
}

func (ossh *OSSHServer) init() {
	ossh.loadData()

	ossh.server = &ssh.Server{
		Addr:                          fmt.Sprintf("%s:%d", Conf.Host, Conf.Port),
		Handler:                       ossh.sessionHandler,
		PasswordHandler:               ossh.authHandler,
		IdleTimeout:                   time.Duration(Conf.MaxIdleTimeout) * time.Second,
		ReversePortForwardingCallback: ossh.reversePortForwardingCallback,
		LocalPortForwardingCallback:   ossh.localPortForwardingCallback,
		PtyCallback:                   ossh.ptyCallback,
		ConnectionFailedCallback:      ossh.connectionFailedCallback,
		SessionRequestCallback:        ossh.sessionRequestCallback,
		Version:                       Conf.Version,
	}

	ossh.fs = &OverlayFSManager{}
	path := filepath.Join(Conf.PathData, "ffs")
	if Conf.PathFFS != "" {
		path = Conf.PathFFS
	}
	err := ossh.fs.Init(path)
	if err != nil {
		log.Fatal(err)
	}
}

func (ossh *OSSHServer) Start() {
	LogOSSHServer.Default("Starting oSSH server on %s...", colorWrap("ssh://"+ossh.server.Addr, colorBrightYellow))
	log.Fatal(ossh.server.ListenAndServe())
}

func NewOSSHServer() *OSSHServer {
	ossh := &OSSHServer{
		Loot:       NewLoot(),
		Logins:     NewLogins(),
		server:     nil,
		Sessions:   NewActiveSessions(Conf.MaxSessionAge),
		TimeWasted: 0,
	}
	ossh.init()
	go func() {
		for {
			time.Sleep(10 * time.Second)
			SrvUI.PushStats(SrvOSSH.statsJSONSimple())
		}
	}()

	return ossh
}
