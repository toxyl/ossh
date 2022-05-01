package main

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
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

type WebinterfaceServer struct {
	Handlers map[string]func(w http.ResponseWriter, r *http.Request)
	Host     string
	Port     int
	CertFile string
	KeyFile  string
	Stats    *WebsocketStream
	Console  *WebsocketStream
}

func (w *WebinterfaceServer) AddHTMLHandler(path string, handler func(w http.ResponseWriter, r *http.Request)) *WebinterfaceServer {
	LogDefault("[UI] Adding HTML handler for path '%s'...\n", path)
	if _, ok := w.Handlers[path]; ok {
		return w
	}
	w.Handlers[path] = handler

	return w
}

func (w *WebinterfaceServer) AddSubscriptionHandler(path string, hub *Hub) *WebinterfaceServer {
	LogDefault("[UI] Adding Subscription handler for path '%s'...\n", path)
	return w.AddHTMLHandler(
		path,
		func(w http.ResponseWriter, r *http.Request) {
			upgrader := websocket.Upgrader{
				ReadBufferSize:  1024,
				WriteBufferSize: 1024,
			}
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				LogDefault("[UI] Connection connection upgrade failed: %s\n", err)
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

func (w *WebinterfaceServer) AddHandler(path string, messageHandler func(message []byte) []byte) *WebinterfaceServer {
	LogDefault("[UI] Adding handler for path '%s'...\n", path)
	if _, ok := w.Handlers[path]; ok {
		return w
	}
	w.Handlers[path] = func(wc http.ResponseWriter, r *http.Request) {
		// Upgrade our raw HTTP connection to a websocket based one
		conn, err := upgrader.Upgrade(wc, r, nil)
		if err != nil {
			LogError("[UI] Error during connection upgrade: %s\n", err.Error())
			return
		}
		defer conn.Close()

		// The event loop
		for {
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				if !strings.Contains(fmt.Sprintf("%s", err), "close 1000 (normal)") {
					LogError("[UI] Error during message reading: %s\n", err.Error())
				}
				break
			}

			message = messageHandler(message)
			err = conn.WriteMessage(messageType, message)
			if err != nil {
				LogError("[UI] Error during message writing: %s\n", err.Error())
				break
			}
		}
	}

	return w
}

func (w *WebinterfaceServer) MakeHTMLHandler(template string, data interface{}) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var tpl bytes.Buffer
		err := ParseTemplateHTML(template, &tpl, data)
		if err != nil {
			LogError("[UI] Failed to parse template: %s\n", err.Error())
			return
		}

		b := tpl.Bytes()
		_, _ = w.Write(b)
	}
}

func (ws *WebinterfaceServer) PushStats(msg string) {
	ws.Stats.Hub.Broadcast(msg)
}

func (ws *WebinterfaceServer) PushLog(msg string) {
	ws.Console.Hub.Broadcast(msg)
}

func (ws *WebinterfaceServer) Start() {
	srv := fmt.Sprintf("%s:%d", ws.Host, ws.Port)
	LogDefault("[UI] Preparing to start at %s...\n", srv)
	for k, v := range ws.Handlers {
		http.HandleFunc(k, v)
	}

	if ws.CertFile != "" && ws.KeyFile != "" {
		server := &http.Server{
			Addr: srv,
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
		LogDefault("[UI] Serving at %s...\n", srv)
		log.Fatal(server.ListenAndServeTLS(ws.CertFile, ws.KeyFile))
		return
	}
	LogDefault("[UI] Serving at %s...\n", srv)
	log.Fatal(http.ListenAndServe(srv, nil))
}

func NewWebinterfaceServer(host string, port int, certFile, keyFile string) *WebinterfaceServer {
	LogDefault("[UI] Creating server at %s:%d...\n", host, port)

	wsscheme := "ws"
	if certFile != "" && keyFile != "" {
		wsscheme = "wss"
	}

	wss := &WebinterfaceServer{
		Handlers: map[string]func(w http.ResponseWriter, r *http.Request){},
		Host:     host,
		Port:     port,
		CertFile: certFile,
		KeyFile:  keyFile,
		Stats:    NewWebsocketStream(),
		Console:  NewWebsocketStream(),
	}
	wss.AddSubscriptionHandler("/stats", wss.Stats.Hub)
	wss.AddSubscriptionHandler("/console", wss.Console.Hub)
	wss.AddHandler("/config", func(message []byte) []byte {
		// TODO: implement saving config and restarting oSSH
		return []byte("OK")
	})
	wss.AddHTMLHandler("/",
		wss.MakeHTMLHandler("index", struct {
			Scheme string
			Config string
		}{
			Scheme: wsscheme,
			Config: getConfig(),
		}),
	)

	return wss
}
