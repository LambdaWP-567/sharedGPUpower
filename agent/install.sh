#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="${SGPU_INSTALL_DIR:-$HOME/.sharedgpu}"
CONFIG_DIR="$INSTALL_DIR"
BINARY="$INSTALL_DIR/agent"
VERSION="${SGPU_VERSION:-latest}"
REPO="lambdawp-567/sharedgpupower"

ARCH=$(uname -m)
OS=$(uname -s | tr '[:upper:]' '[:lower:]')

case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH" && exit 1 ;;
esac

echo "sharedGPUpower agent installer"
echo "  OS   : $OS"
echo "  Arch : $ARCH"
echo "  Dir  : $INSTALL_DIR"
echo ""

mkdir -p "$INSTALL_DIR"

# Download binary
if [ "$VERSION" = "latest" ]; then
  DOWNLOAD_URL="https://github.com/$REPO/releases/latest/download/agent-${OS}-${ARCH}"
else
  DOWNLOAD_URL="https://github.com/$REPO/releases/download/${VERSION}/agent-${OS}-${ARCH}"
fi

echo "Downloading agent from $DOWNLOAD_URL ..."
curl -fsSL "$DOWNLOAD_URL" -o "$BINARY"
chmod +x "$BINARY"

# Prompt for API key
echo ""
echo "Enter your API key from the dashboard (sgpu_...) — press Enter to skip:"
read -r API_KEY

# Create default config if not present
if [ ! -f "$CONFIG_DIR/agent.yaml" ]; then
  echo "Creating default config at $CONFIG_DIR/agent.yaml ..."
  cat > "$CONFIG_DIR/agent.yaml" <<EOF
backend:
  endpoint: "grpc.sharedgpupower.example.com:443"
  insecure: false

resources:
  cpu_percent: 50
  ram_percent: 25
  gpu_layers: 20

ollama:
  host: "http://localhost:11434"
  default_model: "llama3.2:3b"

agent:
  name: "$(hostname)"

auth:
  user_api_key: "${API_KEY}"
  cert_path: "$CONFIG_DIR/agent.crt"
  key_path: "$CONFIG_DIR/agent.key"
  ca_cert_path: "$CONFIG_DIR/ca.crt"
EOF
  echo ""
  echo "Config written to $CONFIG_DIR/agent.yaml"
else
  # Update API key in existing config if provided
  if [ -n "$API_KEY" ]; then
    if grep -q "user_api_key:" "$CONFIG_DIR/agent.yaml"; then
      sed -i "s|user_api_key:.*|user_api_key: \"$API_KEY\"|" "$CONFIG_DIR/agent.yaml"
    else
      cat >> "$CONFIG_DIR/agent.yaml" <<EOF

auth:
  user_api_key: "${API_KEY}"
  cert_path: "$CONFIG_DIR/agent.crt"
  key_path: "$CONFIG_DIR/agent.key"
  ca_cert_path: "$CONFIG_DIR/ca.crt"
EOF
    fi
    echo "API key saved to config."
  fi
fi

# Install as macOS LaunchAgent
if [ "$OS" = "darwin" ]; then
  PLIST="$HOME/Library/LaunchAgents/com.sharedgpupower.agent.plist"
  cat > "$PLIST" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.sharedgpupower.agent</string>
    <key>ProgramArguments</key>
    <array>
        <string>$BINARY</string>
        <string>--config</string>
        <string>$CONFIG_DIR/agent.yaml</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>$INSTALL_DIR/agent.log</string>
    <key>StandardErrorPath</key>
    <string>$INSTALL_DIR/agent-error.log</string>
</dict>
</plist>
EOF
  launchctl load "$PLIST" 2>/dev/null || true
  echo "Installed as launchd service: com.sharedgpupower.agent"
  echo "  Start:   launchctl start com.sharedgpupower.agent"
  echo "  Stop:    launchctl stop com.sharedgpupower.agent"
  echo "  Logs:    tail -f $INSTALL_DIR/agent.log"

# Install as Linux systemd service
elif [ "$OS" = "linux" ]; then
  SERVICE_FILE="/etc/systemd/system/sharedgpupower-agent.service"
  sudo tee "$SERVICE_FILE" > /dev/null <<EOF
[Unit]
Description=sharedGPUpower Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$BINARY --config $CONFIG_DIR/agent.yaml
Restart=on-failure
RestartSec=10
User=$USER
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF
  sudo systemctl daemon-reload
  sudo systemctl enable sharedgpupower-agent
  echo "Installed as systemd service: sharedgpupower-agent"
  echo "  Start:   sudo systemctl start sharedgpupower-agent"
  echo "  Stop:    sudo systemctl stop sharedgpupower-agent"
  echo "  Logs:    journalctl -u sharedgpupower-agent -f"
fi

echo ""
echo "Installation complete."
echo "Edit your config at: $CONFIG_DIR/agent.yaml"
