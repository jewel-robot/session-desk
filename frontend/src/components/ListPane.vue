<script setup lang="ts">
import { AlertCircle, ArrowDownWideNarrow, CircleHelp, Layers3, ListChecks, LoaderCircle, Menu, Monitor, RefreshCw, RotateCcw, Search, Settings2, Trash2, X } from '@lucide/vue'
import DropdownSelect from './DropdownSelect.vue'
import ProviderIcon from './ProviderIcon.vue'
import SessionList from './SessionList.vue'
import TrashList from './TrashList.vue'
import {
  allSelected, confirmTrash, environmentOptions, error, found, load, loadTrash, loading, mutationBusy, mutationErrors, openSources, prepareDelete,
  previewing, provider, providerOptions, query, searchInput, searching, selected, selecting, shownTrash, sidebarOpen, snapshot, sort, sortOptions,
  sourceID, toggleSelected, toggleSelection, trash, trashError, view, visibleKeys,
} from '../store'
</script>

<template>
  <main id="session-content" class="list-pane" tabindex="-1">
    <header class="list-header">
      <div class="page-header">
        <button class="icon-button menu-button" aria-label="打开侧边栏" @click.stop="sidebarOpen = true"><Menu :size="18" /></button>
        <h1>{{ view === 'trash' ? '回收站' : '会话' }}<span v-if="snapshot.demo" class="demo-badge">演示</span></h1>
        <div class="header-actions">
          <button class="icon-button" aria-label="数据源" title="数据源" @click="openSources"><Settings2 :size="16" /></button>
          <button class="icon-button" :aria-label="loading ? '正在扫描' : view === 'trash' ? '刷新回收站' : '刷新会话'" :title="view === 'trash' ? '刷新回收站' : '刷新会话'" :disabled="loading" @click="load(true)"><RefreshCw :size="15" :class="{ spin: loading }" /></button>
        </div>
      </div>
      <label class="search-field"><Search :size="15" /><input ref="searchInput" v-model="query" type="search" maxlength="300" :placeholder="view === 'trash' ? '搜索已删除的会话…' : '搜索标题或对话内容…'" aria-label="搜索会话" @keydown.esc="query = ''" /><LoaderCircle v-if="searching" :size="14" class="spin" /><kbd v-else>Ctrl K</kbd></label>
      <div class="filter-row">
        <DropdownSelect v-model="provider" class="provider-select" :options="providerOptions" aria-label="筛选 Agent"><template #prefix><ProviderIcon v-if="provider" :provider="provider" /><Layers3 v-else :size="13" /></template></DropdownSelect>
        <DropdownSelect v-model="sourceID" class="environment-select" :options="environmentOptions" aria-label="筛选环境"><template #prefix><Monitor :size="13" /></template></DropdownSelect>
        <DropdownSelect v-if="view === 'sessions'" v-model="sort" :options="sortOptions" aria-label="排序方式"><template #prefix><ArrowDownWideNarrow :size="13" /></template></DropdownSelect>
        <span class="filter-spacer"></span>
        <button v-if="view === 'trash'" class="text-button danger-text" :disabled="!trash.length || mutationBusy" @click="confirmTrash('purge', trash)"><Trash2 :size="13" />清空回收站</button>
        <button class="text-button" :aria-pressed="selecting" @click="toggleSelection"><ListChecks :size="14" />{{ selecting ? '取消选择' : '批量选择' }}</button>
      </div>
    </header>

    <div v-if="selecting" class="selection-bar">
      <label class="select-all"><input type="checkbox" :checked="allSelected" :indeterminate="selected.size > 0 && !allSelected" :disabled="!visibleKeys.length" @change="toggleSelected(visibleKeys)" />选择当前结果</label>
      <span class="selection-count">已选择 {{ selected.size }} 项</span>
      <div class="selection-actions">
        <button v-if="view === 'sessions'" class="button small danger-outline" :disabled="!selected.size || previewing || mutationBusy" @click="prepareDelete([...selected])"><LoaderCircle v-if="previewing" :size="13" class="spin" /><Trash2 v-else :size="13" />移入回收站</button>
        <template v-else>
          <button class="button small secondary" :disabled="!selected.size || mutationBusy" @click="confirmTrash('restore', shownTrash.filter(entry => selected.has(entry.key)))"><RotateCcw :size="13" />恢复所选</button>
          <button class="button small danger-outline" :disabled="!selected.size || mutationBusy" @click="confirmTrash('purge', shownTrash.filter(entry => selected.has(entry.key)))"><Trash2 :size="13" />彻底删除所选</button>
        </template>
      </div>
    </div>

    <div class="page-content">
      <div v-if="mutationErrors.length" class="notice error-notice" role="alert"><AlertCircle :size="15" /><div><strong>部分操作未完成</strong><p v-for="message in mutationErrors" :key="message">{{ message }}</p></div><button class="icon-button small" aria-label="关闭操作提示" @click="mutationErrors = []"><X :size="14" /></button></div>
      <div v-if="view === 'trash' && trashError" class="notice error-notice" role="alert"><AlertCircle :size="15" /><span>{{ trashError }}</span><button class="text-button" @click="loadTrash">重试</button></div>
      <div v-if="error" class="notice error-notice" role="alert"><AlertCircle :size="15" /><span>{{ error }}</span><button class="text-button" @click="load(true)">重试</button></div>
      <div v-if="snapshot.warnings.length" class="notice warning-notice"><AlertCircle :size="15" /><details><summary>{{ snapshot.warnings.length }} 项读取提示</summary><p v-for="warning in snapshot.warnings" :key="warning">{{ warning }}</p></details></div>
      <div v-if="found?.limited" class="notice"><CircleHelp :size="15" />仅显示部分结果</div>
      <div v-if="found?.errors.length" class="notice"><AlertCircle :size="15" /><details><summary>{{ found.errors.length }} 项搜索读取提示</summary><p v-for="message in found.errors.slice(0, 5)" :key="message">{{ message }}</p></details></div>

      <SessionList v-if="view === 'sessions'" />
      <TrashList v-else />
    </div>
  </main>
