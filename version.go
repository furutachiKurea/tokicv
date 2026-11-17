package main

import (
	_ "embed"
	"encoding/json"
	"strings"
)

//go:embed mygo.json
var mygoConfig []byte

func resolveVersion(packaged string) string {
	if version := strings.TrimSpace(packaged); version != "" {
		return version
	}

	var config struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(mygoConfig, &config); err != nil {
		return ""
	}
	return strings.TrimSpace(config.Version)
}
