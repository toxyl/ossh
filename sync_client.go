package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
)

type SyncClient struct {
	Host string
	Port int
	conn net.Conn
}

func (sc *SyncClient) connect() {
	c, err := net.Dial("tcp", sc.ID())
	if err != nil {
		LogErrorLn("[Sync Client] Failed to connect: %s", colorError(err))
		return
	}
	sc.conn = c
}

func (sc *SyncClient) write(msg string) {
	if sc.conn != nil {
		fmt.Fprintf(sc.conn, EncodeBase64String(msg)+"\n")
	}
}

func (sc *SyncClient) exit() {
	sc.write("exit")
}

func (sc *SyncClient) Exec(command string) (string, error) {
	sc.connect()
	if sc.conn == nil {
		return "", errors.New("not connected")
	}
	sc.write(command)
	defer sc.exit()

	reader := bufio.NewReader(sc.conn)
	resp, err := reader.ReadString('\n')

	if err != nil && err != io.EOF {
		LogErrorLn("[Sync Client] \"%s\", failed to read response: %s", colorHighlight(command), colorError(err))
		return "", err
	}
	resp = strings.TrimSpace(resp)
	return DecodeBase64String(resp)
}

func (sc *SyncClient) ID() string {
	return fmt.Sprintf("%s:%d", sc.Host, sc.Port)
}

func (sc *SyncClient) AddHost(host string) {
	_, _ = sc.Exec(fmt.Sprintf("ADD-HOST %s", host))
}

func (sc *SyncClient) AddUser(user string) {
	_, _ = sc.Exec(fmt.Sprintf("ADD-USER %s", user))
}

func (sc *SyncClient) AddPassword(password string) {
	_, _ = sc.Exec(fmt.Sprintf("ADD-PASSWORD %s", password))
}

func (sc *SyncClient) AddFingerprint(fingerprint string) {
	_, _ = sc.Exec(fmt.Sprintf("ADD-FINGERPRINT %s", fingerprint))
}

func (sc *SyncClient) SyncData(cmd string, fnGet func() []string, fnAddRemote func(data string)) {
	res, err := sc.Exec(cmd)
	if err != nil {
		LogErrorLn("Failed to get %s: %s", colorHighlight(cmd), colorError(err))
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
