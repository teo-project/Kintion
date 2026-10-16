<script setup lang="ts">
// Журнал дашборда (F-08, F-16): в 0.1 владелец видит всё.
import { watch } from 'vue'
import { RouterLink } from 'vue-router'

import AuditList from '@/components/AuditList.vue'
import { t } from '@/i18n'
import { rememberDashboard } from '@/stores/dashboards'
import KinIcon from '@/components/KinIcon.vue'

const props = defineProps<{ d: string }>()

watch(() => props.d, d => rememberDashboard(d), { immediate: true })
</script>

<template>
  <main class="page kin-page">
    <div class="kin-page-head">
      <RouterLink
        class="kin-ibtn kin-ibtn--ghost back"
        :to="{ name: 'dashboard', params: { d } }"
        :aria-label="t.card.toDashboard"
        :title="t.card.toDashboard"
      >
        <KinIcon name="back" />
      </RouterLink>
      <h1 class="kin-page-title">
        {{ t.audit.title }}
      </h1>
    </div>
    <section class="kin-island">
      <AuditList :dashboard="d" />
    </section>
  </main>
</template>

<style scoped>
.page {
  max-width: 54rem;
}
</style>
