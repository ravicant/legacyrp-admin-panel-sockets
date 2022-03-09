package main

import (
	"encoding/json"
	"io/ioutil"
	"strconv"
	"sync"
)

type VehicleJSON struct {
	Data  map[string]string `json:"data"`
	mutex sync.Mutex
}

func loadVehicleJSON(file string, dst *VehicleJSON) error {
	b, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}

	return json.Unmarshal(b, dst)
}

func (v *VehicleJSON) Find(hash string) (bool, string) {
	_, err := strconv.ParseInt(hash, 10, 64)
	if err != nil {
		return true, hash
	}

	v.mutex.Lock()
	res, ok := v.Data[hash]
	v.mutex.Unlock()

	return ok, res
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
