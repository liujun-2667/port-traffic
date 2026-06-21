<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { api } from '../api/client'
import type { Ship, ShipDetail, StateChange, TimelineEvent } from '../api/types'

const props = defineProps<{
  runId: number | null
  ship: Ship | null
}>()

const emit = defineEmits<{ (e: 'close'): void }>()

const detail = ref<ShipDetail | null>(null)
const loading = ref(false)
const error = ref('')

const STATE_LABEL: Record<string, string> = {
  arrived: '已到达',
  waiting: '等待中',
  inbound: '进港',
  berthing: '靠泊中',
  working: '作业中',
  outbound: '出港',
  departed: '已离港',
  holding: '等待避让'
}

async function loadDetail() {
  if (!props.runId || !props.ship) {
    detail.value = null
    return
  }
  loading.value = true
  error.value = ''
  try {
    detail.value = await api.getShipDetail(props.runId, props.ship.id)
  } catch (e: any) {
    error.value = e.message || String(e)
  } finally {
    loading.value = false
  }
}

watch(() => [props.runId, props.ship?.id], () => loadDetail(), { immediate: true })

const sortedHistory = computed<StateChange[]>(() => {
  if (!detail.value) return []
  return [...detail.value.stateHistory].sort((a, b) => a.minute - b.minute)
})

const sortedEncounters = computed<TimelineEvent[]>(() => {
  if (!detail.value) return []
  return [...detail.value.dangerousEncounters].sort((a, b) => a.minute - b.minute)
})
</script>

<template>
  <div v-if="ship" class="panel flex h-full flex-col overflow-hidden">
    <div class="flex items-center justify-between border-b border-glow-cyan/15 px-4 py-3">
      <div>
        <div class="font-mono text-lg text-glow-cyan">{{ ship.id }}</div>
        <div class="text-xs text-slate-500">{{ ship.type }} · {{ ship.length.toFixed(0) }}m × {{ ship.beam.toFixed(0) }}m</div>
      </div>
      <button class="btn text-xs" @click="emit('close')">✕ 关闭</button>
    </div>

    <div class="flex-1 overflow-y-auto p-4">
      <div class="mb-4 grid grid-cols-2 gap-2 text-xs">
        <div class="rounded-md bg-navy-950/60 p-2">
          <div class="text-slate-500">当前状态</div>
          <div class="mt-0.5 font-mono text-glow-cyan">{{ STATE_LABEL[ship.state] || ship.state }}</div>
        </div>
        <div class="rounded-md bg-navy-950/60 p-2">
          <div class="text-slate-500">当前航速</div>
          <div class="mt-0.5 font-mono text-glow-cyan">{{ ship.speedKn.toFixed(1) }} kn</div>
        </div>
        <div class="rounded-md bg-navy-950/60 p-2">
          <div class="text-slate-500">目标泊位</div>
          <div class="mt-0.5 font-mono text-glow-cyan">{{ ship.targetBerth || '—' }}</div>
        </div>
        <div class="rounded-md bg-navy-950/60 p-2">
          <div class="text-slate-500">吃水深度</div>
          <div class="mt-0.5 font-mono text-glow-cyan">{{ ship.draft.toFixed(1) }}m</div>
        </div>
      </div>

      <div v-if="loading" class="text-sm text-glow-amber animate-pulse">加载详情…</div>
      <div v-else-if="error" class="text-sm text-glow-red">{{ error }}</div>

      <template v-else-if="detail">
        <div class="mb-4">
          <h4 class="panel-title mb-2">状态变化时间线</h4>
          <div v-if="!sortedHistory.length" class="text-sm text-slate-500">暂无记录</div>
          <ol v-else class="relative space-y-2 border-l border-glow-cyan/30 pl-4">
            <li v-for="(h, i) in sortedHistory" :key="i" class="text-xs">
              <span class="font-mono text-glow-cyan">{{ h.clock }}</span>
              <span class="mx-2 text-slate-400">→</span>
              <span class="text-slate-200">{{ STATE_LABEL[h.state] || h.state }}</span>
              <span class="ml-2 text-slate-500">({{ h.x.toFixed(0) }}, {{ h.y.toFixed(0) }})</span>
            </li>
          </ol>
        </div>

        <div>
          <h4 class="panel-title mb-2">危险会遇记录</h4>
          <div v-if="!sortedEncounters.length" class="text-sm text-slate-500">无危险会遇记录 ✓</div>
          <ul v-else class="space-y-2">
            <li
              v-for="(e, i) in sortedEncounters"
              :key="i"
              class="rounded-md border border-glow-red/30 bg-glow-red/5 px-3 py-2 text-xs"
            >
              <div class="flex items-center justify-between">
                <span class="font-mono text-glow-red">{{ e.clock }}</span>
                <span class="text-glow-red">{{ e.type === 'danger' ? '危险会遇' : '碰撞预警' }}</span>
              </div>
              <div class="mt-1 text-slate-300">{{ e.desc }}</div>
            </li>
          </ul>
        </div>
      </template>
    </div>
  </div>
</template>
