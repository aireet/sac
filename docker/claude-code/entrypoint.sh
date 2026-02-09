#!/bin/bash

set -e

echo "Starting Claude Code Sandbox..."
echo "User ID: ${USER_ID}"
echo "Session ID: ${SESSION_ID}"

# Initialize Claude Code config directory
mkdir -p /root/.claude

# Accept Claude Code terms of service non-interactively
touch /root/.claude/.accepted-tos

# Ensure hook scripts are executable (mounted from ConfigMap)
if [ -d /hooks ]; then
  chmod +x /hooks/*.sh 2>/dev/null || true
fi

# Start ttyd with Claude Code CLI
ttyd --writable -p 7681 claude
