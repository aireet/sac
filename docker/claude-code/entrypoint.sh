#!/bin/bash

set -e

echo "Starting Claude Code Sandbox..."
echo "User ID: ${USER_ID}"
echo "Session ID: ${SESSION_ID}"

# Initialize Claude Code config directory and workspace
mkdir -p /root/.claude
mkdir -p /workspace/private /workspace/public

# Accept Claude Code terms of service non-interactively
touch /root/.claude/.accepted-tos

# Configure Claude Code hooks (conversation history sync via Node.js)
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
            "command": "node /hooks/conversation-sync.mjs",
            "async": true
          }
        ]
      }
    ],
    "SubagentStop": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "node /hooks/conversation-sync.mjs",
            "async": true
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
