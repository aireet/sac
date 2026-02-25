<template>
  <div class="skill-card" @click="$emit('view', skill)">
    <div class="card-header">
      <span class="card-icon">{{ skill.icon }}</span>
      <div class="card-badges">
        <n-tag v-if="skill.is_official" type="success" size="small" round>Official</n-tag>
        <n-tag v-else-if="skill.is_public" type="info" size="small" round>Public</n-tag>
        <n-tag v-else size="small" round>Private</n-tag>
        <n-tag v-if="skill.frontmatter?.model" size="small" round type="warning">{{ skill.frontmatter.model }}</n-tag>
        <n-tag v-if="hasToolRestrictions" size="small" round>Tools</n-tag>
        <n-tag v-if="fileCount > 0" size="small" round>{{ fileCount }} file{{ fileCount > 1 ? 's' : '' }}</n-tag>
      </div>
    </div>

    <div class="card-body">
      <n-text strong class="card-name">{{ skill.name }}</n-text>
      <n-text depth="3" class="card-command">/{{ skill.command_name }}</n-text>
      <n-ellipsis :line-clamp="2" :tooltip="false" class="card-desc">
        <n-text depth="3">{{ skill.description || 'No description' }}</n-text>
      </n-ellipsis>
    </div>

    <div class="card-footer" @click.stop>
      <template v-if="owned">
        <n-button size="small" quaternary @click="$emit('edit', skill)">Edit</n-button>
        <n-popconfirm @positive-click="$emit('delete', skill)">
          <template #trigger>
            <n-button size="small" quaternary type="error">Delete</n-button>
          </template>
          Delete this skill?
        </n-popconfirm>
      </template>
      <template v-else>
        <n-button
          v-if="!owned && !skill.is_official"
          size="small"
          quaternary
          @click="$emit('fork', skill)"
        >
          Fork
        </n-button>
      </template>

      <div style="flex: 1" />

      <n-tooltip v-if="!canInstall" trigger="hover">
        <template #trigger>
          <n-button size="small" disabled>Install</n-button>
        </template>
        Select an agent first
      </n-tooltip>
      <n-button
        v-else-if="installed"
        size="small"
        disabled
      >
        Installed
      </n-button>
      <n-button
        v-else
        size="small"
        type="primary"
        :loading="installing"
        @click="$emit('install', skill)"
      >
        Install
      </n-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { NText, NTag, NEllipsis, NButton, NTooltip, NPopconfirm } from 'naive-ui'
import type { Skill } from '../../services/skillAPI'

const props = defineProps<{
  skill: Skill
  installed: boolean
  installing: boolean
  canInstall: boolean
  owned: boolean
}>()

defineEmits<{
  install: [skill: Skill]
  view: [skill: Skill]
  edit: [skill: Skill]
  delete: [skill: Skill]
  fork: [skill: Skill]
}>()

const hasToolRestrictions = computed(() =>
  (props.skill.frontmatter?.allowed_tools?.length ?? 0) > 0
)

const fileCount = computed(() => props.skill.files?.length ?? 0)
</script>

<style scoped>
.skill-card {
  background: rgba(255, 255, 255, 0.04);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 12px;
  padding: 20px;
  cursor: pointer;
  transition: transform 0.2s, box-shadow 0.2s, border-color 0.2s;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.skill-card:hover {
  transform: translateY(-4px);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.3);
  border-color: rgba(255, 255, 255, 0.15);
}

.card-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
}

.card-icon {
  font-size: 36px;
  line-height: 1;
}

.card-badges {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.card-body {
  display: flex;
  flex-direction: column;
  gap: 4px;
  flex: 1;
}

.card-name {
  font-size: 15px;
  line-height: 1.3;
}

.card-command {
  font-size: 12px;
  font-family: monospace;
  opacity: 0.6;
}

.card-desc {
  font-size: 13px;
  margin-top: 4px;
  min-height: 2.6em;
}

.card-footer {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-top: 8px;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
}
</style>
