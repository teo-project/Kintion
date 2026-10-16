<script setup lang="ts">
// «Связи» (F-89, F-39, F-43): строка — форма роли другого участника, его
// подпись и «(через <связь>)»; своя роль не пишется; участники с одинаковой
// формой — одной строкой через запятую. Подписи и связь — ссылки на
// карточки. Прошлые (есть конец) — отдельно и без особых форм (F-74).
// Порядок (F-93): сначала люди через отношения, затем прямые ссылки —
// события и записи из полей-ссылок («Поле: запись»); внутри групп — Д-27.
// Отсюда же — новая связь с этой записью в подходящей роли.
import { PopoverContent, PopoverPortal, PopoverRoot, PopoverTrigger } from 'reka-ui'
import { computed, ref, shallowRef } from 'vue'
import { RouterLink } from 'vue-router'

import type { RelationView } from '@/core/card'
import { fitsRole } from '@/core/participants'
import { treeInput } from '@/core/tree/graph'
import type { RecordDTO, TemplateDef } from '@/core/types'
import { t } from '@/i18n'
import { relatedGroups } from '@/ui/groups'
import { useRecord } from '@/ui/record'
import KinIcon from '../KinIcon.vue'

import CreateRelationDialog from '../participants/CreateRelationDialog.vue'

const props = defineProps<{ relations: RelationView[]; dashboard: string }>()

const s = useRecord()

const current = computed(() => props.relations.filter(r => !r.past))
const past = computed(() => props.relations.filter(r => r.past))

/** шаблоны-контейнеры, где у этой записи есть подходящая роль */
const templates = computed<TemplateDef[]>(() => {
  const reg = s.reg.value
  const mine = s.card.value?.template
  if (!reg || !mine) return []
  return [...reg.templates.values()]
    .map(x => x.definition)
    .filter(x => reg.entities.get(x.entity)?.container
      && (x.roles ?? []).some(r => fitsRole(reg, r, mine)))
})

/** запись есть в дереве — по тем же правилам, что модель дерева (этапы, видимые участники; F-129) */
const inTree = computed(() => {
  const rec = s.rec.value
  const ctx = s.ctx.value
  if (!rec || !ctx || rec.deleted || s.draft.value) return false
  const rels = rec.participates_in
    .map(p => ctx.records[p.container_id])
    .filter((r): r is RecordDTO => !!r && !r.deleted)
  const people = Object.values(ctx.records).filter((r): r is RecordDTO => !!r && !r.deleted)
  return treeInput(ctx, rels, people).persons.some(p => p.id === rec.id)
})

const menuOpen = ref(false)
const dialogOpen = ref(false)
const chosen = shallowRef<TemplateDef | null>(null)
const preset = computed(() => (s.card.value
  ? { id: s.card.value.id, title: s.card.value.title, template: s.card.value.template }
  : null))

/** предложить группы этой записи и связанных с ней (F-04) */
const suggest = computed(() => {
  const rec = s.rec.value
  return rec && s.ctx.value ? [...new Set([...rec.groups, ...relatedGroups(rec, s.ctx.value.records)])] : []
})

function choose(tpl: TemplateDef) {
  chosen.value = tpl
  menuOpen.value = false
  dialogOpen.value = true
}

/** созданная связь появляется в карточке, перейти к ней — по ссылке (F-101) */
function created() {
  void s.load()
}
</script>

