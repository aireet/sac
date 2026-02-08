<template>
  <div class="terminal-wrapper">
    <div ref="terminalContainer" class="terminal-container"></div>
  </div>
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
let resizeObserver: ResizeObserver | null = null
let intentionalClose = false

const initTerminal = () => {
  if (!terminalContainer.value) return
  if (!props.sessionId) return // Don't connect without a valid sessionId

  // Create terminal instance
  terminal = new Terminal({
    cursorBlink: false,
    disableStdin: true,
    fontSize: 14,
    fontFamily: 'Menlo, Monaco, "Courier New", monospace',
    theme: {
      background: '#1e1e1e',
      foreground: '#cccccc',
    },
  })

  // Create fit addon
  fitAddon = new FitAddon()
  terminal.loadAddon(fitAddon)

  // Open terminal in container
  terminal.open(terminalContainer.value)

  // Delay initial fit so the container has its final layout dimensions
  requestAnimationFrame(() => {
    fitAddon!.fit()
  })

  // Connect to WebSocket
  connectWebSocket()

  // Handle terminal resize - notify backend so ttyd can resize the PTY
  terminal.onResize(({ cols, rows }) => {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ type: 'resize', columns: cols, rows: rows }))
    }
  })

  // Use ResizeObserver to track container size changes (sidebar toggle, window resize, etc.)
  resizeObserver = new ResizeObserver(() => {
    if (fitAddon && terminal) {
      fitAddon.fit()
    }
  })
  resizeObserver.observe(terminalContainer.value)
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
    // Re-fit and send accurate dimensions after connection is established
    if (fitAddon && terminal) {
      fitAddon.fit()
      ws!.send(JSON.stringify({ type: 'resize', columns: terminal.cols, rows: terminal.rows }))
    }
  }

  ws.onmessage = (event) => {
    if (terminal) {
      terminal.write(event.data)
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

const sendMessage = (text: string) => {
  if (ws && ws.readyState === WebSocket.OPEN) {
    // Send text and Enter as separate messages so ttyd delivers them
    // as separate PTY writes — this ensures Claude Code sees the Enter key
    ws.send(text)
    setTimeout(() => {
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send('\r')
      }
    }, 50)
  }
}

const sendInterrupt = () => {
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send('\x03')
  }
}

const writeBanner = (agentName: string) => {
  if (!terminal) return
  terminal.clear()
  terminal.writeln('\x1b[2J') // clear screen
  terminal.writeln('')
  terminal.writeln('\x1b[1;36m  ╔══════════════════════════════════════════╗\x1b[0m')
  terminal.writeln(`\x1b[1;36m  ║\x1b[0m  \x1b[1;33m Switched to: \x1b[1;32m${agentName.padEnd(22)}\x1b[1;36m   ║\x1b[0m`)
  terminal.writeln('\x1b[1;36m  ╚══════════════════════════════════════════╝\x1b[0m')
  terminal.writeln('')
}

onMounted(() => {
  initTerminal()
})

// When sessionId changes (e.g. after creating a session), init/reconnect terminal
watch(() => props.sessionId, (newId, oldId) => {
  if (newId !== oldId) {
    // Always clean up old connection first to stop auto-reconnect loops
    cleanup()
  }
  if (newId) {
    if (!terminal) {
      initTerminal()
    } else {
      connectWebSocket()
    }
  }
})

onUnmounted(() => {
  if (resizeObserver) {
    resizeObserver.disconnect()
    resizeObserver = null
  }
  cleanup()

  if (terminal) {
    terminal.dispose()
    terminal = null
  }
})

defineExpose({
  sendMessage,
  sendInterrupt,
  cleanup,
  writeBanner,
})
</script>

<style scoped>
.terminal-wrapper {
  width: 100%;
  height: 100%;
  background: #1e1e1e;
  padding: 10px;
  box-sizing: border-box;
  overflow: hidden;
}

.terminal-container {
  width: 100%;
  height: 100%;
}
</style>
