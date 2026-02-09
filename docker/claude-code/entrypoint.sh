#!/bin/bash

set -e

echo "Starting Claude Code Sandbox..."
echo "User ID: ${USER_ID}"
echo "Session ID: ${SESSION_ID}"

# Initialize Claude Code config directory
mkdir -p /root/.claude

# Accept Claude Code terms of service non-interactively
touch /root/.claude/.accepted-tos

# Ensure hook scripts are executable
if [ -d /hooks ]; then
  chmod +x /hooks/*.sh 2>/dev/null || true
fi

# Configure Claude Code hooks (stop hook for conversation history sync)
# Skip if settings.json already exists (e.g. mounted from K8s ConfigMap)
if [ ! -f /root/.claude/settings.json ]; then
  cat > /root/.claude/settings.json <<SETTINGS
{
  "hooks": {
    "Stop": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "/hooks/stop-hook.sh"
          }
        ]
      }
    ]
  }
}
SETTINGS
fi

# Start ttyd with Claude Code CLI
ttyd --writable -p 7681 claude
