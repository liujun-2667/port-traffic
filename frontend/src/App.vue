<script setup lang="ts">
import { RouterLink, RouterView } from 'vue-router'
import { useSimStore } from './stores/sim'
import { onMounted } from 'vue'

const store = useSimStore()
onMounted(() => {
  store.loadConfig().catch((e) => console.error('config load failed', e))
})
</script>

<template>
  <div class="flex h-full flex-col">
    <header class="flex items-center justify-between border-b border-glow-cyan/15 bg-navy-950/80 px-6 py-3 backdrop-blur">
      <div class="flex items-center gap-3">
        <div class="h-8 w-8 rounded-md bg-gradient-to-br from-glow-cyan to-glow-cyan/30 flex items-center justify-center font-mono text-navy-950 font-bold">PT</div>
        <div>
          <h1 class="text-sm font-semibold tracking-wide">港口船舶交通流仿真与安全评估</h1>
          <p class="text-[10px] text-slate-500 font-mono uppercase tracking-widest">Port Traffic Flow Simulation &amp; Safety Assessment</p>
        </div>
      </div>
      <nav class="flex items-center gap-1 text-sm">
        <RouterLink to="/" class="px-3 py-1.5 rounded-md border border-transparent hover:border-glow-cyan/30 hover:text-glow-cyan transition-colors">实时仿真</RouterLink>
        <RouterLink to="/config" class="px-3 py-1.5 rounded-md border border-transparent hover:border-glow-cyan/30 hover:text-glow-cyan transition-colors">参数配置</RouterLink>
        <RouterLink to="/sensitivity" class="px-3 py-1.5 rounded-md border border-transparent hover:border-glow-cyan/30 hover:text-glow-cyan transition-colors">敏感性实验</RouterLink>
        <RouterLink to="/dredging" class="px-3 py-1.5 rounded-md border border-transparent hover:border-glow-cyan/30 hover:text-glow-cyan transition-colors">航道维护</RouterLink>
        <RouterLink to="/replay" class="px-3 py-1.5 rounded-md border border-transparent hover:border-glow-cyan/30 hover:text-glow-cyan transition-colors">历史回放</RouterLink>
      </nav>
    </header>
    <main class="flex-1 overflow-auto">
      <RouterView />
    </main>
  </div>
</template>
