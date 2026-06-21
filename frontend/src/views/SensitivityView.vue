<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { api } from '../api/client'
import type { DualResult, SinglePoint } from '../api/types'
import BaseChart from '../components/BaseChart.vue'

const params = ['arrivalRate', 'speedLimit', 'windSpeed', 'visibility', 'durationHours']
const metrics = [
  { key: 'dangerous', label: '危险会遇' },
  { key: 'warning', label: '碰撞预警' },
  { key: 'avgWait', label: '平均等待(min)' },
  { key: 'throughput', label: '吞吐量' },
  { key: 'congestion', label: '平均拥堵度' }
]

const single = reactive({ param: 'arrivalRate', from: 2, to: 6, step: 0.5, metric: 'dangerous' })
const singleRes = ref<SinglePoint[]>([])
const singleLoading = ref(false)
const singleError = ref('')

async function runSingle() {
  singleLoading.value = true
  singleError.value = ''
  singleRes.value = []
  try {
    singleRes.value = await api.sensitivitySingle(single.param, single.from, single.to, single.step)
  } catch (e: any) {
    singleError.value = e.message || String(e)
  } finally {
    singleLoading.value = false
  }
}

const singleOption = computed(() => {
  const data = singleRes.value
  const metricKey = single.metric as keyof SinglePoint
  return {
    backgroundColor: 'transparent',
    grid: { left: 44, right: 16, top: 28, bottom: 28 },
    tooltip: { trigger: 'axis' },
    legend: { show: false },
    xAxis: { type: 'category', data: data.map((d) => d.value), name: single.param, nameTextStyle: { color: '#64748b' }, axisLabel: { color: '#64748b', fontSize: 10 } },
    yAxis: { type: 'value', name: metrics.find((m) => m.key === single.metric)?.label, nameTextStyle: { color: '#64748b' }, axisLabel: { color: '#64748b', fontSize: 10 }, splitLine: { lineStyle: { color: 'rgba(148,163,184,0.1)' } } },
    series: [
      {
        type: 'line',
        smooth: true,
        data: data.map((d) => d[metricKey]),
        lineStyle: { color: '#00e5c7', width: 2 },
        symbol: 'circle',
        itemStyle: { color: '#00e5c7' },
        areaStyle: { color: 'rgba(0,229,199,0.12)' }
      }
    ]
  }
})

const dual = reactive({
  paramX: 'arrivalRate', fromX: 2, toX: 6, stepX: 1,
  paramY: 'speedLimit', fromY: 0.8, toY: 1.2, stepY: 0.1,
  metric: 'dangerous'
})
const dualRes = ref<DualResult | null>(null)
const dualLoading = ref(false)
const dualError = ref('')

async function runDual() {
  dualLoading.value = true
  dualError.value = ''
  dualRes.value = null
  try {
    dualRes.value = await api.sensitivityDual({
      paramX: dual.paramX, fromX: dual.fromX, toX: dual.toX, stepX: dual.stepX,
      paramY: dual.paramY, fromY: dual.fromY, toY: dual.toY, stepY: dual.stepY,
      metric: dual.metric
    })
  } catch (e: any) {
    dualError.value = e.message || String(e)
  } finally {
    dualLoading.value = false
  }
}

const dualOption = computed(() => {
  const r = dualRes.value
  if (!r) return {}
  const data: [number, number, number][] = []
  let max = 0
  for (let yi = 0; yi < r.y.length; yi++) {
    for (let xi = 0; xi < r.x.length; xi++) {
      const v = r.matrix[yi][xi]
      data.push([xi, yi, v])
      if (v > max) max = v
    }
  }
  return {
    backgroundColor: 'transparent',
    grid: { left: 70, right: 24, top: 16, bottom: 60 },
    tooltip: {
      position: 'top',
      formatter: (p: any) => `${r.paramX}=${r.x[p.data[0]]}<br/>${r.paramY}=${r.y[p.data[1]]}<br/>${r.metric}=${p.data[2].toFixed(2)}`
    },
    xAxis: { type: 'category', data: r.x, name: r.paramX, nameTextStyle: { color: '#64748b' }, axisLabel: { color: '#64748b', fontSize: 10 }, splitArea: { show: true } },
    yAxis: { type: 'category', data: r.y, name: r.paramY, nameTextStyle: { color: '#64748b' }, axisLabel: { color: '#64748b', fontSize: 10 }, splitArea: { show: true } },
    visualMap: { min: 0, max: max || 1, calculable: true, orient: 'horizontal', left: 'center', bottom: 4, textStyle: { color: '#94a3b8' }, inRange: { color: ['#0e1d35', '#13294a', '#1b3563', '#ffb547', '#ff4d5e'] } },
    series: [{ type: 'heatmap', data, label: { show: false }, emphasis: { itemStyle: { shadowBlur: 8, shadowColor: 'rgba(0,0,0,0.5)' } } }]
  }
})
</script>

