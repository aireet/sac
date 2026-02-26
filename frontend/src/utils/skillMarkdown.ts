import yaml from 'js-yaml'
import type { SkillFrontmatter } from '../generated/sac/v1/skill'

export interface ParsedSkillMD {
  frontmatter: Partial<SkillFrontmatter>
  prompt: string
}

const FRONTMATTER_RE = /^---\n([\s\S]*?)\n---\n?([\s\S]*)$/

/**
 * Build SKILL.md content from frontmatter + prompt.
 * Mirrors backend sync.go:buildSkillMD — only includes non-empty fields.
 */
export function buildSkillMD(frontmatter: Partial<SkillFrontmatter>, prompt: string): string {
  const fm: Record<string, unknown> = {}

  if (frontmatter.allowed_tools?.length) fm.allowed_tools = frontmatter.allowed_tools
  if (frontmatter.model) fm.model = frontmatter.model
  if (frontmatter.context) fm.context = frontmatter.context
  if (frontmatter.agent) fm.agent = frontmatter.agent
  if (frontmatter.disable_model_invocation) fm.disable_model_invocation = true
  if (frontmatter.argument_hint) fm.argument_hint = frontmatter.argument_hint
  if (frontmatter.user_invocable !== undefined && frontmatter.user_invocable !== null) {
    fm.user_invocable = frontmatter.user_invocable
  }

  if (Object.keys(fm).length === 0) return prompt

  const yamlStr = yaml.dump(fm, { lineWidth: -1, noRefs: true }).trimEnd()
  return `---\n${yamlStr}\n---\n${prompt}`
}

/**
 * Parse SKILL.md content into frontmatter + prompt.
 */
export function parseSkillMD(content: string): ParsedSkillMD {
  const match = content.match(FRONTMATTER_RE)
  if (!match) {
    return { frontmatter: {}, prompt: content }
  }

  try {
    const raw = yaml.load(match[1]!) as Record<string, unknown> | null
    if (!raw || typeof raw !== 'object') {
      return { frontmatter: {}, prompt: content }
    }

    const fm: Partial<SkillFrontmatter> = {}
    if (Array.isArray(raw.allowed_tools)) fm.allowed_tools = raw.allowed_tools.map(String)
    if (typeof raw.model === 'string') fm.model = raw.model
    if (typeof raw.context === 'string') fm.context = raw.context
    if (typeof raw.agent === 'string') fm.agent = raw.agent
    if (raw.disable_model_invocation === true) fm.disable_model_invocation = true
    if (typeof raw.argument_hint === 'string') fm.argument_hint = raw.argument_hint
    if (typeof raw.user_invocable === 'boolean') fm.user_invocable = raw.user_invocable

    return { frontmatter: fm, prompt: match[2] ?? '' }
  } catch {
    return { frontmatter: {}, prompt: content }
  }
}

/**
 * Default SKILL.md template for new skills.
 */
export function defaultSkillTemplate(): string {
  return `# Task

Describe what this skill does here.

## Instructions

Write your prompt instructions in Markdown.
Use $ARGUMENTS for user-provided parameters.
`
}
