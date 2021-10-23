package main

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rs/xid"
	"net/http"
	"os"
	"regexp"
	"strings"
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

const (
	SocketTypeMap       = "map"
	SocketTypeStaffChat = "staff"
)

type Connection struct {
	*websocket.Conn
	Mutex   sync.Mutex
	Cluster string
	Type    string
}

func handleSocket(w http.ResponseWriter, r *http.Request, c *gin.Context, typ string) {
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

	cluster := c.Query("cluster")
	if !strings.HasPrefix(server, cluster) {
		log.Debug("Rejected connection to " + server + " as cluster is invalid ('" + server + "', '" + cluster + "')")
		b, _ := json.Marshal(InfoPackage{
			Status:  http.StatusUnauthorized,
			Message: "Cluster invalid",
		})

		_ = conn.WriteMessage(websocket.BinaryMessage, gzipBytes(b))
		_ = conn.Close()
		return
	}

	if os.Getenv(server) == "" {
		log.Debug("Rejected connection to " + server + " as no token is defined")
		b, _ := json.Marshal(InfoPackage{
			Status:  http.StatusNotFound,
			Message: "Not found (no token)",
		})

		_ = conn.WriteMessage(websocket.BinaryMessage, gzipBytes(b))
		_ = conn.Close()
		return
	}

	serverErrorsMutex.Lock()
	e, ok := serverErrors[server]
	serverErrorsMutex.Unlock()

	if ok && e != nil {
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		_ = conn.WriteMessage(websocket.BinaryMessage, gzipBytes(e))
	} else if typ == SocketTypeStaffChat {
		lastStaffChatMutex.Lock()
		b, ok := lastStaffChat[server]
		lastStaffChatMutex.Unlock()

		if ok {
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			_ = conn.WriteMessage(websocket.BinaryMessage, gzipBytes(b))
		}
	}

	connectionsMutex.Lock()
	if serverConnections[server] == nil {
		serverConnections[server] = make(map[string]*Connection)
	}
	connection := &Connection{
		Conn:    conn,
		Cluster: cluster,
		Type:    typ,
	}
	serverConnections[server][connectionID] = connection
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
				connection.Mutex.Lock()
				_ = connection.SetWriteDeadline(time.Now().Add(10 * time.Second))
				err := conn.WriteMessage(websocket.PingMessage, nil)
				connection.Mutex.Unlock()

				if err != nil {
					killConnection(server, connectionID)
					return
				}
			}
		}
	}()
}

func broadcastToSocket(server string, data []byte, typ string) {
	connectionsMutex.Lock()
	connections, ok := serverConnections[server]
	connectionsMutex.Unlock()

	if !ok || len(connections) == 0 {
		return
	}

	for id, conn := range connections {
		if conn != nil {
			if conn.Type != typ {
				continue
			}

			conn.Mutex.Lock()
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			_ = conn.WriteMessage(websocket.BinaryMessage, data)
			conn.Mutex.Unlock()
		} else {
			delete(serverConnections[server], id)
		}
	}
}

func hasSocketConnections(server, typ string) bool {
	connectionsMutex.Lock()
	connections, ok := serverConnections[server]
	connectionsMutex.Unlock()

	if !ok || len(connections) == 0 {
		return false
	}

	for _, conn := range connections {
		if conn != nil && conn.Type == typ {
			return true
		}
	}

	return false
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
