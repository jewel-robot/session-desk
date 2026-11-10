<script setup lang="ts">
import { computed, ref, watch, nextTick } from 'vue'
import { AlertCircle, ArrowLeft, ChevronDown, Clock3, Copy, Download, FileText, FolderOpen, GitBranch, LoaderCircle, MessagesSquare, Play, Terminal, Trash2, X } from 'lucide-vue-next'
import ProviderIcon from './ProviderIcon.vue'
import TranscriptView from './TranscriptView.vue'
import { agents } from '../agents'
import { activeSession, closeSession, continueSession, copyCommand, exportSession, launching, mutationBusy, prepareDelete, previewing, readTalk, showCommand, snapshot, sourceMap, sourceLabel, talks } from '../store'
import { compact, formatDate, size } from '../format'

const session = activeSession
const talk = computed(() => session.value ? talks.get(session.value.key) : undefined)
const source = computed(() => session.value ? sourceMap.value.get(session.value.source_id) : undefined)
const agentName = computed(() => agents.find(agent => agent.id === session.value?.provider)?.name || '')
const scroller = ref<HTMLElement>()

watch(() => session.value?.key, () => { void nextTick(() => { if (scroller.value) scroller.value.scrollTop = 0 }) })
</script>

<template>
  <section class="detail-pane" aria-label="会话详情">
    <div v-if="!session" class="detail-placeholder">
      <MessagesSquare :size="28" :stroke-width="1.5" aria-hidden="true" />
      <span class="sr-only">未选择会话</span>
    </div>
    <div v-else class="session-detail">
      <header class="detail-header">
        <div class="detail-title-row">
          <button class="icon-button detail-back" aria-label="返回列表" @click="closeSession"><ArrowLeft :size="16" /></button>
          <ProviderIcon :provider="session.provider" />
          <div class="detail-title">
            <h2 :title="session.title">{{ session.title }}</h2>
            <div class="detail-subtitle">
              <span>{{ agentName }}</span>
              <span v-if="session.model" class="detail-model">{{ session.model }}</span>
              <span class="environment-badge" :class="source?.kind">{{ sourceLabel(source) }}</span>
              <span v-if="session.parent_id" class="badge accent">分支会话</span>
              <span v-if="session.archived" class="badge">已归档</span>
              <span v-if="session.store === 'agy-summary'" class="badge">摘要记录</span>
            </div>
          </div>
          <button class="icon-button" aria-label="收起详情" title="收起" @click="closeSession"><X :size="16" /></button>
        </div>
        <div class="detail-actions">
          <button class="button primary small" :disabled="launching === session.key" @click="continueSession(session)"><LoaderCircle v-if="launching === session.key" :size="13" class="spin" /><Play v-else :size="13" />{{ snapshot.can_resume ? '继续会话' : '复制命令' }}</button>
          <button v-if="snapshot.can_resume" class="button small secondary" @click="copyCommand(session)"><Copy :size="13" />复制命令</button>
          <button class="button small secondary" @click="exportSession(session)"><Download :size="13" />导出</button>
          <span class="filter-spacer"></span>
          <button class="icon-button small" aria-label="查看命令" title="查看续接命令" @click="showCommand(session)"><Terminal :size="14" /></button>
          <button class="icon-button small danger-text" aria-label="删除此会话" title="移入回收站" :disabled="previewing || mutationBusy" @click="prepareDelete([session.key])"><Trash2 :size="14" /></button>
        </div>
      </header>
      <div ref="scroller" class="talk-scroll">
        <dl class="detail-facts">
          <div><dt><FolderOpen :size="12" />目录</dt><dd class="mono" :title="session.cwd">{{ session.cwd || '未记录工作目录' }}</dd></div>
          <div v-if="session.branch"><dt><GitBranch :size="12" />分支</dt><dd class="mono">{{ session.branch }}</dd></div>
          <div><dt><Clock3 :size="12" />开始</dt><dd>{{ new Date(session.started_at).toLocaleString('zh-CN') }}</dd></div>
          <div><dt><Clock3 :size="12" />更新</dt><dd>{{ formatDate(session.updated_at) }}</dd></div>
          <div><dt><MessagesSquare :size="12" />消息</dt><dd>{{ session.messages }} 条 · {{ size(session.size) }}</dd></div>
          <div v-if="session.tokens.input + session.tokens.output"><dt><FileText :size="12" />Tokens</dt><dd>{{ compact(session.tokens.input + session.tokens.output) }}<span v-if="session.tokens.cached"> · {{ compact(session.tokens.cached) }} 已缓存</span></dd></div>
          <div><dt>ID</dt><dd class="mono">{{ session.id }}</dd></div>
        </dl>
        <p v-if="session.warning" class="detail-warning"><AlertCircle :size="14" />{{ session.warning }}</p>
        <div v-if="talk?.error" class="talk-error" role="alert"><AlertCircle :size="15" />{{ talk.error }}<button class="text-button" @click="readTalk(session.key)">重试</button></div>
        <template v-else>
          <TranscriptView :parts="talk?.parts || []" :provider="session.provider" />
          <div v-if="talk?.busy" class="talk-loading"><LoaderCircle :size="16" class="spin" />正在读取对话</div>
          <button v-else-if="talk?.has_more" class="button secondary load-more" @click="readTalk(session.key, true)">继续加载对话<ChevronDown :size="13" /></button>
          <p v-else-if="!talk?.parts.length" class="talk-empty">{{ session.store === 'agy-summary' ? '仅保留会话摘要' : '暂无正文' }}</p>
        </template>
      </div>
    </div>
  </section>
