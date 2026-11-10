<script setup lang="ts">
import { RotateCcw, Trash2 } from 'lucide-vue-next'
import ProviderIcon from './ProviderIcon.vue'
import { confirmTrash, mutationBusy, query, selected, selecting, shownTrash, sourceMap, toggleSelected, trashError } from '../store'
</script>

<template>
  <div v-if="!shownTrash.length && !trashError" class="empty-state">
    <div class="empty-icon"><Trash2 :size="22" /></div>
    <h2>{{ query ? '没有找到匹配的记录' : '回收站为空' }}</h2>
  </div>
  <div v-else class="trash-list">
    <article v-for="entry in shownTrash" :key="entry.key" class="trash-row" :class="{ checked: selected.has(entry.key) }">
      <input v-if="selecting" type="checkbox" :aria-label="`选择回收条目：${entry.title}`" :checked="selected.has(entry.key)" @change="toggleSelected([entry.key])" />
      <ProviderIcon :provider="entry.provider" />
      <div class="trash-words">
        <strong>{{ entry.title }}<span v-if="entry.sessions.length > 1" class="badge accent">共 {{ entry.sessions.length }} 个会话</span></strong>
        <p class="trash-path">{{ sourceMap.get(entry.source_id)?.name }}<span class="separator">·</span>{{ entry.sessions[0]?.cwd || '未关联项目' }}</p>
        <p>删除于 {{ new Date(entry.deleted_at).toLocaleString('zh-CN') }}<span v-if="entry.offline" class="offline-label"> · 环境已停止</span><span v-if="entry.status === 'pending'" class="danger-text"> · 操作中断，请恢复</span></p>
        <details v-if="entry.sessions.length > 1"><summary>查看会话明细</summary><ul><li v-for="session in entry.sessions" :key="session.key">{{ session.title }}</li></ul></details>
      </div>
      <div class="trash-actions">
        <button class="button small secondary" :disabled="entry.offline || mutationBusy" @click="confirmTrash('restore', [entry])"><RotateCcw :size="13" />恢复</button>
        <button class="icon-button small danger-text" :aria-label="`彻底删除：${entry.title}`" :title="'彻底删除'" :disabled="entry.offline || entry.status !== 'trashed' || mutationBusy" @click="confirmTrash('purge', [entry])"><Trash2 :size="14" /></button>
      </div>
    </article>
  </div>
</template>

<style>
.trash-list { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 420px), 1fr)); gap: var(--space-2); padding: 0 var(--space-3); }
.trash-row {
  display: flex; align-items: flex-start; gap: var(--space-3); padding: var(--space-3) var(--space-3) var(--space-3) var(--space-4);
  border-radius: var(--radius-lg); background: var(--color-panel); box-shadow: var(--shadow-sm); transition: box-shadow var(--duration) var(--ease);
}
.trash-row:hover { box-shadow: var(--shadow-md); }
.trash-row.checked { background: var(--color-hover); }
.trash-row > input { margin-top: 6px; }
.trash-row > .provider-icon { width: 28px; height: 28px; padding: 5px; border-radius: var(--radius-md); background: var(--color-inset); }
.trash-words { flex: 1; min-width: 0; }
.trash-words strong { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2); font-size: var(--text-sm); font-weight: var(--weight-medium); line-height: 20px; overflow-wrap: anywhere; }
.trash-words p { margin-top: 2px; color: var(--color-text-tertiary); font-size: var(--text-2xs); line-height: 16px; overflow-wrap: anywhere; }
.trash-path { font-family: var(--font-mono); }
.trash-words details { margin-top: var(--space-2); color: var(--color-text-secondary); font-size: var(--text-xs); }
.trash-words summary { cursor: pointer; }
.trash-words ul { margin: var(--space-1) 0 0; padding-left: var(--space-5); }
.trash-words li { margin-top: var(--space-1); }
.trash-actions { display: flex; align-items: center; gap: var(--space-1); }
@media (max-width: 520px) {
  .trash-row { flex-wrap: wrap; }
  .trash-actions { width: 100%; justify-content: flex-end; }
}
</style>
