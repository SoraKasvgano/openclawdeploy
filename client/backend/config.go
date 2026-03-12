package backend

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"openclawdeploy/internal/shared"
)

type Config struct {
	DeviceID            string `json:"device_id"`
	DeviceCreatedAt     string `json:"device_created_at"`
	WebUsername         string `json:"web_username"`
	WebPassword         string `json:"web_password"`
	WebPort             int    `json:"web_port"`
	ListenAddr          string `json:"listen_addr"`
	ServerURL           string `json:"server_url"`
	SyncIntervalSeconds int    `json:"sync_interval_seconds"`
	OpenClawConfigPath  string `json:"openclaw_config_path"`
	AllowRemoteReboot   bool   `json:"allow_remote_reboot"`
	ConfigPath          string `json:"-"`
}

func defaultConfig() Config {
	baseDir := shared.RuntimeBaseDir()
	return Config{
		WebUsername:         "admin",
		WebPassword:         "admin",
		WebPort:             17896,
		ListenAddr:          "0.0.0.0",
		ServerURL:           "",
		SyncIntervalSeconds: 30,
		OpenClawConfigPath:  defaultOpenClawPath(),
		AllowRemoteReboot:   false,
		ConfigPath:          filepath.Join(baseDir, "config.json"),
	}
}

func LoadOrCreateConfig() (*Config, error) {
	defaults := defaultConfig()
	cfg := defaults

	if data, err := os.ReadFile(cfg.ConfigPath); err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parse config.json: %w", err)
		}
		cfg.ConfigPath = defaults.ConfigPath
	}

	if normalized := shared.NormalizeDeviceID(cfg.DeviceID); normalized != "" {
		cfg.DeviceID = normalized
	}
	cfg.ServerURL = normalizeServerURL(cfg.ServerURL)

	if cfg.ListenAddr == "" {
		cfg.ListenAddr = "0.0.0.0"
	}
	if cfg.WebUsername == "" {
		cfg.WebUsername = "admin"
	}
	if cfg.WebPassword == "" {
		cfg.WebPassword = "admin"
	}
	if cfg.WebPort == 0 {
		cfg.WebPort = 17896
	}
	if cfg.SyncIntervalSeconds <= 0 {
		cfg.SyncIntervalSeconds = 30
	}
	cfg.OpenClawConfigPath = normalizeOpenClawConfigPath(cfg.OpenClawConfigPath)

	if cfg.DeviceID == "" {
		deviceID, identity, err := GenerateIdentityCode()
		if err != nil {
			return nil, err
		}
		cfg.DeviceID = deviceID
		cfg.DeviceCreatedAt = identity.GeneratedAt
	}

	if err := cfg.Save(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) Save() error {
	data, err := shared.PrettyJSON(c)
	if err != nil {
		return err
	}
	return shared.AtomicWriteFile(c.ConfigPath, data, 0o644)
}

func (c *Config) Address() string {
	return fmt.Sprintf("%s:%d", c.ListenAddr, c.WebPort)
}

func normalizeServerURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "http://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return strings.TrimRight(trimmed, "/")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return strings.TrimRight(parsed.String(), "/")
}
