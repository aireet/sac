#!/bin/bash

set -e

echo "Starting Claude Code Sandbox..."
echo "User ID: ${USER_ID}"
echo "Session ID: ${SESSION_ID}"

# Initialize Claude Code config directory
mkdir -p /root/.claude

# Accept Claude Code terms of service non-interactively
touch /root/.claude/.accepted-tos

# Start ttyd with Claude Code CLI
ttyd --writable -p 7681 claude
