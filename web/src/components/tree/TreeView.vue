<script setup lang="ts">
// Ракурс «Дерево» (F-110…F-119): раскладка core/tree, полотно — Vue Flow
// (F-24). Узел — портрет, имя, даты, примечание (F-116); клик — карточка по
// центру поверх дерева (F-113); (+) — быстрое добавление (F-73); отсылка —
// линия к основному узлу при наведении (F-117); настройки ракурса — у
// аккаунта на сервере (F-119).
import '@vue-flow/core/dist/style.css'
import '@vue-flow/minimap/dist/style.css'

import { BaseEdge, EdgeLabelRenderer, getBezierPath, Handle, Position, VueFlow, useVueFlow, type Edge, type EdgeProps, type Node, type NodeMouseEvent } from '@vue-flow/core'
import { MiniMap } from '@vue-flow/minimap'
import { DialogClose, DialogContent, DialogOverlay, DialogPortal, DialogRoot, DialogTitle } from 'reka-ui'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'

import { api } from '@/api/client'
import type { TreeOut } from '@/api/types'
import KinCheckbox from '@/components/KinCheckbox.vue'
import KinIcon from '@/components/KinIcon.vue'
import RecordAvatar from '@/components/RecordAvatar.vue'
import { photoOf } from '@/core/card'
import { roleLabel } from '@/core/forms'
import { recordTitle, recordTitleById, tplOf } from '@/core/resolver'
import type { Registry } from '@/core/registry'
import { personView, treeInput } from '@/core/tree/graph'
import { BADGE_D, refBadges, type RefBadge } from '@/core/tree/badges'
import { edgeOpen } from '@/core/tree/edgeOpen'
import { upLines } from '@/core/tree/highlight'
import { personsInBox, toggle } from '@/core/tree/select'
import { labelLines, layoutTree, PERSON_H, PERSON_W, PHOTO_MID, ROW } from '@/core/tree/layout'
import { errorText, fmt, t } from '@/i18n'
import { makeCtx } from '@/ui/ctx'
import { setTreePrefs, treePrefs } from '@/ui/prefs'
import RecordView from '@/views/RecordView.vue'

import TreeQuickAdd from './TreeQuickAdd.vue'
import TreeSelection from './TreeSelection.vue'

/** focus — человек в центре при открытии (F-129, иконка дерева в «Связях») */
const props = defineProps<{ d: string; reg: Registry; group: string; focus?: string }>()
/** groups — создана группа (список групп фильтра перечитать) */
const emit = defineEmits<{ groups: [] }>()

const DEPTHS = [3, 5, 7, 10, 99]
const depth = ref(10)
const ribbon = ref(true)
const hideDead = ref(false)
const prefsReady = ref(false)
onMounted(async () => {
  const p = await treePrefs()
  ribbon.value = p.ribbon
  hideDead.value = p.hideDead
  prefsReady.value = true
})
watch([ribbon, hideDead], ([r, h]) => {
  if (prefsReady.value) void setTreePrefs({ ribbon: r, hideDead: h }).catch(() => undefined)
})

const tree = shallowRef<TreeOut | null>(null)
const error = ref('')
/** сообщение без ошибки: созданное не видно в дереве (F-129) */
const notice = ref('')
let seq = 0
let loads = 0
/** удачных загрузок дерева */
let treeLoads = 0
/** keep — сохранить вид (закрытие карточки, F-120); иначе новые данные вписываются */
async function load(keep = false): Promise<boolean> {
  const my = ++seq
  try {
    const out = await api.tree(props.d, props.group || undefined)
    if (my === seq) {
      if (!keep) loads++
      treeLoads++
      tree.value = out
      error.value = ''
      return true
    }
  } catch (e) {
    if (my === seq) error.value = errorText(e)
  }
  return false
}
// другие данные или фильтры — прежнее «не видно» уже не о них
watch(() => [props.d, props.group, depth.value, hideDead.value], () => (notice.value = ''))
watch(() => [props.d, props.group], () => void load(), { immediate: true })

const ctx = computed(() => (tree.value ? makeCtx(props.reg, [...tree.value.relations, ...tree.value.records]) : null))
const input = computed(() => (ctx.value && tree.value ? treeInput(ctx.value, tree.value.relations, tree.value.records) : null))
const fontsVersion = ref(0)
/** ширина подписи на линии — по шрифту страницы (F-129); без canvas — оценка раскладки */
const measure = (() => {
  const c = typeof document !== 'undefined' ? document.createElement('canvas').getContext?.('2d') : null
  if (!c) return undefined
  const cache = new Map<string, number>()
  // шрифт страницы догрузился (font-display: swap) — замеры заново; слушатель снимается с деревом
  const onFonts = () => {
    cache.clear()
    fontsVersion.value++
  }
  document.fonts?.addEventListener?.('loadingdone', onFonts)
  onBeforeUnmount(() => document.fonts?.removeEventListener?.('loadingdone', onFonts))
  return (text: string) => {
    let w = cache.get(text)
    if (w === undefined) {
      c.font = `10px ${getComputedStyle(document.body).fontFamily}`
      w = c.measureText(text).width
      cache.set(text, w)
    }
    return w
  }
})()
const layout = computed(() => {
  void fontsVersion.value
  return input.value ? layoutTree(input.value, { depth: depth.value, hideDead: hideDead.value, measure }) : null
})

