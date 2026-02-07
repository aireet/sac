<template>
  <div ref="terminalContainer" class="terminal-container"></div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted } from 'vue'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'

const props = defineProps<{
  userId: string
  sessionId: string
  wsUrl?: string
  agentId?: number
}>()

const terminalContainer = ref<HTMLElement>()
let terminal: Terminal | null = null
let fitAddon: FitAddon | null = null
let ws: WebSocket | null = null
let reconnectTimer: ReturnType<typeof setTimeout> | null = null
let intentionalClose = false
let inputBuffer = ''

const initTerminal = () => {
  if (!terminalContainer.value) return
  if (!props.sessionId) return // Don't connect without a valid sessionId

  // Create terminal instance
  terminal = new Terminal({
    cursorBlink: true,
    fontSize: 14,
    fontFamily: 'Menlo, Monaco, "Courier New", monospace',
    theme: {
      background: '#1e1e1e',
      foreground: '#cccccc',
    },
    rows: 30,
    cols: 100,
  })

  // Create fit addon
  fitAddon = new FitAddon()
  terminal.loadAddon(fitAddon)

  // Open terminal in container
  terminal.open(terminalContainer.value)
  fitAddon.fit()

  // Connect to WebSocket
  connectWebSocket()

  // Handle user input - buffer locally, send complete line on Enter
  terminal.onData((data) => {
    if (!ws || ws.readyState !== WebSocket.OPEN) return

    for (let i = 0; i < data.length; i++) {
      const char = data[i]
      const code = char.charCodeAt(0)

      if (char === '\r') {
        // Enter: erase local echo, send complete line to server
        // Server PTY will echo the text back naturally
        if (inputBuffer.length > 0) {
          terminal!.write('\b \b'.repeat(inputBuffer.length))
        }
        ws.send(inputBuffer + '\r')
        inputBuffer = ''
      } else if (char === '\x7f') {
        // Backspace: delete last character from buffer
        if (inputBuffer.length > 0) {
          inputBuffer = inputBuffer.slice(0, -1)
          terminal!.write('\b \b')
        }
      } else if (char === '\x03') {
        // Ctrl+C: clear buffer, send interrupt immediately
        if (inputBuffer.length > 0) {
          terminal!.write('\b \b'.repeat(inputBuffer.length))
          inputBuffer = ''
        }
        ws.send('\x03')
      } else if (char === '\x04') {
        // Ctrl+D: send EOF immediately
        ws.send('\x04')
      } else if (char === '\x15') {
        // Ctrl+U: clear entire input buffer
        if (inputBuffer.length > 0) {
          terminal!.write('\b \b'.repeat(inputBuffer.length))
          inputBuffer = ''
        }
      } else if (char === '\x1b') {
        // Escape sequence (arrow keys etc.) - skip entire sequence
        if (i + 1 < data.length && data[i + 1] === '[') {
          i++ // skip '['
          while (i + 1 < data.length) {
            i++
            if (data.charCodeAt(i) >= 0x40 && data.charCodeAt(i) <= 0x7e) break
          }
        }
      } else if (code >= 32) {
        // Printable character: buffer + local echo
        inputBuffer += char
        terminal!.write(char)
      }
    }
  })

  // Handle terminal resize - notify backend so ttyd can resize the PTY
  terminal.onResize(({ cols, rows }) => {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ type: 'resize', columns: cols, rows: rows }))
    }
  })

  // Handle window resize
  window.addEventListener('resize', handleResize)
}

const getWebSocketUrl = (): string => {
  if (props.wsUrl) {
    return props.wsUrl
  }
  // Auto-detect WebSocket URL based on current host
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const host = window.location.hostname
  return `${protocol}//${host}:8081`
}

const cleanup = () => {
  intentionalClose = true
  if (reconnectTimer) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
  if (ws) {
    ws.close()
    ws = null
  }
}

const connectWebSocket = () => {
  if (!props.sessionId) return

  cleanup()
  intentionalClose = false
  inputBuffer = ''

  const baseUrl = getWebSocketUrl()
  let url = `${baseUrl}/ws/${props.userId}/${props.sessionId}`

  // Add agent_id query parameter if provided
  if (props.agentId && props.agentId > 0) {
    url += `?agent_id=${props.agentId}`
  }

  console.log('Connecting to WebSocket:', url)

  ws = new WebSocket(url)

  ws.onopen = () => {
    console.log('WebSocket connected')
    // Send initial resize so the backend knows our terminal dimensions
    if (terminal) {
      ws!.send(JSON.stringify({ type: 'resize', columns: terminal.cols, rows: terminal.rows }))
    }
  }

  ws.onmessage = (event) => {
    if (terminal) {
      // If user has partially typed input, hide it before writing server output,
      // then redraw after, so output doesn't interleave with the input buffer
      if (inputBuffer.length > 0) {
        terminal.write('\b \b'.repeat(inputBuffer.length))
        terminal.write(event.data)
        terminal.write(inputBuffer)
      } else {
        terminal.write(event.data)
      }
    }
  }

  ws.onerror = (error) => {
    console.error('WebSocket error:', error)
    terminal?.writeln('\r\nWebSocket error occurred')
  }

  ws.onclose = () => {
    console.log('WebSocket closed')
    if (intentionalClose) return

    terminal?.writeln('\r\nConnection closed. Attempting to reconnect...')
    reconnectTimer = setTimeout(() => {
      if (terminal && props.sessionId) {
        connectWebSocket()
      }
    }, 3000)
  }
}

const handleResize = () => {
  if (fitAddon && terminal) {
    fitAddon.fit()
  }
}

const sendCommand = (command: string) => {
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(command + '\r')
  }
}

onMounted(() => {
  initTerminal()
})

// When sessionId changes (e.g. after creating a session), init/reconnect terminal
watch(() => props.sessionId, (newId) => {
  if (newId) {
    if (!terminal) {
      initTerminal()
    } else {
      connectWebSocket()
    }
  }
})

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
  cleanup()

  if (terminal) {
    terminal.dispose()
    terminal = null
  }
})

// Expose sendCommand for parent components
defineExpose({
  sendCommand,
})
</script>

<style scoped>
.terminal-container {
  width: 100%;
  height: 100%;
  background: #1e1e1e;
  padding: 10px;
}
</style>
