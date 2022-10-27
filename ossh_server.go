package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/toxyl/glog"
	"github.com/toxyl/gutils"
)

type TimeWastedCounter struct {
	val  int
	lock *sync.Mutex
}

func (twc *TimeWastedCounter) Value() int {
	twc.lock.Lock()
	defer twc.lock.Unlock()
	return twc.val
}

func (twc *TimeWastedCounter) Add(n int) {
	twc.lock.Lock()
	defer twc.lock.Unlock()
	twc.val += n
}

var activeFS *FakeFS

type OSSHServer struct {
	Loot       *Loot
	Logins     *Logins
	Sessions   *Sessions
	TimeWasted *TimeWastedCounter
	server     []*ssh.Server
	fs         *FakeFSManager
	logger     *glog.Logger
}

func (ossh *OSSHServer) stats() *SyncNodeStats {
	uptime := uptime().Round(1 * time.Second).Seconds()
	SrvMetrics.SetTimeOnline(uptime)
	return &SyncNodeStats{
		Hosts:            ossh.Loot.CountHosts(),
		Passwords:        ossh.Loot.CountPasswords(),
		Users:            ossh.Loot.CountUsers(),
		Payloads:         ossh.Loot.CountPayloads(),
		Sessions:         ossh.Sessions.Count(),
		AttemptedLogins:  ossh.Logins.GetAttempts(),
		SuccessfulLogins: ossh.Logins.GetSuccesses(),
		FailedLogins:     ossh.Logins.GetFailures(),
		TimeWasted:       ossh.getWastedTime(),
		Uptime:           uptime,
	}
}

func (ossh *OSSHServer) addWastedTime(seconds int) {
	SrvMetrics.AddTimeWasted(float64(seconds))
	ossh.TimeWasted.Add(seconds)
}

func (ossh *OSSHServer) getWastedTime() float64 {
	return time.Duration(ossh.TimeWasted.Value() * int(time.Second)).Seconds()
}

func (ossh *OSSHServer) statsToJSON() string {
	json, err := json.Marshal(ossh.stats())
	if err != nil {
		ossh.logger.Error("Could not marshal stats data: %s", glog.Error(err))
		return ""
	}

	return string(json)
}

func (ossh *OSSHServer) JSONToStats(jsonString string) *SyncNodeStats {
	data := &SyncNodeStats{}
	err := json.Unmarshal([]byte(jsonString), data)
	if err != nil {
		ossh.logger.Error("Could not unmarshal stats data: %s", glog.Error(err))
		return nil
	}

	return data
}

