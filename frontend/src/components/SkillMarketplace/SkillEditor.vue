<template>
  <div class="skill-editor-overlay">
    <!-- Title bar -->
    <div class="editor-titlebar">
      <div class="titlebar-left">
        <n-button quaternary size="small" @click="handleClose">
          <template #icon><n-icon><ArrowBack /></n-icon></template>
        </n-button>
        <n-text style="font-size: 14px; color: rgba(255,255,255,0.5)">
          .claude / skills / {{ meta.command_name || 'new-skill' }} /
        </n-text>
        <n-text strong style="font-size: 14px">{{ activeFile }}</n-text>
        <span v-if="dirtyFiles.size > 0" class="unsaved-badge">unsaved</span>
      </div>
      <div class="titlebar-right">
        <n-button size="small" quaternary @click="handleClose">Close</n-button>
        <n-button v-if="!readonly" size="small" type="primary" :loading="saving" @click="handleSave">
          {{ skill ? 'Save' : 'Create' }}
        </n-button>
      </div>
    </div>

    <div class="editor-main">
      <!-- Left panel: metadata + file tree -->
      <div class="left-panel">
        <!-- Skill metadata section -->
        <div class="meta-section">
          <div class="meta-section-title">
            <n-text depth="3" style="font-size: 11px; text-transform: uppercase; letter-spacing: 0.5px">Skill Info</n-text>
          </div>

          <div class="meta-field">
            <label class="meta-label">Name</label>
            <n-input v-model:value="meta.name" placeholder="My Skill" size="small" :disabled="readonly" />
          </div>

          <div class="meta-field">
            <label class="meta-label">Command</label>
            <n-input v-model:value="meta.command_name" placeholder="auto-generated" size="small" :disabled="readonly">
              <template #prefix><span style="color: rgba(255,255,255,0.3)">/</span></template>
            </n-input>
          </div>

          <div class="meta-field">
            <label class="meta-label">Description</label>
            <n-input
              v-model:value="meta.description"
              type="textarea"
              placeholder="What does this skill do?"
              size="small"
              :autosize="{ minRows: 2, maxRows: 4 }"
              :disabled="readonly"
            />
          </div>

          <div class="meta-row">
            <div class="meta-field" style="flex: 1">
              <label class="meta-label">Icon</label>
              <n-select
                v-model:value="meta.icon"
                :options="iconOptions"
                :render-label="renderIconLabel"
                size="small"
                :disabled="readonly"
              />
            </div>
            <div class="meta-field" style="flex: 1">
              <label class="meta-label">Visibility</label>
              <n-select
                :value="meta.is_public ? 'public' : 'private'"
                :options="[
                  { label: 'Private', value: 'private' },
                  { label: 'Public', value: 'public' },
                ]"
                size="small"
                :disabled="readonly"
                @update:value="(v: string) => meta.is_public = v === 'public'"
              />
            </div>
          </div>
        </div>

        <!-- File tree -->
        <SkillEditorSidebar
          :files="attachedFiles"
          :active-file="activeFile"
          :dirty-files="dirtyFiles"
          :skill-id="currentSkillId"
          :uploading="uploading"
          :command-name="meta.command_name"
          :readonly="readonly"
          @select="switchFile"
          @delete-file="handleDeleteFile"
          @new-file="handleNewFile"
          @upload="handleUpload"
          @upload-in="handleUploadIn"
          @upload-folder="handleUploadFolder"
        />
      </div>

      <!-- Right: Monaco editor -->
      <div class="editor-pane">
        <div v-if="binaryFile" class="binary-placeholder">
          <n-text depth="3">Binary file — cannot edit in browser</n-text>
        </div>
        <vue-monaco-editor
          v-else
          :value="currentContent"
          :language="currentLanguage"
          theme="vs-dark"
          :options="monacoOptions"
          @change="handleEditorChange"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, h } from 'vue'
import { VueMonacoEditor } from '@guolao/vue-monaco-editor'
import { NInput, NSelect, NButton, NText, NIcon, useMessage } from 'naive-ui'
import { ArrowBack } from '@vicons/ionicons5'
import SkillEditorSidebar from './SkillEditorSidebar.vue'
import {
  createSkill,
  updateSkill,
  listSkillFiles,
  uploadSkillFile,
  deleteSkillFile,
  getSkillFileContent,
  saveSkillFileContent,
  type Skill,
  type SkillFile,
  type SkillFrontmatter,
} from '../../services/skillAPI'
import { buildSkillMD, parseSkillMD, defaultSkillTemplate } from '../../utils/skillMarkdown'
import { extractApiError } from '../../utils/error'

