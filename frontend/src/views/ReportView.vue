<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api/client'
import type { Report } from '../api/types'

const props = defineProps<{ runId: string | string[] }>()
const id = computed(() => Number(Array.isArray(props.runId) ? props.runId[0] : props.runId))

const report = ref<Report | null>(null)
const error = ref('')
const loading = ref(true)

onMounted(async () => {
  try {
    report.value = await api.getReport(id.value)
  } catch (e: any) {
    error.value = e.message || String(e)
  } finally {
    loading.value = false
  }
})

const metricRows = computed(() => {
  if (!report.value) return []
  const m = report.value.metrics
  return [
    { label: '进港船次', value: m.throughputIn },
    { label: '出港船次', value: m.throughputOut },
    { label: '总吞吐量', value: m.totalThroughput },
    { label: '平均等待时间 (min)', value: m.avgWaitMinutes.toFixed(1) },
    { label: '最大等待时间 (min)', value: m.maxWaitMinutes },
    { label: '严重延误 ( >60min ) 船次', value: m.severeDelayCount },
    { label: '危险会遇次数', value: m.dangerousEncounters },
    { label: '碰撞预警次数', value: m.collisionWarnings }
  ]
})

const priorityColor: Record<string, string> = {
  高: 'text-glow-red border-glow-red/40',
  中: 'text-glow-amber border-glow-amber/40',
  低: 'text-glow-cyan border-glow-cyan/40'
}

function exportJSON() {
  if (!report.value) return
  const blob = new Blob([JSON.stringify(report.value, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `port-traffic-report-${id.value}.json`
  a.click()
  URL.revokeObjectURL(url)
}
</script>

<template>
  <div class="mx-auto max-w-5xl space-y-4 p-4">
    <header class="flex items-center justify-between">
      <div>
        <h2 class="text-lg font-semibold">仿真评估报告 · RUN #{{ id }}</h2>
        <p v-if="report" class="text-xs text-slate-500">{{ report.summary.durationMinutes }} 分钟 · 到达率 {{ report.summary.arrivalRate }} 艘/h · 种子 {{ report.summary.seed }} · 风 {{ report.summary.windSpeed }} 节 · 能见度 {{ report.summary.visibility }} 海里</p>
      </div>
      <button class="btn" :disabled="!report" @click="exportJSON">导出 JSON</button>
    </header>

    <p v-if="loading" class="text-sm text-glow-amber animate-pulse">生成报告中…</p>
    <p v-if="error" class="text-sm text-glow-red">{{ error }}</p>

    <template v-if="report">
      <section class="panel p-4">
        <h3 class="panel-title mb-3">安全指标统计</h3>
        <div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <div v-for="r in metricRows" :key="r.label" class="rounded-md border border-glow-cyan/10 bg-navy-950/50 px-3 py-2">
            <div class="text-[10px] uppercase tracking-wider text-slate-500">{{ r.label }}</div>
            <div class="mt-1 font-mono text-lg text-glow-cyan">{{ r.value }}</div>
          </div>
        </div>
      </section>

      <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <section class="panel p-4">
          <h3 class="panel-title mb-3">瓶颈航段识别 (TOP3)</h3>
          <div v-if="!report.bottlenecks.length" class="text-sm text-slate-500">无显著瓶颈。</div>
          <div v-for="b in report.bottlenecks" :key="b.segId" class="mb-2 flex items-center justify-between rounded-md border bg-navy-950/40 px-3 py-2" :class="priorityColor[b.priority] || 'border-glow-cyan/10'">
            <div>
              <div class="font-mono text-sm">{{ b.segId }} · 优先级 {{ b.priority }}</div>
              <div class="text-xs text-slate-500">平均拥堵 {{ (b.avgCongestion * 100).toFixed(0) }}% · 峰值 {{ (b.peakCongestion * 100).toFixed(0) }}%</div>
            </div>
            <span class="text-xs text-slate-400">扩容优先级 {{ b.rank }}</span>
          </div>
        </section>

        <section class="panel p-4">
          <h3 class="panel-title mb-3">调度建议</h3>
          <div v-if="!report.advice.length" class="text-sm text-glow-cyan">当前运行状况良好,无需额外调度。</div>
          <ul class="space-y-2">
            <li v-for="a in report.advice" :key="a.code" class="rounded-md border border-glow-amber/30 bg-glow-amber/5 px-3 py-2 text-sm">
              <span class="font-mono text-xs text-glow-amber">[{{ a.code }}]</span>
              <span class="ml-2 text-slate-200">{{ a.text }}</span>
            </li>
          </ul>
        </section>
      </div>

      <section class="panel p-4">
        <h3 class="panel-title mb-3">关键事件时间线</h3>
        <div v-if="!report.events.length" class="text-sm text-slate-500">本次仿真未记录关键事件。</div>
        <ol v-else class="relative space-y-2 border-l border-glow-red/30 pl-4">
          <li v-for="(e, i) in report.events" :key="i" class="text-sm">
            <span class="font-mono text-xs text-glow-cyan">{{ e.clock }}</span>
            <span class="mx-2 text-slate-300">{{ e.type }}</span>
            <span class="text-slate-400">{{ e.shipA }} ⇄ {{ e.shipB }}</span>
            <span class="ml-2 text-slate-500">{{ e.desc }}</span>
          </li>
        </ol>
      </section>

      <section class="panel p-4">
        <h3 class="panel-title mb-3">各航段平均拥堵度</h3>
        <div class="space-y-1.5">
          <div v-for="s in report.metrics.segmentCongestion" :key="s.segId" class="flex items-center gap-2 text-sm">
            <span class="w-10 font-mono text-glow-cyan">{{ s.segId }}</span>
            <div class="h-2 flex-1 overflow-hidden rounded bg-navy-950">
              <div class="h-full rounded" :class="s.avgCongestion > 0.7 ? 'bg-glow-red' : s.avgCongestion > 0.4 ? 'bg-glow-amber' : 'bg-glow-cyan'" :style="{ width: Math.min(100, s.avgCongestion * 100) + '%' }"></div>
            </div>
            <span class="w-12 text-right font-mono text-slate-400">{{ (s.avgCongestion * 100).toFixed(0) }}%</span>
          </div>
        </div>
      </section>
    </template>
  </div>
</template>
