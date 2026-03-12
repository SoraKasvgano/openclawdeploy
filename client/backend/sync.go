package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"openclawdeploy/internal/shared"
)

type HeartbeatRequest struct {
	DeviceID            string  `json:"device_id"`
	Hostname            string  `json:"hostname"`
	SystemVersion       string  `json:"system_version"`
	OS                  string  `json:"os"`
	Arch                string  `json:"arch"`
	LocalIP             string  `json:"local_ip"`
	MAC                 string  `json:"mac"`
	CPUCount            int     `json:"cpu_count"`
	CPUPercent          float64 `json:"cpu_percent"`
	MemoryPercent       float64 `json:"memory_percent"`
	NetworkOK           bool    `json:"network_ok"`
	OpenClawJSON        string  `json:"openclaw_json"`
	OpenClawHash        string  `json:"openclaw_hash"`
	SyncIntervalSeconds int     `json:"sync_interval_seconds"`
}

type HeartbeatResponse struct {
	ServerTime      string `json:"server_time"`
	ApplyConfig     bool   `json:"apply_config"`
	OpenClawJSON    string `json:"openclaw_json"`
	RestartOpenClaw bool   `json:"restart_openclaw"`
	Message         string `json:"message"`
}

type SyncSnapshot struct {
	LastSyncAt       string `json:"last_sync_at"`
	LastSyncMessage  string `json:"last_sync_message"`
	ServerConfigured bool   `json:"server_configured"`
	Connected        bool   `json:"connected"`
}

type Syncer struct {
	configSnapshot func() Config
	manager        *OpenClawManager
	httpClient     *http.Client

	mu      sync.RWMutex
	lastAt  string
	lastMsg string
	lastOK  bool
	wakeCh  chan struct{}
}

func NewSyncer(configSnapshot func() Config, manager *OpenClawManager) *Syncer {
	return &Syncer{
		configSnapshot: configSnapshot,
		manager:        manager,
		httpClient:     &http.Client{Timeout: 15 * time.Second},
		lastMsg:        "尚未同步",
		wakeCh:         make(chan struct{}, 1),
	}
}

func (s *Syncer) Snapshot() SyncSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cfg := s.currentConfig()
	return SyncSnapshot{
		LastSyncAt:       s.lastAt,
		LastSyncMessage:  s.lastMsg,
		ServerConfigured: normalizeServerURL(cfg.ServerURL) != "",
		Connected:        s.lastOK,
	}
}

func (s *Syncer) Start(ctx context.Context) {
	for {
		cfg := s.currentConfig()
		if normalizeServerURL(cfg.ServerURL) == "" {
			s.setStatus("", "未配置服务端地址，已跳过轮询", false)
		} else {
			_ = s.SyncNow(ctx)
		}

		timer := time.NewTimer(syncInterval(cfg))
		select {
		case <-ctx.Done():
			stopSyncTimer(timer)
			return
		case <-s.wakeCh:
			stopSyncTimer(timer)
		case <-timer.C:
		}
	}
}

func (s *Syncer) SyncNow(ctx context.Context) error {
	cfg := s.currentConfig()
	serverURL := normalizeServerURL(cfg.ServerURL)
	if serverURL == "" {
		s.setStatus("", "未配置服务端地址", false)
		return nil
	}
	if s.manager == nil {
		return fmt.Errorf("openclaw manager is not configured")
	}

	openclawJSON, err := s.manager.Read()
	if err != nil {
		s.setStatus(nowText(), "读取 openclaw.json 失败", false)
		return err
	}

	networkOK := checkNetwork(ctx)
	hostname, _ := os.Hostname()
	payload := HeartbeatRequest{
		DeviceID:            cfg.DeviceID,
		Hostname:            hostname,
		SystemVersion:       detectSystemVersion(),
		OS:                  runtime.GOOS,
		Arch:                runtime.GOARCH,
		LocalIP:             primaryIPv4(),
		MAC:                 primaryMAC(),
		CPUCount:            runtime.NumCPU(),
		CPUPercent:          0,
		MemoryPercent:       0,
		NetworkOK:           networkOK,
		OpenClawJSON:        openclawJSON,
		OpenClawHash:        shared.HashString(openclawJSON),
		SyncIntervalSeconds: cfg.SyncIntervalSeconds,
	}

	requestBody, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	heartbeatURL := serverURL + "/api/v1/client/heartbeat"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, heartbeatURL, bytes.NewReader(requestBody))
	if err != nil {
		s.setStatus(nowText(), "服务端地址格式错误", false)
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.setStatus(nowText(), "服务端同步失败", false)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		s.setStatus(nowText(), fmt.Sprintf("服务端返回 %d", resp.StatusCode), false)
		return fmt.Errorf("heartbeat status: %d", resp.StatusCode)
	}

	var serverResp HeartbeatResponse
	if err := json.NewDecoder(resp.Body).Decode(&serverResp); err != nil {
		return err
	}

	if serverResp.ApplyConfig && strings.TrimSpace(serverResp.OpenClawJSON) != "" {
		changed, err := s.manager.Apply(serverResp.OpenClawJSON)
		if err != nil {
			s.setStatus(nowText(), "收到新配置但写入失败", false)
			return err
		}
		if changed && serverResp.RestartOpenClaw {
			if err := s.manager.RestartGateway(ctx); err != nil {
				s.setStatus(nowText(), "配置已写入，但重启失败", false)
				return err
			}
		}
	}

	message := strings.TrimSpace(serverResp.Message)
	if message == "" {
		message = "与服务端同步成功"
	}
	s.setStatus(nowText(), message, true)
	return nil
}

func (s *Syncer) setStatus(at, message string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastAt = at
	s.lastMsg = message
	s.lastOK = ok
}

func (s *Syncer) NotifyConfigChanged() {
	select {
	case s.wakeCh <- struct{}{}:
	default:
	}
}

func (s *Syncer) currentConfig() Config {
	if s.configSnapshot == nil {
		return Config{}
	}
	return s.configSnapshot()
}

func syncInterval(cfg Config) time.Duration {
	if cfg.SyncIntervalSeconds <= 0 {
		return 30 * time.Second
	}
	return time.Duration(cfg.SyncIntervalSeconds) * time.Second
}

func stopSyncTimer(timer *time.Timer) {
	if timer == nil {
		return
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

func detectSystemVersion() string {
	if output, err := exec.Command("uname", "-sr").Output(); err == nil {
		return strings.TrimSpace(string(output))
	}
	return runtime.GOOS + "/" + runtime.GOARCH
}

func checkNetwork(ctx context.Context) bool {
	dialer := &net.Dialer{Timeout: 3 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", "baidu.com:80")
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func nowText() string {
	return time.Now().Format("2006-01-02 15:04:05")
}
