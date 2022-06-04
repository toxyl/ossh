package main

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
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
			msg = EncodeGzBase64String(msg)
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

func (ss *SyncServer) GetOutOfSyncNodes(fingerprint string) map[string]string {
	res := ss.Broadcast(fmt.Sprintf("SYNC %s", fingerprint))
	return res
}

func (ss *SyncServer) SyncToNodes() {
	time.Sleep(time.Duration(10) * time.Second)
	for {
		ss.busy = true
		fp := SrvOSSH.Loot.Fingerprint()
		fp = strings.TrimSpace(fp)
		LogSyncServer.Debug("Starting sync: %s", colorHighlight(fp))

		for k, v := range ss.GetOutOfSyncNodes(fp) {
			v = strings.TrimSpace(v)
			if v == "" {
				continue // node is already in sync
			}
			LogSyncServer.Debug("Node %s needs update: %s", colorHost(k), colorHighlight(v))
			sections := strings.Split(v, ",")
			parts := strings.Split(k, ":")
			client, err := ss.GetClient(parts[0], StringToInt(parts[1], 0))
			if err != nil {
				LogSyncServer.Error("Failed to get client %s: %s", colorHost(k), colorError(err))
				continue
			}

			for _, section := range sections {
				LogSyncServer.Debug("Sending %s to %s", colorHighlight(section), colorHost(k))
				switch section {
				case "hosts":
					client.SyncData("HOSTS", SrvOSSH.Loot.GetHosts, client.AddHosts)
				case "users":
					client.SyncData("USERS", SrvOSSH.Loot.GetUsers, client.AddUsers)
				case "passwords":
					client.SyncData("PASSWORDS", SrvOSSH.Loot.GetPasswords, client.AddPasswords)
				case "payloads":
					client.SyncData("PAYLOADS", SrvOSSH.Loot.GetPayloads, client.AddPayload)
				}
			}
		}
		LogSyncServer.Debug("Sync complete!")
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

		if ss.busy {
			_, _ = conn.Write([]byte(EmptyCommandResponse))
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
