package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	historyFiles     = make(map[string]*os.File)
	historyFileMutex sync.Mutex

	heatmapMutex sync.Mutex
)

func logCoordsForPlayer(server, steam string, player map[string]interface{}) error {
	day := time.Now().Format("2006-01-02")
	dir := "./history/" + server + "/" + day + "/"
	path := dir + strings.ReplaceAll(steam, "steam:", "") + ".csv"

	_ = os.MkdirAll(dir, 0777)

	_, err := os.Stat(path)
	existed := err == nil

	historyFileMutex.Lock()
	file, ok := historyFiles[path]
	historyFileMutex.Unlock()

	if !ok || file == nil {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0777)
		if err != nil {
			return err
		}

		file = f

		historyFileMutex.Lock()
		historyFiles[path] = file
		historyFileMutex.Unlock()

		if !existed {
			_, _ = file.WriteString("Timestamp,Character ID,X,Y,Z,Heading\n")
		}
	}

	c := getMap("coords", player)
	character := getMap("character", player)

	if c != nil && character != nil {
		t := time.Now().Unix()

		id := getInt64("id", character)

		x, xOk := c["x"].(float64)
		y, yOk := c["y"].(float64)
		z, zOk := c["z"].(float64)

		h := getFloat64("heading", player)

		if xOk && yOk && zOk && id != 0 {
			// Timestamp, Character ID, X, Y, Z, Heading
			_, err := file.WriteString(fmt.Sprintf("%d,%d,%.1f,%.1f,%.1f,%.1f\n", t, id, x, y, z, h))
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func doHistoryCleanup() error {
	_ = os.MkdirAll("./history/", 0777)

	return filepath.Walk("./history", func(server string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return filepath.Walk(server, func(day string, dayInfo os.FileInfo, err error) error {
				if dayInfo != nil && dayInfo.IsDir() {
					t, err := time.Parse("2006-01-02", dayInfo.Name())

					if err == nil && time.Now().Sub(t) > 24*10*time.Hour { // Delete if older than 10 days
						log.Info("Removing historic entries '" + day + "'")
						return os.RemoveAll(day)
					}
				}

				return err
			})
		}

		return err
	})
}

func getHistoricLocation(server, steam string, from, till int64) (interface{}, error) {
	fromDay := time.Unix(from, 0).Format("2006-01-02")
	tillDay := time.Unix(till, 0).Format("2006-01-02")

	if fromDay == tillDay {
		heatmapMutex.Lock()

		path := "./history/" + server + "/" + fromDay + "/" + steam + ".csv"

		if _, err := os.Stat(path); os.IsNotExist(err) {
			heatmapMutex.Unlock()
			return nil, errors.New("no data for that day")
		}

		file, err := os.Open(path)
		if err != nil {
			return nil, errors.New("failed to read data")
		}
		defer func() {
			_ = file.Close()
		}()

		data := make(map[int64]interface{})

		scanner := bufio.NewScanner(file)
		index := 0
		for scanner.Scan() {
			elements := strings.Split(scanner.Text(), ",")

			// Skip csv header
			if index == 0 {
				index++
				continue
			}
			index++

			if len(elements) == 6 {
				timestamp, tErr := strconv.ParseInt(elements[0], 10, 64)
				cid, cErr := strconv.ParseInt(elements[1], 10, 64)
				x, xErr := strconv.ParseFloat(elements[2], 64)
				y, yErr := strconv.ParseFloat(elements[3], 64)

				if tErr == nil && cErr == nil && xErr == nil && yErr == nil {
					data[timestamp] = map[string]interface{}{
						"cid": cid,
						"x":   x,
						"y":   y,
					}
				} else {
					log.Warning("Failed to read csv entry")
				}
			}
		}

		return data, nil
	}

	return nil, errors.New("from and till are on different days (:pain:)")
}

func generateHeatMapForDay(server, day string) (string, error) {
	cache := "./cache/heatmap/" + server + "_" + day + ".json"

	heatmapMutex.Lock()

	stat, err := os.Stat(cache)
	if os.IsNotExist(err) || time.Now().Sub(stat.ModTime()) > 1*time.Hour {
		heatmap := make(map[string]int64)
		dir := "./history/" + server + "/" + day + "/"

		if _, err := os.Stat(dir); os.IsNotExist(err) {
			heatmapMutex.Unlock()
			return cache, errors.New("no data for that day")
		}

		_ = os.MkdirAll(filepath.Dir(cache), 0777)

		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info != nil && !info.IsDir() && strings.HasSuffix(path, ".csv") {
				file, err := os.Open(path)
				if err != nil {
					return err
				}
				defer func() {
					_ = file.Close()
				}()

				scanner := bufio.NewScanner(file)
				index := 0
				for scanner.Scan() {
					elements := strings.Split(scanner.Text(), ",")

					// Skip csv header
					if index == 0 {
						index++
						continue
					}
					index++

					if len(elements) == 6 {
						x, xErr := strconv.ParseFloat(elements[2], 64)
						y, yErr := strconv.ParseFloat(elements[3], 64)

						if xErr == nil && yErr == nil {
							x, y = resolutionDecrease(x, y, 10)

							key := fmt.Sprintf("%.0f/%.0f", x, y)

							heatmap[key]++
						} else {
							log.Warning("Failed to read x/y coordinate")
						}
					}
				}
			}

			return nil
		})

		max := int64(0)
		for _, value := range heatmap {
			if value > max {
				max = value
			}
		}

		normalizedHeatMap := make(map[string]float64)
		for key, value := range heatmap {
			// calculate percentage between 0 and 100 and round to 2 decimal places
			normalizedHeatMap[key] = math.Round((float64(value)/float64(max))*10000) / 100

			// Just some minor cleanup so we don't have to send such huge amounts of data
			if normalizedHeatMap[key] < 0.1 {
				delete(normalizedHeatMap, key)
			}
		}

		b, err := json.Marshal(map[string]interface{}{
			"status": true,
			"data":   normalizedHeatMap,
		})
		if err != nil {
			heatmapMutex.Unlock()
			return cache, err
		}

		err = ioutil.WriteFile(cache, b, 0777)
		if err != nil {
			heatmapMutex.Unlock()
			return cache, err
		}
	}

	heatmapMutex.Unlock()
	return cache, nil
}

func resolutionDecrease(x, y, resolution float64) (float64, float64) {
	x = math.Round(x/resolution) * resolution
	y = math.Round(y/resolution) * resolution

	return x, y
}
