#!/bin/bash

set -e

echo "Starting Claude Code Sandbox..."
echo "User ID: ${USER_ID}"
echo "Session ID: ${SESSION_ID}"

# Initialize Claude Code config directory and workspace
mkdir -p /root/.claude
mkdir -p /workspace/private /workspace/public /workspace/output

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

# Use dtach for session persistence. Unlike tmux, dtach does zero terminal
# interpretation — it passes raw bytes through transparently, so emoji,
# box-drawing characters, and true-color escape sequences render correctly.
# dtach -A: attach to session, create if not exists; -r winch: redraw on attach.
DTACH_SOCKET="/tmp/claude.sock"

# Wrapper script that dtach will run — auto-restarts claude on exit
cat > /tmp/claude-loop.sh <<'LOOP'
#!/bin/bash
while true; do
  claude
  echo "Claude exited. Restarting in 2s..."
  sleep 2
done
LOOP
chmod +x /tmp/claude-loop.sh

exec ttyd --writable -p 7681 dtach -A "$DTACH_SOCKET" -r winch /tmp/claude-loop.sh
