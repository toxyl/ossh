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

func (sc *SyncClient) connect() error {
	c, err := net.DialTimeout("tcp", sc.ID(), 10*time.Second)
	if err != nil {
		if strings.Contains(err.Error(), "connect: connection refused") {
			LogSyncClient.Debug("%s: Node is probably %s (only investigate if every sync fails)", colorHost(sc.Host), colorHighlight("busy syncing"))
			return fmt.Errorf("busy syncing")
		}
		if strings.Contains(err.Error(), "connect: connection timed out") {
			LogSyncClient.Debug("%s: Node is probably %s (only investigate if every sync fails)", colorHost(sc.Host), colorHighlight("down"))
			return fmt.Errorf("down")
		}
		if strings.Contains(err.Error(), "i/o timeout") {
			LogSyncClient.Warning("%s: Node is %s", colorHost(sc.Host), colorHighlight("unreachable"))
			return fmt.Errorf("unreachable")
		}
		LogSyncClient.Error("%s: Failed to connect: %s", colorHost(sc.ID()), colorError(err))
		return err
	}
	LogSyncClient.Debug("%s: connect", colorHost(sc.ID()))
	sc.conn = c
	return nil
}

func (sc *SyncClient) write(msg string) {
	if sc.conn != nil {
		_ = sc.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		msg = strings.TrimSpace(msg)
		msg = EncodeGzBase64String(msg)
		fmt.Fprintf(sc.conn, msg+"\n")
	}
}

func (sc *SyncClient) exit() {
	sc.write("exit")
}

func (sc *SyncClient) Exec(command string) (string, error) {
	err := sc.connect()
	if err != nil {
		return "", err
	}
	if sc.conn == nil {
		return "", errors.New("not connected")
	}
	sc.write(command)
	defer sc.exit()

	_ = sc.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	reader := bufio.NewReader(sc.conn)
	resp, err := reader.ReadString('\n')

	if err != nil && err != io.EOF {
		LogSyncClient.Debug("\"%s\", failed to read response: %s", colorHighlight(command), colorError(err))
		return "", err
	}
	resp = strings.TrimSpace(resp)
	if resp == "" {
		return "", nil
	}
	resp, err = DecodeGzBase64String(resp)
	if err != nil {
		LogSyncClient.Debug("%s: Decoding %s failed: %s", colorHost(sc.ID()), colorHighlight(command), colorError(err))
		LogSyncClient.Debug("%s: Response was: %s", colorHost(sc.ID()), colorHighlight(resp))
		return "", nil
	}

	return resp, nil
}

func (sc *SyncClient) ID() string {
	return fmt.Sprintf("%s:%d", sc.Host, sc.Port)
}

func (sc *SyncClient) AddHosts(hosts string) {
	chunks := ChunkString(hosts, " ", 5000)
	for _, chunk := range chunks {
		_, _ = sc.Exec(fmt.Sprintf("ADD-HOST %s", strings.Join(chunk, " ")))
	}
}

func (sc *SyncClient) AddUsers(users string) {
	chunks := ChunkString(users, " ", 5000)
	for _, chunk := range chunks {
		_, _ = sc.Exec(fmt.Sprintf("ADD-USER %s", strings.Join(chunk, " ")))
	}
}

func (sc *SyncClient) AddPasswords(passwords string) {
	chunks := ChunkString(passwords, " ", 5000)
	for _, chunk := range chunks {
		_, _ = sc.Exec(fmt.Sprintf("ADD-PASSWORD %s", strings.Join(chunk, " ")))
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