const flowId = `tree-${Math.random().toString(36).slice(2)}`
const { fitView, setCenter, getViewport, findNode, screenToFlowCoordinate } = useVueFlow(flowId)

const nodes = computed<Node[]>(() => {
  const c = ctx.value
  if (!layout.value || !c) return []
  return layout.value.nodes.map(n => {
    const rec = n.person ? c.records[n.person] : undefined
    const view = rec ? personView(c, rec) : null
    return {
      id: n.id,
      type: n.type,
      position: { x: n.x, y: n.y },
      data: { ...n, view, photo: rec ? photoOf(c, rec) : null },
    }
  })
})

/** значки отсылок на основных карточках (F-117); размеры узлов — настоящие, после отрисовки */
const badges = computed(() =>
  layout.value
    ? refBadges(layout.value, id => {
        const d = findNode(id)?.dimensions
        return d && d.height ? { w: d.width, h: d.height } : undefined
      })
    : new Map<string, RefBadge[]>(),
)
const hover = ref<string | null>(null)
const highlight = ref<string | null>(null)
const line = { stroke: 'var(--kin-color-text-muted)', strokeWidth: 1.5, opacity: 0.6 }
const dashed = { ...line, strokeDasharray: '5 4' }
const lit = { stroke: 'var(--kin-color-accent)', strokeWidth: 2.5, opacity: 1 }
/** линия под указателем (на тач-устройствах — по первому касанию) и линии вверх от неё (F-127) */
const hoverEdge = ref<string | null>(null)
/** касание линии — держится до касания полотна (уход «указателя» после касания её не снимает); мышью — только наведение */
const tapEdge = ref<string | null>(null)
let lastPointer = ''
const notePointer = (ev: PointerEvent) => (lastPointer = ev.pointerType)
function onEdgeTap(id: string) {
  if (lastPointer === 'touch' || lastPointer === 'pen') tapEdge.value = id
}
const litEdges = computed(() => {
  const e = hoverEdge.value ?? tapEdge.value
  return layout.value && e ? upLines(layout.value, e) : new Set<string>()
})
const edges = computed<Edge[]>(() => {
  if (!layout.value) return []
  const out: Edge[] = layout.value.edges.map(e => ({
    id: e.id,
    type: e.type,
    source: e.source,
    target: e.target,
    sourceHandle: e.sourceHandle,
    targetHandle: e.targetHandle,
    label: e.label,
    zIndex: litEdges.value.has(e.id) ? 1 : 0,
    style: litEdges.value.has(e.id) ? (e.dashed ? { ...lit, strokeDasharray: '5 4' } : lit) : e.dashed ? dashed : line,
    data: { busY: e.busY, via: e.via },
  }))
  // линия «телепортации» отсылка → значок на основном узле (F-117)
  const h = hover.value
  if (h) {
    const hp = layout.value.nodes.find(n => n.id === h)?.person
    for (const n of layout.value.nodes) {
      if (n.type !== 'ref' || (n.id !== h && n.person !== hp) || !n.person) continue
      const m = layout.value.nodes.find(x => x.type === 'person' && x.person === n.person)
      const b = m && badges.value.get(m.id)?.find(x => x.id === n.id)
      if (!m || !b) continue
      out.push({
        id: `tele:${n.id}`, source: n.id, sourceHandle: 'c', target: m.id, targetHandle: 't',
        type: 'tele', animated: true, style: { stroke: 'var(--kin-color-accent)', strokeWidth: 2 },
        data: { x: m.x + b.x, y: m.y + b.y, side: b.side },
      })
    }
  }
  return out
})

/** узел человека на полотне: сам или (если его нет) первая отсылка */
const personNode = (id: string) =>
  layout.value?.nodes.find(n => n.type === 'person' && n.person === id) ?? layout.value?.nodes.find(n => n.type === 'ref' && n.person === id)

/** масштаб «±1,5 поколения»: три ряда во всю высоту, узел ≈ 20% (F-129) */
function focusZoom(): number {
  const h = wrap.value?.clientHeight || window.innerHeight
  return Math.min(2, Math.max(0.05, h / (3 * ROW)))
}

/** кого центрировать после пересчёта: человек из «Связей» или созданный из дерева (F-129); zoom 0 — «±1,5 поколения» */
/** after — номер загрузки дерева, после которой решать (созданное из дерева: до перечитывания его ещё нет) */
let centerNext: { id: string; zoom: number; after?: number } | null = null

/** центрировать отложенное, если узел уже есть */
function centerPending(): boolean {
  const c = centerNext
  const n = c ? personNode(c.id) : undefined
  if (!c || !n) return false
  centerNext = null
  fitPending = false
  setTimeout(() => centerOn(n.id, 400, c.zoom || focusZoom()), 150)
  return true
}

