package main

import (
	"errors"
	"strings"
	"sync"

	"github.com/toxyl/glog"
	"github.com/toxyl/gutils"
)

type SyncNodeStats struct {
	Hosts            int     `json:"hosts"`
	Passwords        int     `json:"passwords"`
	Users            int     `json:"users"`
	Payloads         int     `json:"payloads"`
	Sessions         int     `json:"sessions"`
	AttemptedLogins  uint    `json:"logins_attempted"`
	SuccessfulLogins uint    `json:"logins_successful"`
	FailedLogins     uint    `json:"logins_failed"`
	TimeWasted       float64 `json:"time_wasted"`
	Uptime           float64 `json:"uptime"`
}
type SyncNode struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

func (sn *SyncNode) LogID() string {
	return colorConnID("", sn.Host, sn.Port)
}

type SyncNodes struct {
	nodes   map[string]*SyncNode
	stats   map[string]*SyncNodeStats
	clients map[string]*SyncClient
	logger  *glog.Logger
	lock    *sync.Mutex
}

func (sn *SyncNodes) GetClient(clientID string) (*SyncClient, error) {
	if !sn.HasClient(clientID) {
		return nil, errors.New("Client not found")
	}
	return sn.clients[clientID], nil
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

func (sn *SyncNodes) RemoveClient(id string) {
	if !sn.HasClient(id) {
		return
	}
	sn.lock.Lock()
	defer sn.lock.Unlock()
	delete(sn.clients, id)
}

func (sn *SyncNodes) AddStats(id string, stats *SyncNodeStats) {
	sn.lock.Lock()
	defer sn.lock.Unlock()
	sn.stats[id] = stats
}

// GetStats returns a SyncNodeStats struct with
// the total of all SyncNodes + this oSSH instance.
func (sn *SyncNodes) GetStats(hostStats *SyncNodeStats) *SyncNodeStats {
	sn.lock.Lock()
	defer sn.lock.Unlock()

	total := &SyncNodeStats{
		Hosts:            0,
		Passwords:        0,
		Users:            0,
		Payloads:         0,
		Sessions:         0,
		AttemptedLogins:  0,
		SuccessfulLogins: 0,
		FailedLogins:     0,
		TimeWasted:       0,
		Uptime:           0,
	}
	stats := sn.stats
	stats["_"] = hostStats // we should include ourselves

	for _, s := range stats {
		total.Hosts = gutils.MaxOfInts(total.Hosts, s.Hosts)
		total.Passwords = gutils.MaxOfInts(total.Passwords, s.Passwords)
		total.Users = gutils.MaxOfInts(total.Users, s.Users)
		total.Payloads = gutils.MaxOfInts(total.Payloads, s.Payloads)
		total.Sessions = gutils.SumOfInts(total.Sessions, s.Sessions)
		total.AttemptedLogins = gutils.SumOfUints(total.AttemptedLogins, s.AttemptedLogins)
		total.FailedLogins = gutils.SumOfUints(total.FailedLogins, s.FailedLogins)
		total.SuccessfulLogins = gutils.SumOfUints(total.SuccessfulLogins, s.SuccessfulLogins)
		total.TimeWasted = gutils.SumOfFloats(total.TimeWasted, s.TimeWasted)
		total.Uptime = gutils.SumOfFloats(total.Uptime, s.Uptime)
	}

	return total
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
	if node == nil || sn.Has(host) {
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
	res := map[string]string{}
	for _, c := range sn.clients {
		r, err := c.Exec(command)
		if err == nil && strings.TrimSpace(r) != "" {
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
	for _, c := range sn.clients {
		r, err := c.Exec(command)
		if err == nil && strings.TrimSpace(r) != "" {
			return r
		}
		if err != nil {
			sn.logger.Error("%s: Failed to exec command %s: %s", c.LogID(), glog.Highlight(command), glog.Error(err))
		}
	}
	return ""
}

func NewSyncNodes() *SyncNodes {
	return &SyncNodes{
		nodes:   map[string]*SyncNode{},
		clients: map[string]*SyncClient{},
		stats:   map[string]*SyncNodeStats{},
		logger:  glog.NewLogger("Sync Server", glog.DarkRed, Conf.Debug.SyncServer, false, false, logMessageHandler),
		lock:    &sync.Mutex{},
	}
}
