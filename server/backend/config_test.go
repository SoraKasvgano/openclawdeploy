package backend

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestLoadOrCreateConfigMigratesToServerConfig(t *testing.T) {
	withTempRuntimeDir(t)

	legacyPath := filepath.Join(".", legacyConfigFilename)
	if err := os.WriteFile(legacyPath, []byte(`{"web_port":19090,"listen_addr":"127.0.0.1"}`), 0o644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	cfg, err := LoadOrCreateConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.WebPort != 19090 {
		t.Fatalf("unexpected web_port: %d", cfg.WebPort)
	}
	if cfg.ConfigPath != filepath.Join(mustGetwd(t), serverConfigFilename) {
		t.Fatalf("unexpected config path: %s", cfg.ConfigPath)
	}
	if cfg.AIToken == "" {
		t.Fatalf("expected generated ai token")
	}

	data, err := os.ReadFile(serverConfigFilename)
	if err != nil {
		t.Fatalf("read serverconfig.json: %v", err)
	}

	var stored struct {
		WebPort int    `json:"web_port"`
		AIToken string `json:"ai_token"`
	}
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatalf("parse serverconfig.json: %v", err)
	}
	if stored.WebPort != 19090 {
		t.Fatalf("unexpected stored web_port: %d", stored.WebPort)
	}
	if stored.AIToken == "" {
		t.Fatalf("expected stored ai token")
	}
}

func TestRunHotReloadsPortFromServerConfig(t *testing.T) {
	withTempRuntimeDir(t)

	firstPort := freePort(t)
	secondPort := freePort(t)
	configPath := filepath.Join(".", serverConfigFilename)
	initial := []byte(`{"web_port":` + itoa(firstPort) + `,"listen_addr":"127.0.0.1"}`)
	if err := os.WriteFile(configPath, initial, 0o644); err != nil {
		t.Fatalf("write serverconfig.json: %v", err)
	}

	app, err := New()
	if err != nil {
		t.Fatalf("new app: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Run(ctx)
	}()

	waitForHTTP200(t, firstPort)

	cfg := readStoredConfig(t, configPath)
	cfg.WebPort = secondPort
	writeStoredConfig(t, configPath, cfg)

	waitForHTTP200(t, secondPort)
	waitForPortClosed(t, firstPort)

	cancel()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop")
	}
}

func withTempRuntimeDir(t *testing.T) {
	t.Helper()

	tmpDir := t.TempDir()
	oldWD := mustGetwd(t)
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})
}

func mustGetwd(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return wd
}

func freePort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func waitForHTTP200(t *testing.T, port int) {
	t.Helper()

	client := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(10 * time.Second)
	url := "http://127.0.0.1:" + itoa(port) + "/"
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", url)
}

func waitForPortClosed(t *testing.T, port int) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	address := "127.0.0.1:" + itoa(port)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 300*time.Millisecond)
		if err != nil {
			return
		}
		_ = conn.Close()
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s to close", address)
}

func readStoredConfig(t *testing.T, path string) Config {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	return cfg
}

func writeStoredConfig(t *testing.T, path string, cfg Config) {
	t.Helper()

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func itoa(value int) string {
	return strconv.Itoa(value)
}
