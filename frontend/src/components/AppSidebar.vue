<script setup lang="ts">
import { Info, Layers3, Monitor, Moon, Settings2, Sparkles, Sun, Terminal, Trash2, Plus } from 'lucide-vue-next'
import ProviderIcon from './ProviderIcon.vue'
import { agents } from '../agents'
import { counts, dialogs, goSessions, goTrash, openSources, provider, snapshot, sourceCounts, sourceID, sourceLabel, theme, trash, view } from '../store'
import { update, updateAvailable, versionLabel } from '../app'

function pickProvider(id: string) { provider.value = id }
function pickSource(id: string) { sourceID.value = id }
</script>

<template>
  <aside class="sidebar" aria-label="导航">
    <nav class="sidebar-group">
      <button class="nav-item" :class="{ active: view === 'sessions' }" title="所有会话" @click="goSessions"><Layers3 :size="16" /><span>所有会话</span><em>{{ snapshot.sessions.length }}</em></button>
      <button class="nav-item" :class="{ active: view === 'trash' }" title="回收站" @click="goTrash"><Trash2 :size="16" /><span>回收站</span><em v-if="trash.length">{{ trash.length }}</em></button>
    </nav>

    <div class="sidebar-heading"><span>Agent</span></div>
    <div class="provider-tabs sidebar-group" role="group" aria-label="按工具筛选">
      <button class="filter-item" :class="{ active: provider === '' }" :aria-pressed="provider === ''" :title="`全部 · ${counts.all}`" @click="pickProvider('')"><span class="filter-icon"><Layers3 :size="14" /></span><span class="filter-label">全部</span><em>{{ counts.all }}</em></button>
      <button v-for="agent in agents" :key="agent.id" class="filter-item" :class="{ active: provider === agent.id }" :aria-pressed="provider === agent.id" :title="`${agent.name} · ${counts[agent.id]}`" @click="pickProvider(agent.id)"><ProviderIcon :provider="agent.id" /><span class="filter-label">{{ agent.name }}</span><em>{{ counts[agent.id] }}</em></button>
    </div>

    <div class="sidebar-heading sidebar-environments"><span>环境</span><button class="icon-button small" title="配置数据源" aria-label="配置数据源" @click="openSources"><Plus :size="14" /></button></div>
    <div class="sidebar-group sidebar-environments">
      <button class="source-item" :class="{ selected: !sourceID }" @click="pickSource('')"><span class="filter-icon"><Layers3 :size="14" /></span><span class="filter-label">全部环境</span><small>{{ snapshot.sessions.length }}</small></button>
      <button v-for="source in snapshot.sources" :key="source.id" class="source-item" :class="{ selected: sourceID === source.id }" @click="pickSource(source.id)">
        <span class="filter-icon" :class="source.kind"><Monitor v-if="source.kind === 'windows'" :size="14" /><Terminal v-else :size="14" /></span>
        <span class="filter-label">{{ sourceLabel(source) }}</span>
        <span class="status-dot" :class="{ offline: !source.running }" :title="source.running ? '运行中' : '已停止'"></span>
        <small>{{ sourceCounts.get(source.id) || 0 }}</small>
      </button>
    </div>

    <div class="sidebar-bottom">
      <button class="nav-item" title="数据源" @click="openSources"><Settings2 :size="16" /><span>数据源</span><span class="nav-trailing">{{ snapshot.sources.length }}</span></button>
      <button v-if="updateAvailable" class="update-pill" :title="`新版本 v${update?.latest} 可用`" @click="dialogs.about = true"><Sparkles :size="14" /><span>新版本 v{{ update?.latest }}</span></button>
      <div class="sidebar-footer"><button class="version-button" :title="`关于 Session Desk${versionLabel ? ' ' + versionLabel : ''}（F1）`" aria-label="关于 Session Desk" @click="dialogs.about = true"><Info :size="14" /><code>{{ versionLabel || '关于' }}</code><span v-if="updateAvailable" class="update-dot" aria-label="有可用更新"></span></button><button class="icon-button small" :aria-label="theme === 'dark' ? '切换浅色主题' : '切换深色主题'" :title="theme === 'dark' ? '浅色主题' : '深色主题'" @click="theme = theme === 'dark' ? 'light' : 'dark'"><Sun v-if="theme === 'dark'" :size="15" /><Moon v-else :size="15" /></button></div>
    </div>
  </aside>
</template>