// пересчёт раскладки (глубина, фильтры) — вписать полотно
let fitted = ''
let fitPending = false
watch(layout, l => {
  // tree.value — новые данные (быстрое добавление, правка в карточке) тоже вписываются
  const key = `${props.d}|${props.group}|${depth.value}|${hideDead.value}|${loads}`
  if (!l) return
  if (centerNext && (centerNext.after ?? 0) <= treeLoads && !personNode(centerNext.id)) {
    const want = centerNext.id
    const wanted = input.value?.persons.find(p => p.id === want)
    // отсечён глубиной — показать все поколения (глубина — только на странице, F-120);
    // скрытый умерший («Скрыть умерших») глубиной не вернуть
    if (wanted && depth.value !== 99 && !(hideDead.value && wanted.dead)) {
      depth.value = 99
      return
    }
    // созданного не видно с текущими фильтрами (группа, умершие) — сказать; фокус из «Связей»
    // ждёт, пока фильтр не покажет человека
    if (centerNext.after) {
      notice.value = t.tree.notShown
      centerNext = null
    }
  }
  if (key === fitted) {
    centerPending()
    return
  }
  fitted = key
  fitPending = true
  if (centerPending()) return
  // узлы уже измерены (глубина, «Скрыть умерших») — событие не придёт, вписать после отрисовки
  setTimeout(() => {
    if (fitPending) void fitView({ padding: 0.1 })
  }, 150)
})

/** меню (+) закрыто; создан человек — вид остаётся и центрируется на нём (F-129) */
function onAdded(id?: string) {
  menu.value = null
  notice.value = ''
  // решение — по первому удачно перечитанному дереву (в watch(layout))
  if (id) centerNext = { id, zoom: Math.max(getViewport().zoom, focusZoom()), after: treeLoads + 1 }
  void load(true)
}
/** новые узлы измерены — вписать (первый показ, смена данных) */
function onNodesInitialized() {
  if (!fitPending) return
  fitPending = false
  void fitView({ padding: 0.1 })
}

const SIDE = { top: Position.Top, right: Position.Right, bottom: Position.Bottom, left: Position.Left }
/** линия «телепортации»: от отсылки к её значку на основной карточке (F-117) */
function telePath(p: EdgeProps): string {
  const d = p.data as { x: number; y: number; side: RefBadge['side'] }
  return getBezierPath({ sourceX: p.sourceX, sourceY: p.sourceY, sourcePosition: Position.Top, targetX: d.x, targetY: d.y, targetPosition: SIDE[d.side] })[0]
}

function busPath(p: EdgeProps): string {
  const { busY, via } = p.data as { busY: number; via?: { x: number; y: number } }
  const pre = via ? `V ${via.y} H ${via.x} ` : ''
  return `M ${p.sourceX} ${p.sourceY} ${pre}V ${busY} H ${p.targetX} V ${p.targetY}`
}

/** открытая карточка (F-113) */
const card = ref<string | null>(null)
/** клик по полосе прокрутки оверлея не закрывает карточку */
function onCardOutside(e: CustomEvent<{ originalEvent: PointerEvent }>) {
  const o = e.detail.originalEvent
  const el = o.target as HTMLElement
  if (el.classList?.contains('tree-overlay') && o.offsetX >= el.clientWidth) e.preventDefault()
}

const cardOpen = computed({
  get: () => card.value !== null,
  set: v => {
    if (!v) {
      card.value = null
      void load(true)
    }
  },
})

function centerOn(id: string, duration = 600, zoom = Math.max(getViewport().zoom, 0.9)) {
  const n = layout.value?.nodes.find(x => x.id === id)
  if (!n) return
  const h = findNode(id)?.dimensions.height || (n.type === 'person' || n.type === 'ref' ? PERSON_H : (n.h ?? 22))
  void setCenter(n.x + n.w / 2, n.y + h / 2, { zoom, duration })
}

function onNodeClick(e: NodeMouseEvent) {
  menu.value = null
  const n = e.node.data as { type: string; person?: string; relation?: string }
  // Ctrl + клик — выделение (F-131)
  const ev = e.event as MouseEvent
  if (ev.ctrlKey || ev.metaKey) {
    if (n.person && (n.type === 'person' || n.type === 'ref')) selected.value = toggle(selected.value, n.person)
    return
  }
  if (n.type === 'ref' && n.person) {
    highlight.value = n.person
    centerOn(n.person)
    return
  }
  const id = n.type === 'person' ? n.person : n.relation
  if (!id) return
  highlight.value = e.node.id
  centerOn(e.node.id)
  card.value = id
}

function badgeTitle(b: RefBadge): string {
  const c = ctx.value
  const rec = c && b.near ? c.records[b.near] : undefined
  if (!c || !rec) return t.tree.refBadgeAlone
  const name = recordTitle(c, rec)
  return fmt(t.tree.refBadge, { near: b.nearLabel ? `${name} (${b.nearLabel})` : name })
}
/** клик по значку — перелёт к отсылке */
function onBadge(refId: string) {
  menu.value = null
  edgeMenu.value = null
  highlight.value = refId
  centerOn(refId)
}

