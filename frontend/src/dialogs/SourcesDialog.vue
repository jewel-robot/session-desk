<script setup lang="ts">
import { LoaderCircle, Monitor, Terminal, X } from '@lucide/vue'
import { agents } from '../agents'
import { dialogs, drafts, saveSources, saving, snapshot, sourceLabel, startSource, starting } from '../store'
import { useModal } from './useModal'

const { element, onClose } = useModal(() => dialogs.sources, value => { dialogs.sources = value })
function backdrop(event: MouseEvent) { if (event.target === element.value) dialogs.sources = false }
</script>

<template>
  <dialog ref="element" class="settings-dialog" @close="onClose" @click="backdrop">
    <form @submit.prevent="saveSources">
      <div class="dialog-header"><div><h2>数据源</h2></div><button type="button" class="icon-button" aria-label="关闭数据源" @click="dialogs.sources = false"><X :size="18" /></button></div>
      <div class="dialog-body">
        <section v-for="(source, index) in snapshot.sources" :key="source.id" class="source-card">
          <div class="source-card-heading">
            <span class="source-card-icon" :class="source.kind"><Monitor v-if="source.kind === 'windows'" :size="15" /><Terminal v-else :size="15" /></span>
            <h3>{{ sourceLabel(source) }}</h3>
            <span class="source-status"><span class="status-dot" :class="{ offline: !source.running }"></span>{{ source.running ? '运行中' : '已停止' }}</span>
            <button v-if="!source.running && snapshot.platform === 'windows'" class="text-button" type="button" :disabled="starting === source.id" @click="startSource(source)">{{ starting === source.id ? '正在启动' : '启动 WSL' }}</button>
          </div>
          <p v-if="source.error" class="field-error">{{ source.error }}</p>
          <div v-if="drafts[index]" class="source-fields">
            <label v-for="agent in agents" :key="agent.id"><span>{{ agent.name }}</span><input v-model="drafts[index]![agent.directory]" :disabled="snapshot.demo" spellcheck="false" :aria-label="`${source.name} ${agent.name} 数据目录`" /></label>
          </div>
        </section>
      </div>
      <div class="dialog-footer"><button class="button secondary" type="button" @click="dialogs.sources = false">取消</button><button class="button primary" type="submit" :disabled="saving || snapshot.demo"><LoaderCircle v-if="saving" :size="14" class="spin" />保存并刷新</button></div>
    </form>
  </dialog>
</template>

<style>
.settings-dialog { width: min(640px, calc(100vw - 32px)); }
.source-card { margin-top: var(--space-3); border-radius: var(--radius-lg); background: var(--color-panel); box-shadow: var(--shadow-sm); overflow: hidden; }
.source-card:first-child { margin-top: 0; }
.source-card-heading { display: flex; align-items: center; gap: var(--space-2); padding: var(--space-2-5) var(--space-3); background: var(--color-panel-subtle); border-bottom: 1px solid var(--color-border-subtle); }
.source-card-icon { display: inline-flex; align-items: center; justify-content: center; width: 26px; height: 26px; border-radius: var(--radius-sm); background: var(--color-inset); color: var(--color-text-secondary); }
.source-card-icon.wsl { background: var(--color-wsl-soft); color: var(--color-wsl); }
.source-card-icon.windows { background: var(--color-windows-soft); color: var(--color-windows); }
.source-card-heading h3 { font-size: var(--text-sm); font-weight: var(--weight-semibold); }
.source-status { display: inline-flex; align-items: center; gap: var(--space-1-5); margin-left: auto; color: var(--color-text-tertiary); font-size: var(--text-2xs); }
.source-status .status-dot { width: 6px; height: 6px; }
.source-fields { display: grid; gap: var(--space-2); padding: var(--space-3); }
.source-fields label { display: grid; grid-template-columns: 120px minmax(0, 1fr); align-items: center; gap: var(--space-3); color: var(--color-text-secondary); font-size: var(--text-xs); }
.source-fields input {
  width: 100%; min-width: 0; height: 30px; padding: 0 var(--space-2-5); border: 0; border-radius: var(--radius-sm);
  background: var(--color-inset); color: var(--color-text); font-family: var(--font-mono); font-size: var(--text-2xs);
  box-shadow: inset 0 0 0 1px var(--color-border-subtle); outline: none; transition: box-shadow var(--duration) var(--ease);
}
.source-fields input:focus-visible { box-shadow: inset 0 0 0 1px var(--color-border-strong); }
.source-fields input:disabled { color: var(--color-text-tertiary); cursor: not-allowed; }
.field-error { padding: var(--space-2) var(--space-3) 0; color: var(--color-danger); font-size: var(--text-xs); }
.settings-dialog .dialog-body { background: var(--color-panel-subtle); padding-top: var(--space-3); border-top: 1px solid var(--color-border-subtle); }
@media (max-width: 560px) {
  .source-fields label { grid-template-columns: 1fr; gap: var(--space-1); }
}
</style>
