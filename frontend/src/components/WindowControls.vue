<script setup lang="ts">
import { onMounted, onUnmounted, ref, shallowRef } from 'vue'
import { Copy, Minus, Square, X } from '@lucide/vue'

interface DesktopRuntime {
  Window: {
    Minimise(): Promise<void>
    ToggleMaximise(): Promise<void>
    IsMaximised(): Promise<boolean>
    Close(): Promise<void>
  }
  Events: { On(name: string, callback: () => void): () => void }
}

const runtime = shallowRef<DesktopRuntime>()
const maximised = ref(false)
const error = ref('')
const unsubscribe: (() => void)[] = []

onMounted(async () => {
  try {
    const path = '/wails/runtime.js'
    const loaded: DesktopRuntime = await import(/* @vite-ignore */ path)
    maximised.value = await loaded.Window.IsMaximised()
    unsubscribe.push(
      loaded.Events.On('common:WindowMaximise', () => { maximised.value = true }),
      loaded.Events.On('common:WindowUnMaximise', () => { maximised.value = false }),
      loaded.Events.On('common:WindowRestore', () => { maximised.value = false }),
    )
    runtime.value = loaded
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '窗口控件加载失败'
  }
})

onUnmounted(() => { for (const release of unsubscribe) release() })

async function control(action: 'Minimise' | 'ToggleMaximise' | 'Close') {
  const loaded = runtime.value
  if (!loaded) return
  try {
    await loaded.Window[action]()
    error.value = ''
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '窗口操作失败'
  }
}

defineExpose({ toggleMaximise: () => control('ToggleMaximise') })
</script>

<template>
  <div class="window-controls" role="group" aria-label="窗口控制">
    <span v-if="error" class="window-controls-error" role="alert" :title="error">窗口操作失败</span>
    <button class="window-minimise" aria-label="最小化窗口" title="最小化" :disabled="!runtime" @click="control('Minimise')"><Minus :size="14" :stroke-width="1.25" /></button>
    <button class="window-maximise" :aria-label="maximised ? '还原窗口' : '最大化窗口'" :title="maximised ? '还原' : '最大化'" :disabled="!runtime" @click="control('ToggleMaximise')"><Copy v-if="maximised" :size="12" :stroke-width="1.25" /><Square v-else :size="12" :stroke-width="1.25" /></button>
    <button class="window-close" aria-label="关闭窗口" title="关闭" :disabled="!runtime" @click="control('Close')"><X :size="16" :stroke-width="1.25" /></button>
  </div>
</template>
