#!/usr/bin/env node
// Claude Code hook: runs on Stop/SubagentStop events.
// Reads transcript JSONL, extracts new user/assistant messages,
// and POSTs them to the SAC API Gateway for persistent storage.
//
// Zero external dependencies — uses Node.js 22 built-ins only.

import { readFileSync, writeFileSync, existsSync } from 'node:fs';
import { createReadStream } from 'node:fs';
import { createInterface } from 'node:readline';

const API_URL = process.env.SAC_API_URL || 'http://api-gateway.sac.svc.cluster.local:8080';
const USER_ID = process.env.USER_ID || '';
const AGENT_ID = process.env.AGENT_ID || '';

if (!USER_ID || !AGENT_ID) {
  process.exit(0);
}

// Read hook input from stdin
const chunks = [];
for await (const chunk of process.stdin) {
  chunks.push(chunk);
}
const input = JSON.parse(Buffer.concat(chunks).toString());

// Prevent infinite loops when Stop hook causes Claude to continue
if (input.stop_hook_active) {
  process.exit(0);
}

const sessionId = input.session_id;
const transcriptPath = input.transcript_path;

if (!sessionId || !transcriptPath || !existsSync(transcriptPath)) {
  process.exit(0);
}

// Track last synced line per session for incremental uploads
const syncFile = `/tmp/.last_sync_line_${sessionId}`;
let lastLine = 0;
if (existsSync(syncFile)) {
  lastLine = parseInt(readFileSync(syncFile, 'utf-8').trim(), 10) || 0;
}

// Read transcript JSONL and extract new messages
const messages = [];
let lineNum = 0;

const rl = createInterface({
  input: createReadStream(transcriptPath),
  crlfDelay: Infinity,
});

for await (const line of rl) {
  lineNum++;
  if (lineNum <= lastLine) continue;
  if (!line.trim()) continue;

  let entry;
  try {
    entry = JSON.parse(line);
  } catch {
    continue; // skip malformed lines
  }

  if (entry.type !== 'user' && entry.type !== 'assistant') continue;

  const content = extractContent(entry);
  if (!content) continue;

  messages.push({
    role: entry.type,
    content,
    uuid: entry.uuid || entry.id || '',
    timestamp: entry.timestamp || new Date().toISOString(),
  });
}

const totalLines = lineNum;

// Update sync position even if no messages extracted
writeFileSync(syncFile, String(totalLines));

if (messages.length === 0) {
  process.exit(0);
}

// POST to API Gateway
const payload = {
  user_id: USER_ID,
  agent_id: AGENT_ID,
  session_id: sessionId,
  messages,
};

try {
  const resp = await fetch(`${API_URL}/api/internal/conversations/events`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
    signal: AbortSignal.timeout(10_000),
  });
  if (!resp.ok) {
    console.error(`[conversation-sync] API responded ${resp.status}: ${await resp.text()}`);
  }
} catch (err) {
  console.error(`[conversation-sync] POST failed: ${err.message}`);
}

// --- helpers ---

function extractContent(entry) {
  // entry.message can be: string, object with .content, or array of content blocks
  const msg = entry.message;
  if (msg == null) return '';

  if (typeof msg === 'string') return msg;

  if (Array.isArray(msg)) {
    return joinTextBlocks(msg);
  }

  if (typeof msg === 'object') {
    const c = msg.content;
    if (c == null) return '';
    if (typeof c === 'string') return c;
    if (Array.isArray(c)) return joinTextBlocks(c);
    return String(c);
  }

  return String(msg);
}

function joinTextBlocks(arr) {
  return arr
    .filter((b) => b.type === 'text' && b.text)
    .map((b) => b.text)
    .join('\n');
}