<template>
  <section class="relations">
    <div class="rel-head">
      <h2 class="kin-section-title rel-title">
        {{ t.relations.title }}
      </h2>
      <RouterLink
        v-if="inTree"
        class="kin-ibtn kin-ibtn--sm to-tree"
        :to="{ name: 'dashboard', params: { d: dashboard }, query: { v: 'tree', focus: s.rec.value?.id } }"
        :aria-label="t.relations.tree"
        :title="t.relations.tree"
        data-testid="to-tree"
      >
        <KinIcon name="tree" />
      </RouterLink>
      <PopoverRoot
        v-if="templates.length"
        v-model:open="menuOpen"
      >
        <PopoverTrigger
          class="kin-ibtn kin-ibtn--sm new-relation"
          :aria-label="t.relations.create"
          :title="t.relations.create"
        >
          <KinIcon name="plus" />
        </PopoverTrigger>
        <PopoverPortal>
          <PopoverContent
            class="kin-popover"
            side="bottom"
            align="start"
            :side-offset="4"
          >
            <ul class="menu">
              <li
                v-for="x in templates"
                :key="x.id"
              >
                <button
                  class="kin-btn kin-btn--small"
                  type="button"
                  @click="choose(x)"
                >
                  {{ x.name }}
                </button>
              </li>
            </ul>
          </PopoverContent>
        </PopoverPortal>
      </PopoverRoot>
    </div>
    <p
      v-if="relations.length === 0"
      class="kin-muted"
    >
      {{ t.relations.none }}
    </p>
    <template
      v-for="group in [{ key: 'current', list: current }, { key: 'past', list: past }]"
      :key="group.key"
    >
      <h3
        v-if="group.key === 'past' && group.list.length"
        class="kin-muted past-title"
      >
        {{ t.relations.past }}
      </h3>
      <ul
        v-if="group.list.length"
        class="list"
        :class="group.key"
      >
        <template
          v-for="r in group.list"
          :key="r.key"
        >
          <li
            v-for="(line, i) in r.lines.length ? r.lines : [null]"
            :key="`${r.key}-${i}`"
            class="relation"
            :class="r.kind"
            :data-relation="r.record"
          >
            <template v-if="line">
              <span class="kin-muted role">{{ line.label }}:</span>{{ ' ' }}<span class="people">
                <template
                  v-for="(o, j) in line.people"
                  :key="o.participation"
                >
                  <RouterLink
                    class="other"
                    :to="{ name: 'record', params: { d: dashboard, id: o.record } }"
                  >{{ o.title }}</RouterLink><span v-if="j < line.people.length - 1">, </span>
                </template>
              </span>{{ ' ' }}<span class="kin-muted via">({{ t.relations.via }}
                <RouterLink
                  class="via-link"
                  :to="{ name: 'record', params: { d: dashboard, id: r.record } }"
                >{{ r.title }}</RouterLink>)</span>
            </template>
            <template v-else>
              <template v-if="r.fieldLabel">
                <span class="kin-muted role">{{ r.fieldLabel }}:</span>{{ ' ' }}
              </template>
              <RouterLink
                class="via-link"
                :to="{ name: 'record', params: { d: dashboard, id: r.record } }"
              >
                {{ r.title }}
              </RouterLink>
            </template>
          </li>
        </template>
      </ul>
    </template>
    <CreateRelationDialog
      v-if="s.reg.value"
      v-model:open="dialogOpen"
      :dashboard="dashboard"
      :reg="s.reg.value"
      :template="chosen"
      :preset="preset"
      :suggest-groups="suggest"
      @created="created"
    />
  </section>
</template>

<style scoped>
.rel-head {
  display: flex;
  gap: var(--kin-space-2);
  align-items: center;
  justify-content: space-between;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--kin-color-border);
}

/* иконка дерева — левее (+) (F-129) */
.to-tree {
  margin-left: auto;
}

.rel-title {
  margin: 0;
}

.past-title {
  margin: 14px 0 0;
  font-size: 0.6875rem;
  font-weight: 600;
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.list,
.menu {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  list-style: none;
}

.menu {
  gap: var(--kin-space-1);
}

.relation {
  padding: 8px 0;
  line-height: 1.45;
  border-bottom: 1px solid var(--kin-color-border);
}

.list .relation:last-child {
  border-bottom: none;
}

.role {
  font-size: var(--kin-font-tiny);
}

.other {
  font-weight: 700;
}

.past .relation {
  opacity: 0.7;
}

.via {
  font-size: var(--kin-font-tiny);
}

.menu .kin-btn {
  width: 100%;
}
</style>