const props = defineProps<{
  skill: Skill | null
  readonly?: boolean
}>()

const emit = defineEmits<{
  close: []
  saved: []
}>()

const message = useMessage()

// --- Metadata ---
const meta = ref({
  name: '',
  description: '',
  icon: '📝',
  command_name: '',
  is_public: false,
})

// --- Editor state ---
const currentSkillId = ref<number | null>(null)
const saving = ref(false)
const uploading = ref(false)
const activeFile = ref('SKILL.md')
const attachedFiles = ref<SkillFile[]>([])
const dirtyFiles = ref(new Set<string>())

const fileCache = ref<Map<string, string>>(new Map())
const originalCache = ref<Map<string, string>>(new Map())

const monacoOptions = computed(() => ({
  wordWrap: 'on' as const,
  minimap: { enabled: false },
  fontSize: 14,
  lineNumbers: 'on' as const,
  scrollBeyondLastLine: false,
  automaticLayout: true,
  tabSize: 2,
  readOnly: !!props.readonly,
}))

// --- Computed ---
const currentContent = computed(() => fileCache.value.get(activeFile.value) ?? '')

const currentLanguage = computed(() => {
  if (activeFile.value === 'SKILL.md') return 'markdown'
  return languageFromPath(activeFile.value)
})

const binaryFile = computed(() => {
  if (activeFile.value === 'SKILL.md') return false
  const f = attachedFiles.value.find(f => f.filepath === activeFile.value)
  if (!f) return false
  return isBinaryContentType(f.content_type)
})

// --- Init ---
onMounted(async () => {
  if (props.skill) {
    currentSkillId.value = props.skill.id
    meta.value = {
      name: props.skill.name,
      description: props.skill.description,
      icon: props.skill.icon || '📝',
      command_name: props.skill.command_name || '',
      is_public: props.skill.is_public,
    }
    const fm = props.skill.frontmatter ?? {}
    const md = buildSkillMD(fm, props.skill.prompt || '')
    fileCache.value.set('SKILL.md', md)
    originalCache.value.set('SKILL.md', md)
    await loadFiles()
  } else {
    const tpl = defaultSkillTemplate()
    fileCache.value.set('SKILL.md', tpl)
    originalCache.value.set('SKILL.md', tpl)
  }
})

// --- File operations ---
async function loadFiles() {
  if (!currentSkillId.value) return
  try {
    attachedFiles.value = await listSkillFiles(currentSkillId.value)
  } catch {
    attachedFiles.value = []
  }
}

function switchFile(filepath: string) {
  activeFile.value = filepath
  if (filepath !== 'SKILL.md' && !fileCache.value.has(filepath)) {
    loadFileContent(filepath)
  }
}

async function loadFileContent(filepath: string) {
  if (!currentSkillId.value) return
  const f = attachedFiles.value.find(f => f.filepath === filepath)
  if (f && isBinaryContentType(f.content_type)) return
  try {
    const resp = await getSkillFileContent(currentSkillId.value, filepath)
    fileCache.value.set(filepath, resp.content)
    originalCache.value.set(filepath, resp.content)
  } catch {
    fileCache.value.set(filepath, '')
    originalCache.value.set(filepath, '')
  }
}

function handleEditorChange(value: string | undefined) {
  const v = value ?? ''
  fileCache.value.set(activeFile.value, v)
  const orig = originalCache.value.get(activeFile.value) ?? ''
  if (v !== orig) {
    dirtyFiles.value.add(activeFile.value)
  } else {
    dirtyFiles.value.delete(activeFile.value)
  }
  dirtyFiles.value = new Set(dirtyFiles.value)
}

async function handleUpload(file: File) {
  if (!currentSkillId.value) return
  uploading.value = true
  try {
    await uploadSkillFile(currentSkillId.value, file)
    message.success('File uploaded')
    await loadFiles()
  } catch (error) {
    message.error(extractApiError(error, 'Upload failed'))
  } finally {
    uploading.value = false
  }
}

async function handleDeleteFile(filepath: string) {
  if (!currentSkillId.value) return
  try {
    await deleteSkillFile(currentSkillId.value, filepath)
    message.success('File removed')
    fileCache.value.delete(filepath)
    originalCache.value.delete(filepath)
    dirtyFiles.value.delete(filepath)
    if (activeFile.value === filepath) activeFile.value = 'SKILL.md'
    await loadFiles()
  } catch (error) {
    message.error(extractApiError(error, 'Delete failed'))
  }
}

