package main

import (
	"bufio"
	"errors"
	"os"
	"strconv"
	"strings"
)

type HistoricEntry struct {
	X         float64
	Y         float64
	CID       int64
	Timestamp int64
}

func readHistoric(path string, callback func(HistoricEntry)) error {
	heatmapMutex.Lock()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		heatmapMutex.Unlock()
		return errors.New("no data for that day")
	}

	file, err := os.Open(path)
	if err != nil {
		heatmapMutex.Unlock()
		return errors.New("failed to read data")
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
			timestamp, tErr := strconv.ParseInt(elements[0], 10, 64)
			cid, cErr := strconv.ParseInt(elements[1], 10, 64)
			x, xErr := strconv.ParseFloat(elements[2], 64)
			y, yErr := strconv.ParseFloat(elements[3], 64)

			if tErr == nil && cErr == nil && xErr == nil && yErr == nil {
				entry := HistoricEntry{
					X:         x,
					Y:         y,
					CID:       cid,
					Timestamp: timestamp,
				}

				callback(entry)
			} else {
				log.Warning("Failed to read csv entry")
			}
		}
	}

	heatmapMutex.Unlock()

	return nil
}
