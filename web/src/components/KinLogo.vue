<script setup lang="ts">
// Знак Kintion (docs/assets/logo-mark*.svg) инлайном: цвета — переменные
// --kl-*, под тему приложения. `dark` — всегда тёмный вариант (для вечно тёмных
// поверхностей, например панели входа .kin-nocturne).
import { t } from '@/i18n'

// decorative — рядом уже есть текст «Kintion»: чтец не дублирует название.
defineProps<{ size?: number; dark?: boolean; decorative?: boolean }>()
</script>

<template>
  <svg
    class="kin-logo"
    :class="{ 'kin-logo--dark': dark }"
    :height="size ?? 28"
    viewBox="63 38 286 344"
    :role="decorative ? undefined : 'img'"
    :aria-label="decorative ? undefined : t.appName"
    :aria-hidden="decorative ? 'true' : undefined"
  >
    <g
      class="kl-line"
      stroke-width="9"
    >
      <line
        x1="127"
        y1="210"
        x2="127"
        y2="90"
      />
      <line
        x1="127"
        y1="210"
        x2="127"
        y2="330"
      />
      <line
        x1="127"
        y1="210"
        x2="307"
        y2="90"
      />
      <line
        x1="127"
        y1="210"
        x2="307"
        y2="330"
      />
    </g>
    <g
      v-for="[x, y] in [[127, 76], [127, 316], [307, 76], [307, 316]]"
      :key="`${x}-${y}`"
      :transform="`translate(${x - 127} ${y - 76})`"
    >
      <rect
        class="kl-card"
        x="98.05"
        y="51.05"
        width="57.9"
        height="77.9"
        rx="8.4"
        stroke-width="2.1"
      />
      <circle
        class="kl-sm"
        cx="127"
        cy="76"
        r="15"
      />
      <path
        class="kl-sm"
        d="M105.4 104.5A21.6 15 0 0 1 148.6 104.5Z"
      />
      <rect
        class="kl-sm-a"
        x="109"
        y="111.2"
        width="36"
        height="4.2"
        rx="3"
      />
      <rect
        class="kl-sm-b"
        x="116.2"
        y="119.6"
        width="21.6"
        height="3.5"
        rx="2.5"
      />
    </g>
    <rect
      class="kl-card kl-main"
      x="83.58"
      y="151.57"
      width="86.85"
      height="116.85"
      rx="12.6"
      stroke-width="3.15"
    />
    <circle
      class="kl-big"
      cx="127"
      cy="189"
      r="22.5"
    />
    <path
      class="kl-big"
      d="M94.6 231.75A32.4 22.5 0 0 1 159.4 231.75Z"
    />
    <rect
      class="kl-bar"
      x="100"
      y="241.8"
      width="54"
      height="6.3"
      rx="3"
    />
    <rect
      class="kl-sm-b"
      x="110.8"
      y="254.4"
      width="32.4"
      height="5.25"
      rx="2.5"
    />
  </svg>
</template>

<style scoped>
.kin-logo {
  --kl-line: #9aa9ea;
  --kl-card: #eef1ff;
  --kl-card-edge: #aab6ee;
  --kl-sm: #6b7fe0;
  --kl-sm-a: #98a3d9;
  --kl-sm-b: #c3caf0;
  --kl-big: #2f49b8;
  --kl-bar: #1c2150;

  flex: none;
  width: auto;
}

.kin-logo--dark {
  --kl-line: #5a68be;
  --kl-card: #181d40;
  --kl-card-edge: #505eb0;
  --kl-sm: #7888e6;
  --kl-sm-a: #969ecd;
  --kl-sm-b: #5f69a0;
  --kl-big: #a9b6ff;
  --kl-bar: #e8eaf6;
}

@media (prefers-color-scheme: dark) {
  :root:not([data-theme='light']) .kin-logo {
    --kl-line: #5a68be;
    --kl-card: #181d40;
    --kl-card-edge: #505eb0;
    --kl-sm: #7888e6;
    --kl-sm-a: #969ecd;
    --kl-sm-b: #5f69a0;
    --kl-big: #a9b6ff;
    --kl-bar: #e8eaf6;
  }
}

:root[data-theme='dark'] .kin-logo {
  --kl-line: #5a68be;
  --kl-card: #181d40;
  --kl-card-edge: #505eb0;
  --kl-sm: #7888e6;
  --kl-sm-a: #969ecd;
  --kl-sm-b: #5f69a0;
  --kl-big: #a9b6ff;
  --kl-bar: #e8eaf6;
}

.kl-line {
  stroke: var(--kl-line);
}

.kl-card {
  fill: var(--kl-card);
  stroke: var(--kl-card-edge);
}

.kl-main {
  stroke: var(--kl-big);
}

.kl-sm {
  fill: var(--kl-sm);
}

.kl-sm-a {
  fill: var(--kl-sm-a);
}

.kl-sm-b {
  fill: var(--kl-sm-b);
}

.kl-big {
  fill: var(--kl-big);
}

.kl-bar {
  fill: var(--kl-bar);
}
</style>