/** двойной клик по линии: отношение или меню отношений у курсора (F-127) */
const edgeMenuEl = ref<HTMLElement | null>(null)
const edgeMenu = ref<{ x: number; y: number; items: { id: string; label: string }[] } | null>(null)
/** второй участник отношения из двух ролей («ребёнок» в «Мама — ребёнок») */
function relSecond(id: string): string | undefined {
  const c = ctx.value
  const rel = c?.records[id]
  const roles = c && rel ? (tplOf(c, rel)?.roles ?? []) : []
  return roles.length === 2 ? rel?.participants.find(p => p.role === roles[1]!.key)?.entity_id : undefined
}
/** подпись пункта меню; withSecond — и второй участник (меню общего отрезка к разным детям) */
function relLabel(id: string, withSecond = false): string {
  const c = ctx.value
  const rel = c?.records[id]
  if (!c || !rel) return id
  const roles = tplOf(c, rel)?.roles ?? []
  const first = roles.length === 2 ? rel.participants.find(p => p.role === roles[0]!.key) : undefined
  if (!first) return recordTitle(c, rel)
  const second = withSecond ? relSecond(id) : undefined
  return `${roleLabel(c, rel, roles[0]!.key, first.entity_id)} — ${recordTitleById(c, first.entity_id)}${second ? ` → ${recordTitleById(c, second)}` : ''}`
}
function onEdgeDouble(e: { edge: { id: string }; event: MouseEvent | TouchEvent }) {
  menu.value = null
  edgeMenu.value = null
  if (!layout.value) return
  const p = 'clientX' in e.event ? e.event : e.event.changedTouches[0]
  // точка клика на полотне: общий отрезок нескольких линий несёт отношения их всех
  const at = p ? screenToFlowCoordinate({ x: p.clientX, y: p.clientY }) : undefined
  const r = edgeOpen(layout.value, e.edge.id, at, id => findNode(id)?.dimensions.height || undefined)
  if (r.kind === 'card') card.value = r.id
  else if (r.kind === 'menu') {
    const box = wrap.value?.getBoundingClientRect()
    if (!box || !p) return
    const many = new Set(r.ids.map(relSecond)).size > 1
    edgeMenu.value = { x: Math.max(0, p.clientX - box.left), y: Math.max(0, p.clientY - box.top), items: r.ids.map(id => ({ id, label: relLabel(id, many) })) }
    void nextTick(fitEdgeMenu)
  }
}
/** после отрисовки сдвинуть меню внутрь полотна по его настоящим размерам */
function fitEdgeMenu() {
  const m = edgeMenuEl.value
  const box = wrap.value?.getBoundingClientRect()
  if (!m || !box || !edgeMenu.value) return
  edgeMenu.value = {
    ...edgeMenu.value,
    x: Math.max(0, Math.min(edgeMenu.value.x, box.width - m.offsetWidth)),
    y: Math.max(0, Math.min(edgeMenu.value.y, box.height - m.offsetHeight)),
  }
}
function openFromEdgeMenu(id: string) {
  edgeMenu.value = null
  card.value = id
}

/** меню (+) узла */
const menu = ref<{ id: string; x: number; y: number } | null>(null)
const wrap = ref<HTMLElement | null>(null)
function openMenu(ev: MouseEvent, person: string) {
  const box = wrap.value?.getBoundingClientRect()
  if (!box) return
  menu.value = { id: person, x: ev.clientX - box.left, y: ev.clientY - box.top }
}

// миникарта: перемещение без анимации; соответствие координат — с начала
// перетаскивания, чтобы смена рамки миникарты не давала рывков
let drag: DOMMatrix | null = null
function miniCenter(ev: PointerEvent) {
  if (!drag) return
  const p = new DOMPoint(ev.clientX, ev.clientY).matrixTransform(drag)
  void setCenter(p.x, p.y, { zoom: getViewport().zoom, duration: 0 })
}
function miniDown(ev: PointerEvent) {
  const svg = (ev.target as Element | null)?.closest('.vue-flow__minimap')?.querySelector('svg')
  const m = svg?.getScreenCTM()
  if (!svg || !m) return
  ev.preventDefault()
  ev.stopPropagation()
  drag = m.inverse()
  miniCenter(ev)
}
const miniUp = () => (drag = null)

// ---------- выделение (F-131): Ctrl + рамка, Ctrl + клик ----------

