package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
)

type Payload struct {
	hash    string
	file    string
	payload string
}

func (p *Payload) Exists() bool {
	return FileExists(p.file)
}

func (p *Payload) Save() {
	if p.Exists() {
		return // no need to save, we already have this payload
	}

	err := os.WriteFile(p.file, []byte(p.payload), 0744)
	if err == nil {
		LogSuccessLn("Payload saved: %s", colorFile(p.file))
	}
}

func (p *Payload) Read() (string, error) {
	if !p.Exists() {
		return "", fmt.Errorf("%s was not found.", p.hash)
	}

	data, err := os.ReadFile(p.file)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func (p *Payload) DecodeFromString(encodedPayload string) bool {
	encodedPayload = strings.TrimSpace(encodedPayload)
	if encodedPayload == "" {
		return false
	}
	pd, err := base64.RawStdEncoding.DecodeString(encodedPayload)
	if err == nil {
		p.payload = strings.TrimSpace(string(pd))
	}
	return true
}

func (p *Payload) Download(hash string) bool {
	if SrvSync == nil {
		return false
	}
	res := SrvSync.GetPayload(hash)
	if res != "" {
		if p.DecodeFromString(res) {
			p.Save()
			return true
		}
	}
	return false
}

func (p *Payload) SetHash(hash string) {
	p.hash = hash
	p.file = fmt.Sprintf("%s/payload-%s.cast", Conf.PathCaptures, hash)
}

func (p *Payload) Set(payload string) {
	hash := StringToSha1(payload)
	p.SetHash(hash)
	p.payload = payload
}

func NewPayload() *Payload {
	return &Payload{
		hash:    "",
		file:    "",
		payload: "",
	}
}

type Payloads struct {
	payloads map[string]*Payload
	lock     *sync.Mutex
}

func (ps *Payloads) Has(sha1 string) bool {
	ps.lock.Lock()
	defer ps.lock.Unlock()
	if _, ok := ps.payloads[sha1]; !ok {
		return false
	}
	return true
}

func (ps *Payloads) Add(payload *Payload) {
	if ps.Has(payload.hash) {
		return
	}
	ps.lock.Lock()
	defer ps.lock.Unlock()
	ps.payloads[payload.hash] = payload
}

func (ps *Payloads) Get(sha1 string) (*Payload, error) {
	if !ps.Has(sha1) {
		return nil, errors.New("not found")
	}
	ps.lock.Lock()
	defer ps.lock.Unlock()
	return ps.payloads[sha1], nil
}

func NewPayloads() *Payloads {
	return &Payloads{
		payloads: map[string]*Payload{},
		lock:     &sync.Mutex{},
	}
}
