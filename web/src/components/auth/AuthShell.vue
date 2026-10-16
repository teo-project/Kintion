<script setup lang="ts">
import { ref } from 'vue'

import KinIcon from '@/components/KinIcon.vue'
import KinLogo from '@/components/KinLogo.vue'
import ThemeSwitcher from '@/components/ThemeSwitcher.vue'
import { t } from '@/i18n'
import { PROJECT_REPO_URL } from '@/ui/project'

import TreeBackdrop from './TreeBackdrop.vue'

defineProps<{ title: string; hint?: string }>()

const news = ref<HTMLElement | null>(null)

/** На телефоне блок новостей — второй экран: стрелка внизу первого прокручивает к нему. */
function showNews() {
  const calm = globalThis.matchMedia?.('(prefers-reduced-motion: reduce)').matches
  news.value?.scrollIntoView({ behavior: calm ? 'auto' : 'smooth' })
}
</script>

<template>
  <div class="shell kin-nocturne">
    <section
      class="form-side"
      :aria-label="title"
    >
      <main class="island">
        <h1>{{ title }}</h1>
        <p
          v-if="hint"
          class="kin-muted hint"
        >
          {{ hint }}
        </p>
        <slot />
        <div class="theme">
          <ThemeSwitcher />
        </div>
      </main>
      <button
        class="more"
        type="button"
        :aria-label="t.auth.newsTitle"
        :title="t.auth.newsTitle"
        @click="showNews"
      >
        <svg
          viewBox="0 0 24 24"
          aria-hidden="true"
        >
          <path d="M6 9l6 6 6-6" />
        </svg>
      </button>
    </section>

    <div class="brand-bg">
      <TreeBackdrop />
    </div>

    <section
      ref="news"
      class="news"
      aria-labelledby="auth-news-title"
    >
      <div class="news-card">
        <div class="logo">
          <KinLogo
            :size="96"
            dark
            decorative
          />
          {{ t.appName }}
        </div>
        <div>
          <p class="tagline">
            {{ t.tagline }}
          </p>
          <p class="lead">
            {{ t.auth.newsLead }}
          </p>
        </div>
        <hr>
        <h2 id="auth-news-title">
          {{ t.auth.newsTitle }}
        </h2>
        <ul>
          <li>
            <KinIcon name="tree" />
            {{ t.auth.newsTree }}
          </li>
          <li>
            <svg
              viewBox="0 0 24 24"
              aria-hidden="true"
            >
              <path d="M3 12a9 9 0 109-9 9 9 0 00-6.4 2.6L3 8M3 3v5h5M12 7v5l3 2" />
            </svg>
            {{ t.auth.newsHistory }}
          </li>
          <li>
            <svg
              viewBox="0 0 24 24"
              aria-hidden="true"
            >
              <rect
                x="3"
                y="5"
                width="18"
                height="16"
                rx="3"
              />
              <path d="M3 10h18M8 3v4M16 3v4" />
            </svg>
            {{ t.auth.newsDates }}
          </li>
          <li>
            <svg
              viewBox="0 0 24 24"
              aria-hidden="true"
            >
              <path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01" />
            </svg>
            {{ t.auth.newsAudit }}
          </li>
          <li>
            <KinIcon name="user" />
            {{ t.auth.newsGroups }}
          </li>
          <li>
            <KinIcon name="settings" />
            {{ t.auth.newsPacks }}
          </li>
        </ul>
        <a
          class="repo"
          :href="PROJECT_REPO_URL"
          target="_blank"
          rel="noopener"
          :aria-label="t.auth.sourceCode"
          :title="t.auth.sourceCode"
        ><KinIcon name="github" /></a>
      </div>
    </section>
  </div>
</template>

<style scoped>
/*
 * Окно входа (F-90): слева — анимация и полупрозрачный блок новостей, справа —
 * форма. На узком экране: вход на первом экране с анимацией снизу, блок новостей —
 * на втором. Цвета — только токены (--kin-brand-*, --kin-tree-*, обычные).
 */
.shell {
  display: grid;
  grid-template-columns: 1fr;
  grid-template-rows: minmax(100dvh, auto) minmax(100dvh, auto);
  min-height: 100dvh;
  color: var(--kin-color-text);
  background: var(--kin-brand-bg);
  font-family: var(--kin-font-body);
}

.form-side {
  position: relative;
  z-index: 2;
  display: flex;
  grid-column: 1;
  grid-row: 1;
  align-items: flex-start;
  justify-content: center;
  padding: 4.5rem var(--kin-space-3) var(--kin-space-3);
  box-sizing: border-box;
}

