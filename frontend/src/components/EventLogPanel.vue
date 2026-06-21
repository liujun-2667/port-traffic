<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import type { TimelineEvent } from '../api/types'

const props = defineProps<{ events: TimelineEvent[] }>()

const expanded = ref(true)
const logRef = ref<HTMLDivElement | null>(null)

const isDangerType = (t: string) => t === 'danger' || t === 'warning'

const sortedEvents = computed(() => [...props.events].sort((a, b) => a.minute - b.minute))

watch(
  () => props.events.length,
  async () => {
    await nextTick()
    if (logRef.value && expanded.value) {
      logRef.value.scrollTop = logRef.value.scrollHeight
    }
  }
)
</script>

<template>
  <div class="panel overflow-hidden">
    <button
      class="flex w-full items-center justify-between px-4 py-2.5 text-left hover:bg-glow-cyan/5"
      @click="expanded = !expanded"
    >
      <div class="flex items-center gap-2">
        <span class="panel-title">实时事件日志</span>
        <span class="rounded-full bg-navy-950 px-2 py-0.5 text-[10px] font-mono text-slate-400">{{ sortedEvents.length }}/200</span>
      </div>
      <span class="text-xs text-slate-400">{{ expanded ? '▼ 收起' : '▲ 展开' }}</span>
    </button>
    <div
      v-show="expanded"
      ref="logRef"
      class="h-48 overflow-y-auto border-t border-glow-cyan/10 px-2 py-1.5"
    >
      <div v-if="!sortedEvents.length" class="px-2 py-4 text-center text-xs text-slate-500">暂无事件,启动仿真后将在此显示关键事件…</div>
      <div
        v-for="(e, i) in sortedEvents"
        :key="i"
        class="flex items-start gap-2 rounded px-2 py-1 text-xs"
        :class="isDangerType(e.type) ? 'bg-glow-red/10 text-glow-red' : 'text-slate-300'"
      >
        <span class="shrink-0 font-mono text-slate-500">[{{ e.clock }}]</span>
        <span
          class="shrink-0 rounded px-1.5 font-mono text-[10px]"
          :class="isDangerType(e.type) ? 'bg-glow-red/20 text-glow-red' : 'bg-navy-950 text-slate-400'"
        >{{ e.type }}</span>
        <span class="min-w-0 flex-1 truncate" :class="isDangerType(e.type) ? 'text-glow-red' : 'text-slate-300'">{{ e.desc }}</span>
      </div>
    </div>
  </div>
</template>
