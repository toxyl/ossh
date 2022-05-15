package main

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
)

const InvalidCommand = "Command not recognized"
const EmptyCommandResponse = ""

type SyncServer struct {
	listener net.Listener
	conn     net.Conn
	nodes    *SyncNodes
	busy     bool
}

func (ss *SyncServer) close() {
	if ss.conn != nil {
		ss.conn.Close()
	}
}

func (ss *SyncServer) write(msg string) {
	if ss.conn != nil {
		msg = strings.TrimSpace(msg)
		if msg != "" {
			LogSyncServer.Debug("%s:%s: write: %s", colorHost(Conf.SyncServer.Host), colorInt(int(Conf.SyncServer.Port)), colorHighlight(msg))
			msg = EncodeGzBase64String(msg)
			LogSyncServer.Debug("%s:%s: write: %s", colorHost(Conf.SyncServer.Host), colorInt(int(Conf.SyncServer.Port)), colorHighlight(msg))
		}
		_, _ = ss.conn.Write([]byte(msg))
	}
}

func (ss *SyncServer) process(cmd string) {
	str := strings.Split(cmd, " ")

	if len(str) <= 0 {
		ss.write(InvalidCommand)
		return
	}

	command := str[0]

	if _, ok := SyncCommands[command]; ok {
		res, err := SyncCommands[command](str[1:])
		if err != nil {
			LogSyncServer.Error("Command %s failed: %s", colorHighlight(command), colorError(err))
			ss.write(EmptyCommandResponse)
			return
		}
		LogSyncServer.Debug("Command %s => %s", colorHighlight(command), colorHighlight(res))
		ss.write(res)
	}
}

func (ss *SyncServer) handleConnection() {
	defer ss.close()

	s := bufio.NewScanner(ss.conn)
	for s.Scan() {
		data := s.Text()
		data, err := DecodeGzBase64String(data)
		if err != nil {
			LogSyncServer.Error("Could not decode input: %s", colorError(err))
			return
		}

		LogSyncServer.Debug("processing: %s", colorHighlight(data))

		if data == EmptyCommandResponse {
			ss.write(EmptyCommandResponse)
			continue
		}

		if data == "exit" {
			return
		}

		ss.process(data)
		return
	}
}

func (ss *SyncServer) HasNode(host string) bool {
	return ss.nodes.Has(host)
}

func (ss *SyncServer) GetNode(host string) (*SyncNode, error) {
	return ss.nodes.Get(host)
}

func (ss *SyncServer) GetClient(host string, port int) (*SyncClient, error) {
	cid := fmt.Sprintf("%s:%d", host, port)
	if ss.nodes.HasClient(cid) {
		return ss.nodes.GetClient(cid)
	}
	return nil, errors.New("sync client not found")
}

func (ss *SyncServer) AddClient(host string, port int) {
	if host == Conf.SyncServer.Host && port == int(Conf.SyncServer.Port) {
		return // so we don't accidentally add ourselves
	}
	ss.nodes.AddClient(NewSyncClient(host, port))
}

func (ss *SyncServer) Broadcast(msg string) map[string]string {
	return ss.nodes.ExecBroadcast(msg)
}

func (ss *SyncServer) Exec(msg string) string {
	return ss.nodes.Exec(msg)
}

func (ss *SyncServer) GetPayload(fingerprint string) string {
	return ss.Exec(fmt.Sprintf("GET-PAYLOAD %s", fingerprint))
}

func (ss *SyncServer) GetOutOfSyncNodes() map[string]string {
	res := ss.Broadcast(fmt.Sprintf("SYNC %s", SrvOSSH.Loot.Fingerprint()))
	LogSyncServer.Debug("out of sync nodes: %s", colorHighlight(spew.Sdump(res)))
	return res
}

