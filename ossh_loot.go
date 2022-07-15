package main

import (
	"fmt"
	"strings"
	"sync"

	"github.com/toxyl/gutils"
	"golang.org/x/exp/maps"
)

type Loot struct {
	users     map[string]bool
	passwords map[string]bool
	hosts     map[string]bool
	payloads  *Payloads
	lock      *sync.Mutex
}

func (l *Loot) HasUser(user string) bool {
	l.lock.Lock()
	defer l.lock.Unlock()

	if _, ok := l.users[user]; !ok {
		return false
	}
	return true
}

func (l *Loot) HasPassword(password string) bool {
	l.lock.Lock()
	defer l.lock.Unlock()

	if _, ok := l.passwords[password]; !ok {
		return false
	}
	return true
}

func (l *Loot) HasHost(host string) bool {
	l.lock.Lock()
	defer l.lock.Unlock()

	if _, ok := l.hosts[host]; !ok {
		return false
	}
	return true
}

func (l *Loot) HasPayload(fingerprint string) bool {
	return l.payloads.Has(fingerprint)
}

func (l *Loot) AddUser(user string) bool {
	user = strings.TrimSpace(user)
	if user == "" {
		return false
	}

	if l.HasUser(user) {
		return false
	}
	l.lock.Lock()
	defer l.lock.Unlock()
	l.users[user] = true
	SrvMetrics.IncrementKnownUsers()
	return true
}

func (l *Loot) AddUsers(users []string) int {
	added := 0
	for _, u := range users {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if l.AddUser(u) {
			added++
		}
	}
	return added
}

func (l *Loot) AddPassword(password string) bool {
	password = strings.TrimSpace(password)
	if password == "" {
		return false
	}

	if l.HasPassword(password) {
		return false
	}
	l.lock.Lock()
	defer l.lock.Unlock()
	l.passwords[password] = true
	SrvMetrics.IncrementKnownPasswords()
	return true
}

func (l *Loot) AddPasswords(passwords []string) int {
	added := 0
	for _, p := range passwords {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if l.AddPassword(p) {
			added++
		}
	}
	return added
}

func (l *Loot) AddHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}

	if isIPWhitelisted(host) {
		return false // we never add whitelisted hosts
	}
	if l.HasHost(host) {
		return false
	}
	l.lock.Lock()
	defer l.lock.Unlock()
	l.hosts[host] = true
	SrvMetrics.IncrementKnownHosts()
	return true
}

func (l *Loot) AddHosts(hosts []string) int {
	added := 0
	for _, h := range hosts {
		h = strings.TrimSpace(h)
		if h == "" || isIPWhitelisted(h) {
			continue
		}
		if l.AddHost(h) {
			added++
		}
	}
	return added
}

func (l *Loot) AddPayload(fingerprint string) bool {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return false
	}

	if !l.HasPayload(fingerprint) {
		p := NewPayload()
		p.SetHash(fingerprint)
		l.payloads.Add(p)
		SrvMetrics.IncrementKnownPayloads()
	}
	return true
}

func (l *Loot) CountUsers() int {
	l.lock.Lock()
	defer l.lock.Unlock()
	return len(l.users)
}

func (l *Loot) CountPasswords() int {
	l.lock.Lock()
	defer l.lock.Unlock()
	return len(l.passwords)
}

func (l *Loot) CountHosts() int {
	l.lock.Lock()
	defer l.lock.Unlock()
	return len(l.hosts)
}

func (l *Loot) CountPayloads() int {
	l.lock.Lock()
	defer l.lock.Unlock()
	return len(l.payloads.GetKeys())
}

func (l *Loot) GetUsers() []string {
	l.lock.Lock()
	defer l.lock.Unlock()
	return maps.Keys(l.users)
}

func (l *Loot) GetPasswords() []string {
	l.lock.Lock()
	defer l.lock.Unlock()
	return maps.Keys(l.passwords)
}

func (l *Loot) GetHosts() []string {
	l.lock.Lock()
	defer l.lock.Unlock()
	return maps.Keys(l.hosts)
}

func (l *Loot) GetPayloads() []string {
	l.lock.Lock()
	defer l.lock.Unlock()
	res := []string{}
	for _, fp := range l.payloads.GetKeys() {
		p := NewPayload()
		p.SetHash(fp)
		if p.Exists() {
			res = append(res, p.hash)
		}
	}
	return res
}

func (l *Loot) GetPayloadsWithTimestamp() []string {
	l.lock.Lock()
	defer l.lock.Unlock()
	res := []string{}
	for _, fp := range l.payloads.GetKeys() {
		p := NewPayload()
		p.SetHash(fp)
		if p.Exists() {
			m, err := gutils.FileModTime(p.file)
			if err == nil {
				res = append(res, fmt.Sprintf("%d-%s", m.UnixMilli(), p.hash))
			}
		}
	}
	return res
}

func (l *Loot) Fingerprint() string {
	return fmt.Sprintf(
		"%s:%s:%s:%s",
		l.FingerprintHosts(),
		l.FingerprintUsers(),
		l.FingerprintPasswords(),
		l.FingerprintPayloads(),
	)
}

func (l *Loot) FingerprintHosts() string {
	return gutils.StringSliceToSha256(l.GetHosts())
}

func (l *Loot) FingerprintUsers() string {
	return gutils.StringSliceToSha256(l.GetUsers())
}

func (l *Loot) FingerprintPasswords() string {
	return gutils.StringSliceToSha256(l.GetPasswords())
}

func (l *Loot) FingerprintPayloads() string {
	return gutils.StringSliceToSha256(l.GetPayloads())
}

func NewLoot() *Loot {
	return &Loot{
		users:     map[string]bool{},
		passwords: map[string]bool{},
		hosts:     map[string]bool{},
		payloads:  NewPayloads(),
		lock:      &sync.Mutex{},
	}
}
