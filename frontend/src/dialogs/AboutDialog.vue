<script setup lang="ts">
import { computed } from 'vue'
import { ArrowUpRight, CircleAlert, CircleCheck, Download, FolderOpen, LoaderCircle, RefreshCw, ScrollText, Sparkles, X } from 'lucide-vue-next'
import { dialogs, notify } from '../store'
import { appInfo, checkUpdates, checkingUpdate, openLink, setAutoUpdateCheck, update, versionLabel } from '../app'
import { useModal } from './useModal'

const { element, onClose } = useModal(() => dialogs.about, value => { dialogs.about = value })
function backdrop(event: MouseEvent) { if (event.target === element.value) dialogs.about = false }

const shortcuts = [
  { keys: ['Ctrl', 'K'], label: '搜索会话' },
  { keys: ['↑', '↓'], label: '切换会话' },
  { keys: ['Esc'], label: '关闭菜单或清空搜索' },
  { keys: ['F1'], label: '打开关于与帮助' },
]

const built = computed(() => {
  const date = appInfo.value?.date
  if (!date || date === 'unknown') return ''
  const parsed = new Date(date)
  return Number.isNaN(parsed.getTime()) ? date : parsed.toLocaleString('zh-CN', { dateStyle: 'medium', timeStyle: 'short' })
})

const status = computed(() => {
  const result = update.value
  if (checkingUpdate.value) return { tone: 'muted', text: '正在检查更新…' }
  if (!result) return { tone: 'muted', text: '尚未检查更新' }
  switch (result.status) {
    case 'available': return { tone: 'accent', text: `发现新版本 v${result.latest}` }
    case 'latest': return { tone: 'success', text: '已是最新版本' }
    case 'none': return { tone: 'muted', text: '暂未找到已发布的版本' }
    case 'disabled': return { tone: 'muted', text: appInfo.value?.update_check === false ? '自动检查已关闭' : '尚未检查更新' }
    default: return { tone: 'error', text: result.error || '检查更新失败' }
  }
})

async function toggleAuto(event: Event) {
  const enabled = (event.target as HTMLInputElement).checked
  try {
    await setAutoUpdateCheck(enabled)
  } catch (cause) {
    (event.target as HTMLInputElement).checked = !enabled
    notify(cause instanceof Error ? cause.message : '保存失败', true)
  }
}

async function open(url: string | undefined) {
  if (!url) return
  try {
    await openLink(url)
  } catch (cause) {
    notify(cause instanceof Error ? cause.message : '打开链接失败', true)
  }
}

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    notify('已复制路径')
  } catch {
    notify('复制失败', true)
  }
}
</script>

<template>
  <dialog ref="element" class="about-dialog" aria-labelledby="about-title" @close="onClose" @click="backdrop">
    <div class="dialog-header">
      <div class="about-identity">
        <img src="/icon.svg" alt="" width="52" height="52" draggable="false" />
        <div>
          <h2 id="about-title">Session Desk</h2>
          <p class="about-version"><code>{{ versionLabel || '未知版本' }}</code><span v-if="appInfo?.commit && appInfo.commit !== 'unknown'">· {{ appInfo.commit }}</span><span v-if="built">· {{ built }}</span></p>
        </div>
      </div>
      <button class="icon-button" aria-label="关闭关于" @click="dialogs.about = false"><X :size="18" /></button>
    </div>

    <div class="dialog-body">
      <section v-if="appInfo?.repository" class="about-card" aria-labelledby="about-update-title">
        <div class="about-card-heading">
          <h3 id="about-update-title">更新</h3>
          <span class="update-status" :class="status.tone" role="status">
            <LoaderCircle v-if="checkingUpdate" :size="14" class="spin" />
            <Sparkles v-else-if="status.tone === 'accent'" :size="14" />
            <CircleCheck v-else-if="status.tone === 'success'" :size="14" />
            <CircleAlert v-else-if="status.tone === 'error'" :size="14" />
            {{ status.text }}
          </span>
        </div>
        <p v-if="update?.status === 'available' && update.notes" class="update-notes">{{ update.notes }}</p>
        <div class="about-actions">
          <button v-if="update?.status === 'available' && update.download_url" class="button primary small" @click="open(update.download_url)"><Download :size="14" />下载 {{ update.asset_name || '安装包' }}</button>
          <button v-if="update?.status === 'available' && update.url" class="button secondary small" @click="open(update.url)">发布说明<ArrowUpRight :size="14" /></button>
          <button class="button secondary small" :disabled="checkingUpdate" @click="checkUpdates"><RefreshCw :size="14" :class="{ spin: checkingUpdate }" />检查更新</button>
        </div>
        <label class="switch-row">
          <input type="checkbox" role="switch" :checked="appInfo?.update_check !== false" :disabled="!appInfo" @change="toggleAuto" />
          <span class="switch" aria-hidden="true"></span>
          <span>启动时自动检查更新</span>
        </label>
      </section>

      <section class="about-card" aria-labelledby="about-keys-title">
        <div class="about-card-heading"><h3 id="about-keys-title">键盘快捷键</h3></div>
        <ul class="shortcut-list">
          <li v-for="item in shortcuts" :key="item.label"><span>{{ item.label }}</span><span class="shortcut-keys"><kbd v-for="key in item.keys" :key="key">{{ key }}</kbd></span></li>
        </ul>
      </section>

      <section v-if="appInfo?.config_dir || appInfo?.log_path" class="about-card" aria-labelledby="about-files-title">
        <div class="about-card-heading"><h3 id="about-files-title">文件位置</h3></div>
        <dl class="path-list">
          <div v-if="appInfo?.config_dir"><dt><FolderOpen :size="14" />设置与缓存</dt><dd><code>{{ appInfo.config_dir }}</code><button class="text-button" @click="copy(appInfo.config_dir)">复制</button></dd></div>
          <div v-if="appInfo?.log_path"><dt><ScrollText :size="14" />运行日志</dt><dd><code>{{ appInfo.log_path }}</code><button class="text-button" @click="copy(appInfo.log_path)">复制</button></dd></div>
        </dl>
      </section>
    </div>

    <div class="dialog-footer">
      <button v-if="appInfo?.repository" class="button ghost small about-link" @click="open(appInfo.repository)">项目主页<ArrowUpRight :size="14" /></button>
      <button class="button secondary" @click="dialogs.about = false">关闭</button>
    </div>
  </dialog>
