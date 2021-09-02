package main

import (
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rs/xid"
	"net/http"
	"os"
	"regexp"
	"sync"
	"time"
)

var (
	wsupgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	serverConnections = make(map[string]map[string]*Connection)
	connectionsMutex  sync.Mutex

	serverErrors      = make(map[string][]byte)
	serverErrorsMutex sync.Mutex
)

type Connection struct {
	websocket.Conn
	Mutex sync.Mutex
}

func handleSocket(w http.ResponseWriter, r *http.Request, c *gin.Context) {
	conn, err := wsupgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Warning("Failed to set websocket upgrade: " + err.Error())
		log.Debug(r.Header)
		return
	}

	server := c.Query("server")
	rgx := regexp.MustCompile(`(?m)^c\d+s\d+$`)
	if !rgx.MatchString(server) {
		_ = conn.Close()
		return
	}
	connectionID := xid.New().String()

	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))

	if os.Getenv(server) == "" {
		log.Debug("Rejected connection to " + server + " as no token is defined")
		_ = conn.WriteMessage(websocket.TextMessage, []byte("null")) // Just a small update telling the client there is no data
		_ = conn.Close()
		return
	}

	serverErrorsMutex.Lock()
	e, ok := serverErrors[server]
	serverErrorsMutex.Unlock()

	if ok && e != nil {
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		_ = conn.WriteMessage(websocket.TextMessage, e)
	}

	connectionsMutex.Lock()
	if serverConnections[server] == nil {
		serverConnections[server] = make(map[string]*Connection)
	}
	serverConnections[server][connectionID] = &Connection{
		Conn: *conn,
	}
	connectionsMutex.Unlock()

	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer func() {
			ticker.Stop()
			killConnection(server, connectionID)
		}()
		for {
			select {
			case <-ticker.C:
				_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					killConnection(server, connectionID)
					return
				}
			}
		}
	}()
}

func broadcastToSocket(server string, data []byte) {
	connectionsMutex.Lock()
	connections, ok := serverConnections[server]
	if !ok || len(connections) == 0 {
		connectionsMutex.Unlock()
		return
	}

	for id, conn := range serverConnections[server] {
		if conn != nil {
			conn.Mutex.Lock()
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			_ = conn.WriteMessage(websocket.TextMessage, data)
			conn.Mutex.Unlock()
		} else {
			delete(serverConnections[server], id)
		}
	}
	connectionsMutex.Unlock()
}

func killConnection(server string, connectionID string) {
	connectionsMutex.Lock()
	_, ok := serverConnections[server]
	if !ok {
		connectionsMutex.Unlock()
		return
	}

	conn := serverConnections[server][connectionID]
	delete(serverConnections[server], connectionID)
	connectionsMutex.Unlock()

	if conn == nil {
		return
	}

	ip := conn.RemoteAddr().String()

	_ = conn.Close()

	log.Info("Disconnected socket client " + ip)
}
