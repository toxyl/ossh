package main

import (
	"fmt"
	"net"
	"strconv"
	"sync"

	"github.com/gliderlabs/ssh"
)

type Session struct {
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
		LogErrorLn("Invalid session ID %s: %s\nNote: format must be 'host:port'!", colorReason(id), colorError(err))
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
	return fmt.Sprintf(
		"%s@%s:%s",
		colorUser(s.User),
		colorHost(s.Host),
		colorInt(s.Port),
	)
}

func NewSession() *Session {
	s := &Session{
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
	ss.lock.Lock()
	defer ss.lock.Unlock()
	delete(ss.sessions, sessionID)
}

func (ss *Sessions) Count() int {
	ss.lock.Lock()
	defer ss.lock.Unlock()
	return len(ss.sessions)
}

func NewActiveSessions(autoCreate bool) *Sessions {
	return &Sessions{
		sessions: map[string]*Session{},
		lock:     &sync.Mutex{},
	}
}
