package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/subosito/gotenv"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"regexp"
	"sync"
	"time"
)

type Data struct {
	Players []map[string]interface{}
}

type MovementLog struct {
	Time   int64
	Coords string
}

var (
	lastError      = make(map[string]*time.Time)
	lastErrorMutex sync.Mutex

	lastPosition      = make(map[string]map[string]MovementLog)
	lastPositionSave  = time.Unix(0, 0)
	lastPositionMutex sync.Mutex

	lastInvisible      = make(map[string]map[string]int64)
	lastInvisibleMutex sync.Mutex

	lastDuty      = make(map[string]OnDutyList)
	lastDutyMutex sync.Mutex

	loggedHashes      = make(map[string]bool)
	loggedHashesMutex sync.Mutex
)

type InfoPackage struct {
	Message string `json:"message"`
	Status  int    `json:"status"`
}

func startDataLoop() {
	b, _ := ioutil.ReadFile(".env")
	env := gotenv.Parse(bytes.NewReader(b))

	servers := make([]string, 0)
	for server := range env {
		rgx := regexp.MustCompile(`(?m)^c\d+s\d+$`)
		if rgx.MatchString(server) && os.Getenv(server) != "" {
			servers = append(servers, server)
		}
	}

	for _, s := range servers {
		go func(server string) {
			for {
				data, timeout, info := getData(server)

				extraData(server, data)

				var b []byte
				if data == nil {
					now := time.Now()

					lastErrorMutex.Lock()
					if lastError[server] == nil || now.Sub(*lastError[server]) > 30*time.Minute {
						log.Warning("Failed to load data from " + server)
						lastError[server] = &now
					}
					lastErrorMutex.Unlock()

					if info != nil {
						b, _ = json.Marshal(info)

						serverErrorsMutex.Lock()
						serverErrors[server] = b
						serverErrorsMutex.Unlock()
					} else {
						b, _ = json.Marshal(nil)
					}
				} else {
					lastDutyMutex.Lock()
					last, ok := lastDuty[server]
					lastDutyMutex.Unlock()

					if !ok {
						last.Police = []string{}
						last.EMS = []string{}
					}

					b, _ = json.Marshal(map[string]interface{}{
						"p": CompressPlayers(server, data.Players),
						"d": map[string][]string{
							"p": last.Police,
							"e": last.EMS,
						},
						"s": getSteamIdentifiersByTypeAndServer(SocketTypeMap, server),
					})

					serverErrorsMutex.Lock()
					serverErrors[server] = nil
					serverErrorsMutex.Unlock()
				}

				broadcastToSocket(server, gzipBytes(b), SocketTypeMap)

				if timeout != nil {
					log.Debug(server + " - sleeping for " + timeout.String())
					time.Sleep(*timeout)
				}

				time.Sleep(1 * time.Second)
			}
		}(s)
	}
}

func getData(server string) (*Data, *time.Duration, *InfoPackage) {
	token := os.Getenv(server)
	if token == "" {
		log.Error(server + " - No token defined")
		return nil, nil, &InfoPackage{"Missing token", http.StatusNotImplemented}
	}

	url := "https://" + server + ".op-framework.com/op-framework/world.json"

	client := &http.Client{
		Timeout: 3 * time.Second,
	}

	override := os.Getenv(server + "_map")
	if override != "" {
		url = "https://" + override + "/op-framework/world.json"

		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Error(server + " - Failed to create request: " + err.Error())
		return nil, nil, nil
	}
	req.Header.Set("Authorization", "Bearer "+token)

	time10 := 10 * time.Minute
	time15s := 10 * time.Second

	resp, err := client.Do(req)
	if err != nil {
		log.Warning(server + " - Retrying data load in 15 sec")
		time.Sleep(time15s)
		resp, err = client.Do(req)

		if err != nil {
			if nErr, ok := err.(net.Error); ok && nErr.Timeout() {
				log.Error(server + " - Connection timed out")
				return nil, &time10, &InfoPackage{"Connection timed out (likely rate-limit)", http.StatusGatewayTimeout}
			}

			log.Error(server + " - Failed to do request: " + err.Error())
			return nil, nil, &InfoPackage{"Failed to get data", http.StatusInternalServerError}
		}
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error(server + " - Failed to read body: " + err.Error())
		return nil, nil, nil
	}

	sleep15 := 15 * time.Minute
	sleep5 := 5 * time.Minute
	switch resp.StatusCode {
	case 401:
		log.Warning(server + " - 401 Unauthorized (invalid token?)")
		return nil, &sleep15, &InfoPackage{"Unauthorized (server)", http.StatusServiceUnavailable}
	case 504:
		log.Warning(server + " - 504 Gateway timeout (origin error)")
		return nil, &sleep15, &InfoPackage{"Gateway timeout", http.StatusServiceUnavailable}
	case 502:
		log.Warning(server + " - 502 Bad Gateway (origin error)")
		return nil, &sleep15, &InfoPackage{"Bad Gateway", http.StatusServiceUnavailable}
	case 521:
		log.Warning(server + " - 521 Origin Down (server down/restarting)")
		return nil, &sleep5, &InfoPackage{"Origin Down", http.StatusServiceUnavailable}
	case 522:
		log.Warning(server + " - 522 Origin Connection Time-out (possibly server down/restarting)")
		return nil, &sleep5, &InfoPackage{"Origin Connection Time-out", http.StatusServiceUnavailable}
	}

	var data struct {
		Status int64 `json:"statusCode"`
		Data   *Data `json:"data"`
	}
	err = json.Unmarshal(body, &data)
	if err != nil {
		log.Debug(url)
		log.Debug(string(body))
		log.Error(server + " - Failed parse response: " + err.Error())
		return nil, nil, &InfoPackage{"Invalid response from server", http.StatusBadGateway}
	}

	if data.Status != 200 {
		if data.Status == 401 {
			log.Warning(server + " - 401 Unauthorized (route says: invalid token)")
			return nil, &sleep15, &InfoPackage{"Unauthorized (route)", http.StatusServiceUnavailable}
		}

		log.Warning(fmt.Sprintf(server+" - Status code for "+server+" is not 200 but %d", data.Status))
	}

	return data.Data, nil, nil
}

