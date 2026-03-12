package backend

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"openclawdeploy/internal/shared"
)

const deployTemplate = `#!/usr/bin/env bash
set -euo pipefail

APP_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN_NAME="%s"
BIN_PATH="$APP_DIR/$BIN_NAME"
SERVICE_NAME="openclaw-client"
INSTALL_CMD='curl -fsSL https://clawd.org.cn/install.sh | bash -s -- --registry https://registry.npmmirror.com'

if [[ ! -f "$BIN_PATH" ]]; then
  echo "client binary not found: $BIN_PATH"
  exit 1
fi

chmod +x "$BIN_PATH"

if ! command -v curl >/dev/null 2>&1; then
  if command -v apt-get >/dev/null 2>&1; then
    sudo apt-get update
    sudo apt-get install -y curl
  else
    echo "curl is required and apt-get is unavailable"
    exit 1
  fi
fi

if command -v systemctl >/dev/null 2>&1; then
  sudo tee /etc/systemd/system/$SERVICE_NAME.service >/dev/null <<SERVICE
[Unit]
Description=OpenClaw Deploy Client
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=$APP_DIR
ExecStart=$BIN_PATH
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
SERVICE

  sudo systemctl daemon-reload
  sudo systemctl enable --now $SERVICE_NAME
else
  "$BIN_PATH"
fi

eval "$INSTALL_CMD"
`

func EnsureDeployScript(logger *log.Logger) error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	scriptPath := filepath.Join(shared.RuntimeBaseDir(), "clientdeploy.sh")
	content := fmt.Sprintf(deployTemplate, filepath.Base(exePath))
	if err := shared.AtomicWriteFile(scriptPath, []byte(content), 0o755); err != nil {
		return err
	}

	logger.Printf("deploy script prepared: %s", scriptPath)
	return nil
}