async function handleNewFile(filepath: string) {
  if (!currentSkillId.value) return
  try {
    await saveSkillFileContent(currentSkillId.value, filepath, '')
    message.success('File created')
    await loadFiles()
    fileCache.value.set(filepath, '')
    originalCache.value.set(filepath, '')
    activeFile.value = filepath
  } catch (error) {
    message.error(extractApiError(error, 'Failed to create file'))
  }
}

async function handleUploadIn(file: File, dirPath: string) {
  if (!currentSkillId.value) return
  uploading.value = true
  try {
    await uploadSkillFile(currentSkillId.value, file, { dirPath: dirPath + '/' })
    message.success('File uploaded')
    await loadFiles()
  } catch (error) {
    message.error(extractApiError(error, 'Upload failed'))
  } finally {
    uploading.value = false
  }
}

async function handleUploadFolder(files: { file: File; filepath: string }[]) {
  if (!currentSkillId.value || files.length === 0) return
  uploading.value = true
  try {
    for (let i = 0; i < files.length; i++) {
      const f = files[i]!
      message.info(`Uploading ${i + 1}/${files.length}: ${f.filepath}`, { duration: 1500 })
      await uploadSkillFile(currentSkillId.value!, f.file, { filepath: f.filepath })
    }
    message.success(`Uploaded ${files.length} files`)
    await loadFiles()
  } catch (error) {
    message.error(extractApiError(error, 'Folder upload failed'))
  } finally {
    uploading.value = false
  }
}

// --- Save ---
async function handleSave() {
  if (!meta.value.name.trim()) {
    message.warning('Skill name is required')
    return
  }

  const mdContent = fileCache.value.get('SKILL.md') ?? ''
  const { frontmatter, prompt } = parseSkillMD(mdContent)

  if (!prompt.trim()) {
    message.warning('Prompt content is required')
    return
  }

  const fm: SkillFrontmatter = {
    allowed_tools: frontmatter.allowed_tools ?? [],
    model: frontmatter.model ?? '',
    context: frontmatter.context ?? '',
    agent: frontmatter.agent ?? '',
    disable_model_invocation: frontmatter.disable_model_invocation ?? false,
    argument_hint: frontmatter.argument_hint ?? '',
    user_invocable: frontmatter.user_invocable,
  }

  saving.value = true
  try {
    if (currentSkillId.value) {
      await updateSkill(currentSkillId.value, {
        name: meta.value.name,
        description: meta.value.description,
        icon: meta.value.icon,
        command_name: meta.value.command_name,
        is_public: meta.value.is_public,
        prompt,
        frontmatter: fm,
      })
      await saveDirtyFiles()
      message.success('Skill saved')
      originalCache.value.set('SKILL.md', mdContent)
      dirtyFiles.value.clear()
      dirtyFiles.value = new Set(dirtyFiles.value)
      emit('saved')
    } else {
      const created = await createSkill({
        name: meta.value.name,
        description: meta.value.description,
        icon: meta.value.icon,
        command_name: meta.value.command_name,
        is_public: meta.value.is_public,
        prompt,
        category: '',
        parameters: [],
        frontmatter: fm,
      })
      currentSkillId.value = created.id
      // Update command_name from server (auto-generated if empty)
      if (created.command_name) {
        meta.value.command_name = created.command_name
      }
      message.success('Skill created — file operations now available')
      originalCache.value.set('SKILL.md', mdContent)
      dirtyFiles.value.delete('SKILL.md')
      dirtyFiles.value = new Set(dirtyFiles.value)
      emit('saved')
    }
  } catch (error) {
    message.error(extractApiError(error, 'Failed to save'))
  } finally {
    saving.value = false
  }
}

async function saveDirtyFiles() {
  if (!currentSkillId.value) return
  for (const filepath of dirtyFiles.value) {
    if (filepath === 'SKILL.md') continue
    const content = fileCache.value.get(filepath)
    if (content === undefined) continue
    try {
      await saveSkillFileContent(currentSkillId.value, filepath, content)
      originalCache.value.set(filepath, content)
    } catch (error) {
      message.error(extractApiError(error, `Failed to save ${filepath}`))
    }
  }
}