func wasHashLogged(hash string) bool {
	loggedHashesMutex.Lock()
	res := loggedHashes[hash]

	if !res {
		loggedHashes[hash] = true
	}
	loggedHashesMutex.Unlock()

	return res
}

func extraData(server string, data *Data) {
	if data == nil {
		return
	}

	lastPositionMutex.Lock()
	_, ok := lastPosition[server]
	if !ok {
		lastPosition[server] = make(map[string]MovementLog)
	}
	lastPositionMutex.Unlock()

	lastInvisibleMutex.Lock()
	_, ok = lastInvisible[server]
	if !ok {
		lastInvisible[server] = make(map[string]int64)
	}
	lastInvisibleMutex.Unlock()

	now := time.Now().Unix()

	validIDs := make(map[string]bool, 0)
	for i, player := range data.Players {
		id := player["steamIdentifier"].(string)
		validIDs[id] = true

		invisible, ok := player["invisible"].(bool)
		if ok {
			id := player["steamIdentifier"].(string)

			lastInvisibleMutex.Lock()
			t, ok := lastInvisible[server][id]

			if invisible && !ok {
				lastInvisible[server][id] = now
				t = now
			} else if !invisible && ok {
				delete(lastInvisible[server], id)
			} else if t == 0 {
				t = now
			}
			lastInvisibleMutex.Unlock()

			data.Players[i]["invisible_since"] = now - t
		}

		vehicle := player["vehicle"]
		if vehicle != nil {
			v, ok := vehicle.(map[string]interface{})

			if ok && v != nil {
				hash, ok := v["model"].(float64)

				var modelName string
				if ok {
					key := fmt.Sprintf("%.0f", hash)

					vehicleMapMutex.Lock()
					replace, ok := vehicleMap[key]
					modelName = replace
					vehicleMapMutex.Unlock()

					if ok {
						v["model"] = replace
					} else if !wasHashLogged("hash_" + key) {
						log.Warning(fmt.Sprintf("No hash mapping found for hash %s", key))
					}

					data.Players[i]["vehicle"] = v
				} else if model, ok := v["model"].(string); ok {
					modelName = model
				}

				displayMapMutex.Lock()
				v["name"], ok = displayMap[modelName]
				displayMapMutex.Unlock()

				if !ok && !wasHashLogged("name_"+modelName) {
					log.Warning(fmt.Sprintf("No name mapping found for model %s", modelName))
				}
			}
		}

		err := logCoordsForPlayer(server, id, player)
		if err != nil {
			log.Warning("Failed to log historic data for '" + id + "': " + err.Error())
		}
	}

	lastInvisibleMutex.Lock()
	for id := range lastInvisible[server] {
		if !validIDs[id] {
			delete(lastInvisible[server], id)
		}
	}
	lastInvisibleMutex.Unlock()

	lastPositionMutex.Lock()
	for id := range lastPosition[server] {
		if !validIDs[id] {
			delete(lastPosition[server], id)
		}
	}

	if time.Now().Sub(lastPositionSave) > 5*time.Minute {
		b, _ := json.Marshal(lastPosition)
		_ = ioutil.WriteFile("afk.json", b, 0777)
	}
	lastPositionMutex.Unlock()
}

func getSteamIdentifiersByTypeAndServer(typ, server string) []string {
	steamIdentifiers := make([]string, 0)

	connectionsMutex.Lock()
	connections, ok := serverConnections[server]
	connectionsMutex.Unlock()

	if !ok {
		return steamIdentifiers
	}

	for id, conn := range connections {
		if conn != nil {
			if conn.Type != typ {
				continue
			}

			conn.Mutex.Lock()
			steamIdentifiers = append(steamIdentifiers, conn.Steam)
			conn.Mutex.Unlock()
		} else {
			delete(serverConnections[server], id)
		}
	}

	return steamIdentifiers
}
