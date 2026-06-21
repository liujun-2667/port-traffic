<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useSimStore } from '../stores/sim'

const store = useSimStore()

const sim = reactive({
  arrivalRate: 3,
  durationHours: 24,
  seed: 42,
  safeSpacingShips: 3,
  encounterSafeRatio: 2
})
const weather = reactive({ windSpeed: 8, visibility: 5, swell: 0.6 })
const saved = ref(false)

watch(
  () => store.config,
  (c) => {
    if (!c) return
    Object.assign(sim, c.sim)
    Object.assign(weather, c.weather)
  },
  { immediate: true }
)

async function apply() {
  await store.updateConfig({ sim: { ...sim }, weather: { ...weather } })
  saved.value = true
  setTimeout(() => (saved.value = false), 1500)
}

const cfg = computed(() => store.config)
</script>

<template>
  <div class="mx-auto max-w-6xl space-y-4 p-4">
    <header class="flex items-center justify-between">
      <div>
        <h2 class="text-lg font-semibold">参数配置</h2>
        <p class="text-xs text-slate-500">修改仿真与气象参数后点击「应用热更新」,无需重启服务即时生效。航道/泊位等结构参数请编辑 <span class="font-mono text-glow-cyan">backend/config/port.yaml</span>。</p>
      </div>
    </header>

    <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
      <section class="panel space-y-3 p-4">
        <h3 class="panel-title">仿真参数 (sim)</h3>
        <div class="grid grid-cols-2 gap-3">
          <div><label class="label">到达率 (艘/h)</label><input v-model.number="sim.arrivalRate" type="number" step="0.5" class="input" /></div>
          <div><label class="label">默认时长 (h)</label><input v-model.number="sim.durationHours" type="number" class="input" /></div>
          <div><label class="label">随机种子</label><input v-model.number="sim.seed" type="number" class="input" /></div>
          <div><label class="label">安全间距 (×船长)</label><input v-model.number="sim.safeSpacingShips" type="number" step="0.5" class="input" /></div>
          <div><label class="label">会遇安全比 (×船宽和)</label><input v-model.number="sim.encounterSafeRatio" type="number" step="0.5" class="input" /></div>
        </div>
      </section>

      <section class="panel space-y-3 p-4">
        <h3 class="panel-title">气象参数 (weather)</h3>
        <div class="grid grid-cols-3 gap-3">
          <div><label class="label">风速 (节)</label><input v-model.number="weather.windSpeed" type="number" class="input" /></div>
          <div><label class="label">能见度 (海里)</label><input v-model.number="weather.visibility" type="number" step="0.1" class="input" /></div>
          <div><label class="label">涌浪 (m)</label><input v-model.number="weather.swell" type="number" step="0.1" class="input" /></div>
        </div>
        <div class="flex items-center gap-3 pt-1">
          <button class="btn" @click="apply">应用热更新</button>
          <span v-if="saved" class="text-xs text-glow-cyan">已生效 ✓</span>
        </div>
      </section>
    </div>

    <section v-if="cfg" class="panel p-4">
      <h3 class="panel-title mb-3">航道模型 (segments)</h3>
      <div class="overflow-x-auto">
        <table class="w-full text-left text-sm">
          <thead class="text-xs text-slate-500">
            <tr><th class="py-1 pr-3">编号</th><th class="pr-3">宽度(m)</th><th class="pr-3">基准水深(m)</th><th class="pr-3">限速(节)</th><th class="pr-3">最大吨位</th><th class="pr-3">起点</th><th>终点</th></tr>
          </thead>
          <tbody class="font-mono">
            <tr v-for="s in cfg.port.segments" :key="s.id" class="border-t border-glow-cyan/10">
              <td class="py-1 pr-3 text-glow-cyan">{{ s.id }}</td>
              <td class="pr-3">{{ s.width }}</td>
              <td class="pr-3">{{ s.baseDepth }}</td>
              <td class="pr-3">{{ s.speedLimit }}</td>
              <td class="pr-3">{{ s.maxTonnage }}</td>
              <td class="pr-3 text-slate-400">({{ s.from.x }},{{ s.from.y }})</td>
              <td class="text-slate-400">({{ s.to.x }},{{ s.to.y }})</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <div v-if="cfg" class="grid grid-cols-1 gap-4 lg:grid-cols-3">
      <section class="panel p-4">
        <h3 class="panel-title mb-3">泊位 (berths)</h3>
        <table class="w-full text-left text-sm">
          <thead class="text-xs text-slate-500"><tr><th class="pr-2">编号</th><th class="pr-2">类型</th><th>最大吨位</th></tr></thead>
          <tbody class="font-mono">
            <tr v-for="b in cfg.port.berths" :key="b.id" class="border-t border-glow-cyan/10"><td class="py-1 pr-2 text-glow-cyan">{{ b.id }}</td><td class="pr-2">{{ b.type }}</td><td>{{ b.maxTonnage }}</td></tr>
          </tbody>
        </table>
      </section>
      <section class="panel p-4">
        <h3 class="panel-title mb-3">潮汐分潮 (tide)</h3>
        <table class="w-full text-left text-sm">
          <thead class="text-xs text-slate-500"><tr><th class="pr-2">名称</th><th class="pr-2">振幅(m)</th><th class="pr-2">相位</th><th>角速度</th></tr></thead>
          <tbody class="font-mono">
            <tr v-for="c in cfg.tide.components" :key="c.name" class="border-t border-glow-cyan/10"><td class="py-1 pr-2 text-glow-cyan">{{ c.name }}</td><td class="pr-2">{{ c.amplitude }}</td><td class="pr-2">{{ c.phase }}</td><td>{{ c.speed }}</td></tr>
          </tbody>
        </table>
        <p class="mt-2 text-xs text-slate-500">吃水裕度系数: <span class="font-mono text-slate-300">{{ cfg.tide.draftMargin }}</span> · 平均海面: <span class="font-mono text-slate-300">{{ cfg.tide.meanSeaLevel }}m</span></p>
      </section>
      <section class="panel p-4">
        <h3 class="panel-title mb-3">锚地 / 调头区</h3>
        <div v-for="a in cfg.port.anchorages" :key="a.id" class="text-sm font-mono">锚地 {{ a.id }} · 容量 {{ a.capacity }}</div>
        <div v-for="t in cfg.port.turningAreas" :key="t.id" class="mt-1 text-sm font-mono">调头区 {{ t.id }} · 直径 {{ t.diameter }}m</div>
        <div v-for="z in cfg.port.encounterZones" :key="z.id" class="mt-1 text-sm font-mono">会遇区 {{ z.id }} · 半径 {{ z.radius }}m</div>
      </section>
    </div>
  </div>
</template>
