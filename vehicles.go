package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"sync"
)

type VehicleJSON struct {
	Data struct {
		Addon []struct {
			Model string `json:"modelName"`
			Label string `json:"label"`
		} `json:"addon"`
	} `json:"data"`
	mutex sync.Mutex

	models map[string]string
	labels map[string]string
}

func loadVehicleJSON(file string, dst *VehicleJSON) error {
	b, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}

	return json.Unmarshal(b, dst)
}

func (v *VehicleJSON) Find(hash string) (bool, string, string) {
	v.mutex.Lock()

	res, ok := v.models[hash]
	if ok {
		label := v.labels[hash]

		v.mutex.Unlock()

		return true, res, label
	}

	for _, vehicle := range v.Data.Addon {
		h := fmt.Sprint(joaat(vehicle.Model))

		v.models[h] = vehicle.Model
		v.labels[h] = vehicle.Label

		if h == hash {
			v.mutex.Unlock()

			return true, vehicle.Model, vehicle.Label
		}
	}

	v.mutex.Unlock()

	return false, "", ""
}

func joaat(key string) (hash uint32) {
	var i int = 0

	for i != len(key) {
		hash += uint32(key[i])
		hash += hash << 10
		hash ^= hash >> 6

		i += 1
	}

	hash += hash << 3
	hash ^= hash >> 11
	hash += hash << 15

	return
}