func (ossh *OSSHServer) loadDataFile(path, contentType string, fnAdd func(s string) bool) {
	if gutils.FileExists(path) {
		content, err := os.ReadFile(path)
		if err != nil {
			ossh.logger.Error("Failed to read %s file: %s", glog.Highlight(contentType), glog.Error(err))
			return
		}
		items := strings.Split(string(content), "\n")

		ossh.logger.OK("Loading %s %s", glog.Int(len(items)), contentType)
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
	ossh.logger.Debug("Loaded data files")
}

func (ossh *OSSHServer) saveDataFile(path, contentType string, lines []string) {
	data := strings.Join(lines, "\n") + "\n"
	err := os.WriteFile(path, []byte(data), 0644)
	if err != nil {
		ossh.logger.Error("Failed to write %s file: %s", glog.Highlight(contentType), glog.Error(err))
	}
}

func (ossh *OSSHServer) SaveData() {
	ossh.saveDataFile(Conf.PathHosts, "hosts", ossh.Loot.GetHosts())
	ossh.saveDataFile(Conf.PathUsers, "users", ossh.Loot.GetUsers())
	ossh.saveDataFile(Conf.PathPasswords, "passwords", ossh.Loot.GetPasswords())
	ossh.saveDataFile(Conf.PathPayloads, "payloads", ossh.Loot.GetPayloads())
	ossh.logger.Debug("Saved data files")
}

func (ossh *OSSHServer) syncCredentials(user, password, host string) {
	SrvSync.Broadcast(fmt.Sprintf("ADD-USER %s", user))
	SrvSync.Broadcast(fmt.Sprintf("ADD-PASSWORD %s", password))
	SrvSync.Broadcast(fmt.Sprintf("ADD-HOST %s", host))
}

func (ossh *OSSHServer) addLoginFailure(s *Session, reason string) {
	s.UpdateActivity()
	if s.Password == "" {
		s.Password = "(empty)"
	}

	if isIPWhitelisted(s.Host) {
		ossh.logger.NotOK("%s failed to login: %s.", s.LogID(), glog.Reason(reason))
		return // we don't want stats for whitelisted IPs
	}

	// go ossh.syncCredentials(s.User, s.Password, s.Host)

	ossh.Loot.AddUser(s.User)
	ossh.Loot.AddPassword(s.Password)
	ossh.Loot.AddHost(s.Host)
	ossh.Logins.Get(s.Host).AddFailure()
	ossh.logger.NotOK(
		"%s: Failed to login with password %s: %s. (%d attempts; %d failed; %d success)",
		s.LogID(),
		glog.Password(s.Password),
		glog.Reason(reason),
		ossh.Logins.Get(s.Host).GetAttempts(),
		ossh.Logins.Get(s.Host).GetFailures(),
		ossh.Logins.Get(s.Host).GetSuccesses(),
	)
}

func (ossh *OSSHServer) addLoginSuccess(s *Session, reason string) {
	s.UpdateActivity()
	if s.Password == "" {
		s.Password = "(empty)"
	}

	if isIPWhitelisted(s.Host) {
		ossh.logger.OK("Elvis (disguised as %s) logged in.", s.LogID())
		return // we don't want stats for whitelisted IPs
	}

	// go ossh.syncCredentials(s.User, s.Password, s.Host)

	ossh.Loot.AddUser(s.User)
	ossh.Loot.AddPassword(s.Password)
	ossh.Loot.AddHost(s.Host)
	ossh.Logins.Get(s.Host).AddSuccess()
	ossh.logger.OK(
		"%s: Logged in with password %s: %s. (%d attempts; %d failed; %d success)",
		s.LogID(),
		glog.Password(s.Password),
		glog.Reason(reason),
		ossh.Logins.Get(s.Host).GetAttempts(),
		ossh.Logins.Get(s.Host).GetFailures(),
		ossh.Logins.Get(s.Host).GetSuccesses(),
	)
}

func (ossh *OSSHServer) initOverlayFS() {
	overlayFS, err := ossh.fs.NewSession(fmt.Sprintf("%d", time.Now().Unix()))
	if err != nil {
		ossh.logger.Error("Failed to initialize fake file system: %s", glog.Error(err))
		os.Exit(2)
		return
	}

	if overlayFS != nil {
		err = overlayFS.Mount()
		if err != nil {
			ossh.logger.Error("Failed to mount fake file system: %s", glog.Error(err))
			os.Exit(3)
			return
		}

		if !overlayFS.DirExists("/home") {
			err := overlayFS.Mkdir("/home", 700)
			if err != nil {
				ossh.logger.Error("Failed to create /home dir in fake file system: %s", glog.Error(err))
				os.Exit(4)
			}
		}

		activeFS = overlayFS
	}
}

func (ossh *OSSHServer) sessionHandler(sess ssh.Session) {
	// Catch panics, so a bug triggered in a SSH session doesn't crash the whole service
	defer func() {
		if err := recover(); err != nil {
			ossh.logger.Error("Fatal error: %s", glog.Reason(fmt.Sprint(err)))
		}
	}()
	s := ossh.Sessions.Create(sess.RemoteAddr().String()).SetSSHSession(&sess)
	if s == nil {
		ossh.logger.Error("Failed to create oSSH session!")
		sess.Close()
		return
	}

	s.RandomSleep(1, 250)
	s.SetShell()
	stats := s.Shell.Process(s)

	if !s.Whitelisted {
		ossh.logger.Success("%s: Finished running %s command(s)",
			s.LogID(),
			glog.Int(int(stats.CommandsExecuted)),
		)
	} else {
		ossh.logger.Success("%s: %s",
			s.LogID(),
			glog.Reason("Elvis has left the building."),
		)
	}

	if !s.Whitelisted {
		ossh.SaveData()
		pl := stats.ToPayload()
		SrvOSSH.Loot.AddPayload(pl.hash)
		pl.Save()
	}

	ossh.Sessions.Remove(s.ID, "")
}

func (ossh *OSSHServer) localPortForwardingCallback(ctx ssh.Context, bindHost string, bindPort uint32) bool {
	s := ossh.Sessions.Create(ctx.RemoteAddr().String())
	ossh.logger.Warning("%s: Tried to locally port forward to %s. Request denied!",
		s.LogID(),
		glog.AddrHostPort(bindHost, int(bindPort), true),
	)
	ossh.Sessions.Remove(s.ID, "local port forwarding denied")
	return false
}

func (ossh *OSSHServer) reversePortForwardingCallback(ctx ssh.Context, bindHost string, bindPort uint32) bool {
	s := ossh.Sessions.Create(ctx.RemoteAddr().String())
	ossh.logger.Warning("%s: Tried to reverse port forward to %s:%s. Request denied!",
		s.LogID(),
		glog.AddrHostPort(bindHost, int(bindPort), true),
	)
	ossh.Sessions.Remove(s.ID, "reverse port forwarding denied")
	return false
}

func (ossh *OSSHServer) ptyCallback(ctx ssh.Context, pty ssh.Pty) bool {
	s := ossh.Sessions.Create(ctx.RemoteAddr().String()).SetTerm(pty.Term)
	if !s.Whitelisted {
		ossh.logger.OK("%s: Requested %s PTY session",
			s.LogID(),
			glog.Highlight(s.Term),
		)
	}
	s.RandomSleep(1, 250)
	return true
}

func (ossh *OSSHServer) sessionRequestCallback(sess ssh.Session, requestType string) bool {
	s := ossh.Sessions.Create(sess.RemoteAddr().String()).SetType(requestType)
	if !s.Whitelisted {
		ossh.logger.OK("%s: Requested %s session",
			s.LogID(),
			glog.Highlight(s.Type),
		)
	}
	s.RandomSleep(1, 250)
	return true
}

func (ossh *OSSHServer) connectionFailedCallback(conn net.Conn, err error) {
	s := ossh.Sessions.Create(conn.RemoteAddr().String())
	e := err.Error()

	if e == "EOF" {
		// that's normal, we can ignore it
		return
	} else if strings.Contains(e, "no auth passed yet, permission denied") {
		// probably because we denied it
	} else if strings.Contains(e, "ssh: disconnect, reason 11:") {
		// bot chickened out
	} else if strings.Contains(e, "unmarshal error for field Language of type disconnectMsg") {
		// seems harmless, no need to log it
	} else if strings.Contains(e, "read: connection reset by peer") {
		// the bot doesn't want to talk to us anymore. that's fine, you do you.
	} else if strings.Contains(e, "read: connection timed out") {
		// ok, that's fine, come back another time.
	} else {
		// ok, this might be relevant
		ossh.Sessions.Remove(s.ID, e)
		return
	}

	ossh.Sessions.Remove(s.ID, "")
}

func (ossh *OSSHServer) authHandler(ctx ssh.Context, pwd string) bool {
	s := ossh.Sessions.Create(ctx.RemoteAddr().String()).SetUser(ctx.User()).SetPassword(pwd)

	if s.Whitelisted {
		ossh.addLoginSuccess(s, "host is whitelisted")
		return true // I know you, have fun
	}

	// we know the host, but let's add some dice to the mix anyway
	if ossh.Loot.HasHost(s.Host) && time.Now().Unix()%7 != 0 {
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

func (ossh *OSSHServer) connectionCallback(ctx ssh.Context, conn net.Conn) net.Conn {
	s := ossh.Sessions.Create(conn.RemoteAddr().String())
	if s != nil {
		s.RandomSleep(1, 250)
	}
	return conn
}

func (ossh *OSSHServer) publicKeyHandler(ctx ssh.Context, key ssh.PublicKey) bool {
	s := ossh.Sessions.Create(ctx.RemoteAddr().String()).SetUser(ctx.User())
	if isIPWhitelisted(s.Host) {
		ossh.addLoginSuccess(s, "Elvis entered the building with a key")
		return true
	}

	kb := key.Marshal()
	sha1 := gutils.StringToSha1(string(kb))
	fpath := fmt.Sprintf("%s/%s/%s.pub", Conf.PathCaptures, "ssh-keys", sha1)

	if !gutils.FileExists(fpath) {
		_ = os.WriteFile(fpath, kb, 0400)
		ossh.logger.OK("%s: SSH key saved to %s", s.LogID(), glog.File(fpath))
		ossh.addLoginSuccess(s, "host gave us a public key")
		return true
	}

	// we already know the key, let's force the bot to retry
	ossh.addLoginFailure(s, "public key rejected, host lost a game of dice")
	return false
}

func (ossh *OSSHServer) updateStatsWorker() {
	gutils.RandomSleep(30, 60, time.Second)

	for {
		hs := ossh.stats()
		data := struct {
			Node *SyncNodeStats `json:"node"`
		}{
			Node: hs,
		}

		jsonStats, err := json.Marshal(data)
		if err == nil && SrvUI != nil {
			SrvUI.PushStats(string(jsonStats))
		}
		ms := runtime.MemStats{}
		runtime.ReadMemStats(&ms)
		ossh.logger.OK(
			"[STATS UPDATE] %s up, %s wasted, %s, %s, %s, %s (%s, %s), %s, %s, %s, %s",
			glog.Duration(uint(hs.Uptime)),
			glog.Duration(uint(hs.TimeWasted)),
			glog.IntAmount(hs.Sessions, "session", "sessions"),
			glog.IntAmount(runtime.NumGoroutine(), "goroutine", "goroutines"),
			glog.IntAmount(int(ms.HeapSys/1024/1024), "MB RAM", "MB RAM"),
			glog.IntAmount(int(hs.AttemptedLogins), "login", "logins"),
			glog.IntAmount(int(hs.FailedLogins), "failed", "failed"),
			glog.IntAmount(int(hs.SuccessfulLogins), "successful", "successful"),
			glog.IntAmount(int(hs.Hosts), "host", "hosts"),
			glog.IntAmount(int(hs.Users), "user", "users"),
			glog.IntAmount(int(hs.Passwords), "password", "passwords"),
			glog.IntAmount(int(hs.Payloads), "payload", "payloads"),
		)
		time.Sleep(INTERVAL_UI_STATS_UPDATE)
	}
}

func (ossh *OSSHServer) broadcastStatsWorker() {
	gutils.RandomSleep(30, 60, time.Second)

	for {
		ossh.logger.Debug("Executing stats broadcast...")
		hs := ossh.stats()
		ts := SrvSync.nodes.GetStats(hs)
		data := struct {
			Total *SyncNodeStats `json:"total"`
		}{
			Total: ts,
		}

		jsonStats, err := json.Marshal(data)
		if err == nil && SrvUI != nil {
			SrvUI.PushStats(string(jsonStats))
		}

		jsonStats, err = json.Marshal(hs)
		if err == nil {
			_ = SrvSync.Broadcast(fmt.Sprintf("ADD-STATS %s", string(jsonStats)))
		}

		time.Sleep(INTERVAL_STATS_BROADCAST)
	}
}

func (ossh *OSSHServer) init() {
	ossh.loadData()

	ossh.fs = &FakeFSManager{}
	path := filepath.Join(Conf.PathData, "ffs")
	if Conf.PathFFS != "" {
		path = Conf.PathFFS
	}
	err := ossh.fs.Init(path)
	if err != nil {
		ossh.fs.logger.Error("%s", glog.Error(err))
	}
	ossh.initOverlayFS()

	for _, srv := range Conf.Servers {
		ossh.server = append(ossh.server, &ssh.Server{
			Addr:                          fmt.Sprintf("%s:%d", srv.Host, srv.Port),
			Handler:                       ossh.sessionHandler,
			PasswordHandler:               ossh.authHandler,
			IdleTimeout:                   time.Duration(Conf.MaxIdleTimeout) * time.Second,
			MaxTimeout:                    time.Duration(Conf.MaxSessionAge) * time.Second,
			ReversePortForwardingCallback: ossh.reversePortForwardingCallback,
			LocalPortForwardingCallback:   ossh.localPortForwardingCallback,
			PtyCallback:                   ossh.ptyCallback,
			ConnectionFailedCallback:      ossh.connectionFailedCallback,
			SessionRequestCallback:        ossh.sessionRequestCallback,
			Version:                       Conf.Version,
			ConnCallback:                  ossh.connectionCallback,
			PublicKeyHandler:              ossh.publicKeyHandler,
		})
	}
}

func (ossh *OSSHServer) Start() {
	go ossh.updateStatsWorker()
	go ossh.broadcastStatsWorker()

	var wg sync.WaitGroup
	n := len(ossh.server)
	wg.Add(n)
	for _, srv := range ossh.server {
		go func(srv *ssh.Server) {
			defer wg.Done()
			ossh.logger.Default("Starting oSSH server on %s...", glog.Wrap("ssh://"+srv.Addr, glog.BrightYellow))
			ossh.logger.Error("%s", glog.Error(srv.ListenAndServe()))
		}(srv)
	}
	wg.Wait()
}

func NewOSSHServer() *OSSHServer {
	ossh := &OSSHServer{
		Loot:     NewLoot(),
		Logins:   NewLogins(),
		server:   []*ssh.Server{},
		Sessions: NewActiveSessions(Conf.MaxSessionAge, glog.NewLogger("Sessions", glog.DarkOrange, Conf.Debug.Sessions, false, false, logMessageHandler)),
		TimeWasted: &TimeWastedCounter{
			val:  0,
			lock: &sync.Mutex{},
		},
		logger: glog.NewLogger("oSSH Server", glog.Lime, Conf.Debug.OSSHServer, false, false, logMessageHandler),
	}
	ossh.init()

	return ossh
}
