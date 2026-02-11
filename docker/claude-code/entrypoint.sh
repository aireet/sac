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

# Start Claude Code inside a persistent tmux session.
# Each WS connection spawns `tmux attach`, so disconnecting only kills
# the attach process — the tmux server + claude process keep running.
# Reconnecting attaches to the same session with full terminal state.
TMUX_SESSION="claude-main"
if ! tmux has-session -t "$TMUX_SESSION" 2>/dev/null; then
  tmux new-session -d -s "$TMUX_SESSION" -x 200 -y 50 \
    'while true; do claude; echo "Claude exited. Restarting in 2s..."; sleep 2; done'
fi
exec ttyd --writable -p 7681 tmux attach-session -t "$TMUX_SESSION"
