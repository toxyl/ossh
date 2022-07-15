package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/toxyl/glog"
	"github.com/toxyl/gutils"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
)

var (
	upgrader = websocket.Upgrader{} // use default options
	newline  = []byte{'\n'}
)

// https://github.com/golang/go/issues/26918#issuecomment-974257205
type serverErrorLogWriter struct {
	logger *glog.Logger
}

func (selw *serverErrorLogWriter) Write(p []byte) (int, error) {
	m := string(p)
	// https://github.com/golang/go/issues/26918
	if strings.HasPrefix(m, "http: TLS handshake error") {
		return 0, nil // we don't care about these
	} else {
		selw.logger.Error("%s", m)
	}
	return len(p), nil
}

type Client struct {
	Hub  *Hub
	Conn *websocket.Conn
	Send chan []byte
}

func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.Send:
			_ = c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			_, _ = w.Write(message)

			n := len(c.Send)
			for i := 0; i < n; i++ {
				_, _ = w.Write(newline)
				_, _ = w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	Register   chan *Client
	unregister chan *Client
}

func (h *Hub) Broadcast(msg string) {
	h.broadcast <- []byte(msg)
}

func (h *Hub) Broadcastf(format string, data ...interface{}) {
	h.broadcast <- []byte(fmt.Sprintf(format, data...))
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.clients[client] = true
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.Send)
			}
		case message := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.Send <- message:
				default:
					close(client.Send)
					delete(h.clients, client)
				}
			}
		}
	}
}

func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte),
		Register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

type WebsocketStream struct {
	Hub *Hub
}

func NewWebsocketStream() *WebsocketStream {
	ws := &WebsocketStream{
		Hub: NewHub(),
	}
	go ws.Hub.Run()
	return ws
}

type UIServer struct {
	Handlers map[string]func(w http.ResponseWriter, r *http.Request)
	Host     string
	Port     int
	CertFile string
	KeyFile  string
	Stats    *WebsocketStream
	Console  *WebsocketStream
	server   *http.Server
	logger   *glog.Logger
}

func (uis *UIServer) AddHTMLHandler(path string, handler func(w http.ResponseWriter, r *http.Request)) *UIServer {
	if _, ok := uis.Handlers[path]; ok {
		return uis
	}
	uis.Handlers[path] = handler

	return uis
}

func (uis *UIServer) AddSubscriptionHandler(path string, hub *Hub) *UIServer {
	return uis.AddHTMLHandler(
		path,
		func(w http.ResponseWriter, r *http.Request) {
			upgrader := websocket.Upgrader{
				ReadBufferSize:  1024,
				WriteBufferSize: 1024,
			}
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				uis.logger.Default("Connection connection upgrade failed: %s", err)
				return
			}
			client := &Client{
				Hub:  hub,
				Conn: conn,
				Send: make(chan []byte, 256)}
			client.Hub.Register <- client

			go client.WritePump()
		},
	)
}

func (uis *UIServer) AddHandler(path string, messageHandler func(message []byte) []byte) *UIServer {
	if _, ok := uis.Handlers[path]; ok {
		return uis
	}
	uis.Handlers[path] = func(wc http.ResponseWriter, r *http.Request) {
		// Upgrade our raw HTTP connection to a websocket based one
		conn, err := upgrader.Upgrade(wc, r, nil)
		if err != nil {
			uis.logger.Error("Error during connection upgrade: %s", err.Error())
			return
		}
		defer conn.Close()

		// The event loop
		for {
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				if !strings.Contains(err.Error(), "close 1000 (normal)") &&
					!strings.Contains(err.Error(), "close 1001 (going away)") {
					uis.logger.Error("Error during message reading: %s", err.Error())
				}
				break
			}

			message = messageHandler(message)
			err = conn.WriteMessage(messageType, message)
			if err != nil {
				uis.logger.Error("Error during message writing: %s", err.Error())
				break
			}
		}
	}

	return uis
}

func (uis *UIServer) MakeHTMLHandler(template string, data interface{}) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var tpl bytes.Buffer
		err := ParseTemplateHTML(template, &tpl, data)
		if err != nil {
			uis.logger.Error("Failed to parse template: %s", err.Error())
			return
		}

		b := tpl.Bytes()
		_, _ = w.Write(b)
	}
}

func (uis *UIServer) PushStats(msg string) {
	if uis.Stats == nil || uis.Stats.Hub == nil {
		return
	}
	uis.Stats.Hub.Broadcast(msg)
}

func (uis *UIServer) PushLog(msg string) {
	if uis.Console == nil || uis.Console.Hub == nil {
		return
	}
	uis.Console.Hub.Broadcast(msg)
}

func (uis *UIServer) Serve() {
	err := uis.server.ListenAndServeTLS(uis.CertFile, uis.KeyFile)
	if !strings.Contains(err.Error(), "Server closed") {
		uis.logger.Error("Server stopped: %s", glog.Error(err))
	}
}