.theme {
  display: flex;
  justify-content: flex-end;
  margin-top: var(--kin-space-3);
  font-size: var(--kin-font-tiny);
}

.island {
  box-sizing: border-box;
  width: 100%;
  max-width: 22.5rem;
  padding: 1.75rem;
  background: var(--kin-color-surface);
  border: 1px solid var(--kin-color-border);
  border-radius: var(--kin-radius-island);
  box-shadow: var(--kin-shadow-island);
}

h1 {
  margin: 0;
  font-size: 1.75rem;
  font-weight: 800;
  letter-spacing: -0.02em;
}

.hint {
  margin: var(--kin-space-1) 0 0;
  font-size: var(--kin-font-small);
}

.more {
  position: absolute;
  bottom: var(--kin-space-3);
  left: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 2.75rem;
  height: 2.75rem;
  background: var(--kin-brand-panel);
  border: 1px solid var(--kin-brand-panel-border);
  border-radius: 50%;
  cursor: pointer;
  transform: translateX(-50%);
}

.more svg {
  width: 1.25rem;
  height: 1.25rem;
  fill: none;
  stroke: var(--kin-brand-text);
  stroke-width: 2.2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.brand-bg {
  grid-column: 1;
  grid-row: 1;
  align-self: end;
  height: 440px;
  min-height: 0;
  pointer-events: none;
}

.news {
  display: flex;
  grid-column: 1;
  grid-row: 2;
  align-items: center;
  justify-content: center;
  padding: var(--kin-space-3);
  box-sizing: border-box;
  background:
    radial-gradient(70% 40% at 15% 10%, var(--kin-brand-glow-1), transparent 70%),
    radial-gradient(70% 40% at 85% 90%, var(--kin-brand-glow-2), transparent 70%);
}

.news-card {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  box-sizing: border-box;
  width: 100%;
  max-width: 30rem;
  padding: 1.5rem;
  color: var(--kin-brand-text);
  background: var(--kin-brand-panel);
  border: 1px solid var(--kin-brand-panel-border);
  border-radius: var(--kin-radius-island);
  backdrop-filter: blur(6px);
}

.logo {
  display: flex;
  gap: var(--kin-space-3);
  align-items: center;
  font-size: 2rem;
  font-weight: 800;
  letter-spacing: -0.02em;
}

/* знак — главный элемент панели: здесь его единственное место «рассмотреть» */
.logo svg {
  flex: none;
  width: auto;
  height: clamp(64px, 12vw, 96px);
}

.tagline {
  margin: 0;
  font-size: 1.6rem;
  font-weight: 800;
  letter-spacing: -0.02em;
  line-height: 1.2;
}

.lead {
  margin: var(--kin-space-2) 0 0;
  color: var(--kin-brand-text-muted);
  font-size: var(--kin-font-small);
  line-height: 1.5;
}

hr {
  width: 100%;
  height: 1px;
  margin: 0;
  background: var(--kin-brand-panel-border);
  border: 0;
}

h2 {
  margin: 0;
  color: var(--kin-brand-text-faint);
  font-size: var(--kin-font-tiny);
  font-weight: 600;
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

ul {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  margin: 0;
  padding: 0;
  list-style: none;
}

li {
  display: flex;
  gap: 0.75rem;
  align-items: flex-start;
  font-size: var(--kin-font-small);
  line-height: 1.45;
}

li svg {
  flex: none;
  width: 18px;
  height: 18px;
  margin-top: 2px;
  fill: none;
  stroke: var(--kin-brand-accent);
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

@media (min-width: 800px) {
  .shell {
    grid-template-columns: 11fr 9fr;
    grid-template-rows: minmax(100dvh, auto);
    background: var(--kin-color-bg);
  }

  .form-side {
    grid-column: 2;
    grid-row: 1;
    align-items: center;
    padding: var(--kin-space-4);
    background: var(--kin-color-bg);
  }

  .more {
    display: none;
  }

  .brand-bg {
    grid-column: 1;
    grid-row: 1;
    align-self: stretch;
    height: auto;
    background:
      radial-gradient(60% 45% at 18% 12%, var(--kin-brand-glow-1), transparent 70%),
      radial-gradient(50% 40% at 85% 92%, var(--kin-brand-glow-2), transparent 70%),
      var(--kin-brand-bg);
  }

  .news {
    z-index: 1;
    grid-column: 1;
    grid-row: 1;
    background: none;
  }
}

.repo {
  display: inline-flex;
  align-self: flex-start;
  color: var(--kin-brand-text-faint);
}

.repo svg {
  width: 18px;
  height: 18px;
}

.repo:hover,
.repo:focus-visible {
  color: var(--kin-brand-text-muted);
}
</style>
