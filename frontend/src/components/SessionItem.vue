<script setup lang="ts">
import { computed } from 'vue'
import { Copy, Download, GitBranch, Play, Trash2 } from '@lucide/vue'
import ProviderIcon from './ProviderIcon.vue'
import type { Session } from '../types'
import { activeKey, continueSession, copyCommand, exportSession, mutationBusy, pickSelected, prepareDelete, previewing, renderedKeys, searchHits, selected, selecting, snapshot, toggleSession } from '../store'
import { openContextMenu, type MenuItem } from '../contextMenu'
import { shortDate, size } from '../format'

const props = defineProps<{ session: Session }>()
const active = computed(() => activeKey.value === props.session.key)
const excerpt = computed(() => searchHits.value.get(props.session.key))

function menu(event: MouseEvent) {
  const session = props.session
  // In batch mode a selected row acts for the whole selection.
  if (selecting.value && selected.value.has(session.key) && selected.value.size > 1) {
    openContextMenu(event, '所选会话', [
      { label: '移入回收站', icon: Trash2, danger: true, disabled: previewing.value || mutationBusy.value, action: () => prepareDelete([...selected.value]) },
    ])
    return
  }
  const items: MenuItem[] = [
    { label: snapshot.value.can_resume ? '继续会话' : '复制命令', icon: snapshot.value.can_resume ? Play : Copy, action: () => continueSession(session) },
  ]
  if (snapshot.value.can_resume) items.push({ label: '复制命令', icon: Copy, action: () => copyCommand(session) })
  items.push(
    { label: '导出', icon: Download, action: () => exportSession(session) },
    { label: '移入回收站', icon: Trash2, danger: true, separated: true, disabled: previewing.value || mutationBusy.value, action: () => prepareDelete([session.key]) },
  )
  openContextMenu(event, `会话：${session.title}`, items)
}
</script>

<template>
  <article class="session-item" :class="{ active, checked: selected.has(session.key) }" :data-key="session.key" @contextmenu="menu">
    <input v-if="selecting" type="checkbox" :aria-label="`选择会话：${session.title}`" :checked="selected.has(session.key)" @click="pickSelected(session.key, $event.shiftKey, renderedKeys)" />
    <ProviderIcon :provider="session.provider" />
    <button class="session-title-button" :aria-label="`${active ? '收起' : '展开'}会话：${session.title}`" :aria-expanded="active" @click="toggleSession(session)">
      <span class="session-title-line"><span class="session-title">{{ session.title }}</span><span class="session-date">{{ shortDate(session.updated_at) }}</span></span>
      <span class="session-meta">
        <span v-if="session.model" class="model-name">{{ session.model }}</span>
        <span>{{ session.messages }} 条消息</span>
        <span class="session-size">{{ size(session.size) }}</span>
        <span v-if="session.branch" class="branch"><GitBranch :size="11" />{{ session.branch }}</span>
        <span v-if="session.parent_id" class="badge accent">分支会话</span>
        <span v-if="session.archived" class="badge">已归档</span>
        <span v-if="session.store === 'agy-summary'" class="badge">摘要记录</span>
      </span>
      <span v-if="excerpt" class="search-excerpt">…{{ excerpt }}…</span>
    </button>
  </article>
</template>

<style>
.session-item {
  position: relative; display: flex; align-items: flex-start; gap: var(--space-2-5);
  padding: var(--space-2-5) var(--space-2) var(--space-2-5) var(--space-2-5); border-radius: var(--radius-lg);
  transition: background-color var(--duration-fast) var(--ease);
}
.session-item:hover { background: var(--color-hover); }
.session-item.checked { background: var(--color-hover); }
.session-item.active { background: var(--color-active); }
.session-title-button:focus-visible { box-shadow: none; }
.session-item:has(.session-title-button:focus-visible):not(.active) { background: var(--color-hover); }
.session-item > input { position: relative; z-index: 1; margin-top: 7px; }
.session-item > .provider-icon { width: 28px; height: 28px; margin-top: 1px; padding: 5px; border-radius: var(--radius-md); background: var(--color-panel); box-shadow: var(--shadow-sm); }
.session-title-button { display: flex; flex-direction: column; gap: 3px; flex: 1; min-width: 0; text-align: left; border-radius: var(--radius-sm); }
.session-title-button::after { content: ''; position: absolute; inset: 0; border-radius: var(--radius-lg); }
.session-title-line { display: flex; align-items: baseline; gap: var(--space-2); width: 100%; }
.session-title { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: var(--text-sm); font-weight: var(--weight-medium); line-height: 20px; color: var(--color-text); }
.session-date { flex: none; color: var(--color-text-tertiary); font-size: var(--text-2xs); font-variant-numeric: tabular-nums; }
.session-meta { display: flex; align-items: center; gap: var(--space-1-5); max-width: 100%; overflow: hidden; white-space: nowrap; color: var(--color-text-tertiary); font-size: var(--text-2xs); line-height: 18px; }
.session-meta > span:not(.badge) + span:not(.badge)::before { content: '·'; margin-right: var(--space-1-5); color: var(--color-border-strong); }
.session-meta .model-name { font-family: var(--font-mono); color: var(--color-text-secondary); }
.session-meta .branch { display: inline-flex; align-items: center; gap: 3px; }
.search-excerpt {
  display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; margin-top: 2px;
  padding: 3px var(--space-2); border-radius: var(--radius-sm); background: var(--color-highlight);
  color: var(--color-text-secondary); font-size: var(--text-2xs); line-height: 16px; white-space: normal;
}
</style>
