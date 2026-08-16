#!/bin/sh
set -eu

# -- DN42 Peer Finder Agent – Installer --------------------------
AGENT_URL="https://peerfinder.dn42.dev/agent/peerfinder-agent.py"
SERVICE_URL="https://peerfinder.dn42.dev/agent/peerfinder-agent.service"
AGENT_PATH="/usr/local/bin/peerfinder-agent.py"
UNIT_PATH="/etc/systemd/system/peerfinder-agent.service"
OVERRIDE_DIR="/etc/systemd/system/peerfinder-agent.service.d"
KEY_PATH="/etc/peerfinder/secret.key"

if [ "$(id -u)" -ne 0 ]; then
  echo "Run as root (use sudo)."
  exit 1
fi

# -- Parse --secret argument -------------------------------------
SECRET_KEY=""

while [ "$#" -gt 0 ]; do
  case "$1" in
    --secret=*)
      SECRET_KEY="${1#*=}"
      ;;
    --secret)
      shift
      SECRET_KEY="${1:-}"
      ;;
  esac
  shift
done

# -- Use existing key if present and no --secret was given -------
if [ -z "$SECRET_KEY" ] && [ -f "$KEY_PATH" ]; then
  echo "==> Found existing secret key at $KEY_PATH"
  SECRET_KEY="already-stored"
fi

# -- Ask for secret key if still not available --------------------
if [ -z "$SECRET_KEY" ]; then
  printf "Enter your Peer Finder secret key: "
  read -r SECRET_KEY </dev/tty
  if [ -z "$SECRET_KEY" ]; then
    echo "No key provided — aborting."
    exit 1
  fi
fi

# -- Ensure python3 is available (stdlib required) ---------------
if ! command -v python3 >/dev/null 2>&1; then
  echo "==> python3 not found — installing python3-minimal…"
  if command -v apt-get >/dev/null 2>&1; then
    apt-get update && apt-get install -y python3-minimal
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y python3
  elif command -v yum >/dev/null 2>&1; then
    yum install -y python3
  elif command -v apk >/dev/null 2>&1; then
    apk add python3
  elif command -v pacman >/dev/null 2>&1; then
    pacman -S --noconfirm python3
  elif command -v zypper >/dev/null 2>&1; then
    zypper install -y python3
  else
    echo "Unsupported package manager — install python3 manually."
    exit 1
  fi
fi

# -- Download agent & systemd unit -------------------------------
echo "==> Downloading agent…"
curl -fsSL "$AGENT_URL" -o "$AGENT_PATH"
chmod 0755 "$AGENT_PATH"

echo "==> Downloading systemd unit…"
curl -fsSL "$SERVICE_URL" -o "$UNIT_PATH"

# -- Store secret key (readable by root only) --------------------
echo "==> Storing secret key…"
mkdir -p /etc/peerfinder
printf '%s' "$SECRET_KEY" > "$KEY_PATH"
chmod 600 "$KEY_PATH"

# -- Drop-in override: unencrypted systemd credential ------------
echo "==> Writing systemd drop-in override…"
mkdir -p "$OVERRIDE_DIR"
cat > "$OVERRIDE_DIR/override.conf" <<'OVERRIDE'
[Service]
LoadCredential=SECRET_KEY_FILE:/etc/peerfinder/secret.key
Environment="SECRET_KEY_FILE=%d/SECRET_KEY_FILE"
OVERRIDE

# -- Enable & start ----------------------------------------------
echo "==> Enabling and starting service…"
systemctl daemon-reload
systemctl enable --now peerfinder-agent

echo "✓ Done. Check status with: systemctl status peerfinder-agent"
