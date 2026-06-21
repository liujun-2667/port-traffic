<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { api } from '../api/client'
import type { Frame, RunMeta, Ship, ShipState, ShipType, TrajectoryRow } from '../api/types'
import PortCanvas from '../components/PortCanvas.vue'
import { useSimStore } from '../stores/sim'

const store = useSimStore()
const runs = ref<RunMeta[]>([])
const selectedRun = ref<number | null>(null)
const rows = ref<TrajectoryRow[]>([])
const maxMinute = ref(0)
const curMinute = ref(0)
const playing = ref(false)
const loadingRows = ref(false)
let timer: number | null = null

const TYPES: ShipType[] = ['container', 'bulk', 'tanker', 'other']
function hashType(id: string): ShipType {
  let h = 0
  for (let i = 0; i < id.length; i++) h = (h * 31 + id.charCodeAt(i)) >>> 0
  return TYPES[h % 4]
}

function fmtClock(min: number) {
  const h = Math.floor(min / 60)
  const m = min % 60
  return `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}`
}

onMounted(async () => {
  try {
    runs.value = await api.listRuns()
  } catch {
    runs.value = []
  }
  if (!store.config) await store.loadConfig().catch(() => {})
})

async function selectRun(id: number) {
  selectedRun.value = id
  loadingRows.value = true
  rows.value = []
  try {
    const r = await api.getTrajectory(id)
    rows.value = r
    maxMinute.value = r.reduce((m, row) => Math.max(m, row.minute), 0)
    curMinute.value = 0
  } finally {
    loadingRows.value = false
  }
}

const rowsByMinute = computed(() => {
  const map = new Map<number, TrajectoryRow[]>()
  for (const r of rows.value) {
    let arr = map.get(r.minute)
    if (!arr) {
      arr = []
      map.set(r.minute, arr)
    }
    arr.push(r)
  }
  return map
})

const replayFrame = computed<Frame | null>(() => {
  const cfg = store.config
  if (!cfg) return null
  const list = rowsByMinute.value.get(curMinute.value) ?? []
  const ships: Ship[] = list.map((r) => ({
    id: r.shipId,
    type: hashType(r.shipId),
    length: 150,
    beam: 24,
    draft: 10,
    dwt: 0,
    targetBerth: '',
    speedKn: r.speed,
    plannedSpeed: r.speed,
    maneuver: { turningRadius: 400, stopDistance: 700, accelRate: 0.4, decelRate: 0.6 },
    state: r.state as ShipState,
    position: { x: r.x, y: r.y },
    route: [],
    routeIdx: 0,
    segOffset: 0,
    arrivalMinute: 0,
    enterMinute: 0,
    berthMinute: 0,
    workDuration: 0,
    waitMinutes: 0,
    direction: 1
  }))
  return {
    minute: curMinute.value,
    clock: fmtClock(curMinute.value),
    done: curMinute.value >= maxMinute.value,
    ships,
    segmentCongestion: [],
    encounters: [],
    kpi: { inPort: ships.length, queueLength: 0, congestedSegments: 0, cumDangerous: 0, cumWarnings: 0, throughputIn: 0, throughputOut: 0, avgWait: 0, maxWait: 0 },
    throughput: [],
    tideLevel: 0,
    navigableDepth: 0,
    berths: cfg.port.berths.map((b) => ({ id: b.id, type: b.type, occupied: false, shipId: '' })),
    anchorage: { id: cfg.port.anchorages[0]?.id ?? '', count: 0, capacity: cfg.port.anchorages[0]?.capacity ?? 0 },
    events: [],
    closedSegments: [],
    strategy: { strategy: 'free_flow', tidalThresholdMeters: 0, oneWaySwitchMinutes: 30, oneWaySegments: [] }
  }
})

function togglePlay() {
  if (playing.value) {
    playing.value = false
    if (timer) window.clearInterval(timer)
    timer = null
    return
  }
  if (curMinute.value >= maxMinute.value) curMinute.value = 0
  playing.value = true
  timer = window.setInterval(() => {
    if (curMinute.value < maxMinute.value) {
      curMinute.value += 1
    } else {
      playing.value = false
      if (timer) window.clearInterval(timer)
      timer = null
    }
  }, 250)
}

watch(playing, (p) => {
  if (!p && timer) {
    window.clearInterval(timer)
    timer = null
  }
})

onBeforeUnmount(() => {
  if (timer) window.clearInterval(timer)
})

function runLabel(r: RunMeta) {
  const params = typeof r.paramsJson === 'string' ? r.paramsJson : String.fromCharCode(...(r.paramsJson as number[]))
  let rate = ''
  try {
    rate = JSON.parse(params).arrivalRate ?? ''
  } catch {
    /* ignore */
  }
  return `#${r.id} · ${new Date(r.startedAt).toLocaleString()} · λ=${rate} · ${r.durationMinutes}min · ${r.status}`
}
</script>

<template>
  <div class="mx-auto flex h-full max-w-6xl flex-col gap-3 p-4">
    <header>
      <h2 class="text-lg font-semibold">历史回放</h2>
      <p class="text-xs text-slate-500">选择一次历史仿真记录,拖动时间轴或点击播放回放完整船舶轨迹动画。</p>
    </header>

    <div class="flex gap-3">
      <select v-model="selectedRun" class="input max-w-xl" @change="selectedRun && selectRun(selectedRun)">
        <option :value="null" disabled>选择仿真记录…</option>
        <option v-for="r in runs" :key="r.id" :value="r.id">{{ runLabel(r) }}</option>
      </select>
      <span v-if="loadingRows" class="self-center text-sm text-glow-amber animate-pulse">加载轨迹…</span>
    </div>

    <div v-if="replayFrame" class="panel relative min-h-[420px] flex-1 overflow-hidden">
      <div class="absolute left-3 top-3 z-10 panel-title">回放 · #{{ selectedRun }}</div>
      <PortCanvas :config="store.config" :frame="replayFrame" />
    </div>

    <div v-if="selectedRun && rows.length" class="panel flex items-center gap-3 px-4 py-3">
      <button class="btn" @click="togglePlay">{{ playing ? '暂停' : '播放' }}</button>
      <input v-model.number="curMinute" type="range" min="0" :max="maxMinute" class="flex-1 accent-glow-cyan" />
      <span class="font-mono text-sm text-glow-cyan">{{ fmtClock(curMinute) }} / {{ fmtClock(maxMinute) }}</span>
    </div>
    <p v-else-if="selectedRun && !loadingRows" class="text-sm text-slate-500">该记录无轨迹数据(可能仿真未正常完成或未启用持久化)。</p>
  </div>
</template>
