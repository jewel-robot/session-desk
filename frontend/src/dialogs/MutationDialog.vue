<script setup lang="ts">
import { GitBranch, LoaderCircle, RotateCcw, Trash2, X } from '@lucide/vue'
import ProviderIcon from '../components/ProviderIcon.vue'
import { deletePlan, dialogs, mutation, mutationBusy, mutationTitle, runMutation, sourceMap, trashTargets } from '../store'
import { useModal } from './useModal'

const { element, onClose } = useModal(() => dialogs.mutation, value => { dialogs.mutation = value })
function backdrop(event: MouseEvent) { if (event.target === element.value && !mutationBusy.value) dialogs.mutation = false }
</script>

<template>
  <dialog ref="element" class="mutation-dialog" :class="mutation" aria-labelledby="mutation-title" :aria-describedby="mutation === 'purge' ? 'mutation-description' : undefined" @cancel="mutationBusy && $event.preventDefault()" @close="onClose" @click="backdrop">
    <div class="dialog-header">
      <div class="mutation-heading">
        <span class="mutation-icon"><RotateCcw v-if="mutation === 'restore'" :size="17" /><Trash2 v-else :size="17" /></span>
        <div>
          <h2 id="mutation-title">{{ mutationTitle }}</h2>
          <p v-if="mutation === 'purge'" id="mutation-description">此操作无法撤销</p>
        </div>
      </div>
      <button class="icon-button" aria-label="关闭操作确认" :disabled="mutationBusy" @click="dialogs.mutation = false"><X :size="18" /></button>
    </div>
    <div class="dialog-body">
      <p v-if="mutation === 'delete' && deletePlan?.related" class="related-note"><GitBranch :size="14" />包含 {{ deletePlan.related }} 个关联子会话</p>
      <ul class="confirmation-list">
        <template v-if="mutation === 'delete'"><li v-for="session in deletePlan?.targets" :key="session.key"><ProviderIcon :provider="session.provider" /><span>{{ session.title }}<small>{{ sourceMap.get(session.source_id)?.name }} · {{ session.cwd || session.id }}</small></span><span v-if="session.parent_id" class="badge accent">关联会话</span></li></template>
        <template v-else><li v-for="entry in trashTargets" :key="entry.key"><ProviderIcon :provider="entry.provider" /><span>{{ entry.title }}<small>{{ sourceMap.get(entry.source_id)?.name }} · {{ entry.sessions.length }} 个会话</small></span></li></template>
      </ul>
    </div>
    <div class="dialog-footer"><button class="button secondary" :disabled="mutationBusy" autofocus @click="dialogs.mutation = false">取消</button><button class="button" :class="mutation === 'restore' ? 'primary' : 'danger'" :disabled="mutationBusy" @click="runMutation"><LoaderCircle v-if="mutationBusy" :size="14" class="spin" /><RotateCcw v-else-if="mutation === 'restore'" :size="14" /><Trash2 v-else :size="14" />{{ mutationBusy ? '正在处理' : mutationTitle }}</button></div>
  </dialog>
</template>

<style>
.mutation-heading { display: flex; gap: var(--space-3); }
.mutation-icon { display: inline-flex; align-items: center; justify-content: center; flex: none; width: 36px; height: 36px; border-radius: var(--radius-lg); background: var(--color-danger-soft); color: var(--color-danger); }
.mutation-dialog.restore .mutation-icon { background: var(--color-accent-soft); color: var(--color-accent-text); }
.confirmation-list { margin: 0; padding: 0; list-style: none; border-radius: var(--radius-lg); box-shadow: var(--shadow-sm); overflow: hidden; }
.confirmation-list li { display: flex; align-items: center; gap: var(--space-3); padding: var(--space-2-5) var(--space-3); }
.confirmation-list li + li { border-top: 1px solid var(--color-border-subtle); }
.confirmation-list .provider-icon { width: 20px; height: 20px; }
.confirmation-list li > span:not(.provider-icon):not(.badge) { flex: 1; min-width: 0; overflow-wrap: anywhere; font-size: var(--text-sm); }
.confirmation-list small { display: block; margin-top: 2px; color: var(--color-text-tertiary); font-size: var(--text-2xs); font-family: var(--font-mono); }
.related-note { display: flex; align-items: center; gap: var(--space-2); margin-bottom: var(--space-3); padding: var(--space-2) var(--space-3); border-radius: var(--radius-md); background: var(--color-accent-soft); color: var(--color-accent-text); font-size: var(--text-xs); }
</style>
