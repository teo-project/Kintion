<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'

import { errorText, t } from '@/i18n'
import { useDashboardsStore } from '@/stores/dashboards'

const dashboards = useDashboardsStore()
const router = useRouter()
const error = ref('')

onMounted(async () => {
  try {
    await dashboards.load()
    const d = dashboards.initial()
    if (d) await router.replace({ name: 'dashboard', params: { d: d.id } })
  } catch (e) {
    error.value = errorText(e)
  }
})
</script>

<template>
  <main class="home kin-page">
    <h1 class="kin-page-title">
      {{ t.appName }}
    </h1>
    <p
      v-if="error"
      class="kin-error"
      role="alert"
    >
      {{ error }}
    </p>
    <p
      v-else
      class="kin-muted"
    >
      {{ t.common.loading }}
    </p>
  </main>
</template>

<style scoped>
.home {
  align-items: flex-start;
}
</style>