const selected = ref<Set<string>>(new Set())
const selectedIds = computed(() => [...selected.value])
/** рамка в координатах обёртки */
const box = ref<{ x0: number; y0: number; x1: number; y1: number } | null>(null)
const boxStyle = computed(() => {
  const b = box.value
  if (!b) return {}
  return { left: `${Math.min(b.x0, b.x1)}px`, top: `${Math.min(b.y0, b.y1)}px`, width: `${Math.abs(b.x1 - b.x0)}px`, height: `${Math.abs(b.y1 - b.y0)}px` }
})
function local(ev: PointerEvent) {
  const r = wrap.value!.getBoundingClientRect()
  return { x: ev.clientX - r.left, y: ev.clientY - r.top }
}
function boxDown(ev: PointerEvent) {
  if (!(ev.ctrlKey || ev.metaKey) || ev.button !== 0) return
  const target = ev.target as Element | null
  if (!target?.closest('.vue-flow__pane') || target.closest('.vue-flow__node, .vue-flow__minimap')) return
  ev.preventDefault()
  ev.stopPropagation()
  const p = local(ev)
  box.value = { x0: p.x, y0: p.y, x1: p.x, y1: p.y }
}
function boxMove(ev: PointerEvent) {
  if (!box.value) return
  const p = local(ev)
  box.value = { ...box.value, x1: p.x, y1: p.y }
}
function boxUp() {
  const b = box.value
  box.value = null
  if (!b || !layout.value || (Math.abs(b.x1 - b.x0) < 4 && Math.abs(b.y1 - b.y0) < 4)) return
  const vp = getViewport()
  const flow = (x: number, y: number) => ({ x: (x - vp.x) / vp.zoom, y: (y - vp.y) / vp.zoom })
  const a = flow(b.x0, b.y0)
  const c = flow(b.x1, b.y1)
  // высота узла — отрисованная (ФИО переносится, F-123)
  const add = personsInBox(layout.value, { x0: a.x, y0: a.y, x1: c.x, y1: c.y }, id => findNode(id)?.dimensions.height || PERSON_H)
  selected.value = new Set([...selected.value, ...add])
}
// другой дашборд или фильтр — выделение снимается; ушедшие из дерева — из выделения
watch(() => [props.d, props.group], () => (selected.value = new Set()))
watch(layout, l => {
  if (!l || !selected.value.size) return
  const shown = new Set(l.nodes.map(n => n.person).filter(Boolean))
  const keep = [...selected.value].filter(id => shown.has(id))
  if (keep.length !== selected.value.size) selected.value = new Set(keep)
})
/** метки изменились: при фильтре по группе дерево меняется — перечитать, вид сохранить */
function onSelChanged() {
  if (props.group) void load(true)
}

onMounted(() => {
  wrap.value?.addEventListener('pointerdown', miniDown, true)
  wrap.value?.addEventListener('pointerdown', boxDown, true)
  wrap.value?.addEventListener('pointerdown', notePointer, true)
  window.addEventListener('pointermove', miniCenter)
  window.addEventListener('pointerup', miniUp)
  window.addEventListener('pointermove', boxMove)
  window.addEventListener('pointerup', boxUp)
})
onBeforeUnmount(() => {
  wrap.value?.removeEventListener('pointerdown', miniDown, true)
  wrap.value?.removeEventListener('pointerdown', boxDown, true)
  wrap.value?.removeEventListener('pointerdown', notePointer, true)
  window.removeEventListener('pointermove', miniCenter)
  window.removeEventListener('pointerup', miniUp)
  window.removeEventListener('pointermove', boxMove)
  window.removeEventListener('pointerup', boxUp)
})

// человек из «Связей» (F-129): карточка и меню закрываются, вид — на нём
watch(() => props.focus, f => {
  if (!f) return
  card.value = null
  menu.value = null
  centerNext = { id: f, zoom: 0 }
  if (layout.value) centerPending()
}, { immediate: true })

const letter = (s: string) => s.trim().charAt(0).toUpperCase() || '·'
</script>

