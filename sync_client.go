package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/toxyl/glog"
	"github.com/toxyl/gutils"
)

type SyncClient struct {
	Host   string
	Port   int
	conn   net.Conn
	logger *glog.Logger
	lock   *sync.Mutex
}

func (sc *SyncClient) LogID() string {
	if sc.conn != nil {
		lhost, lport := gutils.SplitHostPortFromAddr(sc.conn.LocalAddr())
		rhost, rport := gutils.SplitHostPortFromAddr(sc.conn.RemoteAddr())
		return fmt.Sprintf("%s -> %s", colorConnID("", lhost, lport), colorConnID("", rhost, rport))
	}
	return fmt.Sprintf("(not connected) %s", colorConnID("", sc.Host, sc.Port))
}

func (sc *SyncClient) connect() error {
	c, err := net.DialTimeout("tcp", sc.ID(), 10*time.Second)
	if err != nil {
		if strings.Contains(err.Error(), "connect: connection refused") {
			return errors.New("connection refused")
		}
		if strings.Contains(err.Error(), "connect: connection timed out") {
			return errors.New("timed out")
		}
		if strings.Contains(err.Error(), "connect: no route to host") || strings.Contains(err.Error(), "i/o timeout") {
			return errors.New("unreachable")
		}
		return err
	}

	sc.conn = c
	return nil
}

func (sc *SyncClient) disconnect() error {
	return sc.write(ExitCommand)
}

func (sc *SyncClient) write(msg string) error {
	if sc.conn == nil {
		return errors.New("not connected!")
	}
	_ = sc.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	msg = strings.TrimSpace(msg)
	msg = gutils.EncodeGzBase64String(msg)
	fmt.Fprintf(sc.conn, msg+"\n")
	return nil
}

func (sc *SyncClient) read() (string, error) {
	if sc.conn == nil {
		return "", errors.New("not connected!")
	}
	_ = sc.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	reader := bufio.NewReader(sc.conn)
	resp, err := reader.ReadString('\n')

	if err != nil && err != io.EOF {
		return "", err
	}
	resp = strings.TrimSpace(resp)
	if resp == "" {
		return "", nil
	}
	resp, err = gutils.DecodeGzBase64String(resp)
	if err != nil {
		return "", fmt.Errorf("Decoding failed: %s. Response was: %s.", glog.Error(err), glog.Highlight(resp))
	}

	return resp, nil
}

func (sc *SyncClient) Exec(command string) (string, error) {
	sc.lock.Lock()
	defer func() {
		_ = sc.disconnect()
		sc.lock.Unlock()
	}()

	err := sc.connect()
	if err != nil {
		return "", err
	}

	err = sc.write(command)
	if err != nil {
		return "", err
	}

	return sc.read()
}

func (sc *SyncClient) ID() string {
	return fmt.Sprintf("%s:%d", sc.Host, sc.Port)
}

func (sc *SyncClient) AddChunked(section, data string) {
	chunks := gutils.ChunkString(data, " ", 5000)
	for _, chunk := range chunks {
		_, _ = sc.Exec(fmt.Sprintf("ADD-%s %s", section, strings.Join(chunk, " ")))
	}
}

func (sc *SyncClient) AddHosts(hosts string) {
	sc.AddChunked("HOST", hosts)
}

func (sc *SyncClient) AddUsers(users string) {
	sc.AddChunked("USER", users)
}

func (sc *SyncClient) AddPasswords(passwords string) {
	sc.AddChunked("PASSWORD", passwords)
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
			sc.logger.Info("%s: We can't send payload %s, we don't have it.", sc.LogID(), glog.Highlight(fp))
			continue
		}

		pl, err := SrvOSSH.Loot.payloads.Get(fp)
		if err != nil {
			sc.logger.Error("%s: Looks like we can't give them the payload %s, we got an error retrieving it: %s", sc.LogID(), glog.Highlight(fp), glog.Error(err))
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
	sc.logger.Debug("%s: Syncing %s", sc.LogID(), glog.Highlight(cmd))
	res, err := sc.Exec(cmd)
	if err != nil {
		// chances are that the node refused the connection because it's busy with syncing itself
		sc.logger.Debug("%s: Failed to get %s: %s", sc.LogID(), glog.Highlight(cmd), glog.Error(err))
		return
	}

	fnAddRemote(strings.Join(gutils.StringSliceDifference(fnGet(), gutils.ExplodeLines(res)), " "))
}

func NewSyncClient(host string, port int) *SyncClient {
	sc := &SyncClient{
		Host:   host,
		Port:   port,
		logger: glog.NewLogger("Sync Client", glog.Blue, Conf.Debug.SyncClient, false, false, logMessageHandler),
		lock:   &sync.Mutex{},
	}
	return sc
}