func (uis *UIServer) Start() {
	gutils.SleepSeconds(10)

	mux := http.NewServeMux()
	uis.init()
	srv := fmt.Sprintf("%s:%d", uis.Host, uis.Port)
	for k, v := range uis.Handlers {
		mux.HandleFunc(k, v)
	}

	if !gutils.FileExists(uis.CertFile) || !gutils.FileExists(uis.KeyFile) {
		err := gutils.GenerateSelfSignedCertificate("local.ossh", "oSSH", uis.KeyFile, uis.CertFile)
		if err != nil {
			panic(fmt.Sprintf("could not create self signed certificate for UI server: %s", err.Error()))
		}
	}

	uis.server = &http.Server{
		ErrorLog: log.New(&serverErrorLogWriter{logger: uis.logger}, "", 0),
		Addr:     srv,
		TLSConfig: &tls.Config{
			MinVersion:               tls.VersionTLS12,
			CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
			PreferServerCipherSuites: true,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
				tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_RSA_WITH_AES_256_CBC_SHA,
			},
		},
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler), 0),
	}

	uis.server.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		addr := gutils.RealAddr(req)

		if !isIPWhitelisted(addr) && addr != Conf.Host {
			rt := fmt.Sprintf("https://%s%s", addr, req.URL.Path)
			http.Redirect(w, req, rt, 307) // let's give them their request back
			uis.logger.OK("%s: Redirected request to source: %s", glog.Addr(req.RemoteAddr, true), glog.Highlight(req.URL.Path))
			return
		}
		mux.ServeHTTP(w, req)
	})

	uis.logger.Default("Starting UI server on %s...", glog.Wrap("https://"+srv, glog.BrightYellow))
	go uis.Serve()
}

func (uis *UIServer) Shutdown() {
	ctxShutDown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer func() {
		cancel()
	}()

	if err := uis.server.Shutdown(ctxShutDown); err != nil {
		uis.logger.Error("Shutdown failed: %s", glog.Error(err))
	}

	uis.Handlers = nil

	uis.logger.OK("Shutdown complete")
}

func (uis *UIServer) Reload() {
	uis.Shutdown()
	go uis.Start()
}

func (uis *UIServer) init() {
	uis.Host = Conf.Webinterface.Host
	uis.Port = int(Conf.Webinterface.Port)
	uis.CertFile = Conf.Webinterface.CertFile
	uis.KeyFile = Conf.Webinterface.KeyFile
	wsscheme := "ws"
	if uis.CertFile != "" && uis.KeyFile != "" {
		wsscheme = "wss"
	}
	uis.Handlers = map[string]func(w http.ResponseWriter, r *http.Request){}
	uis.Stats = NewWebsocketStream()
	uis.Console = NewWebsocketStream()

	uis.AddSubscriptionHandler("/stats", uis.Stats.Hub)
	uis.AddSubscriptionHandler("/console", uis.Console.Hub)
	uis.AddHandler("/config", func(config []byte) []byte {
		err := updateConfig(config)
		if err != nil {
			return nil
		}
		SrvUI.Reload()
		return nil
	})
	uis.AddHandler("/payloads", func(msg []byte) []byte {
		if string(msg) == "list" {
			return []byte(fmt.Sprintf("list:%s", strings.Join(SrvOSSH.Loot.GetPayloadsWithTimestamp(), ",")))
		}
		p, err := SrvOSSH.Loot.payloads.Get(string(msg))
		if err != nil {
			uis.logger.Error("Could not retrieve payload %s: %s", glog.Highlight(string(msg)), glog.Error(err))
			return nil
		}
		if p == nil {
			uis.logger.Error("Could not retrieve payload %s: %s", glog.Highlight(string(msg)), glog.Error(errors.New("no error reported")))
			return nil
		}
		if !p.Exists() {
			uis.logger.Warning("Could not find payload %s: %s", glog.Highlight(string(msg)), glog.Error(errors.New("file does not exist")))
			return nil
		}

		pl, err := p.Read()
		if err != nil {
			uis.logger.Error("Could not read payload %s: %s", glog.Highlight(string(msg)), glog.Error(err))
			return nil
		}
		return []byte(gutils.EncodeBase64String(pl))
	})
	uis.AddHTMLHandler("/",
		uis.MakeHTMLHandler("index", struct {
			Scheme         string
			Config         string
			HostName       string
			TerminalWidth  int
			TerminalHeight int
		}{
			Scheme:         wsscheme,
			Config:         getConfig(),
			HostName:       Conf.HostName,
			TerminalWidth:  fakeShellInitialWidth,
			TerminalHeight: fakeShellInitialHeight,
		}),
	)
}

func NewUIServer() *UIServer {
	if !Conf.Webinterface.Enabled {
		return nil
	}
	return &UIServer{
		Handlers: map[string]func(w http.ResponseWriter, r *http.Request){},
		Host:     "",
		Port:     0,
		CertFile: "",
		KeyFile:  "",
		Stats:    nil,
		Console:  nil,
		server:   nil,
		logger:   glog.NewLogger("UI Server", glog.Cyan, Conf.Debug.UIServer, false, false, logMessageHandler),
	}
}
