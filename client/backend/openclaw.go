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
	return &OpenClawManager{path: path, logger: logger}
}

func (m *OpenClawManager) Path() string {
	return m.path
}

func (m *OpenClawManager) EnsureFile() error {
	_, err := os.Stat(m.path)
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return shared.AtomicWriteFile(m.path, []byte(defaultOpenClawJSON), 0o644)
}

func (m *OpenClawManager) Read() (string, error) {
	if err := m.EnsureFile(); err != nil {
		return "", err
	}
	data, err := os.ReadFile(m.path)
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

	return shared.AtomicWriteFile(m.path, append(pretty, '\n'), 0o644)
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