<style>
.sidebar {
  grid-area: sidebar; display: flex; flex-direction: column; min-height: 0; overflow-y: auto;
  padding: var(--space-2) var(--space-2) var(--space-3); background: var(--color-canvas);
}
.sidebar-group { display: flex; flex-direction: column; gap: 1px; }
.sidebar-heading {
  display: flex; align-items: center; justify-content: space-between; height: 28px;
  margin-top: var(--space-4); padding: 0 var(--space-1) 0 var(--space-2-5);
  color: var(--color-text-tertiary); font-size: var(--text-2xs); font-weight: var(--weight-medium); text-transform: uppercase; letter-spacing: .4px;
}
.nav-item, .filter-item, .source-item {
  display: flex; align-items: center; gap: var(--space-2-5); width: 100%; height: 32px; padding: 0 var(--space-2-5);
  border-radius: var(--radius-md); text-align: left; color: var(--color-text-secondary); font-size: var(--text-sm);
  transition: background-color var(--duration-fast) var(--ease), color var(--duration-fast) var(--ease);
}
.nav-item:hover, .filter-item:hover, .source-item:hover { background: var(--color-hover); color: var(--color-text); }
.nav-item.active, .filter-item.active, .source-item.selected { background: var(--color-panel); color: var(--color-text); font-weight: var(--weight-medium); box-shadow: var(--shadow-sm); }
.nav-item > span, .filter-label { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.nav-item em, .filter-item em, .source-item small, .nav-trailing { flex: none; color: var(--color-text-tertiary); font-size: var(--text-2xs); font-variant-numeric: tabular-nums; font-weight: var(--weight-normal); }
.filter-icon { display: inline-flex; align-items: center; justify-content: center; width: 18px; height: 18px; color: var(--color-text-tertiary); }
.filter-icon.wsl { color: var(--color-wsl); }
.filter-item .provider-icon { width: 18px; height: 18px; border-radius: 4px; }
.source-item .status-dot { width: 6px; height: 6px; box-shadow: none; }
.sidebar-bottom { display: flex; flex-direction: column; gap: 1px; margin-top: auto; padding-top: var(--space-6); }
.local-note { display: flex; align-items: center; gap: var(--space-2); padding: var(--space-1) var(--space-2-5) var(--space-2); color: var(--color-text-tertiary); font-size: var(--text-xs); }
.sidebar-footer { display: flex; align-items: center; justify-content: space-between; padding: var(--space-2) var(--space-1) 0 var(--space-2-5); margin-top: var(--space-1); border-top: 1px solid var(--color-border-subtle); color: var(--color-text-tertiary); font-size: var(--text-2xs); }
.sidebar-footer code { font-size: var(--text-2xs); }
.version-button { position: relative; display: inline-flex; align-items: center; gap: var(--space-1-5); height: 26px; margin-left: calc(-1 * var(--space-1-5)); padding: 0 var(--space-1-5); border-radius: var(--radius-sm); color: var(--color-text-tertiary); transition: background-color var(--duration-fast) var(--ease), color var(--duration-fast) var(--ease); }
.version-button:hover { background: var(--color-hover); color: var(--color-text); }
.update-dot { width: 6px; height: 6px; border-radius: 50%; background: var(--color-accent); box-shadow: 0 0 0 2px var(--color-accent-soft); }
.update-pill { display: flex; align-items: center; gap: var(--space-2); height: 30px; margin: var(--space-1) 0; padding: 0 var(--space-2-5); border-radius: var(--radius-md); background: var(--color-accent-soft); color: var(--color-accent-text); font-size: var(--text-xs); font-weight: var(--weight-medium); text-align: left; }
.update-pill:hover { box-shadow: inset 0 0 0 1px var(--color-accent); }
.update-pill span { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

/* Icon rail between tablet and small desktop widths */
@media (min-width: 768px) and (max-width: 1279px) {
  .sidebar { padding: var(--space-2) var(--space-2) var(--space-3); align-items: stretch; }
  .sidebar .nav-item, .sidebar .filter-item { justify-content: center; padding: 0; height: 36px; }
  .sidebar .nav-item > span, .sidebar .nav-item em, .sidebar .filter-label, .sidebar .filter-item em, .sidebar .sidebar-environments, .sidebar .local-note, .version-button code, .update-pill span { display: none; }
  .sidebar .sidebar-heading { justify-content: center; padding: 0; height: 1px; margin: var(--space-3) var(--space-2); background: var(--color-border); font-size: 0; }
  .sidebar-footer { flex-direction: column; gap: var(--space-1); justify-content: center; padding: var(--space-2) 0 0; }
  .version-button { margin-left: 0; justify-content: center; width: 32px; padding: 0; }
  .update-dot { position: absolute; top: 4px; right: 5px; }
  .update-pill { justify-content: center; padding: 0; }
}
</style>
