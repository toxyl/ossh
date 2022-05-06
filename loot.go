package main

import (
	"encoding/json"
	"strings"
	"sync"

	"golang.org/x/exp/maps"
)

type LootJSON struct {
	Hosts        []string `json:"hosts"`
	Users        []string `json:"users"`
	Passwords    []string `json:"passwords"`
	Fingerprints []string `json:"fingerprints"`
}

type Loot struct {
	users        map[string]bool
	passwords    map[string]bool
	hosts        map[string]bool
	fingerprints map[string]bool
	payloads     *Payloads
	lock         *sync.Mutex
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

func (l *Loot) HasFingerprint(fingerprint string) bool {
	l.lock.Lock()
	defer l.lock.Unlock()

	if _, ok := l.fingerprints[fingerprint]; !ok {
		return false
	}
	return true
}

func (l *Loot) HasPayload(fingerprint string) bool {
	return l.payloads.Has(fingerprint)
}

func (l *Loot) AddUser(user string) {
	user = strings.TrimSpace(user)
	if user == "" {
		return
	}

	if l.HasUser(user) {
		return
	}
	l.lock.Lock()
	defer l.lock.Unlock()
	l.users[user] = true
}

func (l *Loot) AddPassword(password string) {
	password = strings.TrimSpace(password)
	if password == "" {
		return
	}

	if l.HasPassword(password) {
		return
	}
	l.lock.Lock()
	defer l.lock.Unlock()
	l.passwords[password] = true
}

func (l *Loot) AddHost(host string) {
	host = strings.TrimSpace(host)
	if host == "" {
		return
	}

	if isIPWhitelisted(host) {
		return // we never add whitelisted hosts
	}
	if l.HasHost(host) {
		return
	}
	l.lock.Lock()
	defer l.lock.Unlock()
	l.hosts[host] = true
}

func (l *Loot) AddFingerprint(fingerprint string) {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return
	}

	if l.HasFingerprint(fingerprint) {
		return
	}
	l.lock.Lock()
	defer l.lock.Unlock()
	l.fingerprints[fingerprint] = true

	l.AddPayload(fingerprint)
}

func (l *Loot) AddPayload(fingerprint string) {
	p := NewPayload()
	p.hash = fingerprint
	if !p.Exists() {
		if !p.Download(fingerprint) { // try to download from known nodes
			return
		}
	}
	l.payloads.Add(p)
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

func (l *Loot) CountFingerprints() int {
	l.lock.Lock()
	defer l.lock.Unlock()
	return len(l.fingerprints)
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

func (l *Loot) GetFingerprints() []string {
	l.lock.Lock()
	defer l.lock.Unlock()
	return maps.Keys(l.fingerprints)
}

func (l *Loot) JSON() string {
	data := LootJSON{
		Hosts:        maps.Keys(l.hosts),
		Users:        maps.Keys(l.users),
		Passwords:    maps.Keys(l.passwords),
		Fingerprints: maps.Keys(l.fingerprints),
	}
	json, err := json.Marshal(data)
	if err != nil {
		LogErrorLn("Could not marshal sync data: %s", colorError(err))
		return ""
	}

	return string(json)
}

func (l *Loot) Fingerprint() string {
	return StringToSha256(l.JSON())
}

func NewLoot() *Loot {
	return &Loot{
		users:        map[string]bool{},
		passwords:    map[string]bool{},
		hosts:        map[string]bool{},
		fingerprints: map[string]bool{},
		payloads:     NewPayloads(),
		lock:         &sync.Mutex{},
	}
}
