<script setup lang="ts">
import { ArrowUpRight, ChevronDown, ChevronRight, FolderOpen, Monitor, Search, Terminal } from '@lucide/vue'
import SessionItem from './SessionItem.vue'
import { groupOpen, groups, loading, openSources, pageSizes, query, selected, selecting, snapshot, sourceLabel, toggleGroup, toggleSelected } from '../store'
import { shortDate } from '../format'
</script>

<template>
  <div v-if="loading && !snapshot.sessions.length" class="loading-state" aria-busy="true" aria-label="正在读取会话">
    <div v-for="i in 7" :key="i" class="skeleton-row"><span class="skeleton-icon"></span><div><span class="skeleton-line"></span><span class="skeleton-line short"></span></div></div>
  </div>
  <div v-else-if="!groups.length" class="empty-state">
    <div class="empty-icon"><Search v-if="query" :size="22" /><FolderOpen v-else :size="22" /></div>
    <h2>{{ query ? '没有找到匹配的会话' : '还没有找到会话' }}</h2>
    <button v-if="query" class="button secondary" @click="query = ''">清除搜索</button>
    <button v-else class="button secondary" @click="openSources">配置数据源<ArrowUpRight :size="14" /></button>
  </div>
  <div v-else class="project-list">
    <section v-for="group in groups" :key="group.key" class="project-group">
      <div class="project-heading-row">
        <input v-if="selecting" type="checkbox" :aria-label="`选择项目：${group.project}`" :checked="group.sessions.every(session => selected.has(session.key))" :indeterminate="group.sessions.some(session => selected.has(session.key)) && !group.sessions.every(session => selected.has(session.key))" @change="toggleSelected(group.sessions.map(session => session.key))" />
        <button class="project-heading" :aria-expanded="groupOpen(group.key)" :title="group.cwd" @click="toggleGroup(group.key)">
          <ChevronRight :size="13" class="project-chevron" :class="{ rotated: groupOpen(group.key) }" />
          <span class="project-name">{{ group.project }}</span>
          <span class="environment-badge" :class="group.source?.kind"><Monitor v-if="group.source?.kind === 'windows'" :size="10" /><Terminal v-else :size="10" />{{ sourceLabel(group.source) }}</span>
          <span v-if="!group.source?.running" class="offline-label">已停止 · 缓存</span>
          <span class="project-meta"><span>{{ group.sessions.length }}</span><span class="project-updated">{{ shortDate(group.sessions[0]!.updated_at) }}</span></span>
        </button>
      </div>
      <div v-if="groupOpen(group.key)" class="project-sessions">
        <SessionItem v-for="session in group.sessions.slice(0, pageSizes.get(group.key) || 30)" :key="session.key" :session="session" />
        <button v-if="group.sessions.length > (pageSizes.get(group.key) || 30)" class="text-button show-more" @click="pageSizes.set(group.key, (pageSizes.get(group.key) || 30) + 30)">再显示 {{ Math.min(30, group.sessions.length - (pageSizes.get(group.key) || 30)) }} 个会话<ChevronDown :size="13" /></button>
      </div>
    </section>
  </div>
</template>

<style>
.project-list { padding: 0 var(--space-2); }
.project-group + .project-group { margin-top: var(--space-1); }
.project-heading-row {
  position: sticky; top: calc(-1 * var(--space-1)); z-index: 3; display: flex; align-items: center; gap: var(--space-2);
  margin: 0 calc(-1 * var(--space-2)); padding: var(--space-1) var(--space-2); background: var(--color-panel);
}
.project-heading-row > input { margin-left: var(--space-2); }
.project-heading {
  display: flex; align-items: center; gap: var(--space-2); flex: 1; min-width: 0; height: 30px; padding: 0 var(--space-2);
  border-radius: var(--radius-sm); text-align: left; transition: background-color var(--duration-fast) var(--ease);
}
.project-heading:hover { background: var(--color-hover); }
.project-chevron { color: var(--color-text-tertiary); transition: transform var(--duration) var(--ease); }
.project-chevron.rotated { transform: rotate(90deg); }
.project-name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: var(--text-xs); font-weight: var(--weight-semibold); color: var(--color-text); }
.project-meta { display: flex; align-items: center; gap: var(--space-2); margin-left: auto; color: var(--color-text-tertiary); font-size: var(--text-2xs); font-variant-numeric: tabular-nums; white-space: nowrap; }
.project-meta > span:first-child { min-width: 18px; height: 16px; padding: 0 5px; border-radius: var(--radius-full); background: var(--color-inset); text-align: center; line-height: 16px; color: var(--color-text-secondary); }
.project-sessions { display: flex; flex-direction: column; gap: 1px; padding-bottom: var(--space-2); }
.show-more { align-self: flex-start; margin: var(--space-1) 0 0 26px; }

.loading-state { padding: var(--space-2); }
.skeleton-row { display: flex; gap: var(--space-3); align-items: center; padding: var(--space-3); }
.skeleton-row > div { flex: 1; }
.skeleton-icon, .skeleton-line { display: block; background: var(--color-inset); border-radius: var(--radius-xs); animation: pulse 1.6s ease-in-out infinite; }
.skeleton-icon { width: 28px; height: 28px; border-radius: var(--radius-md); }
.skeleton-line { height: 10px; width: 70%; }
.skeleton-line.short { width: 40%; margin-top: var(--space-2); }
</style>