</template>

<style>
.list-pane {
  grid-area: list; display: flex; flex-direction: column; min-width: 0; min-height: 0; outline: none;
  background: var(--color-panel); border-top-left-radius: var(--radius-xl);
  box-shadow: -1px 0 0 var(--color-border), 0 -1px 0 var(--color-border);
}
.list-header { flex-shrink: 0; padding: var(--space-3) var(--space-3) var(--space-2); }
.page-header { display: flex; align-items: center; gap: var(--space-2); height: 36px; padding-left: var(--space-1); }
.page-header h1 { display: flex; align-items: center; gap: var(--space-2); flex: 1; min-width: 0; font-size: var(--text-xl); font-weight: var(--weight-semibold); letter-spacing: -.3px; line-height: 1; }
.header-actions { display: flex; align-items: center; gap: var(--space-0-5); }
.menu-button { display: none; }
.search-field {
  display: flex; align-items: center; gap: var(--space-2); height: 34px; margin-top: var(--space-2); padding: 0 var(--space-1-5) 0 var(--space-2-5);
  border-radius: var(--radius-md); color: var(--color-text-tertiary); background: var(--color-inset);
  box-shadow: inset 0 0 0 1px transparent; transition: box-shadow var(--duration) var(--ease), background-color var(--duration) var(--ease);
}
.search-field:hover { box-shadow: inset 0 0 0 1px var(--color-border); }
.search-field:focus-within { box-shadow: inset 0 0 0 1px var(--color-border); color: var(--color-text-secondary); }
.search-field input { flex: 1; min-width: 0; height: 100%; border: 0; padding: 0; background: none; color: var(--color-text); font-size: var(--text-sm); }
.search-field input::placeholder { color: var(--color-text-tertiary); }
.filter-row { display: flex; align-items: center; flex-wrap: wrap; gap: var(--space-1); margin-top: var(--space-2); }
.filter-spacer { flex: 1; }
.filter-row .provider-select { display: none; }
.environment-select .select-value { max-width: 140px; overflow: hidden; text-overflow: ellipsis; }

.selection-bar { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2) var(--space-3); margin: 0 var(--space-3) var(--space-2); padding: var(--space-2) var(--space-3); border-radius: var(--radius-md); background: var(--color-accent-soft); font-size: var(--text-xs); flex-shrink: 0; }
.select-all { display: flex; align-items: center; gap: var(--space-2); cursor: pointer; font-weight: var(--weight-medium); }
.selection-count { color: var(--color-accent-text); font-variant-numeric: tabular-nums; }
.selection-actions { display: flex; gap: var(--space-1-5); margin-left: auto; }

.page-content { flex: 1; min-height: 0; overflow-y: auto; overscroll-behavior: contain; padding: var(--space-1) 0 var(--space-3); border-top: 1px solid var(--color-border-subtle); }

@media (max-width: 1023px) {
  .list-pane { border-top-left-radius: var(--radius-xl); }
}
@media (max-width: 767px) {
  .list-pane { border-radius: 0; box-shadow: 0 -1px 0 var(--color-border); }
  .menu-button { display: inline-flex; }
  .filter-row .provider-select { display: inline-flex; }
}
</style>
