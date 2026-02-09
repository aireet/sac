<template>
  <div class="terminal-wrapper">
    <div ref="terminalContainer" class="terminal-container"></div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebglAddon } from '@xterm/addon-webgl'
import { Unicode11Addon } from '@xterm/addon-unicode11'
import '@xterm/xterm/css/xterm.css'
import { useAuthStore } from '../../stores/auth'

const props = defineProps<{
  sessionId: string
  wsUrl?: string
  agentId?: number
  interactiveMode?: boolean
}>()

const authStore = useAuthStore()
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
    cursorBlink: !!props.interactiveMode,
    disableStdin: !props.interactiveMode,
    fontSize: 14,
    fontFamily: 'Menlo, Monaco, "Courier New", monospace',
    allowProposedApi: true,
    theme: {
      background: '#1e1e1e',
      foreground: '#cccccc',
    },
  })

  // Create fit addon
  fitAddon = new FitAddon()
  terminal.loadAddon(fitAddon)

  // Load Unicode11 addon for proper CJK wide-character handling
  const unicode11 = new Unicode11Addon()
  terminal.loadAddon(unicode11)
  terminal.unicode.activeVersion = '11'

  // Open terminal in container
  terminal.open(terminalContainer.value)

  // Activate WebGL renderer for pixel-perfect character grid alignment.
  // The default DOM renderer uses <span> elements whose widths accumulate
  // sub-pixel rounding errors, causing columns to drift. The WebGL renderer
  // draws each character cell at exact pixel coordinates, eliminating this.
  try {
    const webgl = new WebglAddon()
    webgl.onContextLoss(() => {
      webgl.dispose()
    })
    terminal.loadAddon(webgl)
  } catch (e) {
    console.warn('WebGL renderer not available, using default DOM renderer:', e)
  }

  // Delay initial fit so the container has its final layout dimensions
  requestAnimationFrame(() => {
    fitAddon!.fit()
  })

  // Connect to WebSocket
  connectWebSocket()

  // In interactive mode, forward keystrokes directly to the PTY via WebSocket
  if (props.interactiveMode) {
    terminal.onData((data) => {
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(data)
      }
    })
  }

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
  // Development: localhost with separate ws-proxy port
  if (host === 'localhost' || host === '127.0.0.1') {
    return `${protocol}//${host}:8081`
  }
  // Production / Gateway: same-origin routing (works with any port)
  return `${protocol}//${window.location.host}`
}

const cleanup = () => {
  intentionalClose = true
  if (reconnectTimer) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
  if (ws) {
    // Remove event handlers before closing to prevent stale onclose
    // from racing with a new connection and triggering reconnect loops
    ws.onopen = null
    ws.onmessage = null
    ws.onerror = null
    ws.onclose = null
    ws.close()
    ws = null
  }
}

const connectWebSocket = () => {
  if (!props.sessionId) return

  cleanup()
  intentionalClose = false

  const baseUrl = getWebSocketUrl()
  let url = `${baseUrl}/ws/${props.sessionId}`

  // Add token and agent_id query parameters
  const params = new URLSearchParams()
  if (authStore.token) {
    params.set('token', authStore.token)
  }
  if (props.agentId && props.agentId > 0) {
    params.set('agent_id', String(props.agentId))
  }
  const qs = params.toString()
  if (qs) {
    url += `?${qs}`
  }

  console.log('Connecting to WebSocket:', url)

  ws = new WebSocket(url)
  // Receive PTY output as raw binary to preserve byte stream integrity
  // (prevents UTF-8 fragmentation issues with TextMessage)
  ws.binaryType = 'arraybuffer'

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
      if (event.data instanceof ArrayBuffer) {
        // Binary frame from proxy — pass raw bytes to xterm.js
        // xterm.js handles UTF-8 decoding internally, including buffering
        // incomplete multi-byte sequences across messages
        terminal.write(new Uint8Array(event.data))
      } else {
        // Text frame (e.g. error messages during connection setup)
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

const sendMessage = (text: string) => {
  console.log('[Terminal] sendMessage:', text, 'ws:', !!ws, 'readyState:', ws?.readyState)
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(text)
    setTimeout(() => {
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send('\r')
      }
    }, 50)
  } else {
    console.warn('[Terminal] WebSocket not connected, cannot send message')
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

// When interactiveMode changes, re-create the terminal with new settings
watch(() => props.interactiveMode, () => {
  // Tear down old terminal and WebSocket
  cleanup()
  if (resizeObserver) {
    resizeObserver.disconnect()
    resizeObserver = null
  }
  if (terminal) {
    terminal.dispose()
    terminal = null
  }
  // Re-create with new mode
  nextTick(() => {
    initTerminal()
  })
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
