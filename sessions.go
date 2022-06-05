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
	CreatedAt   time.Time
	ID          string
	Type        string
	Shell       *FakeShell
	SSHSession  *ssh.Session
	Term        string
	User        string
	Password    string
	Host        string
	Port        int
	Whitelisted bool
	lock        *sync.Mutex
}

func (s *Session) SetID(id string) *Session {
	s.lock.Lock()
	defer s.lock.Unlock()
	ip, port, err := net.SplitHostPort(id)
	if err != nil {
		LogOSSHServer.Error("Invalid session ID %s: %s\nNote: format must be 'host:port'!", colorReason(id), colorError(err))
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
	return s
}

func (s *Session) SetShell(shell *FakeShell) *Session {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.Shell = shell
	return s
}

func (s *Session) SetSSHSession(sshSession *ssh.Session) *Session {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.SSHSession = sshSession
	return s
}

func (s *Session) SetTerm(term string) *Session {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.Term = term
	return s
}

func (s *Session) SetUser(user string) *Session {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.User = user
	return s
}

func (s *Session) SetPassword(password string) *Session {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.Password = password
	return s
}

func (s *Session) SetHost(host string) *Session {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.Host = host
	s.updateID()
	return s
}

func (s *Session) SetPort(port int) *Session {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.Port = port
	s.updateID()
	return s
}

func (s *Session) LogID() string {
	return colorConnID(s.User, s.Host, s.Port)
}

func (s *Session) Uptime() time.Duration {
	return time.Since(s.CreatedAt)
}

// Expire checks if the session exists and is older than the given age.
// It will then exit the session with code -1 and close the connection.
// The function returns true if the session is expired, else false.
func (s *Session) Expire(age uint) bool {
	if s.SSHSession == nil {
		return true // maybe it was never established
	}
	if s.Uptime().Seconds() > float64(age) {
		LogOSSHServer.Info("%s: Expiring session...", s.LogID())
		_ = (*s.SSHSession).Exit(-1) // clean up
		return true
	}
	return false
}

func NewSession() *Session {
	s := &Session{
		CreatedAt:  time.Now(),
		ID:         "",
		Shell:      nil,
		SSHSession: nil,
		User:       "",
		Password:   "",
		Host:       "",
		Port:       0,
		Term:       "",
		lock:       &sync.Mutex{},
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
	}
	ss.lock.Lock()
	defer ss.lock.Unlock()
	return ss.sessions[sessionID]
}

func (ss *Sessions) Remove(sessionID string) {
	if ss.Has(sessionID) {
		ss.lock.Lock()
		defer ss.lock.Unlock()
		s := ss.sessions[sessionID]
		tw := int(s.Uptime().Seconds())
		LogOSSHServer.OK("%s is gone, it wasted %s",
			s.LogID(),
			colorDuration(uint(tw)),
		)
		SrvOSSH.TimeWasted += tw
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
			ss.Remove(sessionID)
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
