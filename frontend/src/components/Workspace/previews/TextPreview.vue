<template>
  <textarea
    :value="content"
    class="text-editor"
    spellcheck="false"
    :readonly="!canSave"
    @input="$emit('update:content', ($event.target as HTMLTextAreaElement).value)"
  />
</template>

<script setup lang="ts">
defineProps<{
  content: string
  canSave: boolean
}>()

defineEmits<{
  'update:content': [value: string]
}>()
</script>

<style scoped>
.text-editor {
  width: 100%;
  flex: 1;
  min-height: 0;
  resize: none;
  border: none;
  outline: none;
  background: #1a1a1a;
  color: rgba(255, 255, 255, 0.87);
  font-family: 'SF Mono', 'Fira Code', 'Cascadia Code', monospace;
  font-size: 13px;
  line-height: 1.6;
  padding: 16px;
  tab-size: 2;
}

.text-editor:focus {
  outline: none;
}
</style>
