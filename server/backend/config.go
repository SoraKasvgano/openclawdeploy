package backend

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"openclawdeploy/internal/shared"
)

const (
	defaultWebPort         = 18080
	defaultListenAddr      = "0.0.0.0"
	defaultSessionTTLHours = 72
	defaultSMTPPort        = 25
	serverConfigFilename   = "serverconfig.json"
	stateDBFilename        = "server-state.sqlite"
	legacyStateFilename    = "server-state.json"
	legacyConfigFilename   = "config.json"
	olderConfigFilename    = "server-config.json"
)

type Config struct {
	WebPort         int        `json:"web_port"`
	ListenAddr      string     `json:"listen_addr"`
	PublicBaseURL   string     `json:"public_base_url"`
	SessionTTLHours int        `json:"session_ttl_hours"`
	AIToken         string     `json:"ai_token"`
	SMTP            SMTPConfig `json:"smtp"`
	ConfigPath      string     `json:"-"`
	StatePath       string     `json:"-"`
}

func defaultConfig() Config {
	baseDir := shared.RuntimeBaseDir()
	return Config{
		WebPort:         defaultWebPort,
		ListenAddr:      defaultListenAddr,
		PublicBaseURL:   "",
		SessionTTLHours: defaultSessionTTLHours,
		SMTP: SMTPConfig{
			Port: defaultSMTPPort,
		},
		ConfigPath: filepath.Join(baseDir, serverConfigFilename),
		StatePath:  filepath.Join(baseDir, "data", stateDBFilename),
	}
}

func LoadOrCreateConfig() (*Config, error) {
	defaults := defaultConfig()
	cfg, loadedFrom, changed, err := loadConfigWithFallback(defaults, []string{
		defaults.ConfigPath,
		filepath.Join(shared.RuntimeBaseDir(), legacyConfigFilename),
		filepath.Join(shared.RuntimeBaseDir(), olderConfigFilename),
	})
	if err != nil {
		return nil, err
	}

	if loadedFrom == "" || loadedFrom != defaults.ConfigPath || changed {
		if err := cfg.Save(); err != nil {
			return nil, err
		}
	}
	return &cfg, nil
}

func LoadConfigFromPath(path string) (*Config, error) {
	defaults := defaultConfig()
	defaults.ConfigPath = path

	cfg, changed, err := loadConfigFile(path, defaults)
	if err != nil {
		return nil, err
	}
	if changed {
		if err := cfg.Save(); err != nil {
			return nil, err
		}
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

func (c Config) Equals(other Config) bool {
	return c.WebPort == other.WebPort &&
		c.ListenAddr == other.ListenAddr &&
		c.PublicBaseURL == other.PublicBaseURL &&
		c.SessionTTLHours == other.SessionTTLHours &&
		c.AIToken == other.AIToken &&
		c.SMTP == other.SMTP &&
		c.ConfigPath == other.ConfigPath &&
		c.StatePath == other.StatePath
}

func loadConfigWithFallback(defaults Config, candidates []string) (Config, string, bool, error) {
	for _, candidate := range candidates {
		cfg, changed, err := loadConfigFile(candidate, defaults)
		if err == nil {
			cfg.ConfigPath = defaults.ConfigPath
			cfg.StatePath = defaults.StatePath
			return cfg, candidate, changed, nil
		}
		if !os.IsNotExist(err) {
			return Config{}, candidate, false, err
		}
	}
	changed, err := normalizeConfig(&defaults)
	return defaults, "", changed, err
}

func loadConfigFile(path string, defaults Config) (Config, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, false, err
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})

	cfg := defaults
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, false, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	cfg.ConfigPath = defaults.ConfigPath
	cfg.StatePath = defaults.StatePath
	changed, err := normalizeConfig(&cfg)
	return cfg, changed, err
}

func normalizeConfig(cfg *Config) (bool, error) {
	changed := false
	if cfg.WebPort == 0 {
		cfg.WebPort = defaultWebPort
		changed = true
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = defaultListenAddr
		changed = true
	}
	if cfg.SessionTTLHours <= 0 {
		cfg.SessionTTLHours = defaultSessionTTLHours
		changed = true
	}
	if cfg.SMTP.Port <= 0 {
		cfg.SMTP.Port = defaultSMTPPort
		changed = true
	}
	if cfg.AIToken == "" {
		token, err := generateAPIToken()
		if err != nil {
			return false, err
		}
		cfg.AIToken = token
		changed = true
	}
	return changed, nil
}

func generateAPIToken() (string, error) {
	buffer := make([]byte, 24)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
