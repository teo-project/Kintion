<script setup lang="ts">
import { RouterLink } from 'vue-router'

import { t } from '@/i18n'
</script>

<template>
  <main class="not-found kin-page">
    <h1 class="kin-page-title">
      {{ t.notFound }}
    </h1>
    <RouterLink to="/">
      {{ t.toHome }}
    </RouterLink>
  </main>
</template>

<style scoped>
.not-found {
  align-items: flex-start;
}
</style>
