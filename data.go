package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/subosito/gotenv"
	"io/ioutil"
	"log"
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
	lastPositionMutex sync.Mutex
)

func startDataLoop() {
	b, _ := ioutil.ReadFile(".env")
	env := gotenv.Parse(bytes.NewReader(b))

	servers := make([]string, 0)
	for server := range env {
		rgx := regexp.MustCompile(`(?m)^c\d+s\d+$`)
		if rgx.MatchString(server) {
			servers = append(servers, server)
		}
	}

	for {
		var wg sync.WaitGroup
		for _, s := range servers {
			wg.Add(1)

			go func(server string) {
				data := getData(server)

				extraData(server, data)

				var b []byte
				if data == nil {
					now := time.Now()

					if lastError[server] == nil || now.Sub(*lastError[server]) > 30*time.Minute {
						log.Println("Failed to load data from " + server)
						lastError[server] = &now
					}
					b, _ = json.Marshal(nil)
				} else {
					b, _ = json.Marshal(data.Players)

					logCoordinates(data.Players, server)
				}

				broadcastToSocket(server, b)

				wg.Done()
			}(s)
		}

		wg.Wait()

		time.Sleep(1 * time.Second)
	}
}

func getData(server string) *Data {
	token := os.Getenv(server)
	if token == "" {
		log.Println("No token defined for " + server)
		return nil
	}

	url := "https://" + server + ".op-framework.com/op-framework/world.json"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Println("Failed to create request: " + err.Error())
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Failed to do request: " + err.Error())
		return nil
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println("Failed to read body: " + err.Error())
		return nil
	}

	var data struct {
		Status int64 `json:"statusCode"`
		Data   *Data `json:"data"`
	}
	err = json.Unmarshal(body, &data)
	if err != nil {
		log.Println("Failed parse response: " + err.Error())
		return nil
	}

	return data.Data
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
				hash := fmt.Sprintf("%.2f|%.2f", c["x"].(float64), c["y"].(float64))
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
	}
}
