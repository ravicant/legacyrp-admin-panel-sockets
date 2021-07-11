package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"
)

type Data struct {
	Players []interface{}
}

var (
	lastError = make(map[string]*time.Time)
)

func startDataLoop() {
	for {
		connectionsMutex.Lock()
		servers := make([]string, 0)
		for server := range serverConnections {
			servers = append(servers, server)
		}
		connectionsMutex.Unlock()

		for _, server := range servers {
			connectionsMutex.Lock()
			count := len(serverConnections[server])
			connectionsMutex.Unlock()

			if count > 0 {
				data := getData(server)

				var b []byte
				if data == nil {
					now := time.Now()

					if lastError[server] == nil || now.Sub(*lastError[server]) > 5*time.Minute {
						log.Println("Failed to load data from " + server)
						lastError[server] = &now
					}
					b, _ = json.Marshal(nil)
				} else {
					b, _ = json.Marshal(data.Players)
				}

				broadcastToSocket(server, b)
			}
		}

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