function handleClose() {
  if (dirtyFiles.value.size > 0) {
    if (!window.confirm('You have unsaved changes. Discard?')) return
  }
  emit('close')
}

// --- Helpers ---
function languageFromPath(filepath: string): string {
  const ext = filepath.split('.').pop()?.toLowerCase() ?? ''
  const map: Record<string, string> = {
    js: 'javascript', ts: 'typescript', py: 'python', rb: 'ruby',
    go: 'go', rs: 'rust', java: 'java', kt: 'kotlin',
    md: 'markdown', json: 'json', yaml: 'yaml', yml: 'yaml',
    xml: 'xml', html: 'html', css: 'css', scss: 'scss',
    sh: 'shell', bash: 'shell', sql: 'sql', txt: 'plaintext',
    csv: 'plaintext', toml: 'ini', cfg: 'ini', ini: 'ini',
  }
  return map[ext] || 'plaintext'
}

function isBinaryContentType(ct: string): boolean {
  if (!ct) return false
  return ct.startsWith('image/') || ct.startsWith('audio/') || ct.startsWith('video/')
    || ct === 'application/octet-stream' || ct === 'application/zip'
    || ct === 'application/pdf' || ct === 'application/gzip'
}

const iconOptions = [
  { label: '📝 Note', value: '📝' },
  { label: '🔍 Search', value: '🔍' },
  { label: '⚙️ Settings', value: '⚙️' },
  { label: '💡 Bulb', value: '💡' },
  { label: '🚀 Rocket', value: '🚀' },
  { label: '🎯 Target', value: '🎯' },
  { label: '📊 Chart', value: '📊' },
  { label: '📦 Package', value: '📦' },
  { label: '🛠️ Tools', value: '🛠️' },
  { label: '💬 Chat', value: '💬' },
  { label: '🔥 Fire', value: '🔥' },
  { label: '⚡ Lightning', value: '⚡' },
  { label: '🎨 Palette', value: '🎨' },
  { label: '📂 Folder', value: '📂' },
  { label: '📈 Trending', value: '📈' },
  { label: '💰 Money', value: '💰' },
  { label: '📅 Calendar', value: '📅' },
  { label: '📧 Email', value: '📧' },
  { label: '🔗 Link', value: '🔗' },
  { label: '⭐ Star', value: '⭐' },
]

const renderIconLabel = (option: any) => {
  return h('div', { style: 'display: flex; align-items: center; gap: 6px;' }, [
    h('span', { style: 'font-size: 16px;' }, option.value),
    h('span', { style: 'font-size: 13px; color: rgba(255,255,255,0.6);' }, option.label.substring(3)),
  ])
}
</script>

<style scoped>
.skill-editor-overlay {
  position: fixed;
  inset: 0;
  z-index: 2000;
  background: #1e1e1e;
  display: flex;
  flex-direction: column;
}

/* Title bar */
.editor-titlebar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 12px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  background: #181818;
  flex-shrink: 0;
  min-height: 40px;
}

.titlebar-left {
  display: flex;
  align-items: center;
  gap: 6px;
}

.titlebar-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.unsaved-badge {
  font-size: 11px;
  color: #e2b340;
  background: rgba(226, 179, 64, 0.12);
  padding: 1px 6px;
  border-radius: 3px;
  margin-left: 4px;
}

/* Main layout */
.editor-main {
  flex: 1;
  display: flex;
  overflow: hidden;
}

/* Left panel: metadata + file tree */
.left-panel {
  width: 260px;
  min-width: 260px;
  display: flex;
  flex-direction: column;
  border-right: 1px solid rgba(255, 255, 255, 0.08);
  background: #181818;
  overflow: hidden;
}

/* Metadata section */
.meta-section {
  padding: 12px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  flex-shrink: 0;
  overflow-y: auto;
  max-height: 50%;
}

.meta-section-title {
  margin-bottom: 10px;
}

.meta-field {
  margin-bottom: 10px;
}

.meta-field:last-child {
  margin-bottom: 0;
}

.meta-label {
  display: block;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.45);
  margin-bottom: 4px;
}

.meta-row {
  display: flex;
  gap: 8px;
  margin-bottom: 10px;
}

.meta-row:last-child {
  margin-bottom: 0;
}

/* Editor pane */
.editor-pane {
  flex: 1;
  overflow: hidden;
}

.binary-placeholder {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  background: rgba(0, 0, 0, 0.1);
}
</style>