<template>
  <div class="tree-view">
    <div class="tree-bar kin-island">
      <slot />
      <label class="kin-row">
        {{ t.tree.depth }}
        <select
          v-model.number="depth"
          class="kin-input depth"
        >
          <option
            v-for="x in DEPTHS"
            :key="x"
            :value="x"
          >{{ x === 99 ? t.tree.depthAll : x }}</option>
        </select>
      </label>
      <KinCheckbox v-model="ribbon">
        {{ t.tree.ribbon }}
      </KinCheckbox>
      <KinCheckbox v-model="hideDead">
        {{ t.tree.hideDead }}
      </KinCheckbox>
      <button
        class="kin-btn kin-btn--small"
        type="button"
        @click="fitView({ padding: 0.1, duration: 400 })"
      >
        {{ t.tree.fit }}
      </button>
    </div>
    <p
      v-if="error"
      class="kin-notice kin-notice--error"
      role="alert"
    >
      {{ error }}
    </p>
    <p
      v-else-if="notice"
      class="kin-notice"
      role="status"
      @click="notice = ''"
    >
      {{ notice }}
    </p>
    <div
      ref="wrap"
      class="tree-canvas"
      :class="{ 'no-ribbon': !ribbon }"
    >
      <p
        v-if="layout && !layout.nodes.length"
        class="kin-muted tree-empty"
      >
        {{ t.tree.empty }}
      </p>
      <VueFlow
        :id="flowId"
        :nodes="nodes"
        :edges="edges"
        :min-zoom="0.05"
        :max-zoom="2"
        :nodes-draggable="false"
        :nodes-connectable="false"
        :elements-selectable="false"
        @nodes-initialized="onNodesInitialized"
        @node-click="onNodeClick"
        @node-mouse-enter="hover = $event.node.id"
        @node-mouse-leave="hover = null"
        @edge-mouse-enter="hoverEdge = $event.edge.id"
        @edge-mouse-leave="hoverEdge = null"
        @edge-click="onEdgeTap($event.edge.id)"
        @edge-double-click="onEdgeDouble"
        @pane-click="menu = null; edgeMenu = null; highlight = null; tapEdge = null; hoverEdge = null"
        @move-start="menu = null; edgeMenu = null"
      >
        <template #node-person="{ id, data }">
          <div
            class="tn-person"
            :class="{ 'is-hl': highlight === id, 'is-sel': selected.has(data.person) }"
            data-testid="tree-person"
          >
            <Handle
              id="t"
              type="target"
              :position="Position.Top"
            />
            <Handle
              id="l"
              type="target"
              :position="Position.Left"
            />
            <Handle
              id="r"
              type="source"
              :position="Position.Right"
            />
            <Handle
              id="b"
              type="source"
              :position="Position.Bottom"
            />
            <div class="tn-photo">
              <span
                v-if="data.view?.dead"
                class="tn-ribbon"
              />
              <RecordAvatar
                v-if="data.photo"
                :dashboard="d"
                :photo="data.photo"
                :title="data.view?.title ?? ''"
                :size="256"
                place="tree"
                shape="rect"
              />
              <span v-else>{{ letter(data.view?.title ?? '') }}</span>
            </div>
            <div class="tn-name">
              {{ data.view?.title }}
            </div>
            <div
              v-if="data.view?.dates"
              class="tn-dates"
            >
              {{ data.view.dates }}
            </div>
            <div
              v-if="data.view?.note"
              class="tn-note"
              :class="{ 'tn-note-long': data.view.note.length > 48 }"
            >
              {{ data.view.note }}
            </div>
            <button
              class="tn-plus nodrag nopan"
              type="button"
              :aria-label="t.tree.add"
              :title="t.tree.add"
              @click.stop="openMenu($event, data.person)"
            >
              <KinIcon name="plus" />
            </button>
          </div>
          <button
            v-for="b in badges.get(id) ?? []"
            :key="b.id"
            class="tn-badge nodrag nopan"
            type="button"
            data-testid="tree-ref-badge"
            :data-ref="b.id"
            :style="{ left: `${b.x}px`, top: `${b.y}px` }"
            :title="badgeTitle(b)"
            :aria-label="badgeTitle(b)"
            @click.stop="onBadge(b.id)"
          >
            ↪
          </button>
        </template>
        <template #node-ref="{ id, data }">
          <div
            class="tn-person tn-ref"
            :class="{ 'is-hl': highlight === id, 'is-sel': selected.has(data.person) }"
            :title="fmt(t.tree.refTitle, { name: data.view?.title ?? '' })"
          >
            <Handle
              id="t"
              type="target"
              :position="Position.Top"
            />
            <Handle
              id="c"
              type="source"
              :position="Position.Top"
            />
            <Handle
              id="l"
              type="target"
              :position="Position.Left"
            />
            <Handle
              id="r"
              type="source"
              :position="Position.Right"
            />
            <Handle
              id="b"
              type="source"
              :position="Position.Bottom"
            />
            <div class="tn-photo">
              <span
                v-if="data.view?.dead"
                class="tn-ribbon"
              />
              <RecordAvatar
                v-if="data.photo"
                :dashboard="d"
                :photo="data.photo"
                :title="data.view?.title ?? ''"
                :size="256"
                place="tree"
                shape="rect"
              />
              <span v-else>{{ letter(data.view?.title ?? '') }}</span>
            </div>
            <div class="tn-name">
              ↪ {{ data.view?.title }}
            </div>
            <div
              v-if="data.view?.dates"
              class="tn-dates"
            >
              {{ data.view.dates }}
            </div>
          </div>
        </template>
        <template #node-union="{ data }">
          <div
            class="tn-union"
            :style="{ minWidth: `${data.w}px`, minHeight: `${data.h}px` }"
          >
            <Handle
              id="l"
              type="target"
              :position="Position.Left"
            />
            <Handle
              id="r"
              type="source"
              :position="Position.Right"
            />
            <Handle
              id="b"
              type="source"
              :position="Position.Bottom"
            />
            <span
              v-for="(s, i) in labelLines(data.label ?? '')"
              :key="i"
            >{{ s }}</span>
          </div>
        </template>
        <template #node-siblings="{ data }">
          <div
            class="tn-union"
            :style="{ minWidth: `${data.w}px`, minHeight: `${data.h}px` }"
          >
            <Handle
              id="b"
              type="source"
              :position="Position.Bottom"
            />
            <span
              v-for="(s, i) in labelLines(data.label ?? '')"
              :key="i"
            >{{ s }}</span>
          </div>
        </template>
        <template #node-dot="{ data }">
          <div
            class="tn-dot"
            :class="{ 'tn-many': data.many }"
          >
            <Handle
              id="t"
              type="target"
              :position="Position.Top"
            />
            <Handle
              id="l"
              type="target"
              :position="Position.Left"
            />
            <Handle
              id="r"
              type="source"
              :position="Position.Right"
            />
            <Handle
              id="b"
              type="source"
              :position="Position.Bottom"
            />
          </div>
        </template>
        <template #edge-tele="p">
          <BaseEdge
            :id="p.id"
            :path="telePath(p)"
            :style="p.style"
          />
        </template>
        <template #edge-bus="p">
          <BaseEdge
            :id="p.id"
            :path="busPath(p)"
            :style="p.style"
          />
          <EdgeLabelRenderer v-if="p.label">
            <div
              class="tn-elabel"
              :style="{ transform: `translate(-50%, -50%) translate(${p.targetX}px, ${(p.data.busY + p.targetY) / 2}px)` }"
            >
              <span
                v-for="(s, i) in labelLines(String(p.label))"
                :key="i"
              >{{ s }}</span>
            </div>
          </EdgeLabelRenderer>
        </template>
        <MiniMap
          :node-color="'var(--kin-color-accent)'"
          :mask-color="'var(--kin-color-overlay)'"
        />
      </VueFlow>
      <div
        v-if="box"
        class="tree-box"
        :style="boxStyle"
      />
      <TreeSelection
        v-if="selected.size"
        :d="d"
        :ids="selectedIds"
        @clear="selected = new Set()"
        @changed="onSelChanged"
        @groups="emit('groups')"
      />
      <ul
        v-if="edgeMenu"
        ref="edgeMenuEl"
        class="tree-edge-menu kin-popover"
        role="menu"
        :aria-label="t.tree.edgeMenu"
        :style="{ left: `${edgeMenu.x}px`, top: `${edgeMenu.y}px` }"
        data-testid="tree-edge-menu"
      >
        <li
          v-for="it in edgeMenu.items"
          :key="it.id"
        >
          <button
            class="kin-btn kin-btn--small tree-edge-item"
            type="button"
            role="menuitem"
            @click="openFromEdgeMenu(it.id)"
          >
            {{ it.label }}
          </button>
        </li>
      </ul>
      <TreeQuickAdd
        v-if="menu"
        :id="menu.id"
        :key="menu.id"
        :d="d"
        :x="menu.x"
        :y="menu.y"
        :group="group"
        @close="onAdded"
      />
    </div>

    <DialogRoot v-model:open="cardOpen">
      <DialogPortal>
        <!-- прокручивается оверлей, а не окно карточки: низ длинной карточки всегда достижим -->
        <DialogOverlay class="kin-overlay tree-overlay">
          <DialogContent
            class="kin-dialog tree-card"
            :aria-describedby="undefined"
            @pointer-down-outside="onCardOutside"
          >
            <DialogTitle class="sr-only">
              {{ t.tree.tree }}
            </DialogTitle>
            <DialogClose
              class="kin-ibtn kin-dialog-x"
              :aria-label="t.common.close"
              :title="t.common.close"
            >
              <KinIcon name="close" />
            </DialogClose>
            <RecordView
              v-if="card"
              :id="card"
              :d="d"
              embedded
            />
          </DialogContent>
        </DialogOverlay>
      </DialogPortal>
    </DialogRoot>
  </div>
