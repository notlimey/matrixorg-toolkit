package main

import (
	"encoding/json"
	"os"

	"maunium.net/go/mautrix/id"
)

type botConfig struct {
	WatchedRoom id.RoomID `json:"watched_room"`
}

var configPath = "config.json" // overridden by CONFIG_PATH env var in main

func loadConfig() botConfig {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return botConfig{} // first run — no config yet
	}
	var cfg botConfig
	json.Unmarshal(data, &cfg)
	return cfg
}

func saveConfig() {
	mu.Lock()
	cfg := botConfig{
		WatchedRoom: watchedRoom,
	}
	mu.Unlock()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(configPath, data, 0600)
}
