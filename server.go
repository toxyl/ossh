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
	KnownNodes *KnownNodes
	TimeWasted int
	server     *ssh.Server
	fs         *OverlayFSManager
}

func (ossh *OSSHServer) statsJSONSimple() string {
	data := struct {
		Hosts            int    `json:"hosts"`
		Passwords        int    `json:"passwords"`
		Users            int    `json:"users"`
		Fingerprints     int    `json:"fingerprints"`
		Sessions         int    `json:"sessions"`
		AttemptedLogins  uint   `json:"logins_attempted"`
		SuccessfulLogins uint   `json:"logins_successful"`
		FailedLogins     uint   `json:"logins_failed"`
		TimeWasted       string `json:"time_wasted"`
	}{
		Hosts:            ossh.Loot.CountHosts(),
		Passwords:        ossh.Loot.CountPasswords(),
		Users:            ossh.Loot.CountUsers(),
		Fingerprints:     ossh.Loot.CountFingerprints(),
		Sessions:         ossh.Sessions.Count(),
		AttemptedLogins:  ossh.Logins.GetAttempts(),
		SuccessfulLogins: ossh.Logins.GetSuccesses(),
		FailedLogins:     ossh.Logins.GetFailures(),
		TimeWasted:       time.Duration(ossh.TimeWasted * int(time.Second)).String(),
	}
	json, err := json.Marshal(data)
	if err != nil {
		LogErrorLn("Could not marshal web interface stats data: %s", colorError(err))
		return ""
	}

	return string(json)
}

func (ossh *OSSHServer) loadDataFile(path, contentType string, addFunction func(s string)) {
	if FileExists(path) {
		content, err := os.ReadFile(path)
		if err != nil {
			LogErrorLn("Failed to read %s file: %s", colorHighlight(contentType), colorError(err))
			return
		}
		items := strings.Split(string(content), "\n")

		LogOKLn("Loading %s %s", colorInt(len(items)), contentType)
		for _, fp := range items {
			addFunction(fp)
		}
	}
}

func (ossh *OSSHServer) loadData() {
	ossh.loadDataFile(Conf.PathHosts, "hosts", ossh.Loot.AddHost)
	ossh.loadDataFile(Conf.PathUsers, "users", ossh.Loot.AddUser)
	ossh.loadDataFile(Conf.PathPasswords, "passwords", ossh.Loot.AddPassword)
	ossh.loadDataFile(Conf.PathFingerprints, "fingerprints", ossh.Loot.AddFingerprint)
}

func (ossh *OSSHServer) saveDataFile(path, contentType string, lines []string) {
	data := strings.Join(lines, "\n") + "\n"
	err := os.WriteFile(path, []byte(data), 0644)
	if err != nil {
		LogErrorLn("Failed to write %s file: %s", colorHighlight(contentType), colorError(err))
	}
}

func (ossh *OSSHServer) saveData() {
	ossh.saveDataFile(Conf.PathHosts, "hosts", ossh.Loot.GetHosts())
	ossh.saveDataFile(Conf.PathUsers, "users", ossh.Loot.GetUsers())
	ossh.saveDataFile(Conf.PathPasswords, "passwords", ossh.Loot.GetPasswords())
	ossh.saveDataFile(Conf.PathFingerprints, "fingerprints", ossh.Loot.GetFingerprints())
}

func (ossh *OSSHServer) addLoginFailure(s *Session, reason string) {
	if s.Password == "" {
		s.Password = "(empty)"
	}

	if isIPWhitelisted(s.Host) {
		LogNotOKLn(
			"%s failed to login: %s.",
			s.LogID(),
			colorReason(reason),
		)
		return // we don't want stats for whitelisted IPs
	}

	ossh.Loot.AddUser(s.User)
	ossh.Loot.AddPassword(s.Password)
	ossh.Loot.AddHost(s.Host)
	ossh.Logins.Get(s.Host).AddFailure()
	LogNotOKLn(
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
		LogOKLn(
			"%s logged in.",
			s.LogID(),
		)
		return // we don't want stats for whitelisted IPs
	}

	ossh.Loot.AddUser(s.User)
	ossh.Loot.AddPassword(s.Password)
	ossh.Loot.AddHost(s.Host)
	ossh.Logins.Get(s.Host).AddSuccess()
	LogOKLn(
		"%s logged in with password %s: %s. (%d attempts; %d failed; %d success)",
		s.LogID(),
		colorPassword(s.Password),
		colorReason(reason),
		ossh.Logins.Get(s.Host).GetAttempts(),
		ossh.Logins.Get(s.Host).GetFailures(),
		ossh.Logins.Get(s.Host).GetSuccesses(),
	)
}

func (ossh *OSSHServer) GracefulCloseOnError(err error, sess *ssh.Session) {
	// TODO  graceful fallback?
	LogErrorLn("[SSH] Graceful close because %s.", colorError(err))
	(*sess).Close()
}

