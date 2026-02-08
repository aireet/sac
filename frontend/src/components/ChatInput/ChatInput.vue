<template>
  <div class="chat-input-bar">
    <div class="input-wrapper">
      <n-input
        v-model:value="inputText"
        type="textarea"
        :autosize="{ minRows: 3, maxRows: 8 }"
        placeholder="Type a message to Claude Code..."
        :disabled="disabled"
        @keydown="handleKeydown"
      />
    </div>
    <div class="button-group">
      <n-button
        type="warning"
        size="medium"
        secondary
        @click="$emit('interrupt')"
        title="Send Ctrl+C to stop"
      >
        Stop
      </n-button>
      <n-button
        type="primary"
        size="medium"
        :disabled="!inputText.trim() || disabled"
        @click="handleSend"
      >
        Send
      </n-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { NInput, NButton } from 'naive-ui'

defineProps<{
  disabled?: boolean
}>()

const emit = defineEmits<{
  send: [text: string]
  interrupt: []
}>()

const inputText = ref('')

const handleSend = () => {
  const text = inputText.value.trim()
  if (!text) return
  emit('send', text)
  inputText.value = ''
}

const handleKeydown = (e: KeyboardEvent) => {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    handleSend()
  }
}
</script>

<style scoped>
.chat-input-bar {
  display: flex;
  align-items: flex-end;
  gap: 12px;
  padding: 16px 20px 24px;
  background: #252525;
  border-top: 1px solid #3a3a3a;
  flex-shrink: 0;
  margin-bottom: 24px;
}

.input-wrapper {
  flex: 1;
  min-width: 0;
}

.input-wrapper :deep(.n-input) {
  font-family: Menlo, Monaco, 'Courier New', monospace;
  font-size: 14px;
}

.input-wrapper :deep(.n-input .n-input__textarea-el) {
  font-family: Menlo, Monaco, 'Courier New', monospace;
  font-size: 14px;
  text-align: left;
}

.input-wrapper :deep(.n-input .n-input__placeholder) {
  text-align: left;
}

.button-group {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
  padding-bottom: 4px;
}
</style>
