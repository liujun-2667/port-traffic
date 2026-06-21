<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useSimStore } from '../stores/sim'

const store = useSimStore()

const params = reactive({
  durationHours: 24,
  arrivalRate: 3,
  seed: 42,
  windSpeed: 8,
  visibility: 5,
  speedLimitScale: 1
})

const rates = [
  { label: '1x', value: 1 },
  { label: '5x', value: 5 },
  { label: '20x', value: 20 },
  { label: '最快', value: 0 }
]
const activeRate = ref(1)

async function start() {
  activeRate.value = 1
  await store.startRun({ ...params, speedFactor: 1 })
}
async function setRate(v: number) {
  activeRate.value = v
  await store.control('set_rate', v)
}
</script>

<template>
  <div class="panel flex flex-wrap items-end gap-3 px-4 py-3">
    <div class="flex flex-col">
      <label class="label">仿真时长 (h)</label>
      <input v-model.number="params.durationHours" type="number" min="1" max="168" class="input w-20" />
    </div>
    <div class="flex flex-col">
      <label class="label">到达率 (艘/h)</label>
      <input v-model.number="params.arrivalRate" type="number" min="0.5" max="20" step="0.5" class="input w-20" />
    </div>
    <div class="flex flex-col">
      <label class="label">随机种子</label>
      <input v-model.number="params.seed" type="number" class="input w-20" />
    </div>
    <div class="flex flex-col">
      <label class="label">风速 (节)</label>
      <input v-model.number="params.windSpeed" type="number" min="0" max="60" class="input w-20" />
    </div>
    <div class="flex flex-col">
      <label class="label">能见度 (海里)</label>
      <input v-model.number="params.visibility" type="number" min="0.1" max="10" step="0.1" class="input w-20" />
    </div>
    <div class="flex flex-col">
      <label class="label">限速倍率</label>
      <input v-model.number="params.speedLimitScale" type="number" min="0.5" max="1.5" step="0.1" class="input w-20" />
    </div>
    <button class="btn" :disabled="store.connecting" @click="start">
      {{ store.connecting ? '启动中…' : '启动仿真' }}
    </button>

    <div class="mx-2 h-8 w-px bg-glow-cyan/15"></div>

    <div class="flex flex-col">
      <label class="label">播放倍率</label>
      <div class="flex gap-1">
        <button v-for="r in rates" :key="r.value" class="btn" :class="{ 'bg-glow-cyan/15': activeRate === r.value }" :disabled="!store.runId || store.done" @click="setRate(r.value)">{{ r.label }}</button>
      </div>
    </div>
    <button v-if="store.playing" class="btn btn-amber" :disabled="!store.runId || store.done" @click="store.control('pause')">暂停</button>
    <button v-else class="btn btn-amber" :disabled="!store.runId || store.done" @click="store.control('resume')">继续</button>

    <div class="ml-auto flex items-center gap-2 text-xs font-mono">
      <span v-if="store.runId" class="text-slate-400">RUN #{{ store.runId }}</span>
      <span v-if="store.connecting" class="text-glow-amber animate-pulse">连接中…</span>
      <span v-else-if="store.done" class="text-glow-cyan">已完成</span>
      <span v-else-if="store.playing" class="text-glow-cyan animate-pulse">● LIVE</span>
      <span v-if="store.error" class="text-glow-red">{{ store.error }}</span>
    </div>
  </div>
</template>
