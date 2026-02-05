<template>
  <div ref="terminalContainer" class="terminal-container"></div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'

const props = defineProps<{
  userId: string
  sessionId: string
  wsUrl?: string
}>()

const terminalContainer = ref<HTMLElement>()
let terminal: Terminal | null = null
let fitAddon: FitAddon | null = null
let ws: WebSocket | null = null

const initTerminal = () => {
  if (!terminalContainer.value) return

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

  // Handle user input
  terminal.onData((data) => {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(data)
    }
  })

  // Handle window resize
  window.addEventListener('resize', handleResize)
}

const connectWebSocket = () => {
  const baseUrl = props.wsUrl || 'ws://localhost:8081'
  const url = `${baseUrl}/ws/${props.userId}/${props.sessionId}`

  console.log('Connecting to WebSocket:', url)

  ws = new WebSocket(url)

  ws.onopen = () => {
    console.log('WebSocket connected')
    terminal?.writeln('Connected to Claude Code sandbox...')
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
    terminal?.writeln('\r\nConnection closed. Attempting to reconnect...')

    // Attempt to reconnect after 3 seconds
    setTimeout(() => {
      if (terminal) {
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

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)

  if (ws) {
    ws.close()
  }

  if (terminal) {
    terminal.dispose()
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