</template>

<style>
.about-dialog { width: min(560px, calc(100vw - 32px)); }
.about-identity { display: flex; align-items: center; gap: var(--space-3); }
.about-identity img { flex: none; filter: drop-shadow(0 4px 10px rgba(70, 80, 192, .25)); }
.about-version { display: flex; flex-wrap: wrap; gap: var(--space-1-5, 6px); color: var(--color-text-tertiary) !important; font-size: var(--text-xs) !important; }
.about-version code { color: var(--color-accent-text); font-weight: var(--weight-medium); }
.about-card { margin-top: var(--space-3); padding: var(--space-3) var(--space-4); border-radius: var(--radius-lg); background: var(--color-panel); box-shadow: var(--shadow-sm); }
.about-card:first-child { margin-top: 0; }
.about-card-heading { display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); min-height: 24px; }
.about-card h3 { font-size: var(--text-sm); font-weight: var(--weight-semibold); }
.update-status { display: inline-flex; align-items: center; gap: var(--space-1-5, 6px); font-size: var(--text-xs); color: var(--color-text-secondary); }
.update-status.accent { color: var(--color-accent-text); font-weight: var(--weight-medium); }
.update-status.success { color: var(--color-success); }
.update-status.error { color: var(--color-danger); }
.update-notes { margin-top: var(--space-2); max-height: 120px; overflow: auto; padding: var(--space-2) var(--space-3); border-radius: var(--radius-md); background: var(--color-inset); font-size: var(--text-xs); line-height: 20px; white-space: pre-wrap; color: var(--color-text-secondary); }
.about-actions { display: flex; flex-wrap: wrap; gap: var(--space-2); margin-top: var(--space-3); }
.switch-row { position: relative; display: flex; align-items: center; gap: var(--space-2-5); margin-top: var(--space-3); font-size: var(--text-sm); color: var(--color-text); cursor: pointer; width: fit-content; }
.switch-row input { position: absolute; left: 0; top: 0; margin: 0; opacity: 0; width: 1px; height: 1px; }
.switch { position: relative; flex: none; width: 30px; height: 18px; border-radius: 999px; background: var(--color-border-strong); transition: background-color var(--duration-fast) var(--ease); }
.switch::after { content: ''; position: absolute; top: 2px; left: 2px; width: 14px; height: 14px; border-radius: 50%; background: #fff; box-shadow: 0 1px 2px rgba(0, 0, 0, .2); transition: transform var(--duration-fast) var(--ease); }
.switch-row input:checked + .switch { background: var(--color-accent); }
.switch-row input:checked + .switch::after { transform: translateX(12px); }
.switch-row input:focus-visible + .switch { box-shadow: var(--focus-ring); }
.switch-row input:disabled + .switch { opacity: .5; }
.shortcut-list { list-style: none; margin: var(--space-2) 0 0; padding: 0; }
.shortcut-list li { display: flex; align-items: center; justify-content: space-between; height: 32px; font-size: var(--text-sm); color: var(--color-text-secondary); border-top: 1px solid var(--color-border-subtle); }
.shortcut-list li:first-child { border-top: 0; }
.shortcut-keys { display: inline-flex; gap: 4px; }
.path-list { margin: var(--space-2) 0 0; }
.path-list > div { padding: var(--space-1-5, 6px) 0; border-top: 1px solid var(--color-border-subtle); }
.path-list > div:first-child { border-top: 0; }
.path-list dt { display: flex; align-items: center; gap: var(--space-1-5, 6px); color: var(--color-text-tertiary); font-size: var(--text-xs); }
.path-list dd { display: flex; align-items: center; gap: var(--space-2); margin: 4px 0 0; }
.path-list code { flex: 1; min-width: 0; font-size: var(--text-xs); color: var(--color-text); overflow-wrap: anywhere; }
.about-link { margin-right: auto; }
</style>
