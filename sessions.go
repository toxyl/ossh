package main

import (
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/gliderlabs/ssh"
)

type SessionLog struct {
	time.Time
	string
}

type Session struct {
	CreatedAt    time.Time
	LastActivity time.Time
	ActivityLog  []*SessionLog
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

func (s *Session) UpdateActivity(action string) {
	if !s.Orphan {
		SrvOSSH.lock.Lock()
		SrvOSSH.TimeWasted += int(time.Now().Sub(s.LastActivity).Seconds())
		SrvOSSH.lock.Unlock()
	}

	s.LastActivity = time.Now()
	s.ActivityLog = append(s.ActivityLog, &SessionLog{s.LastActivity, action})
	LogSessions.Debug("%s: %s", s.LogIDFull(), colorHighlight(action))
}

func (s *Session) RandomSleep(min, max int) {
	if !s.Whitelisted {
		wait := time.Duration(GetRandomInt(min, max)) * time.Second
		s.lock.Lock()
		s.UpdateActivity("sleep start")
		s.lock.Unlock()
		time.Sleep(wait)
		s.lock.Lock()
		s.UpdateActivity("sleep end")
		s.lock.Unlock()
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
	defer s.lock.Unlock()
	s.Type = sessionType
	s.UpdateActivity("set type")
	return s
}

func (s *Session) SetShell(shell *FakeShell) *Session {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.Shell = shell
	s.UpdateActivity("set shell")
	return s
}

func (s *Session) SetSSHSession(sshSession *ssh.Session) *Session {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.SSHSession = sshSession
	s.UpdateActivity("set session")
	return s
}

func (s *Session) SetTerm(term string) *Session {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.Term = term
	s.UpdateActivity("set term")
	return s
}

func (s *Session) SetUser(user string) *Session {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.User = user
	s.UpdateActivity("set user")
	return s
}

func (s *Session) SetPassword(password string) *Session {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.Password = password
	s.UpdateActivity("set password")
	return s
}

func (s *Session) SetHost(host string) *Session {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.Host = host
	s.updateID()
	s.UpdateActivity("set host")
	return s
}

func (s *Session) SetPort(port int) *Session {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.Port = port
	s.updateID()
	s.UpdateActivity("set port")
	return s
}

func (s *Session) LogID() string {
	return colorConnID(s.User, s.Host, s.Port)
}

func (s *Session) LogIDFull() string {
	return fmt.Sprintf("%s @ %s", s.LogID(), colorDuration(uint(s.ActiveFor().Seconds())))
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
		ActivityLog:  []*SessionLog{},
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

func (ss *Sessions) Create(sessionID string) *Session {
	if !ss.Has(sessionID) {
		s := NewSession().SetID(sessionID)
		if s == nil {
			return nil
		}
		ss.Add(s)
		s.lock.Lock()
		s.UpdateActivity("created")
		s.lock.Unlock()
		LogSessions.OK("%s: New session started", s.LogID())
	}
	ss.lock.Lock()
	defer ss.lock.Unlock()
	return ss.sessions[sessionID]
}

func (ss *Sessions) Remove(sessionID, reason string) {
	if ss.Has(sessionID) {
		ss.lock.Lock()
		defer ss.lock.Unlock()
		s := ss.sessions[sessionID]
		tw := 0
		cid := colorConnID("", s.Host, s.Port)
		if s.Orphan {
			tw = int(s.ActiveFor().Seconds())
			cid += " (orphan)"
		} else {
			tw = int(s.Uptime().Seconds())
		}
		s.lock.Lock()
		s.UpdateActivity("remove")
		s.lock.Unlock()

		if reason == "" {
			LogSessions.OK("%s: Session removed (was active for %s)", cid, colorDuration(uint(tw)))
		} else {
			LogSessions.OK("%s: Session removed because %s (was active for %s)", cid, colorReason(reason), colorDuration(uint(tw)))
		}
		delete(ss.sessions, sessionID)
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
