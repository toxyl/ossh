package main

import (
	"errors"
	"sync"
)

type KnownNodes struct {
	nodes map[string]*SyncNode
	lock  *sync.Mutex
}

func (kn *KnownNodes) Has(host string) bool {
	kn.lock.Lock()
	defer kn.lock.Unlock()
	if _, ok := kn.nodes[host]; !ok {
		return false
	}
	return true
}

func (kn *KnownNodes) Add(host string, node *SyncNode) {
	if kn.Has(host) {
		return
	}
	kn.lock.Lock()
	defer kn.lock.Unlock()
	kn.nodes[host] = node
}

func (kn *KnownNodes) Get(host string) (*SyncNode, error) {
	if !kn.Has(host) {
		return nil, errors.New("Node not found")
	}
	kn.lock.Lock()
	defer kn.lock.Unlock()
	return kn.nodes[host], nil
}

func NewKnownNodes() *KnownNodes {
	return &KnownNodes{
		nodes: map[string]*SyncNode{},
		lock:  &sync.Mutex{},
	}
}
