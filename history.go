package main

import (
	"encoding/json"
	"io/ioutil"
	"math"
	"os"
	"strings"
	"sync"
	"time"
)

type Point struct {
	X    int64 `json:"x"`
	Y    int64 `json:"y"`
	Z    int64 `json:"z"`
	Time int64 `json:"t"`
}

var (
	history      = make(map[string]map[string]map[string][]Point)
	historyMutex sync.Mutex

	lastSave = time.Unix(0, 0)
)

func logCoordinates(players []map[string]interface{}, server string) {
	unix := time.Now().Unix()
	day := time.Now().Format("2006-01-02")
	dir := "history/"

	_ = os.MkdirAll(dir, 0777)

	historyMutex.Lock()
	_, ok := history[server]
	if !ok {
		history[server] = make(map[string]map[string][]Point)
	}
	historyMutex.Unlock()

	identifiers := make(map[string]bool, 0)

	for _, player := range players {
		if player["coords"] != nil {
			character, ok := player["character"].(bool)
			if ok && !character {
				continue
			}

			invisible, ok := player["invisible"].(bool)
			if ok && invisible {
				continue
			}

			raw, ok := player["coords"].(map[string]interface{})

			if ok && player["steamIdentifier"] != nil {
				coords := Point{
					X:    int64(math.Round(raw["x"].(float64))),
					Y:    int64(math.Round(raw["y"].(float64))),
					Z:    int64(math.Round(raw["z"].(float64))),
					Time: unix,
				}
				identifier := strings.ReplaceAll(player["steamIdentifier"].(string), ":", "_")

				historyMutex.Lock()
				_, ok := history[server][identifier]
				if !ok {
					history[server][identifier] = loadPlayer(server, identifier)
				}

				_, ok = history[server][identifier][day]
				if !ok {
					history[server][identifier][day] = make([]Point, 0)
				}

				history[server][identifier][day] = append(history[server][identifier][day], coords)

				historyMutex.Unlock()

				identifiers[identifier] = true
			}
		}
	}

	now := time.Now()
	for identifier := range identifiers {
		historyMutex.Lock()
		days := history[server][identifier]
		historyMutex.Unlock()

		for day := range days {
			d, _ := time.Parse("2006-01-02", day)

			if now.Sub(d) > (7*24)*time.Hour {
				historyMutex.Lock()
				delete(history[server][identifier], day)
				delete(days, day)
				historyMutex.Unlock()
			}
		}

		if now.Sub(lastSave) > 5*time.Minute {
			storePlayer(server, identifier, days)
		}
	}

	if now.Sub(lastSave) > 5*time.Minute {
		lastSave = now
	}

	// Cleanup memory
	historyMutex.Lock()
	for identifier := range history[server] {
		if !identifiers[identifier] {
			delete(history[server], identifier)
		}
	}
	historyMutex.Unlock()
}

func loadPlayer(server, identifier string) map[string][]Point {
	dir := "history/" + server + "/"
	file := dir + identifier + ".json"

	_ = os.MkdirAll(dir, 0777)

	if _, err := os.Stat(file); err == nil {
		b, _ := ioutil.ReadFile(file)
		var res map[string][]Point

		_ = json.Unmarshal(b, &res)

		return res
	}

	return make(map[string][]Point, 0)
}

func storePlayer(server, identifier string, list map[string][]Point) {
	b, _ := json.Marshal(list)

	_ = ioutil.WriteFile("history/"+server+"/"+identifier+".json", b, 0777)
}
