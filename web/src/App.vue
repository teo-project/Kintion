<script setup lang="ts">
// Каркас приложения (F-90): верхняя панель на всю ширину окна — логотип,
// разделы текущего дашборда, общий поиск (пока не работает, F-92), тема и
// аккаунт; ниже — экран по маршруту.
import '@fontsource-variable/manrope/wght.css'
import { computed } from 'vue'
import { RouterLink, RouterView, useRoute } from 'vue-router'

import DashboardSwitcher from '@/components/DashboardSwitcher.vue'
import KinIcon from '@/components/KinIcon.vue'
import KinLogo from '@/components/KinLogo.vue'
import ThemeSwitcher from '@/components/ThemeSwitcher.vue'
import { t } from '@/i18n'
import { useDashboardsStore } from '@/stores/dashboards'
import { useSessionStore } from '@/stores/session'

const session = useSessionStore()
const dashboards = useDashboardsStore()
const route = useRoute()

/** дашборд для разделов панели: из адреса, иначе последний открытый */
const dash = computed(() => {
  const d = route.params.d
  return typeof d === 'string' && d ? d : (dashboards.initial()?.id ?? '')
})

/** ракурсы дашборда (F-121): «Граф» — пока заготовка */
const VIEWS = [
  { v: 'tree', label: () => t.nav.tree },
  { v: 'graph', label: () => t.nav.graph },
] as const
const view = computed(() => (route.name === 'dashboard' && typeof route.query.v === 'string' ? route.query.v : ''))
/** фильтр по группе сохраняется при смене ракурса */
const keep = computed(() => (route.name === 'dashboard' && typeof route.query.g === 'string' ? { g: route.query.g } : {}))
const NAV = [
  { name: 'dashboard', label: () => t.nav.lists },
  { name: 'groups', label: () => t.groups.title },
  { name: 'audit', label: () => t.audit.title },
  { name: 'trash', label: () => t.trash.title },
] as const

/** активный раздел: карточка записи относится к дашборду */
function active(name: string): boolean {
  if (name === 'dashboard') return (route.name === name && !view.value) || route.name === 'record'
  return route.name === name
}

const initials = computed(() => (session.account?.login ?? '').slice(0, 2).toUpperCase())
</script>

<template>
  <header
    v-if="!route.meta.bare"
    class="app-bar"
  >
    <RouterLink
      class="brand"
      to="/"
    >
      <KinLogo
        :size="26"
        decorative
      />
      {{ t.appName }}
    </RouterLink>
    <DashboardSwitcher
      v-if="dash && session.account"
      :current="dash"
    />
    <nav
      v-if="dash"
      class="nav"
      :aria-label="t.nav.sections"
    >
      <span
        class="views"
        role="group"
        :aria-label="t.nav.views"
      >
        <RouterLink
          v-for="x in VIEWS"
          :key="x.v"
          class="nav-item"
          :class="{ active: view === x.v }"
          :aria-current="view === x.v ? 'page' : undefined"
          :to="{ name: 'dashboard', params: { d: dash }, query: { ...keep, v: x.v } }"
        >
          {{ x.label() }}
        </RouterLink>
      </span>
      <RouterLink
        v-for="n in NAV"
        :key="n.name"
        class="nav-item"
        :class="{ active: active(n.name) }"
        :aria-current="active(n.name) ? 'page' : undefined"
        :to="{ name: n.name, params: { d: dash }, query: n.name === 'dashboard' ? keep : undefined }"
      >
        {{ n.label() }}
      </RouterLink>
    </nav>
    <span class="kin-spacer" />
    <!-- общий поиск (F-92): функции пока нет, только место в панели;
         readonly + aria-disabled, чтобы чтец экрана дошёл до поля и прочёл подпись -->
    <label
      v-if="session.account"
      class="global-search"
    >
      <KinIcon name="search" />
      <input
        class="kin-input"
        type="search"
        readonly
        aria-disabled="true"
        :placeholder="t.nav.searchSoon"
        :aria-label="t.nav.searchSoonLabel"
        :title="t.nav.searchSoon"
      >
    </label>
    <ThemeSwitcher />
    <RouterLink
      v-if="session.account"
      class="account-avatar"
      :class="{ active: route.name === 'account' }"
      :to="{ name: 'account' }"
      :aria-label="session.account.login"
      :title="`${t.auth.account}: ${session.account.login}`"
    >
      {{ initials }}
    </RouterLink>
  </header>
  <RouterView />
</template>

<style>
/* экран под панелью занимает остаток высоты окна (дерево, F-120) */
#app {
  display: flex;
  flex-direction: column;
  min-height: 100dvh;
}
</style>

<style scoped>
.app-bar {
  display: flex;
  box-sizing: border-box;
  gap: var(--kin-space-2) 20px;
  align-items: center;
  min-height: var(--kin-bar-height);
  padding: var(--kin-space-1) 20px;
  background: var(--kin-color-bar);
  border-bottom: 1px solid var(--kin-color-border);
}

.brand {
  display: inline-flex;
  gap: var(--kin-space-2);
  align-items: center;
  color: var(--kin-color-text);
  font-size: 1.125rem;
  font-weight: 800;
  letter-spacing: -0.02em;
  text-decoration: none;
}

.nav {
  display: flex;
  gap: 2px;
  min-width: 0;
  overflow-x: auto;
}

.views {
  display: flex;
  gap: 2px;
  margin-right: var(--kin-space-2);
  padding: 2px;
  background: var(--kin-color-bg);
  border-radius: calc(var(--kin-radius) + 2px);
}
.views .nav-item {
  padding: 4px 10px;
}
.nav-item {
  padding: 6px 12px;
  color: var(--kin-color-text-muted);
  border-radius: var(--kin-radius);
  font-weight: 600;
  text-decoration: none;
  white-space: nowrap;
}

.nav-item:hover {
  color: var(--kin-color-text);
  text-decoration: none;
}

.nav-item.active {
  color: var(--kin-color-accent-text);
  background: var(--kin-color-surface-raised);
}

.global-search {
  position: relative;
  display: flex;
  flex: 0 1 240px;
  align-items: center;
  min-width: 0;
  color: var(--kin-color-text-muted);
}

.global-search .kin-icon {
  position: absolute;
  left: 10px;
  pointer-events: none;
}

.global-search .kin-input {
  width: 100%;
  min-width: 0;
  padding-left: 32px;
  background: var(--kin-color-bg);
  border-color: transparent;
  cursor: not-allowed;
}

.account-avatar {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  color: var(--kin-color-accent-contrast);
  background: var(--kin-color-accent);
  border-radius: 50%;
  font-size: 0.6875rem;
  font-weight: 800;
  text-decoration: none;
}

.account-avatar:hover,
.account-avatar.active {
  text-decoration: none;
  box-shadow: 0 0 0 2px var(--kin-color-bar), 0 0 0 4px var(--kin-color-accent);
}

@media (max-width: 640px) {
  .app-bar {
    flex-wrap: wrap;
    column-gap: var(--kin-space-2);
    padding: var(--kin-space-1) var(--kin-space-3);
  }

  .global-search {
    flex: 1 1 6rem;
  }

  .nav {
    order: 3;
    flex-basis: 100%;
  }
}
</style>
