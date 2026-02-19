const TEXT_EXTENSIONS = new Set([
  'md', 'txt', 'json', 'yaml', 'yml', 'xml', 'css',
  'js', 'ts', 'jsx', 'tsx', 'vue', 'py', 'go', 'rs', 'java', 'c', 'cpp', 'h',
  'sh', 'bash', 'zsh', 'fish', 'bat', 'ps1', 'rb', 'php', 'pl', 'lua',
  'sql', 'toml', 'ini', 'cfg', 'conf', 'env', 'gitignore', 'dockerignore',
  'csv', 'log', 'diff', 'patch',
])

const IMAGE_EXTENSIONS = new Set([
  'png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp', 'ico', 'svg',
])

const EXTENSIONLESS_TEXT = new Set([
  'Makefile', 'Dockerfile', 'README', 'LICENSE', 'CHANGELOG',
  'Vagrantfile', 'Gemfile', 'Rakefile', 'Procfile',
])

export type FileCategory = 'text' | 'image' | 'csv' | 'html' | 'binary'

export function getFileCategory(fileName: string): FileCategory {
  const baseName = fileName.split('/').pop() || fileName
  if (EXTENSIONLESS_TEXT.has(baseName)) return 'text'

  const dotIdx = baseName.lastIndexOf('.')
  if (dotIdx === -1) return 'binary'

  const ext = baseName.slice(dotIdx + 1).toLowerCase()
  if (ext === 'csv' || ext === 'tsv') return 'csv'
  if (ext === 'html' || ext === 'htm') return 'html'
  if (TEXT_EXTENSIONS.has(ext)) return 'text'
  if (IMAGE_EXTENSIONS.has(ext)) return 'image'
  return 'binary'
}

export const MAX_TEXT_PREVIEW_BYTES = 5 * 1024 * 1024   // 5MB
export const MAX_CSV_PREVIEW_BYTES = 2 * 1024 * 1024    // 2MB
export const MAX_CSV_PREVIEW_ROWS = 10000
export const MAX_IMAGE_PREVIEW_BYTES = 20 * 1024 * 1024 // 20MB

export function getFileIcon(name: string): string {
  const ext = name.split('.').pop()?.toLowerCase() || ''
  const iconMap: Record<string, string> = {
    js: 'code', ts: 'code', py: 'code', go: 'code', vue: 'code',
    jsx: 'code', tsx: 'code', rs: 'code', java: 'code', c: 'code',
    cpp: 'code', h: 'code', rb: 'code', php: 'code', lua: 'code',
    sh: 'code', bash: 'code',
    json: 'settings', yaml: 'settings', yml: 'settings', toml: 'settings',
    ini: 'settings', cfg: 'settings', conf: 'settings',
    md: 'document-text', txt: 'document-text', csv: 'document-text',
    log: 'document-text',
    png: 'image', jpg: 'image', jpeg: 'image', svg: 'image',
    gif: 'image', webp: 'image', bmp: 'image', ico: 'image',
  }
  return iconMap[ext] || 'document'
}
