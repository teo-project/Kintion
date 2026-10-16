<script setup lang="ts">
// Оглавление демо-сборки: все экраны и состояния со ссылками. Ссылка
// `?demo=<id>` загружает страницу заново: данные — из снимка, затем шаги
// состояния (states.ts).
import { computed } from 'vue'

import { DEMO_GROUPS } from './states'

const states = window.__kinDemo?.states ?? []
const groups = computed(() => DEMO_GROUPS.map(g => ({ name: g, list: states.filter(s => s.group === g) })))

function href(id: string): string {
  return `${location.pathname}?demo=${encodeURIComponent(id)}`
}
</script>

<template>
  <main class="demo-index">
    <h1>Демо: экраны и состояния</h1>
    <p class="kin-muted">
      Настоящий интерфейс Kintion без сервера: данные — демо-набор
      (deploy/scripts/seed-demo.sh), изменения живут до перезагрузки страницы.
      Каждая ссылка открывает экран заново и приводит его в нужное состояние.
    </p>
    <section
      v-for="g in groups"
      :key="g.name"
      class="group"
    >
      <h2>{{ g.name }}</h2>
      <ul>
        <li
          v-for="s in g.list"
          :key="s.id"
        >
          <a
            :href="href(s.id)"
            :data-state="s.id"
          >{{ s.title }}</a>
          <span
            v-if="s.note"
            class="kin-muted note"
          >{{ s.note }}</span>
          <code class="kin-muted path">{{ s.path }}</code>
        </li>
      </ul>
    </section>
  </main>
</template>

<style scoped>
.demo-index {
  max-width: 56rem;
  padding: var(--kin-space-3);
}

.group ul {
  display: flex;
  flex-direction: column;
  gap: var(--kin-space-1);
  padding-inline-start: var(--kin-space-3);
}

.note {
  margin-inline-start: var(--kin-space-2);
  font-size: var(--kin-font-small);
}

.path {
  display: block;
  overflow-wrap: anywhere;
  font-size: var(--kin-font-small);
}
</style>
