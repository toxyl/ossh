package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/exp/maps"
)

type LootJSON struct {
	Hosts     []string `json:"hosts"`
	Users     []string `json:"users"`
	Passwords []string `json:"passwords"`
	Payloads  []string `json:"payloads"`
}

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
	return true
}

func (l *Loot) AddUsers(users []string) int {
	l.lock.Lock()
	defer l.lock.Unlock()
	added := 0
	for _, u := range users {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if _, ok := l.users[u]; !ok {
			l.users[u] = true
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
	return true
}

func (l *Loot) AddPasswords(passwords []string) int {
	l.lock.Lock()
	defer l.lock.Unlock()
	added := 0
	for _, p := range passwords {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := l.passwords[p]; !ok {
			l.passwords[p] = true
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
	return true
}

func (l *Loot) AddHosts(hosts []string) int {
	l.lock.Lock()
	defer l.lock.Unlock()
	added := 0
	for _, h := range hosts {
		h = strings.TrimSpace(h)
		if h == "" || isIPWhitelisted(h) {
			continue
		}
		if _, ok := l.hosts[h]; !ok {
			l.hosts[h] = true
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
			m, err := FileModTime(p.file)
			if err == nil {
				res = append(res, fmt.Sprintf("%d-%s", m.UnixMilli(), p.hash))
			}
		}
	}
	return res
}

func (l *Loot) JSON() string {
	data := LootJSON{
		Hosts:     l.GetHosts(),
		Users:     l.GetUsers(),
		Passwords: l.GetPasswords(),
		Payloads:  l.GetPayloads(),
	}
	json, err := json.Marshal(data)
	if err != nil {
		LogOSSHServer.Error("Could not marshal sync data: %s", colorError(err))
		return ""
	}

	return string(json)
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
	return StringSliceToSha256(l.GetHosts())
}

func (l *Loot) FingerprintUsers() string {
	return StringSliceToSha256(l.GetUsers())
}

func (l *Loot) FingerprintPasswords() string {
	return StringSliceToSha256(l.GetPasswords())
}

func (l *Loot) FingerprintPayloads() string {
	return StringSliceToSha256(l.GetPayloads())
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
