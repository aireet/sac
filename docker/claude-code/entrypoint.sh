#!/bin/bash

set -e

echo "Starting Claude Code Sandbox..."
echo "User ID: ${USER_ID}"
echo "Session ID: ${SESSION_ID}"

# Initialize Claude Code if needed
if [ ! -d "/root/.claude" ]; then
    mkdir -p /root/.claude
    echo "Claude Code directory initialized"
fi

# Start ttyd with Claude Code CLI
# The --writable flag allows terminal input
# The -p flag sets the port
ttyd --writable -p 7681 bash

# If Claude Code CLI is properly installed, use:
# ttyd --writable -p 7681 claude