</template>


<style>
.detail-pane {
  grid-area: detail; display: flex; flex-direction: column; min-width: 0; min-height: 0;
  background: var(--color-panel-subtle); box-shadow: -1px 0 0 var(--color-border), 0 -1px 0 var(--color-border);
}
.detail-placeholder { display: flex; flex: 1; flex-direction: column; align-items: center; justify-content: center; gap: var(--space-2); padding: var(--space-8); text-align: center; }
.detail-placeholder { color: var(--color-border-strong); }
.sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; border: 0; }
.session-detail { display: flex; flex-direction: column; flex: 1; min-height: 0; }
.detail-header { flex-shrink: 0; padding: var(--space-3) var(--space-4) 0; background: var(--color-panel); border-bottom: 1px solid var(--color-border-subtle); }
.detail-title-row { display: flex; align-items: flex-start; gap: var(--space-3); }
.detail-title-row > .provider-icon { width: 36px; height: 36px; padding: 7px; border-radius: var(--radius-lg); background: var(--color-panel); box-shadow: var(--shadow-sm); }
.detail-back { display: none; }
.detail-title { flex: 1; min-width: 0; }
.detail-title h2 { display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; font-size: var(--text-lg); line-height: 22px; font-weight: var(--weight-semibold); letter-spacing: -.2px; overflow-wrap: anywhere; }
.detail-subtitle { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-1-5); margin-top: var(--space-1); color: var(--color-text-secondary); font-size: var(--text-xs); }
.detail-model { font-family: var(--font-mono); font-size: var(--text-2xs); color: var(--color-text-tertiary); }
.detail-actions { display: flex; align-items: center; gap: var(--space-1-5); padding: var(--space-3) 0; }
.talk-scroll { flex: 1; min-height: 0; overflow-y: auto; padding: var(--space-4) var(--space-5) var(--space-8); scroll-padding-top: var(--space-4); }
.detail-facts {
  display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 150px), 1fr)); gap: var(--space-2) var(--space-4);
  max-width: 880px; margin: 0 auto var(--space-4); padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-lg); background: var(--color-panel); box-shadow: var(--shadow-sm);
}
.detail-facts > div { min-width: 0; }
.detail-facts dt { display: flex; align-items: center; gap: var(--space-1); color: var(--color-text-tertiary); font-size: var(--text-2xs); }
.detail-facts dd { margin: 2px 0 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: var(--text-xs); color: var(--color-text); font-variant-numeric: tabular-nums; }
.detail-facts dd.mono { font-family: var(--font-mono); font-size: var(--text-2xs); }
.detail-warning { display: flex; align-items: center; gap: var(--space-2); max-width: 880px; margin: 0 auto var(--space-4); padding: var(--space-2) var(--space-3); border-radius: var(--radius-md); background: var(--color-warning-soft); color: var(--color-warning-text); font-size: var(--text-xs); }
.talk-error, .talk-loading { display: flex; align-items: center; justify-content: center; gap: var(--space-2); padding: var(--space-8) var(--space-4); color: var(--color-text-tertiary); }
.talk-error { color: var(--color-danger); }
.talk-empty { max-width: 880px; margin: var(--space-6) auto 0; text-align: center; color: var(--color-text-tertiary); font-size: var(--text-xs); }
.load-more { display: flex; margin: var(--space-4) auto 0; }

@media (max-width: 1023px) {
  .detail-pane { animation: pop-in var(--duration) var(--ease); }
  .detail-back { display: inline-flex; }
}
@media (max-width: 560px) {
  .detail-header { padding: var(--space-2) var(--space-3) 0; }
  .detail-title-row > .provider-icon { display: none; }
  .talk-scroll { padding: var(--space-3) var(--space-3) var(--space-6); }
  .detail-actions { flex-wrap: wrap; }
  .detail-facts { grid-template-columns: repeat(2, minmax(0, 1fr)); padding: var(--space-3); }
}
</style>