func (ossh *OSSHServer) sessionHandler(sess ssh.Session) {
	s := ossh.Sessions.Create(sess.RemoteAddr().String()).SetSSHSession(&sess)
	if s == nil {
		ossh.GracefulCloseOnError(errors.New("[SSH] Failed to create oSSH session."), &sess)
		return
	}

	s.SSHSession = &sess

	overlayFS, err := ossh.fs.NewSession(s.Host)
	if err != nil {
		ossh.GracefulCloseOnError(errors.New("[SSH] Failed to create FFS session."), s.SSHSession)
		return
	}

	err = overlayFS.Mount()
	if err != nil {
		ossh.GracefulCloseOnError(errors.New("[SSH] Failed to mount FFS."), s.SSHSession)
		return
	}
	defer func() {
		err := overlayFS.Close()
		if err != nil {
			LogErrorLn(colorError(err))
		}
	}()

	s.SetShell(NewFakeShell((*s.SSHSession), overlayFS))
	stats := s.Shell.Process()

	if !ossh.KnownNodes.Has(s.Host) && !s.Whitelisted {
		ossh.TimeWasted += int(stats.TimeSpent)

		LogSuccessLn("%s spent %s running %s command(s)",
			s.LogID(),
			colorDuration(stats.TimeSpent),
			colorInt(int(stats.CommandsExecuted)),
		)
	} else {
		LogSuccessLn("%s: %s",
			s.LogID(),
			colorReason("Elvis has left the building."),
		)
	}

	ossh.saveData()

	if !ossh.KnownNodes.Has(s.Host) && !s.Whitelisted {
		stats.SaveCapture()
	}

	ossh.Sessions.Remove(s.ID)
}

func (ossh *OSSHServer) localPortForwardingCallback(ctx ssh.Context, bindHost string, bindPort uint32) bool {
	s := ossh.Sessions.Create(ctx.RemoteAddr().String())
	LogWarningLn("%s tried to locally port forward to %s:%s. Request denied!",
		s.LogID(),
		colorHost(bindHost),
		colorInt(int(bindPort)),
	)

	return false
}

func (ossh *OSSHServer) reversePortForwardingCallback(ctx ssh.Context, bindHost string, bindPort uint32) bool {
	s := ossh.Sessions.Create(ctx.RemoteAddr().String())
	LogWarningLn("%s tried to reverse port forward to %s:%s. Request denied!",
		s.LogID(),
		colorHost(bindHost),
		colorInt(int(bindPort)),
	)

	return false
}

func (ossh *OSSHServer) ptyCallback(ctx ssh.Context, pty ssh.Pty) bool {
	s := ossh.Sessions.Create(ctx.RemoteAddr().String()).SetTerm(pty.Term)
	if ossh.KnownNodes.Has(s.Host) || s.Whitelisted {
		return true
	}
	LogOKLn("%s started %s PTY session",
		s.LogID(),
		colorHighlight(s.Term),
	)
	return true
}

func (ossh *OSSHServer) sessionRequestCallback(sess ssh.Session, requestType string) bool {
	s := ossh.Sessions.Create(sess.RemoteAddr().String()).SetType(requestType)
	if ossh.KnownNodes.Has(s.Host) || s.Whitelisted {
		return true
	}
	LogOKLn("%s requested %s session",
		s.LogID(),
		colorHighlight(s.Type),
	)
	return true
}

func (ossh *OSSHServer) connectionFailedCallback(conn net.Conn, err error) {
	s := ossh.Sessions.Create(conn.RemoteAddr().String())

	if err.Error() != "EOF" {
		ossh.Sessions.Remove(s.ID)
		LogWarningLn("%s's connection failed: %s",
			s.LogID(),
			colorError(err),
		)
	}
}

func (ossh *OSSHServer) authHandler(ctx ssh.Context, pwd string) bool {
	s := ossh.Sessions.Create(ctx.RemoteAddr().String())
	s.SetUser(ctx.User()).SetPassword(pwd)

	for _, node := range Conf.Sync.Nodes {
		if s.User == node.User && s.Password == node.Password && node.Host == s.Host {
			// secret credentials hit, let's mark as a sync client
			ossh.KnownNodes.Add(s.Host, &node)
			return true
		}
	}

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
	LogDefaultLn("Starting oSSH Server on %v", colorHost(ossh.server.Addr))
	log.Fatal(ossh.server.ListenAndServe())
}

func NewOSSHServer() *OSSHServer {
	ossh := &OSSHServer{
		Loot:       NewLoot(),
		Logins:     NewLogins(),
		server:     nil,
		Sessions:   NewActiveSessions(true),
		KnownNodes: NewKnownNodes(),
		TimeWasted: 0,
	}
	ossh.init()
	go func() {
		for {
			time.Sleep(time.Duration(Conf.Sync.Interval) * time.Minute)
			for _, node := range Conf.Sync.Nodes {
				_ = executeSSHCommand(node.Host, node.Port, node.User, node.Password, "check")
			}
		}
	}()
	go func() {
		for {
			time.Sleep(10 * time.Second)
			WebServer.PushStats(Server.statsJSONSimple())
		}
	}()
	return ossh
}
