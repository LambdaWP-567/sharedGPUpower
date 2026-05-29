package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Hub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]struct{}
	log     *zap.Logger
}

func NewHub(log *zap.Logger) *Hub {
	return &Hub{
		clients: make(map[*websocket.Conn]struct{}),
		log:     log,
	}
}

func (hub *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		hub.log.Error("ws upgrade failed", zap.Error(err))
		return
	}
	defer conn.Close()

	hub.mu.Lock()
	hub.clients[conn] = struct{}{}
	hub.mu.Unlock()
	defer func() {
		hub.mu.Lock()
		delete(hub.clients, conn)
		hub.mu.Unlock()
	}()

	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (hub *Hub) Broadcast(event string, data any) {
	msg, err := json.Marshal(map[string]any{"event": event, "data": data})
	if err != nil {
		return
	}
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	for conn := range hub.clients {
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		conn.WriteMessage(websocket.TextMessage, msg)
	}
}

func (hub *Hub) StartPing(interval time.Duration) {
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for range t.C {
			hub.mu.RLock()
			for conn := range hub.clients {
				conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
				conn.WriteMessage(websocket.PingMessage, nil)
			}
			hub.mu.RUnlock()
		}
	}()
}
