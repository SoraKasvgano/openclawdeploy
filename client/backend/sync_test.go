package backend

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientServerConfigCanBeUpdatedViaAPI(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &Config{
		WebUsername:        "admin",
		WebPassword:        "admin",
		ConfigPath:         filepath.Join(tempDir, "config.json"),
		OpenClawConfigPath: filepath.Join(tempDir, "openclaw.json"),
	}
	app := &App{
		cfg:    cfg,
		logger: log.New(io.Discard, "", 0),
		syncer: NewSyncer(func() Config { return *cfg }, nil),
	}

	server := httptest.NewServer(app.routes())
	defer server.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("create cookie jar: %v", err)
	}
	client := server.Client()
	client.Jar = jar

	loginResp, err := client.Post(server.URL+"/api/v1/client/auth/login", "application/json", strings.NewReader(`{"username":"admin","password":"admin"}`))
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected login status: %d", loginResp.StatusCode)
	}

	updateResp, err := client.Post(server.URL+"/api/v1/client/server", "application/json", strings.NewReader(`{"server_url":"127.0.0.1:18080"}`))
	if err != nil {
		t.Fatalf("update request failed: %v", err)
	}
	defer updateResp.Body.Close()
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected update status: %d", updateResp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(updateResp.Body).Decode(&body); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if body["server_url"] != "http://127.0.0.1:18080" {
		t.Fatalf("unexpected response server_url: %s", body["server_url"])
	}
	if cfg.ServerURL != "http://127.0.0.1:18080" {
		t.Fatalf("unexpected cfg server_url: %s", cfg.ServerURL)
	}

	data, err := os.ReadFile(cfg.ConfigPath)
	if err != nil {
		t.Fatalf("read config file: %v", err)
	}

	var saved Config
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("decode config file: %v", err)
	}
	if saved.ServerURL != "http://127.0.0.1:18080" {
		t.Fatalf("unexpected saved server_url: %s", saved.ServerURL)
	}
}