func (ss *SyncServer) SyncToNodes() {
	time.Sleep(time.Duration(10) * time.Second)
	for {
		ss.busy = true
		fp := SrvOSSH.Loot.Fingerprint()
		LogSyncServer.Debug("syncing to nodes: %s", colorHighlight(fp))

		fpLocal := strings.Split(fp, ":")

		for k, v := range ss.GetOutOfSyncNodes() {
			if v == "" {
				continue // node is already in sync
			}
			LogSyncServer.Debug("syncing to node %s", colorHost(k))
			fpRemote := strings.Split(v, ":")
			parts := strings.Split(k, ":")
			client, err := ss.GetClient(parts[0], StringToInt(parts[1], 0))
			if err != nil {
				LogSyncServer.Error("Failed to get client %s: %s", colorHost(k), colorError(err))
				continue
			}

			if len(fpLocal) != 4 || len(fpRemote) != 4 {
				// probably something went wrong with the transmission,
				// let's ignore it for now
				continue
			}

			if fpLocal[0] != fpRemote[0] {
				LogSyncServer.Debug("%s are outdated", colorHighlight("hosts"))
				client.SyncData("HOSTS", SrvOSSH.Loot.GetHosts, client.AddHost)
			}

			if fpLocal[1] != fpRemote[1] {
				LogSyncServer.Debug("%s are outdated", colorHighlight("users"))
				client.SyncData("USERS", SrvOSSH.Loot.GetUsers, client.AddUser)
			}

			if fpLocal[2] != fpRemote[2] {
				LogSyncServer.Debug("%s are outdated", colorHighlight("passwords"))
				client.SyncData("PASSWORDS", SrvOSSH.Loot.GetPasswords, client.AddPassword)
			}

			if fpLocal[3] != fpRemote[3] {
				LogSyncServer.Debug("%s are outdated", colorHighlight("fingerprints"))
				client.SyncData("FINGERPRINTS", SrvOSSH.Loot.GetFingerprints, client.AddFingerprint)
			}
		}
		LogSyncServer.Debug("sync complete")
		ss.busy = false

		time.Sleep(time.Duration(Conf.Sync.Interval) * time.Minute)
	}
}

func (ss *SyncServer) Start() {
	// initialize sync clients
	for _, node := range Conf.Sync.Nodes {
		if node.Host != Conf.SyncServer.Host || node.Port != int(Conf.SyncServer.Port) {
			LogSyncServer.Debug("adding client: %s:%s", colorHost(node.Host), colorInt(node.Port))
			ss.nodes.AddClient(NewSyncClient(node.Host, node.Port))
		}
	}
	srv := fmt.Sprintf("%s:%d", Conf.SyncServer.Host, Conf.SyncServer.Port)
	LogSyncServer.Default("Starting sync server on %s...", colorWrap("tcp://"+srv, colorBrightYellow))
	listener, err := net.Listen("tcp", srv)
	if err != nil {
		panic(err)
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			LogSyncServer.Error("Accept failed: %s", colorError(err))
			conn.Close()
			continue
		}

		ss.conn = conn

		host, _, err := net.SplitHostPort(ss.conn.RemoteAddr().String())
		if err != nil {
			LogSyncServer.Error("Could not process remote address: %s", colorError(err))
			ss.close()
			continue
		}

		if !ss.nodes.IsAllowedHost(host) {
			LogSyncServer.NotOK("%s is not a sync node, I'll give a bullshit response.", colorHost(host))
			ss.write(GenerateGarbageString(1000))
			ss.close()
			continue
		}

		if ss.busy {
			LogSyncServer.Info("%s, don't you see I'm busy?", colorHost(host))
			ss.write(EmptyCommandResponse)
			ss.close()
			continue
		}

		ss.write(EmptyCommandResponse)

		go ss.handleConnection()
	}
}

func NewSyncServer() *SyncServer {
	ss := &SyncServer{
		listener: nil,
		conn:     nil,
		nodes:    NewSyncNodes(),
		busy:     false,
	}
	return ss
}
