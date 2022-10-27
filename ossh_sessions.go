package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/toxyl/glog"
	"github.com/toxyl/gutils"
)

type Session struct {
	CreatedAt    time.Time
	LastActivity time.Time
	ID           string
	Type         string
	Shell        *FakeShell
	SSHSession   *ssh.Session
	Term         string
	User         string
	Password     string
	Host         string
	Port         int
	Whitelisted  bool
	Orphan       bool
	logger       *glog.Logger
	lock         *sync.Mutex
}

func (s *Session) Lock() {
	s.lock.Lock()
}

func (s *Session) Unlock() {
	s.lock.Unlock()
}

func (s *Session) UpdateActivity() {
	s.Lock()
	defer s.Unlock()
	if !s.Orphan {
		SrvOSSH.addWastedTime(int(time.Since(s.LastActivity).Seconds()))
		s.LastActivity = time.Now()
	}
}

func (s *Session) RandomSleep(min, max int) {
	if !s.Whitelisted {
		gutils.RandomSleep(min, max, time.Millisecond)
		s.UpdateActivity()
	}
}

func (s *Session) SetID(id string) *Session {
	s.Lock()
	defer s.Unlock()
	ip, port := gutils.SplitHostPort(id)
	if ip == "" || port == 0 {
		s.logger.Error("Invalid session ID %s. Format must be 'host:port'!", glog.Reason(id))
		return nil
	}
	s.Host = ip
	s.Port = port
	s.updateID()
	return s
}

func (s *Session) updateID() {
	s.ID = fmt.Sprintf("%s:%d", s.Host, s.Port)
	s.Whitelisted = isIPWhitelisted(s.Host)
}

func (s *Session) SetType(sessionType string) *Session {
	s.Lock()
	s.Type = sessionType
	s.Unlock()
	s.UpdateActivity()
	return s
}

func (s *Session) SetShell() *Session {
	s.Lock()
	s.Shell = NewFakeShell(s)
	s.Unlock()
	s.UpdateActivity()
	return s
}

func (s *Session) SetSSHSession(sshSession *ssh.Session) *Session {
	s.Lock()
	s.SSHSession = sshSession
	s.Unlock()
	s.UpdateActivity()
	return s
}

func (s *Session) SetTerm(term string) *Session {
	s.Lock()
	s.Term = term
	s.Unlock()
	s.UpdateActivity()
	return s
}

func (s *Session) SetUser(user string) *Session {
	s.Lock()
	s.User = user
	s.Unlock()
	s.UpdateActivity()
	return s
}

func (s *Session) SetPassword(password string) *Session {
	s.Lock()
	s.Password = password
	s.Unlock()
	s.UpdateActivity()
	return s
}

func (s *Session) SetHost(host string) *Session {
	s.Lock()
	s.Host = host
	s.updateID()
	s.Unlock()
	s.UpdateActivity()
	return s
}

func (s *Session) SetPort(port int) *Session {
	s.Lock()
	s.Port = port
	s.updateID()
	s.Unlock()
	s.UpdateActivity()
	return s
}

func (s *Session) LogID() string {
	return fmt.Sprintf("%s @ %s", colorConnID(s.User, s.Host, s.Port), glog.Duration(uint(s.Uptime().Seconds())))
}

func (s *Session) Uptime() time.Duration {
	return time.Since(s.CreatedAt)
}

func (s *Session) StaleSince() time.Duration {
	return time.Since(s.LastActivity)
}

func (s *Session) ActiveFor() time.Duration {
	return s.LastActivity.Sub(s.CreatedAt)
}

// expire checks if the session exists and is older than the given age.
// It will then exit the session with code -1 and close the connection.
// The function returns true if the session is expired, else false.
func (s *Session) expire(age uint) bool {
	s.Lock()
	defer s.Unlock()

	if s.SSHSession == nil && s.StaleSince().Seconds() > 600 {
		// if 10 minutes have passed without establishing a connection,
		// we consider this to be an orphan
		s.Orphan = true
		return true
	}
	if s.StaleSince().Seconds() > float64(age) {
		s.logger.Info("%s: Expiring session...", s.LogID())
		_ = (*s.SSHSession).Exit(-1) // clean up
		return true
	}
	return false
}

func NewSession(logger *glog.Logger) *Session {
	s := &Session{
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
		ID:           "",
		Shell:        nil,
		SSHSession:   nil,
		User:         "",
		Password:     "",
		Host:         "",
		Port:         0,
		Term:         "",
		Whitelisted:  false,
		Orphan:       false,
		logger:       logger,
		lock:         &sync.Mutex{},
	}
	return s
}

