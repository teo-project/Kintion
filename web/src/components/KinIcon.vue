<script lang="ts">
// Иконки интерфейса — Lucide (F-91): штрих 2, скруглённые концы, сетка 24;
// цвет — currentColor (токены задаёт кнопка), размер — класс kin-icon.
// Подпись — у кнопки (aria-label, title); сама иконка скрыта от чтеца.
import {
  ArrowDown,
  ArrowLeft,
  ArrowRight,
  ArrowUp,
  Check,
  ChevronDown,
  Crop,
  Github,
  History,
  Link,
  Lock,
  LogOut,
  Monitor,
  Network,
  Moon,
  PenLine,
  Plus,
  Search,
  SlidersVertical,
  Star,
  Sun,
  Trash2,
  Undo2,
  Upload,
  User,
  X,
} from 'lucide-vue-next'

const ICONS = {
  back: ArrowLeft,
  forward: ArrowRight,
  edit: PenLine,
  history: History,
  plus: Plus,
  check: Check,
  close: X,
  search: Search,
  moon: Moon,
  sun: Sun,
  monitor: Monitor,
  trash: Trash2,
  upload: Upload,
  restore: Undo2,
  settings: SlidersVertical,
  link: Link,
  up: ArrowUp,
  down: ArrowDown,
  star: Star,
  logout: LogOut,
  user: User,
  more: ChevronDown,
  crop: Crop,
  tree: Network,
  lock: Lock,
  github: Github,
} as const

export type IconName = keyof typeof ICONS
</script>

<script setup lang="ts">
defineProps<{ name: IconName }>()
</script>

<template>
  <component
    :is="ICONS[name]"
    class="kin-icon"
    aria-hidden="true"
    focusable="false"
  />
</template>
