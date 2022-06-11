package main

import (
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/gliderlabs/ssh"
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
	lock         *sync.Mutex
}

func (s *Session) UpdateActivity() {
	s.lock.Lock()
	defer s.lock.Unlock()
	if !s.Orphan {
		SrvOSSH.addWastedTime(int(time.Since(s.LastActivity).Seconds()))
		s.LastActivity = time.Now()
	}
}

func (s *Session) RandomSleep(min, max int) {
	if !s.Whitelisted {
		wait := time.Duration(GetRandomInt(min, max)) * time.Millisecond
		time.Sleep(wait)
		s.UpdateActivity()
	}
}

func (s *Session) SetID(id string) *Session {
	s.lock.Lock()
	defer s.lock.Unlock()
	ip, port, err := net.SplitHostPort(id)
	if err != nil {
		LogSessions.Error("Invalid session ID %s: %s. Format must be 'host:port'!", colorReason(id), colorError(err))
		return nil
	}
	p, err := strconv.Atoi(port)
	if err != nil {
		p = 0
	}
	s.Host = ip
	s.Port = p
	s.updateID()
	return s
}

func (s *Session) updateID() {
	s.ID = fmt.Sprintf("%s:%d", s.Host, s.Port)
	s.Whitelisted = isIPWhitelisted(s.Host)
}

func (s *Session) SetType(sessionType string) *Session {
	s.lock.Lock()
	s.Type = sessionType
	s.lock.Unlock()
	s.UpdateActivity()
	return s
}

func (s *Session) SetShell(shell *FakeShell) *Session {
	s.lock.Lock()
	s.Shell = shell
	s.lock.Unlock()
	s.UpdateActivity()
	return s
}

func (s *Session) SetSSHSession(sshSession *ssh.Session) *Session {
	s.lock.Lock()
	s.SSHSession = sshSession
	s.lock.Unlock()
	s.UpdateActivity()
	return s
}

func (s *Session) SetTerm(term string) *Session {
	s.lock.Lock()
	s.Term = term
	s.lock.Unlock()
	s.UpdateActivity()
	return s
}

func (s *Session) SetUser(user string) *Session {
	s.lock.Lock()
	s.User = user
	s.lock.Unlock()
	s.UpdateActivity()
	return s
}

func (s *Session) SetPassword(password string) *Session {
	s.lock.Lock()
	s.Password = password
	s.lock.Unlock()
	s.UpdateActivity()
	return s
}

func (s *Session) SetHost(host string) *Session {
	s.lock.Lock()
	s.Host = host
	s.updateID()
	s.lock.Unlock()
	s.UpdateActivity()
	return s
}

func (s *Session) SetPort(port int) *Session {
	s.lock.Lock()
	s.Port = port
	s.updateID()
	s.lock.Unlock()
	s.UpdateActivity()
	return s
}

func (s *Session) LogID() string {
	return colorConnID(s.User, s.Host, s.Port)
}

func (s *Session) LogIDFull() string {
	return fmt.Sprintf("%s @ %s", s.LogID(), colorDuration(uint(s.Uptime().Seconds())))
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

// Expire checks if the session exists and is older than the given age.
// It will then exit the session with code -1 and close the connection.
// The function returns true if the session is expired, else false.
func (s *Session) Expire(age uint) bool {
	if s.SSHSession == nil && s.StaleSince().Seconds() > 600 {
		// if 10 minutes have passed without establishing a connection,
		// we consider this to be an orphan
		s.lock.Lock()
		s.Orphan = true
		s.lock.Unlock()
		return true
	}
	if s.StaleSince().Seconds() > float64(age) {
		LogSessions.Info("%s: Expiring session...", s.LogID())
		_ = (*s.SSHSession).Exit(-1) // clean up
		return true
	}
	return false
}

func NewSession() *Session {
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
		lock:         &sync.Mutex{},
	}
	return s
}

type Sessions struct {
	sessions map[string]*Session
	lock     *sync.Mutex
}

func (ss *Sessions) Has(sessionID string) bool {
	ss.lock.Lock()
	defer ss.lock.Unlock()
	if _, ok := ss.sessions[sessionID]; !ok {
		return false
	}
	return true
}

func (ss *Sessions) Add(session *Session) {
	if ss.Has(session.ID) {
		return
	}
	ss.lock.Lock()
	defer ss.lock.Unlock()
	ss.sessions[session.ID] = session
}

func (ss *Sessions) CountActiveSessions(host string) int {
	ss.lock.Lock()
	defer ss.lock.Unlock()
	active := 0
	for _, v := range ss.sessions {
		if v.Host == host {
			active++
		}
	}
	return active
}

func (ss *Sessions) Create(sessionID string) *Session {
	if !ss.Has(sessionID) {
		s := NewSession().SetID(sessionID)
		if s == nil {
			return nil
		}
		ss.Add(s)
		s.UpdateActivity()
		cnts := len(ss.sessions)
		active := ss.CountActiveSessions(s.Host)
		LogSessions.OK("%s: Session started, host now uses %s of %s.", s.LogID(), colorInt(active), colorIntAmount(cnts, "active session", "active sessions"))
	}
	ss.lock.Lock()
	defer ss.lock.Unlock()
	return ss.sessions[sessionID]
}

func (ss *Sessions) Remove(sessionID, reason string) {
	if ss.Has(sessionID) {
		ss.lock.Lock()
		s := ss.sessions[sessionID]
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
		delete(ss.sessions, sessionID)
		cnts := len(ss.sessions)
		ss.lock.Unlock()
		active := ss.CountActiveSessions(sh)

		if reason == "" {
			LogSessions.OK(
				"%s: Session removed (was active for %s), host now uses %s of %s.",
				cid, colorDuration(uint(tw)), colorInt(active), colorIntAmount(cnts, "active session", "active sessions"))
		} else {
			LogSessions.OK(
				"%s: Session removed because %s (was active for %s), host now uses %s of %s.",
				cid, colorReason(reason), colorDuration(uint(tw)), colorInt(active), colorIntAmount(cnts, "active session", "active sessions"))
		}
	}
}

func (ss *Sessions) Count() int {
	ss.lock.Lock()
	defer ss.lock.Unlock()
	return len(ss.sessions)
}

func (ss *Sessions) CleanUp(age uint) {
	ss.lock.Lock()
	defer ss.lock.Unlock()

	for sessionID, session := range ss.sessions {
		if session.Expire(age) {
			ss.lock.Unlock()
			ss.Remove(sessionID, "session has expired")
			ss.lock.Lock()
		}
	}
}

func NewActiveSessions(maxAge uint) *Sessions {
	ss := &Sessions{
		sessions: map[string]*Session{},
		lock:     &sync.Mutex{},
	}
	go func(maxAge uint) {
		for {
			time.Sleep(INTERVAL_SESSIONS_CLEANUP)
			ss.CleanUp(maxAge)
		}
	}(maxAge)
	return ss
}