<template>
  <div class="mx-auto max-w-6xl space-y-4 p-4">
    <header>
      <h2 class="text-lg font-semibold">参数敏感性实验</h2>
      <p class="text-xs text-slate-500">批量扫描仿真参数,每组采用 3 个不同随机种子取均值以消除随机波动。计算量较大,请耐心等待。</p>
    </header>

    <section class="panel p-4">
      <h3 class="panel-title mb-3">单参数扫描</h3>
      <div class="flex flex-wrap items-end gap-3">
        <div class="flex flex-col"><label class="label">参数</label>
          <select v-model="single.param" class="input w-40"><option v-for="p in params" :key="p" :value="p">{{ p }}</option></select>
        </div>
        <div class="flex flex-col"><label class="label">起始</label><input v-model.number="single.from" type="number" step="0.5" class="input w-20" /></div>
        <div class="flex flex-col"><label class="label">终止</label><input v-model.number="single.to" type="number" step="0.5" class="input w-20" /></div>
        <div class="flex flex-col"><label class="label">步长</label><input v-model.number="single.step" type="number" step="0.5" class="input w-20" /></div>
        <div class="flex flex-col"><label class="label">指标</label>
          <select v-model="single.metric" class="input w-36"><option v-for="m in metrics" :key="m.key" :value="m.key">{{ m.label }}</option></select>
        </div>
        <button class="btn" :disabled="singleLoading" @click="runSingle">{{ singleLoading ? '计算中…' : '运行扫描' }}</button>
      </div>
      <div v-if="singleError" class="mt-3 text-sm text-glow-red">{{ singleError }}</div>
      <div v-if="singleRes.length" class="mt-3"><BaseChart :option="singleOption" height="260px" /></div>
    </section>

    <section class="panel p-4">
      <h3 class="panel-title mb-3">双参数热力图</h3>
      <div class="flex flex-wrap items-end gap-3">
        <div class="flex flex-col"><label class="label">参数 X</label>
          <select v-model="dual.paramX" class="input w-36"><option v-for="p in params" :key="p" :value="p">{{ p }}</option></select>
        </div>
        <div class="flex flex-col"><label class="label">X 起止步</label><div class="flex gap-1"><input v-model.number="dual.fromX" type="number" step="0.5" class="input w-16" /><input v-model.number="dual.toX" type="number" step="0.5" class="input w-16" /><input v-model.number="dual.stepX" type="number" step="0.5" class="input w-16" /></div></div>
        <div class="flex flex-col"><label class="label">参数 Y</label>
          <select v-model="dual.paramY" class="input w-36"><option v-for="p in params" :key="p" :value="p">{{ p }}</option></select>
        </div>
        <div class="flex flex-col"><label class="label">Y 起止步</label><div class="flex gap-1"><input v-model.number="dual.fromY" type="number" step="0.5" class="input w-16" /><input v-model.number="dual.toY" type="number" step="0.5" class="input w-16" /><input v-model.number="dual.stepY" type="number" step="0.5" class="input w-16" /></div></div>
        <div class="flex flex-col"><label class="label">指标</label>
          <select v-model="dual.metric" class="input w-32"><option v-for="m in metrics" :key="m.key" :value="m.key">{{ m.label }}</option></select>
        </div>
        <button class="btn" :disabled="dualLoading" @click="runDual">{{ dualLoading ? '计算中…' : '运行热力图' }}</button>
      </div>
      <div v-if="dualError" class="mt-3 text-sm text-glow-red">{{ dualError }}</div>
      <div v-if="dualRes" class="mt-3"><BaseChart :option="dualOption" height="360px" /></div>
    </section>
  </div>
</template>
