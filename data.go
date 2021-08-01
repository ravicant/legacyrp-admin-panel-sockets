package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/subosito/gotenv"
	"io/ioutil"
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
	lastError = make(map[string]*time.Time)

	lastPosition      = make(map[string]map[string]MovementLog)
	lastPositionSave  = time.Unix(0, 0)
	lastPositionMutex sync.Mutex
)

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
				data, timeout := getData(server)

				extraData(server, data)

				var b []byte
				if data == nil {
					now := time.Now()

					if lastError[server] == nil || now.Sub(*lastError[server]) > 30*time.Minute {
						log.Warning("Failed to load data from " + server)
						lastError[server] = &now
					}
					b, _ = json.Marshal(nil)
				} else {
					b, _ = json.Marshal(data.Players)

					logCoordinates(data.Players, server)
				}

				broadcastToSocket(server, b)

				if timeout != nil {
					log.Debug(server + " - sleeping for " + timeout.String())
					time.Sleep(*timeout)
				}

				time.Sleep(1 * time.Second)
			}
		}(s)
	}
}

func getData(server string) (*Data, *time.Duration) {
	token := os.Getenv(server)
	if token == "" {
		log.Error(server + " - No token defined")
		return nil, nil
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
		return nil, nil
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		log.Error(server + " - Failed to do request: " + err.Error())
		return nil, nil
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error(server + " - Failed to read body: " + err.Error())
		return nil, nil
	}

	sleep15 := 15 * time.Minute
	sleep5 := 5 * time.Minute
	switch resp.StatusCode {
	case 504:
		log.Warning(server + " - 504 Gateway timeout (origin error)")
		return nil, &sleep15
	case 502:
		log.Warning(server + " - 502 Bad Gateway (origin error)")
		return nil, &sleep15
	case 521:
		log.Warning(server + " - 521 Origin Down (server down/restarting)")
		return nil, &sleep5
	case 522:
		log.Warning(server + " - 522 Origin Connection Time-out (possibly server down/restarting)")
		return nil, &sleep5
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
		return nil, nil
	}

	if data.Status != 200 {
		log.Warning(fmt.Sprintf(server+" - Status code for "+server+" is not 200 (%d)", data.Status))
	}

	return data.Data, nil
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

	now := time.Now().Unix()

	for i, player := range data.Players {
		coords := player["coords"]
		if coords != nil {
			c, ok := coords.(map[string]interface{})

			if ok && c != nil {
				x, ok1 := c["x"].(float64)
				y, ok2 := c["y"].(float64)
				if !ok1 || !ok2 {
					b, _ := json.Marshal(c)
					log.Debug(server + " - Weird coordinate thingy: " + string(b))

					if !ok1 {
						x = 0
					}
					if !ok2 {
						y = 0
					}
				}

				hash := fmt.Sprintf("%.2f|%.2f", x, y)
				id := player["steamIdentifier"].(string)

				lastPositionMutex.Lock()
				pos, ok := lastPosition[server][id]
				if !ok || pos.Coords != hash {
					pos := MovementLog{
						Time:   now,
						Coords: hash,
					}
					lastPosition[server][id] = pos
				}
				lastPositionMutex.Unlock()

				data.Players[i]["afk"] = now - pos.Time
			}
		}

		vehicle := player["vehicle"]
		if vehicle != nil {
			v, ok := vehicle.(map[string]interface{})

			if ok && v != nil {
				hash, ok := v["model"].(float64)

				if ok {
					key := fmt.Sprintf("%d", int64(hash))

					vehicleMapMutex.Lock()
					replace, ok := vehicleMap[key]
					vehicleMapMutex.Unlock()

					if ok {
						v["model"] = replace
						data.Players[i]["vehicle"] = v
					}
				}
			}
		}
	}

	lastPositionMutex.Lock()
	if time.Now().Sub(lastPositionSave) > 5*time.Minute {
		b, _ := json.Marshal(lastPosition)
		_ = ioutil.WriteFile("afk.json", b, 0777)
	}
	lastPositionMutex.Unlock()
}