func TestSyncerCanHotApplyServerURLWithoutRestart(t *testing.T) {
	tempDir := t.TempDir()
	openclawPath := filepath.Join(tempDir, "openclaw.json")
	manager := NewOpenClawManager(openclawPath, log.New(io.Discard, "", 0))
	if err := manager.EnsureFile(); err != nil {
		t.Fatalf("ensure openclaw file: %v", err)
	}

	var cfgMu sync.RWMutex
	cfg := Config{
		DeviceID:            "device-hot-update-1",
		ServerURL:           "",
		SyncIntervalSeconds: 3600,
		OpenClawConfigPath:  openclawPath,
	}

	snapshot := func() Config {
		cfgMu.RLock()
		defer cfgMu.RUnlock()
		return cfg
	}

	var heartbeatCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/client/heartbeat" {
			http.NotFound(w, r)
			return
		}
		heartbeatCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"server_time":"2026-03-12T00:00:00Z","message":"ok"}`))
	}))
	defer server.Close()

	syncer := NewSyncer(snapshot, manager)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go syncer.Start(ctx)
	time.Sleep(200 * time.Millisecond)
	if heartbeatCount.Load() != 0 {
		t.Fatalf("unexpected heartbeat before server url update: %d", heartbeatCount.Load())
	}

	cfgMu.Lock()
	cfg.ServerURL = server.URL
	cfgMu.Unlock()
	syncer.NotifyConfigChanged()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if heartbeatCount.Load() > 0 && syncer.Snapshot().Connected {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("syncer did not hot-apply server url; heartbeat_count=%d connected=%v", heartbeatCount.Load(), syncer.Snapshot().Connected)
}

func TestOpenClawPathCanBeUpdatedViaAPI(t *testing.T) {
	tempDir := t.TempDir()
	currentPath := filepath.Join(tempDir, "runtime", "openclaw.json")
	manager := NewOpenClawManager(currentPath, log.New(io.Discard, "", 0))
	if err := manager.EnsureFile(); err != nil {
		t.Fatalf("ensure current openclaw file: %v", err)
	}
	if err := manager.Write(`{"gateway":{"port":19001}}`); err != nil {
		t.Fatalf("seed openclaw file: %v", err)
	}

	cfg := &Config{
		WebUsername:        "admin",
		WebPassword:        "admin",
		ConfigPath:         filepath.Join(tempDir, "config.json"),
		OpenClawConfigPath: currentPath,
	}
	app := &App{
		cfg:      cfg,
		logger:   log.New(io.Discard, "", 0),
		openclaw: manager,
		syncer:   NewSyncer(func() Config { return *cfg }, manager),
	}

	server := httptest.NewServer(app.routes())
	defer server.Close()

	client := authedClient(t, server)
	targetDir := filepath.Join(tempDir, "custom", ".openclaw")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("create target dir: %v", err)
	}

	resp, err := client.Post(server.URL+"/api/v1/client/openclaw/path", "application/json", strings.NewReader(`{"openclaw_config_path":"`+filepath.ToSlash(targetDir)+`/"}`))
	if err != nil {
		t.Fatalf("update openclaw path request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected openclaw path status: %d", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode update response: %v", err)
	}

	wantPath := filepath.Join(targetDir, "openclaw.json")
	if body["openclaw_config_path"] != wantPath {
		t.Fatalf("unexpected response path: %s", body["openclaw_config_path"])
	}
	if cfg.OpenClawConfigPath != wantPath {
		t.Fatalf("unexpected config path: %s", cfg.OpenClawConfigPath)
	}
	if manager.Path() != wantPath {
		t.Fatalf("unexpected manager path: %s", manager.Path())
	}

	data, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read migrated openclaw file: %v", err)
	}
	if !strings.Contains(string(data), `"port": 19001`) {
		t.Fatalf("unexpected migrated openclaw content: %s", string(data))
	}
}

func TestBrowseLocalFSReturnsDirectoriesAndJSONFiles(t *testing.T) {
	tempDir := t.TempDir()
	manager := NewOpenClawManager(filepath.Join(tempDir, "openclaw.json"), log.New(io.Discard, "", 0))
	if err := manager.EnsureFile(); err != nil {
		t.Fatalf("ensure openclaw file: %v", err)
	}
	if err := os.Mkdir(filepath.Join(tempDir, "configs"), 0o755); err != nil {
		t.Fatalf("create configs dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "custom.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write json file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "notes.txt"), []byte("ignore"), 0o644); err != nil {
		t.Fatalf("write txt file: %v", err)
	}

	cfg := &Config{
		WebUsername:        "admin",
		WebPassword:        "admin",
		ConfigPath:         filepath.Join(tempDir, "config.json"),
		OpenClawConfigPath: manager.Path(),
	}
	app := &App{
		cfg:      cfg,
		logger:   log.New(io.Discard, "", 0),
		openclaw: manager,
		syncer:   NewSyncer(func() Config { return *cfg }, manager),
	}

	server := httptest.NewServer(app.routes())
	defer server.Close()

	client := authedClient(t, server)
	resp, err := client.Get(server.URL + "/api/v1/client/fs/browse?path=" + url.QueryEscape(tempDir))
	if err != nil {
		t.Fatalf("browse request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected browse status: %d", resp.StatusCode)
	}

	var browser localPathBrowserResponse
	if err := json.NewDecoder(resp.Body).Decode(&browser); err != nil {
		t.Fatalf("decode browse response: %v", err)
	}
	if browser.CurrentPath != tempDir {
		t.Fatalf("unexpected current path: %s", browser.CurrentPath)
	}
	if browser.SuggestedFilePath != filepath.Join(tempDir, "openclaw.json") {
		t.Fatalf("unexpected suggested file path: %s", browser.SuggestedFilePath)
	}

	var foundDir bool
	var foundJSON bool
	for _, entry := range browser.Entries {
		if entry.Name == "configs" && entry.IsDir {
			foundDir = true
		}
		if entry.Name == "custom.json" && !entry.IsDir {
			foundJSON = true
		}
		if entry.Name == "notes.txt" {
			t.Fatalf("unexpected non-json file in browser entries")
		}
	}
	if !foundDir {
		t.Fatalf("expected directory entry in browser response")
	}
	if !foundJSON {
		t.Fatalf("expected json file entry in browser response")
	}
}

func authedClient(t *testing.T, server *httptest.Server) *http.Client {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("create cookie jar: %v", err)
	}
	client := server.Client()
	client.Jar = jar

	loginResp, err := client.Post(server.URL+"/api/v1/client/auth/login", "application/json", strings.NewReader(`{"username":"admin","password":"admin"}`))
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected login status: %d", loginResp.StatusCode)
	}

	return client
}