</template>

<style scoped>
/* полотно — во всю площадь родителя, панель и ошибка — поверх (F-120) */
.tree-view {
  position: absolute;
  inset: 0;
}

.tree-bar {
  position: absolute;
  z-index: 5;
  top: var(--kin-space-2);
  left: var(--kin-space-2);
  max-width: calc(100% - 2 * var(--kin-space-2));
  box-sizing: border-box;
  display: flex;
  flex-wrap: wrap;
  gap: var(--kin-space-3);
  align-items: center;
  padding: 8px 12px;
  border-radius: 16px;
  font-size: var(--kin-font-small);
}

.depth {
  width: auto;
  padding: 2px 8px;
}

.tree-view > .kin-notice {
  position: absolute;
  z-index: 5;
  right: var(--kin-space-2);
  bottom: var(--kin-space-2);
  left: var(--kin-space-2);
  margin: 0;
}

.tree-canvas {
  position: absolute;
  inset: 0;
  overflow: hidden;
}

.tree-empty {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  margin: 0;
  padding: var(--kin-space-4);
  text-align: center;
}

.tree-edge-menu {
  position: absolute;
  box-sizing: border-box;
  width: min(260px, 100%); /* как в расчёте положения: не шире, чем учтено у края */
  z-index: 20;
  display: flex;
  flex-direction: column;
  gap: var(--kin-space-1);
  margin: 0;
  padding: var(--kin-space-1);
  list-style: none;
}

.tree-edge-item {
  width: 100%;
  height: auto;
  min-height: 28px;
  text-align: left;
  white-space: normal;
  overflow-wrap: anywhere;
  justify-content: flex-start;
}

.tn-person {
  box-sizing: border-box;
  width: v-bind('`${PERSON_W}px`');
  padding: 8px;
  border: 1px solid var(--kin-color-border);
  border-radius: var(--kin-radius);
  background: var(--kin-color-surface);
  color: var(--kin-color-text);
  text-align: center;
  cursor: pointer;
  position: relative;
  overflow: hidden;
}

/* выделенные (F-131) */
.tn-person.is-sel {
  border-color: var(--kin-color-accent);
  background: var(--kin-color-accent-soft);
}

/* значок отсылки на основной карточке (F-117): центр — в точке из badges.ts */
.tn-badge {
  position: absolute;
  z-index: 2;
  box-sizing: border-box;
  display: grid;
  place-items: center;
  width: v-bind('`${BADGE_D}px`');
  height: v-bind('`${BADGE_D}px`');
  padding: 0;
  transform: translate(-50%, -50%);
  border-radius: 50%;
  border: 1px solid var(--kin-color-accent);
  background: var(--kin-color-surface);
  color: var(--kin-color-accent);
  font-size: 12px;
  line-height: 1;
  cursor: pointer;
}

