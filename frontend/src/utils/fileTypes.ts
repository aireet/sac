const TEXT_EXTENSIONS = new Set([
  'md', 'txt', 'json', 'yaml', 'yml', 'xml', 'html', 'htm', 'css',
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

export type FileCategory = 'text' | 'image' | 'binary'

export function getFileCategory(fileName: string): FileCategory {
  const baseName = fileName.split('/').pop() || fileName
  if (EXTENSIONLESS_TEXT.has(baseName)) return 'text'

  const dotIdx = baseName.lastIndexOf('.')
  if (dotIdx === -1) return 'binary'

  const ext = baseName.slice(dotIdx + 1).toLowerCase()
  if (TEXT_EXTENSIONS.has(ext)) return 'text'
  if (IMAGE_EXTENSIONS.has(ext)) return 'image'
  return 'binary'
}

export const MAX_TEXT_PREVIEW_BYTES = 5 * 1024 * 1024   // 5MB
export const MAX_IMAGE_PREVIEW_BYTES = 20 * 1024 * 1024 // 20MB
