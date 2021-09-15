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

type DutyResponse struct {
	StatusCode int64 `json:"statusCode"`
	Data       struct {
		Police []string `json:"Law Enforcement"`
		EMS    []string `json:"Medical"`
	} `json:"data"`
}
type EmptyDutyResponse struct {
	StatusCode int64         `json:"statusCode"`
	Data       []interface{} `json:"data"`
}

type OnDutyList struct {
	Police []string `json:"police"`
	EMS    []string `json:"ems"`
}

func startDutyLoop() {
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
				onDutyList := getDuty(server)

				lastDutyMutex.Lock()
				lastDuty[server] = onDutyList
				lastDutyMutex.Unlock()

				time.Sleep(15 * time.Second)
			}
		}(s)
	}
}

func getDuty(server string) OnDutyList {
	emptyList := OnDutyList{
		Police: []string{},
		EMS:    []string{},
	}
	url := "https://" + server + ".op-framework.com/op-framework/duty.json"

	client := &http.Client{
		Timeout: 3 * time.Second,
	}

	override := os.Getenv(server + "_map")
	if override != "" {
		url = "https://" + override + "/op-framework/duty.json"

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

	var duty DutyResponse
	err = json.Unmarshal(body, &duty)
	if err != nil {
		var empty EmptyDutyResponse
		err = json.Unmarshal(body, &empty)
		if err != nil {
			log.Error(server + " - Failed parse response: " + err.Error())
		}
		return emptyList
	}

	if duty.StatusCode != 200 {
		return emptyList
	}

	return OnDutyList{
		Police: duty.Data.Police,
		EMS:    duty.Data.EMS,
	}
}
