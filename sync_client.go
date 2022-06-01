package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

type SyncClient struct {
	Host string
	Port int
	conn net.Conn
}

func (sc *SyncClient) connect() {
	c, err := net.Dial("tcp", sc.ID())
	if err != nil {
		if strings.Contains(err.Error(), "connect: connection refused") {
			LogSyncClient.Warning("%s: Node is probably %s (only investigate if every sync fails)", colorHost(sc.ID()), colorHighlight("busy syncing"))
			return
		}
		LogSyncClient.Error("%s: Failed to connect: %s", colorHost(sc.ID()), colorError(err))
		return
	}
	LogSyncClient.Debug("%s: connect", colorHost(sc.ID()))
	sc.conn = c
}

func (sc *SyncClient) write(msg string) {
	if sc.conn != nil {
		_ = sc.conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
		msg = strings.TrimSpace(msg)
		LogSyncClient.Debug("%s: write: %s", colorHost(sc.ID()), colorReason(msg))
		msg = EncodeGzBase64String(msg)
		LogSyncClient.Debug("%s: write: %s", colorHost(sc.ID()), colorHighlight(msg))
		fmt.Fprintf(sc.conn, msg+"\n")
	}
}

func (sc *SyncClient) exit() {
	LogSyncClient.Debug("%s: exit", colorHost(sc.ID()))
	sc.write("exit")
}

func (sc *SyncClient) Exec(command string) (string, error) {
	sc.connect()
	if sc.conn == nil {
		return "", errors.New("not connected")
	}
	sc.write(command)
	defer sc.exit()

	_ = sc.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	reader := bufio.NewReader(sc.conn)
	resp, err := reader.ReadString('\n')

	if err != nil && err != io.EOF {
		LogSyncClient.Debug("\"%s\", failed to read response: %s", colorHighlight(command), colorError(err))
		return "", err
	}
	resp = strings.TrimSpace(resp)
	LogSyncClient.Debug("%s: read: %s", colorHost(sc.ID()), colorReason(resp))
	if resp == "" {
		return "", nil
	}
	resp, err = DecodeGzBase64String(resp)
	LogSyncClient.Debug("%s: read: %s", colorHost(sc.ID()), colorHighlight(resp))
	if err != nil {
		LogSyncClient.Debug("%s: Decoding %s failed: %s", colorHost(sc.ID()), colorHighlight(command), colorError(err))
		LogSyncClient.Debug("%s: Response was: %s", colorHost(sc.ID()), colorHighlight(resp))
	}

	return resp, nil
}

func (sc *SyncClient) ID() string {
	return fmt.Sprintf("%s:%d", sc.Host, sc.Port)
}

func (sc *SyncClient) AddHosts(hosts string) {
	chunks := ChunkString(sc.Host, " ", 10)
	for _, chunk := range chunks {
		_, _ = sc.Exec(fmt.Sprintf("ADD-HOST %s", chunk))
	}
}

func (sc *SyncClient) AddUsers(users string) {
	chunks := ChunkString(users, " ", 10)
	for _, chunk := range chunks {
		_, _ = sc.Exec(fmt.Sprintf("ADD-USER %s", chunk))
	}
}

func (sc *SyncClient) AddPasswords(passwords string) {
	chunks := ChunkString(passwords, " ", 10)
	for _, chunk := range chunks {
		_, _ = sc.Exec(fmt.Sprintf("ADD-PASSWORD %s", chunk))
	}
}

func (sc *SyncClient) AddPayload(fingerprint string) {
	pls := strings.Split(fingerprint, " ")
	cnt := 0
	for _, fp := range pls {
		fp = strings.TrimSpace(fp)
		if fp == "" {
			continue
		}

		if !SrvOSSH.Loot.payloads.Has(fp) {
			LogSyncClient.Info("%s: We can't send payload %s, we don't have it.", colorHost(sc.ID()), colorHighlight(fp))
			continue
		}

		pl, err := SrvOSSH.Loot.payloads.Get(fp)
		if err != nil {
			LogSyncClient.Error("%s: Looks like we can't give them the payload %s, we got an error retrieving it: %s", colorHost(sc.ID()), colorHighlight(fp), colorError(err))
			continue
		}

		if pl.Exists() {
			penc := pl.EncodeToString()
			if strings.TrimSpace(penc) == "" {
				continue
			}
			_, _ = sc.Exec(fmt.Sprintf("ADD-PAYLOAD %s %s", pl.hash, penc))
			cnt++
		}
	}
}

func (sc *SyncClient) SyncData(cmd string, fnGet func() []string, fnAddRemote func(data string)) {
	LogSyncClient.Debug("%s: syncing %s", colorHost(sc.ID()), colorHighlight(cmd))
	res, err := sc.Exec(cmd)
	if err != nil {
		// chances are that the node refused the connection because it's busy with syncing itself
		LogSyncClient.Debug("Failed to get %s: %s", colorHighlight(cmd), colorError(err))
		return
	}

	fnAddRemote(strings.Join(StringSliceDifference(fnGet(), ExplodeLines(res)), " "))
}

func NewSyncClient(host string, port int) *SyncClient {
	sc := &SyncClient{
		Host: host,
		Port: port,
	}
	return sc
}
