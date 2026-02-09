#!/bin/bash
# Claude Code Stop hook: reads transcript, extracts new user/assistant messages,
# and POSTs them to the SAC API Gateway for persistent storage.

set -euo pipefail

# Auto-install jq if not present (for images that don't include it)
if ! command -v jq &>/dev/null; then
  apt-get update -qq && apt-get install -y -qq jq >/dev/null 2>&1 || true
fi
if ! command -v jq &>/dev/null; then
  exit 0
fi

API_URL="${SAC_API_URL:-http://api-gateway.sac.svc.cluster.local:8080}"
USER_ID="${USER_ID:-}"
AGENT_ID="${AGENT_ID:-}"

if [ -z "$USER_ID" ] || [ -z "$AGENT_ID" ]; then
  exit 0
fi

# Read hook input from stdin (JSON with session_id, transcript_path, etc.)
INPUT=$(cat)
SESSION_ID=$(echo "$INPUT" | jq -r '.session_id // empty')
TRANSCRIPT_PATH=$(echo "$INPUT" | jq -r '.transcript_path // empty')

if [ -z "$SESSION_ID" ] || [ -z "$TRANSCRIPT_PATH" ] || [ ! -f "$TRANSCRIPT_PATH" ]; then
  exit 0
fi

# Track last synced line per session to do incremental uploads
SYNC_FILE="/tmp/.last_sync_line_${SESSION_ID}"
LAST_LINE=0
if [ -f "$SYNC_FILE" ]; then
  LAST_LINE=$(cat "$SYNC_FILE")
fi

# Count total lines
TOTAL_LINES=$(wc -l < "$TRANSCRIPT_PATH")

if [ "$TOTAL_LINES" -le "$LAST_LINE" ]; then
  exit 0
fi

# Extract new lines from the transcript JSONL
# Filter for user/assistant messages and build the payload
MESSAGES=$(tail -n +"$((LAST_LINE + 1))" "$TRANSCRIPT_PATH" | \
  jq -c 'select(.type == "user" or .type == "assistant") | {
    role: .type,
    content: (if (.message // empty) then
                (if (.message | type) == "string" then .message
                 elif (.message | type) == "array" then [.message[] | select(.type == "text") | .text] | join("\n")
                 else (.message | tostring)
                 end)
              elif (.content // empty) then
                (if (.content | type) == "string" then .content
                 elif (.content | type) == "array" then [.content[] | select(.type == "text") | .text] | join("\n")
                 else (.content | tostring)
                 end)
              else ""
              end),
    uuid: (.uuid // .id // ""),
    timestamp: (.timestamp // now | todate)
  } | select(.content != "")' 2>/dev/null | jq -s '.')

if [ -z "$MESSAGES" ] || [ "$MESSAGES" = "[]" ] || [ "$MESSAGES" = "null" ]; then
  # Update sync position even if no messages extracted
  echo "$TOTAL_LINES" > "$SYNC_FILE"
  exit 0
fi

# POST to API Gateway
PAYLOAD=$(jq -n \
  --arg uid "$USER_ID" \
  --arg aid "$AGENT_ID" \
  --arg sid "$SESSION_ID" \
  --argjson msgs "$MESSAGES" \
  '{user_id: $uid, agent_id: $aid, session_id: $sid, messages: $msgs}')

curl -s -X POST \
  "${API_URL}/api/internal/conversations/events" \
  -H "Content-Type: application/json" \
  -d "$PAYLOAD" \
  --max-time 10 \
  > /dev/null 2>&1 || true

# Update sync position
echo "$TOTAL_LINES" > "$SYNC_FILE"