type Sessions struct {
	sessions map[string]*Session
	logger   *glog.Logger
	lock     *sync.Mutex
}

func (ss *Sessions) Lock() {
	ss.lock.Lock()
}

func (ss *Sessions) Unlock() {
	ss.lock.Unlock()
}

func (ss *Sessions) has(sessionID string) bool {
	if _, ok := ss.sessions[sessionID]; !ok {
		return false
	}
	return true
}

func (ss *Sessions) add(session *Session) {
	if ss.has(session.ID) {
		return
	}
	ss.sessions[session.ID] = session
}

func (ss *Sessions) countActiveSessions(host string) int {
	active := 0
	for _, v := range ss.sessions {
		if v.Host == host {
			active++
		}
	}
	return active
}

func (ss *Sessions) Create(sessionID string) *Session {
	ss.Lock()
	defer ss.Unlock()

	if !ss.has(sessionID) {
		s := NewSession(glog.NewLogger("Sessions", glog.DarkOrange, Conf.Debug.Sessions, false, false, logMessageHandler)).SetID(sessionID)
		if s == nil {
			return nil
		}
		ss.add(s)
		s.UpdateActivity()
		cnts := len(ss.sessions)
		active := ss.countActiveSessions(s.Host)
		SrvMetrics.IncrementSessions()
		ss.logger.OK(
			"%s: Session started, host now uses %s of %s.",
			s.LogID(), glog.Int(active), glog.IntAmount(cnts, "active session", "active sessions"))
	}
	return ss.get(sessionID)
}

func (ss *Sessions) Remove(sessionID, reason string) {
	ss.Lock()
	defer ss.Unlock()
	if ss.has(sessionID) {
		s := ss.get(sessionID)
		sh := s.Host
		tw := 0
		cid := colorConnID("", sh, s.Port)
		if s.Orphan {
			tw = int(s.ActiveFor().Seconds())
			cid += " (orphan)"
		} else {
			tw = int(s.Uptime().Seconds())
		}
		s.UpdateActivity()
		ss.delete(sessionID)
		ss.Unlock()
		SrvMetrics.DecrementSessions()
		cnts := ss.Count()
		ss.Lock()
		active := ss.countActiveSessions(sh)

		if reason == "" {
			if cnts == 0 {
				ss.logger.OK(
					"%s: Session removed, host has no more active sessions. It was active for %s.",
					cid, glog.Duration(uint(tw)),
				)
			} else {
				ss.logger.OK(
					"%s: Session removed, host now uses %s of %s. It was active for %s.",
					cid, glog.Int(active), glog.IntAmount(cnts, "active session", "active sessions"), glog.Duration(uint(tw)),
				)
			}
		} else {
			if cnts == 0 {
				ss.logger.OK(
					"%s: Session removed, host has no more active sessions. It was active for %s and removed because %s.",
					cid, glog.Int(active), glog.IntAmount(cnts, "active session", "active sessions"), glog.Duration(uint(tw)), glog.Reason(reason),
				)
			} else {
				ss.logger.OK(
					"%s: Session removed, host now uses %s of %s. It was active for %s and removed because %s.",
					cid, glog.Int(active), glog.IntAmount(cnts, "active session", "active sessions"), glog.Duration(uint(tw)), glog.Reason(reason),
				)
			}
		}
	}
}

func (ss *Sessions) get(sessionID string) *Session {
	if _, ok := ss.sessions[sessionID]; ok {
		return ss.sessions[sessionID]
	}
	return nil
}

func (ss *Sessions) delete(sessionID string) {
	delete(ss.sessions, sessionID)
	ss.Unlock()
	defer ss.Lock()
	ss.Remove(sessionID, "session has been deleted")
}

func (ss *Sessions) Count() int {
	ss.Lock()
	defer ss.Unlock()
	return len(ss.sessions)
}

func (ss *Sessions) cleanUp(age uint) {
	ss.Lock()
	defer ss.Unlock()

	for sessionID, session := range ss.sessions {
		if session.expire(age) {
			ss.Unlock()
			ss.Remove(sessionID, "session has expired")
			ss.Lock()
		}
	}
}

func (ss *Sessions) cleanUpWorker(maxAge uint) {
	gutils.RandomSleep(30, 60, time.Second)
	for {
		time.Sleep(INTERVAL_SESSIONS_CLEANUP)
		ss.cleanUp(maxAge)
	}
}

func NewActiveSessions(maxAge uint, logger *glog.Logger) *Sessions {
	ss := &Sessions{
		sessions: map[string]*Session{},
		lock:     &sync.Mutex{},
		logger:   logger,
	}
	go ss.cleanUpWorker(maxAge)
	return ss
}
