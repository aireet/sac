<template>
  <n-config-provider :theme="darkTheme">
    <n-layout has-sider style="height: 100vh">
      <n-layout-sider
        bordered
        :width="400"
        :collapsed-width="0"
        :show-trigger="'bar'"
        collapse-mode="width"
      >
        <n-tabs type="segment" animated style="padding: 12px">
          <n-tab-pane name="skills" tab="Skills">
            <SkillPanel @execute-command="handleExecuteCommand" />
          </n-tab-pane>
          <n-tab-pane name="register" tab="Manage">
            <SkillEditor />
          </n-tab-pane>
        </n-tabs>
      </n-layout-sider>

      <n-layout-content>
        <Terminal
          ref="terminalRef"
          :user-id="userId"
          :session-id="sessionId"
          :ws-url="wsUrl"
        />
      </n-layout-content>
    </n-layout>
  </n-config-provider>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import {
  NConfigProvider,
  NLayout,
  NLayoutSider,
  NLayoutContent,
  NTabs,
  NTabPane,
  darkTheme,
} from 'naive-ui'
import Terminal from '../components/Terminal/Terminal.vue'
import SkillPanel from '../components/SkillPanel/SkillPanel.vue'
import SkillEditor from '../components/SkillRegister/SkillEditor.vue'

// Configuration - these should come from environment or auth context
const userId = ref('1')
const sessionId = ref('test-session-' + Date.now())
const wsUrl = ref(import.meta.env.VITE_WS_URL || 'ws://localhost:8081')

const terminalRef = ref()

const handleExecuteCommand = (command: string) => {
  if (terminalRef.value) {
    terminalRef.value.sendCommand(command)
  }
}
</script>

<style scoped>
/* Add any custom styles here */
</style>
