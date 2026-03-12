package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"openclawdeploy/internal/shared"
)

const defaultOpenClawJSON = `{
  "meta": {
    "managedBy": "openclawdeploy"
  },
  "gateway": {
    "port": 18789
  },
  "models": {
    "providers": {}
  }
}
`

type OpenClawManager struct {
	mu     sync.RWMutex
	path   string
	logger *log.Logger
}

func defaultOpenClawPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(shared.RuntimeBaseDir(), ".openclaw", "openclaw.json")
	}
	return filepath.Join(home, ".openclaw", "openclaw.json")
}

func NewOpenClawManager(path string, logger *log.Logger) *OpenClawManager {
	return &OpenClawManager{path: normalizeOpenClawConfigPath(path), logger: logger}
}

func (m *OpenClawManager) Path() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.path
}

func (m *OpenClawManager) SetPath(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.path = normalizeOpenClawConfigPath(path)
}

func (m *OpenClawManager) EnsureFile() error {
	path := m.Path()
	_, err := os.Stat(path)
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return shared.AtomicWriteFile(path, []byte(defaultOpenClawJSON), 0o644)
}

func (m *OpenClawManager) Read() (string, error) {
	if err := m.EnsureFile(); err != nil {
		return "", err
	}
	path := m.Path()
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (m *OpenClawManager) Write(content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return fmt.Errorf("openclaw.json content is empty")
	}

	var decoded any
	if err := json.Unmarshal([]byte(content), &decoded); err != nil {
		return fmt.Errorf("openclaw.json is not valid JSON: %w", err)
	}

	pretty, err := shared.PrettyJSON(decoded)
	if err != nil {
		return err
	}

	return shared.AtomicWriteFile(m.Path(), append(pretty, '\n'), 0o644)
}

func (m *OpenClawManager) Apply(content string) (bool, error) {
	current, err := m.Read()
	if err != nil {
		return false, err
	}
	if shared.HashString(current) == shared.HashString(content) {
		return false, nil
	}
	return true, m.Write(content)
}

func (m *OpenClawManager) RestartGateway(ctx context.Context) error {
	candidates := [][]string{
		{"openclaw", "gateway", "restart"},
		{"openclaw-cn", "gateway", "restart"},
	}

	for _, candidate := range candidates {
		if _, err := exec.LookPath(candidate[0]); err != nil {
			continue
		}

		restartCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		output, err := exec.CommandContext(restartCtx, candidate[0], candidate[1:]...).CombinedOutput()
		cancel()
		if err != nil {
			return fmt.Errorf("%s failed: %w (%s)", strings.Join(candidate, " "), err, strings.TrimSpace(string(output)))
		}
		m.logger.Printf("openclaw gateway restarted via %s", candidate[0])
		return nil
	}

	m.logger.Printf("openclaw restart skipped: executable not found")
	return nil
}

func normalizeOpenClawConfigPath(raw string) string {
	trimmed := strings.TrimSpace(strings.Trim(raw, `"'`))
	if trimmed == "" {
		return defaultOpenClawPath()
	}

	hasTrailingSeparator := strings.HasSuffix(trimmed, "/") || strings.HasSuffix(trimmed, `\`)
	path := normalizeFilesystemPath(trimmed)
	if path == "" {
		return defaultOpenClawPath()
	}

	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return filepath.Join(path, "openclaw.json")
	}
	if hasTrailingSeparator {
		return filepath.Join(path, "openclaw.json")
	}

	return path
}

func normalizeFilesystemPath(raw string) string {
	trimmed := strings.TrimSpace(strings.Trim(raw, `"'`))
	if trimmed == "" {
		return ""
	}

	trimmed = os.ExpandEnv(trimmed)
	if strings.HasPrefix(trimmed, "~") {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			switch {
			case trimmed == "~":
				trimmed = home
			case strings.HasPrefix(trimmed, "~/"), strings.HasPrefix(trimmed, `~\`):
				trimmed = filepath.Join(home, trimmed[2:])
			}
		}
	}

	cleaned := filepath.Clean(trimmed)
	if abs, err := filepath.Abs(cleaned); err == nil {
		return abs
	}
	return cleaned
}
