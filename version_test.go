package main

import (
	"encoding/json"
	"os"
	"testing"
)

func TestResolveVersionUsesPackagedVersion(t *testing.T) {
	if got := resolveVersion(" 1.2.3 "); got != "1.2.3" {
		t.Fatalf("resolveVersion() = %q, want %q", got, "1.2.3")
	}
}

func TestResolveVersionFallsBackToMygoConfig(t *testing.T) {
	data, err := os.ReadFile("mygo.json")
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if config.Version == "" {
		t.Fatal("mygo.json has no version")
	}
	if got := resolveVersion(""); got != config.Version {
		t.Fatalf("resolveVersion() = %q, want %q", got, config.Version)
	}
}
