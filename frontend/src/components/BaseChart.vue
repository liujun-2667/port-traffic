<script setup lang="ts">
import * as echarts from 'echarts'
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'

const props = defineProps<{ option: any; height?: string }>()
const el = ref<HTMLDivElement | null>(null)
let chart: echarts.ECharts | null = null

function render() {
  if (chart) chart.setOption(props.option, true)
}

onMounted(() => {
  if (el.value) {
    chart = echarts.init(el.value, 'dark')
    chart.setOption(props.option, true)
    window.addEventListener('resize', resize)
  }
})

function resize() {
  chart?.resize()
}

watch(() => props.option, render, { deep: true })

onBeforeUnmount(() => {
  window.removeEventListener('resize', resize)
  chart?.dispose()
  chart = null
})
</script>

<template>
  <div ref="el" :style="{ width: '100%', height: height || '240px' }"></div>
</template>
