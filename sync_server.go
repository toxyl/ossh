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

type SyncServerConnection struct {
	conn net.Conn
}

func (ssc *SyncServerConnection) close() {
	if ssc.conn != nil {
		ssc.conn.Close()
	}
}

func (ssc *SyncServerConnection) write(msg string) {
	if ssc.conn != nil {
		msg = strings.TrimSpace(msg)
		if msg != "" {
			msg = EncodeGzBase64String(msg)
		}
		_, _ = ssc.conn.Write([]byte(msg))
	}
}

func (ssc *SyncServerConnection) process(cmd string) {
	str := strings.Split(cmd, " ")

	if len(str) <= 0 {
		ssc.write(InvalidCommand)
		return
	}

	command := str[0]

	if _, ok := SyncCommands[command]; ok {
		res, err := SyncCommands[command](ssc, str[1:])
		if err != nil {
			LogSyncServer.Error("Command %s failed: %s", colorHighlight(command), colorError(err))
			ssc.write(EmptyCommandResponse)
			return
		}
		ssc.write(res)
	}
}

func (ssc *SyncServerConnection) handleConnection() {
	defer ssc.close()

	s := bufio.NewScanner(ssc.conn)
	for s.Scan() {
		data := s.Text()
		data, err := DecodeGzBase64String(data)
		if err != nil {
			LogSyncServer.Error("Could not decode input: %s", colorError(err))
			return
		}

		if data == EmptyCommandResponse {
			ssc.write(EmptyCommandResponse)
			continue
		}

		if data == "exit" {
			return
		}

		ssc.process(data)
		return
	}
}

type SyncServer struct {
	listener net.Listener
	conns    map[string]*SyncServerConnection
	nodes    *SyncNodes
}

func (ss *SyncServer) close() {
	for _, v := range ss.conns {
		v.close()
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

func (ss *SyncServer) RemoveClient(host string, port int) {
	ss.nodes.RemoveClient(fmt.Sprintf("%s:%d", host, port))
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
	time.Sleep(time.Duration(GetRandomInt(10, 60)) * time.Second)
	for {
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

		time.Sleep(time.Duration(Conf.Sync.Interval) * time.Minute)
	}
}

func (ss *SyncServer) UpdateClients() {
	// remove existing clients
	for _, c := range ss.nodes.clients {
		c.conn.Close()
		ip := c.Host
		port := c.Port
		index := -1
		for i, v := range Conf.IPWhitelist {
			if v == ip {
				index = i
				break
			}
		}
		if index >= 0 {
			Conf.IPWhitelist = append(Conf.IPWhitelist[:index], Conf.IPWhitelist[index+1:]...)
		}

		ss.RemoveClient(ip, port)
	}

	// add clients
	for _, node := range Conf.Sync.Nodes {
		if node.Host != Conf.SyncServer.Host || node.Port != int(Conf.SyncServer.Port) {
			Conf.IPWhitelist = append(Conf.IPWhitelist, node.Host)
			LogSyncServer.Debug("adding client: %s:%s", colorHost(node.Host), colorInt(node.Port))
			ss.nodes.AddClient(NewSyncClient(node.Host, node.Port))
		}
	}
}

func (ss *SyncServer) Start() {
	ss.UpdateClients()
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

		host, port, err := net.SplitHostPort(conn.RemoteAddr().String())
		if err != nil {
			LogSyncServer.Error("Could not process remote address: %s", colorError(err))
			conn.Close()
			continue
		}

		sid := fmt.Sprintf("%s:%s", host, port)

		if _, ok := ss.conns[sid]; !ok {
			ss.conns[sid] = &SyncServerConnection{
				conn: conn,
			}
		}

		if !ss.nodes.IsAllowedHost(host) {
			LogSyncServer.NotOK("%s is not a sync node, I'll give a bullshit response.", colorHost(host))
			ss.conns[sid].write(GenerateGarbageString(1000))
			ss.conns[sid].close()
			continue
		}

		ss.conns[sid].write(EmptyCommandResponse)

		go ss.conns[sid].handleConnection()
	}
}

func NewSyncServer() *SyncServer {
	ss := &SyncServer{
		listener: nil,
		nodes:    NewSyncNodes(),
		conns:    map[string]*SyncServerConnection{},
	}
	return ss
}
