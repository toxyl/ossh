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
type serverErrorLogWriter struct{}

func (*serverErrorLogWriter) Write(p []byte) (int, error) {
	m := string(p)
	// https://github.com/golang/go/issues/26918
	if strings.HasPrefix(m, "http: TLS handshake error") {
		return 0, nil // we don't care about these
	} else {
		LogUIServer.Error("%s", m)
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
}

func (w *UIServer) AddHTMLHandler(path string, handler func(w http.ResponseWriter, r *http.Request)) *UIServer {
	if _, ok := w.Handlers[path]; ok {
		return w
	}
	w.Handlers[path] = handler

	return w
}

func (w *UIServer) AddSubscriptionHandler(path string, hub *Hub) *UIServer {
	return w.AddHTMLHandler(
		path,
		func(w http.ResponseWriter, r *http.Request) {
			upgrader := websocket.Upgrader{
				ReadBufferSize:  1024,
				WriteBufferSize: 1024,
			}
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				LogUIServer.Default("Connection connection upgrade failed: %s", err)
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

func (w *UIServer) AddHandler(path string, messageHandler func(message []byte) []byte) *UIServer {
	if _, ok := w.Handlers[path]; ok {
		return w
	}
	w.Handlers[path] = func(wc http.ResponseWriter, r *http.Request) {
		// Upgrade our raw HTTP connection to a websocket based one
		conn, err := upgrader.Upgrade(wc, r, nil)
		if err != nil {
			LogUIServer.Error("Error during connection upgrade: %s", err.Error())
			return
		}
		defer conn.Close()

		// The event loop
		for {
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				if !strings.Contains(err.Error(), "close 1000 (normal)") &&
					!strings.Contains(err.Error(), "close 1001 (going away)") {
					LogUIServer.Error("Error during message reading: %s", err.Error())
				}
				break
			}

			message = messageHandler(message)
			err = conn.WriteMessage(messageType, message)
			if err != nil {
				LogUIServer.Error("Error during message writing: %s", err.Error())
				break
			}
		}
	}

	return w
}

func (w *UIServer) MakeHTMLHandler(template string, data interface{}) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var tpl bytes.Buffer
		err := ParseTemplateHTML(template, &tpl, data)
		if err != nil {
			LogUIServer.Error("Failed to parse template: %s", err.Error())
			return
		}

		b := tpl.Bytes()
		_, _ = w.Write(b)
	}
}

func (ws *UIServer) PushStats(msg string) {
	if ws.Stats == nil || ws.Stats.Hub == nil {
		return
	}
	ws.Stats.Hub.Broadcast(msg)
}

func (ws *UIServer) PushLog(msg string) {
	if ws.Console == nil || ws.Console.Hub == nil {
		return
	}
	ws.Console.Hub.Broadcast(msg)
}

func (ws *UIServer) Serve() {
	err := ws.server.ListenAndServeTLS(ws.CertFile, ws.KeyFile)
	if !strings.Contains(err.Error(), "Server closed") {
		LogUIServer.Error("Server stopped: %s", glog.Error(err))
	}
}

func (ws *UIServer) Start() {
	gutils.SleepSeconds(10)

	mux := http.NewServeMux()
	ws.init()
	srv := fmt.Sprintf("%s:%d", ws.Host, ws.Port)
	for k, v := range ws.Handlers {
		mux.HandleFunc(k, v)
	}

	if !gutils.FileExists(ws.CertFile) || !gutils.FileExists(ws.KeyFile) {
		err := gutils.GenerateSelfSignedCertificate("local.ossh", "oSSH", ws.KeyFile, ws.CertFile)
		if err != nil {
			panic(fmt.Sprintf("could not create self signed certificate for UI server: %s", err.Error()))
		}
	}

	ws.server = &http.Server{
		ErrorLog: log.New(&serverErrorLogWriter{}, "", 0),
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

	ws.server.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		addr := gutils.RealAddr(req)

		if !isIPWhitelisted(addr) && addr != Conf.Host {
			rt := fmt.Sprintf("https://%s%s", addr, req.URL.Path)
			http.Redirect(w, req, rt, 307) // let's give them their request back
			LogUIServer.OK("%s: Redirected request to source: %s", glog.Addr(addr, true), glog.Highlight(req.URL.Path))
			return
		}
		mux.ServeHTTP(w, req)
	})

	LogUIServer.Default("Starting UI server on %s...", glog.Wrap("https://"+srv, glog.BrightYellow))
	go ws.Serve()
}

func (ws *UIServer) Shutdown() {
	ctxShutDown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer func() {
		cancel()
	}()

	if err := ws.server.Shutdown(ctxShutDown); err != nil {
		LogUIServer.Error("Shutdown failed: %s", glog.Error(err))
	}

	ws.Handlers = nil

	LogUIServer.OK("Shutdown complete")
}

func (ws *UIServer) Reload() {
	ws.Shutdown()
	go ws.Start()
}

func (ws *UIServer) init() {
	ws.Host = Conf.Webinterface.Host
	ws.Port = int(Conf.Webinterface.Port)
	ws.CertFile = Conf.Webinterface.CertFile
	ws.KeyFile = Conf.Webinterface.KeyFile
	wsscheme := "ws"
	if ws.CertFile != "" && ws.KeyFile != "" {
		wsscheme = "wss"
	}
	ws.Handlers = map[string]func(w http.ResponseWriter, r *http.Request){}
	ws.Stats = NewWebsocketStream()
	ws.Console = NewWebsocketStream()

	ws.AddSubscriptionHandler("/stats", ws.Stats.Hub)
	ws.AddSubscriptionHandler("/console", ws.Console.Hub)
	ws.AddHandler("/config", func(config []byte) []byte {
		err := updateConfig(config)
		if err != nil {
			return nil
		}
		SrvUI.Reload()
		return nil
	})
	ws.AddHandler("/payloads", func(msg []byte) []byte {
		if string(msg) == "list" {
			return []byte(fmt.Sprintf("list:%s", strings.Join(SrvOSSH.Loot.GetPayloadsWithTimestamp(), ",")))
		}
		p, err := SrvOSSH.Loot.payloads.Get(string(msg))
		if err != nil {
			LogUIServer.Error("Could not retrieve payload %s: %s", glog.Highlight(string(msg)), glog.Error(err))
			return nil
		}
		if p == nil {
			LogUIServer.Error("Could not retrieve payload %s: %s", glog.Highlight(string(msg)), glog.Error(errors.New("no error reported")))
			return nil
		}
		if !p.Exists() {
			LogUIServer.Warning("Could not find payload %s: %s", glog.Highlight(string(msg)), glog.Error(errors.New("file does not exist")))
			return nil
		}

		pl, err := p.Read()
		if err != nil {
			LogUIServer.Error("Could not read payload %s: %s", glog.Highlight(string(msg)), glog.Error(err))
			return nil
		}
		return []byte(gutils.EncodeBase64String(pl))
	})
	ws.AddHTMLHandler("/",
		ws.MakeHTMLHandler("index", struct {
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
	return &UIServer{
		Handlers: map[string]func(w http.ResponseWriter, r *http.Request){},
		Host:     "",
		Port:     0,
		CertFile: "",
		KeyFile:  "",
		Stats:    nil,
		Console:  nil,
		server:   nil,
	}
}
