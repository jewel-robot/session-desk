<script setup lang="ts">
import { ref } from 'vue'
import { Brain, ChevronRight, Code2, Terminal, UserRound, SlidersHorizontal } from '@lucide/vue'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import type { Part } from '../types'
import { agents } from '../agents'
import ProviderIcon from './ProviderIcon.vue'

defineProps<{ parts: Part[]; provider: string }>()
const opened = ref(new Set<number>())

function markdown(text: string): string {
  return DOMPurify.sanitize(marked.parse(text, { async: false, gfm: true }), {
    USE_PROFILES: { html: true },
    FORBID_TAGS: ['style', 'form', 'input', 'button', 'img'],
    FORBID_ATTR: ['style'],
  })
}

function linkClick(event: MouseEvent) {
  const anchor = (event.target as HTMLElement).closest('a')
  if (!anchor) return
  event.preventDefault()
  if (/^https?:\/\//i.test(anchor.href)) window.open(anchor.href, '_blank', 'noopener,noreferrer')
}
</script>

<template>
  <div class="transcript" @click="linkClick">
    <template v-for="(part, index) in parts" :key="index">
      <details v-if="part.kind !== 'text'" class="content-fold" :class="part.kind"
        @toggle="($event.target as HTMLDetailsElement).open ? opened.add(index) : opened.delete(index)">
        <summary>
          <ChevronRight :size="13" class="fold-arrow" />
          <Brain v-if="part.kind === 'thinking'" :size="14" />
          <Terminal v-else-if="part.kind === 'tool_result'" :size="14" />
          <SlidersHorizontal v-else-if="part.kind === 'context'" :size="14" />
          <Code2 v-else :size="14" />
          <span>{{ part.kind === 'thinking' ? '思考过程' : part.kind === 'tool_use' ? '工具调用' : part.kind === 'tool_result' ? '工具结果' : part.kind === 'context' ? '会话环境' : '附件' }}</span>
          <code v-if="part.name">{{ part.name }}</code>
          <span class="fold-preview">{{ part.text.slice(0, 90).replace(/\s+/g, ' ') }}</span>
        </summary>
        <pre v-if="opened.has(index)">{{ part.text }}</pre>
      </details>
      <article v-else class="message" :class="part.role">
        <div class="message-head">
          <span v-if="part.role === 'user'" class="user-avatar"><UserRound :size="13" /></span>
          <ProviderIcon v-else :provider="provider" />
          <span>{{ part.role === 'user' ? '你' : agents.find(agent => agent.id === provider)?.name }}</span>
          <span v-if="part.model" class="message-model">{{ part.model }}</span>
        </div>
        <div v-if="part.role === 'user'" class="user-text">{{ part.text }}</div>
        <div v-else class="markdown" v-html="markdown(part.text)"></div>
      </article>
    </template>
  </div>
</template>

<style>
.transcript { display: flex; flex-direction: column; gap: var(--space-1); max-width: 880px; margin: 0 auto; }
.message { padding: var(--space-3) 0; }
.message-head { display: flex; align-items: center; gap: var(--space-2); margin-bottom: var(--space-2); font-size: var(--text-xs); font-weight: var(--weight-semibold); color: var(--color-text); }
.message-head .provider-icon, .user-avatar { display: inline-flex; align-items: center; justify-content: center; width: 22px; height: 22px; padding: 3px; border-radius: var(--radius-sm); background: var(--color-panel); box-shadow: var(--shadow-sm); }
.user-avatar { color: var(--color-accent-text); background: var(--color-accent-soft); box-shadow: none; }
.message-model { font-family: var(--font-mono); font-size: var(--text-2xs); font-weight: var(--weight-normal); color: var(--color-text-tertiary); }
.message.user { display: flex; flex-direction: column; align-items: flex-end; }
.message.user .message-head { flex-direction: row-reverse; }
.user-text {
  max-width: min(640px, 88%); padding: var(--space-2-5) var(--space-3-5, 14px); border-radius: var(--radius-xl) var(--radius-xs) var(--radius-xl) var(--radius-xl);
  background: var(--color-accent); color: var(--color-accent-fg); font-size: var(--text-md); line-height: var(--leading-relaxed);
  white-space: pre-wrap; overflow-wrap: anywhere; box-shadow: 0 1px 2px rgba(0, 0, 0, .08);
}
.markdown { padding-left: 30px; font-size: var(--text-md); line-height: var(--leading-relaxed); color: var(--color-text); overflow-wrap: anywhere; text-wrap: pretty; }
.markdown p { margin: 0 0 var(--space-3); }
.markdown > :last-child { margin-bottom: 0; }
.markdown h1, .markdown h2, .markdown h3, .markdown h4 { margin: var(--space-5) 0 var(--space-2); font-size: var(--text-md); line-height: 22px; font-weight: var(--weight-semibold); text-wrap: balance; }
.markdown h1 { font-size: var(--text-lg); }
.markdown ul, .markdown ol { margin: var(--space-2) 0 var(--space-3); padding-left: var(--space-6); }
.markdown li + li { margin-top: var(--space-1); }
.markdown li::marker { color: var(--color-text-tertiary); }
.markdown blockquote { margin: var(--space-3) 0; padding-left: var(--space-4); border-left: 3px solid var(--color-border-strong); color: var(--color-text-secondary); }
.markdown a { color: var(--color-accent-text); text-underline-offset: 3px; }
.markdown code { padding: 1px 5px; border-radius: var(--radius-xs); background: var(--color-inset); box-shadow: inset 0 0 0 1px var(--color-border-subtle); font-size: .88em; }
.markdown pre { margin: var(--space-3) 0; padding: var(--space-3) var(--space-4); overflow-x: auto; border-radius: var(--radius-lg); background: var(--color-panel); box-shadow: var(--shadow-sm); font-size: var(--text-xs); line-height: 20px; }
.markdown pre code { padding: 0; background: none; box-shadow: none; font-size: inherit; }
.markdown table { display: block; overflow-x: auto; margin: var(--space-3) 0; border-collapse: collapse; font-size: var(--text-xs); }
.markdown td, .markdown th { padding: var(--space-1-5) var(--space-3); border: 1px solid var(--color-border); text-align: left; }
.markdown th { background: var(--color-inset); font-weight: var(--weight-semibold); }
.markdown hr { border: 0; height: 1px; background: var(--color-border); margin: var(--space-4) 0; }

.content-fold { margin: 0 0 0 30px; border-radius: var(--radius-md); background: var(--color-panel); box-shadow: var(--shadow-sm); transition: box-shadow var(--duration) var(--ease); }
.content-fold + .content-fold { margin-top: var(--space-1); }
.content-fold summary { display: flex; align-items: center; gap: var(--space-2); height: 32px; padding: 0 var(--space-3); border-radius: var(--radius-md); color: var(--color-text-secondary); font-size: var(--text-xs); cursor: pointer; list-style: none; }
.content-fold summary:hover { color: var(--color-text); }
.content-fold summary::-webkit-details-marker { display: none; }
.content-fold summary > span:not(.fold-preview) { font-weight: var(--weight-medium); white-space: nowrap; }
.content-fold summary code { padding: 0 var(--space-1-5); border-radius: var(--radius-xs); background: var(--color-inset); font-size: var(--text-2xs); color: var(--color-text-secondary); }
.content-fold.thinking summary > svg:not(.fold-arrow) { color: #a855f7; }
.content-fold.tool_use summary > svg:not(.fold-arrow) { color: var(--color-accent); }
.content-fold.tool_result summary > svg:not(.fold-arrow) { color: var(--color-success); }
.fold-arrow { color: var(--color-text-tertiary); transition: transform var(--duration) var(--ease); }
.content-fold[open] .fold-arrow { transform: rotate(90deg); }
.fold-preview { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--color-text-tertiary); font-family: var(--font-mono); font-size: var(--text-2xs); }
.content-fold[open] .fold-preview { display: none; }
.content-fold pre { margin: 0; padding: var(--space-3) var(--space-4); max-height: 380px; overflow: auto; border-top: 1px solid var(--color-border-subtle); font-size: var(--text-xs); line-height: 20px; white-space: pre-wrap; overflow-wrap: anywhere; color: var(--color-text-secondary); }
.content-fold.thinking pre { font-family: var(--font-sans); font-style: italic; }

@media (max-width: 560px) {
  .markdown, .content-fold { padding-left: 0; margin-left: 0; }
  .user-text { max-width: 100%; }
}
</style>
