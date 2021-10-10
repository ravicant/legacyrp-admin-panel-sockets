package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"github.com/subosito/gotenv"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"time"
)

type StaffChatResponse struct {
	StatusCode int64            `json:"statusCode"`
	Data       []StaffChatEntry `json:"data"`
}

type StaffChatEntry struct {
	User struct {
		SteamIdentifier string `json:"steamIdentifier"`
		PlayerName      string `json:"playerName"`
	} `json:"user"`
	Message   string `json:"message"`
	CreatedAt int64  `json:"createdAt"`
}

func startStaffChatLoop() {
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
				if hasSocketConnections(server, SocketTypeStaffChat) {
					staffChatList := getStaffChat(server)

					b, _ = json.Marshal(staffChatList)

					broadcastToSocket(server, gzipBytes(b), SocketTypeStaffChat)

					time.Sleep(2 * time.Second)
				} else {
					time.Sleep(10 * time.Second)
				}
			}
		}(s)
	}
}

func getStaffChat(server string) []StaffChatEntry {
	emptyList := make([]StaffChatEntry, 0)
	url := "https://" + server + ".op-framework.com/op-framework/staffChat.json"

	token := os.Getenv(server)
	if token == "" {
		log.Error(server + " - No token defined")
		return emptyList
	}

	client := &http.Client{
		Timeout: 3 * time.Second,
	}

	override := os.Getenv(server + "_map")
	if override != "" {
		url = "https://" + override + "/op-framework/staffChat.json"

		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Error(server + " - Failed to create request: " + err.Error())
		return emptyList
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		log.Error(server + " - Failed to do request: " + err.Error())
		return emptyList
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error(server + " - Failed to read body: " + err.Error())
		return emptyList
	}

	var list StaffChatResponse
	err = json.Unmarshal(body, &list)
	if err != nil {
		log.Error(server + " - Failed parse response: " + err.Error())
		return emptyList
	}

	if list.StatusCode != 200 {
		return emptyList
	}

	return list.Data
}
