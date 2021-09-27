package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	historyFiles     = make(map[string]*os.File)
	historyFileMutex sync.Mutex
)

func logCoordsForPlayer(server, steam string, player map[string]interface{}) error {
	day := time.Now().Format("2006-01-02")
	dir := "./history/" + server + "/" + day + "/"
	path := dir + strings.ReplaceAll(steam, "steam:", "") + ".csv"

	_ = os.MkdirAll(dir, 0777)

	_, err := os.Stat("/path/to/whatever")
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
