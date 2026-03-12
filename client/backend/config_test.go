package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"openclawdeploy/internal/shared"
)

func TestLoadOrCreateConfigNormalizesLegacyDeviceID(t *testing.T) {
	withTempClientRuntimeDir(t)

	legacyID := "likeqi|00:15:5d:a3:46:da|2026-03-11 17:07:15|100.64.0.3"
	data, err := json.Marshal(map[string]any{
		"device_id":         legacyID,
		"device_created_at": "2026-03-11 17:07:15",
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(".", "config.json"), data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadOrCreateConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	want := shared.NormalizeDeviceID(legacyID)
	if cfg.DeviceID != want {
		t.Fatalf("unexpected device id: %s", cfg.DeviceID)
	}

	stored, err := os.ReadFile(filepath.Join(".", "config.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	var saved Config
	if err := json.Unmarshal(stored, &saved); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if saved.DeviceID != want {
		t.Fatalf("unexpected saved device id: %s", saved.DeviceID)
	}
}

func TestLoadOrCreateConfigInitializesLocalWebCredentials(t *testing.T) {
	withTempClientRuntimeDir(t)

	cfg, err := LoadOrCreateConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.WebUsername != "admin" {
		t.Fatalf("unexpected web username: %s", cfg.WebUsername)
	}
	if cfg.WebPassword != "admin" {
		t.Fatalf("unexpected web password: %s", cfg.WebPassword)
	}
}

func TestLoadOrCreateConfigNormalizesServerURL(t *testing.T) {
	withTempClientRuntimeDir(t)

	data, err := json.Marshal(map[string]any{
		"server_url": "127.0.0.1:18080",
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(".", "config.json"), data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadOrCreateConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.ServerURL != "http://127.0.0.1:18080" {
		t.Fatalf("unexpected server url: %s", cfg.ServerURL)
	}
}

func withTempClientRuntimeDir(t *testing.T) {
	t.Helper()

	tmpDir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})
}
