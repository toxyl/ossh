package main

import (
	"errors"
	"strings"
	"sync"
)

type SyncNodes struct {
	nodes   map[string]*SyncNode
	clients map[string]*SyncClient
	lock    *sync.Mutex
}

func (sn *SyncNodes) HasClient(clientID string) bool {
	sn.lock.Lock()
	defer sn.lock.Unlock()
	if _, ok := sn.clients[clientID]; !ok {
		return false
	}
	return true
}

func (sn *SyncNodes) AddClient(client *SyncClient) {
	if sn.HasClient(client.ID()) {
		return
	}
	sn.lock.Lock()
	defer sn.lock.Unlock()
	sn.clients[client.ID()] = client
}

func (sn *SyncNodes) GetClient(clientID string) (*SyncClient, error) {
	if !sn.HasClient(clientID) {
		return nil, errors.New("Client not found")
	}
	sn.lock.Lock()
	defer sn.lock.Unlock()
	return sn.clients[clientID], nil
}

func (sn *SyncNodes) Has(host string) bool {
	sn.lock.Lock()
	defer sn.lock.Unlock()
	if _, ok := sn.nodes[host]; !ok {
		return false
	}
	return true
}

func (sn *SyncNodes) IsAllowedHost(host string) bool {
	sn.lock.Lock()
	defer sn.lock.Unlock()
	for _, c := range sn.clients {
		if c.Host == host {
			return true
		}
	}
	return false
}

func (sn *SyncNodes) Add(host string, node *SyncNode) {
	if sn.Has(host) {
		return
	}
	sn.lock.Lock()
	defer sn.lock.Unlock()
	sn.nodes[host] = node
}

func (sn *SyncNodes) Get(host string) (*SyncNode, error) {
	if !sn.Has(host) {
		return nil, errors.New("Node not found")
	}
	sn.lock.Lock()
	defer sn.lock.Unlock()
	return sn.nodes[host], nil
}

// ExecBroadcast runs the command on all known nodes and returns
// a map with the results indexed on node IDs ("ip:port").
func (sn *SyncNodes) ExecBroadcast(command string) map[string]string {
	sn.lock.Lock()
	defer sn.lock.Unlock()
	res := map[string]string{}
	for _, c := range sn.clients {
		r, err := c.Exec(command)
		if err == nil && r != "" {
			res[c.ID()] = r
		}
	}
	return res
}

// Exec runs the command on all known nodes and
// immediately returns once it gets a non-empty result
// from a node. If all nodes have been tried without
// success the return will be an empty string.
func (sn *SyncNodes) Exec(command string) string {
	sn.lock.Lock()
	defer sn.lock.Unlock()
	for _, c := range sn.clients {
		r, err := c.Exec(command)
		if err == nil && strings.TrimSpace(r) != "" {
			return r
		}
		if err != nil {
			LogSyncServer.Error("Failed to exec command %s on node %s: %s", colorHighlight(command), colorHost(c.ID()), colorError(err))
		}
	}
	return ""
}

func NewSyncNodes() *SyncNodes {
	return &SyncNodes{
		nodes:   map[string]*SyncNode{},
		clients: map[string]*SyncClient{},
		lock:    &sync.Mutex{},
	}
}