.tn-badge:hover,
.tn-badge:focus-visible {
  background: var(--kin-color-accent);
  color: var(--kin-color-accent-contrast);
}

.tree-box {
  position: absolute;
  z-index: 7;
  border: 1px dashed var(--kin-color-accent);
  background: color-mix(in srgb, var(--kin-color-accent) 12%, transparent);
  pointer-events: none;
}

.tn-person.is-hl {
  border-color: var(--kin-color-accent);
  box-shadow: 0 0 0 3px var(--kin-color-accent-soft);
}

.tn-photo {
  position: relative;
  width: 100%;
  aspect-ratio: 3 / 4;
  border-radius: 6px;
  overflow: hidden;
  display: grid;
  place-items: center;
  background: var(--kin-color-surface-sunken);
  color: var(--kin-color-text-muted);
  font-weight: 600;
  font-size: 1.4rem;
}

.tn-photo :deep(.avatar) {
  width: 100%;
  height: 100%;
  border-radius: 0;
}

/* траурная ленточка по углу фото (F-116) */
.tn-ribbon {
  position: absolute;
  bottom: 9px;
  right: -16px;
  width: 64px;
  height: 7px;
  background: #0b0b0f;
  transform: rotate(-45deg);
  box-shadow: 0 0 0 1px rgb(255 255 255 / 25%);
  z-index: 1;
}

.no-ribbon .tn-ribbon {
  display: none;
}

/* ФИО полностью, с переносом (F-123) */
.tn-name {
  margin-top: 5px;
  font-size: 12px;
  line-height: 1.2;
  font-weight: 600;
  overflow-wrap: anywhere;
}

.tn-dates {
  margin-top: 2px;
  font-size: 9.5px;
  color: var(--kin-color-text-muted);
}

.tn-note {
  margin-top: 2px;
  font-size: 10.5px;
  font-style: italic;
  line-height: 1.2;
  color: var(--kin-color-text-muted);
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.tn-note.tn-note-long {
  font-size: 9.5px;
  -webkit-line-clamp: 3;
}

.tn-ref {
  border-style: dashed;
  background: transparent;
}

.tn-ref .tn-photo {
  opacity: 0.45;
}

.tn-plus {
  position: absolute;
  top: 12px;
  right: 12px;
  display: grid;
  place-items: center;
  width: 24px;
  height: 24px;
  padding: 0;
  border-radius: 50%;
  border: 0;
  background: var(--kin-color-accent);
  color: var(--kin-color-accent-contrast);
  cursor: pointer;
  opacity: 0;
}

.tn-plus :deep(.kin-icon) {
  width: 14px;
  height: 14px;
}

.tn-person:hover .tn-plus,
.tn-plus:focus-visible {
  opacity: 1;
}

/* подпись на линии — целиком, плашка по тексту, до двух строк (F-129) */
.tn-union,
.tn-elabel {
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  border-radius: 11px;
  border: 1px solid var(--kin-color-border-strong);
  background: var(--kin-color-surface-raised);
  color: var(--kin-color-text-muted);
  font-size: 10px;
  line-height: 13px;
  text-align: center;
  white-space: nowrap;
  cursor: pointer;
}

/* слой подписей — над всеми линиями, включая подсвеченные (F-127) */
/* порядок: линии (подсвеченные — 1) < подписи (2) < узлы (3) */
.tree-canvas :deep(.vue-flow__nodes) {
  z-index: 3;
}

.tree-canvas :deep(.vue-flow__edge-labels) {
  z-index: 2;
}

.tn-elabel {
  position: absolute;
  padding: 2px 7px;
  border-color: var(--kin-color-border);
  background: var(--kin-color-bg);
  cursor: default;
}

.tn-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--kin-color-text-muted);
  opacity: 0.7;
}

.tn-dot.tn-many {
  width: 24px;
  border-radius: 0;
  background: radial-gradient(circle, var(--kin-color-text-muted) 3px, transparent 3.5px) 0 0 / 8px 8px repeat-x;
}

.tree-canvas :deep(.tn-person .vue-flow__handle-left),
.tree-canvas :deep(.tn-person .vue-flow__handle-right) {
  top: v-bind('`${PHOTO_MID}px`');
}

.tree-canvas :deep(.vue-flow__handle) {
  opacity: 0;
  min-width: 0;
  min-height: 0;
  width: 1px;
  height: 1px;
  border: 0;
}

.tree-canvas :deep(.vue-flow__minimap) {
  background: var(--kin-color-surface);
  border: 1px solid var(--kin-color-border);
  border-radius: var(--kin-radius);
  cursor: grab;
}

.tree-overlay {
  display: flex;
  box-sizing: border-box;
  justify-content: center;
  align-items: flex-start;
  padding: var(--kin-space-4) var(--kin-space-3);
  overflow-y: auto;
  overscroll-behavior: contain;
}

.tree-card {
  position: relative;
  top: auto;
  left: auto;
  flex: none;
  margin: auto; /* короткая — по центру, длинная — сверху, прокручивается оверлей */
  width: min(1100px, 100%);
  max-width: none;
  max-height: none;
  overflow: visible;
  transform: none;
}

.tree-card :deep(.record.embedded) {
  padding: 0;
}

.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip-path: inset(50%);
}
</style>
