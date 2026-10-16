<script setup lang="ts">
// Список записей дашборда: подписи — резолвером по правилу шаблона (F-71),
// мини-фото — поле photo (F-72), метки групп (F-02). Отношение (F-109):
// слева — участники, справа — шаблон и подпись в скобках.
import { computed } from 'vue'
import { RouterLink } from 'vue-router'

import type { GroupDTO } from '@/api/types'
import { photoOf } from '@/core/card'
import { relationRow, templateWithTitle } from '@/core/listRow'
import { recordTitle, tplOf, type Ctx } from '@/core/resolver'
import type { RecordDTO } from '@/core/types'
import { fmt, t } from '@/i18n'

import KinCheckbox from './KinCheckbox.vue'
import RecordAvatar from './RecordAvatar.vue'

const props = defineProps<{
  dashboard: string
  items: RecordDTO[]
  ctx: Ctx
  emptyText: string
  /** группы дашборда — для меток */
  groups?: GroupDTO[]
  /** режим выделения (F-131): галочки у строк */
  selectable?: boolean
  selected?: ReadonlySet<string>
}>()
const emit = defineEmits<{ toggle: [id: string, on: boolean] }>()

const groupNames = computed(() => new Map((props.groups ?? []).map(g => [g.id, g.name])))

const rows = computed(() => props.items.map(rec => {
  const title = recordTitle(props.ctx, rec)
  const template = tplOf(props.ctx, rec)?.name ?? ''
  const rel = relationRow(props.ctx, rec)
  return {
    id: rec.id,
    title,
    rel,
    more: rel?.more ? fmt(t.dashboard.moreParticipants, { n: rel.more }) : '',
    template: rel ? templateWithTitle(template, title) : template,
    photo: photoOf(props.ctx, rec),
    groups: rec.groups.map(id => groupNames.value.get(id)).filter((n): n is string => !!n),
  }
}))
</script>

<template>
  <p
    v-if="rows.length === 0"
    class="kin-muted empty"
  >
    {{ emptyText }}
  </p>
  <ul
    v-else
    class="record-list"
  >
    <li
      v-for="r in rows"
      :key="r.id"
      class="item"
      :class="{ 'is-sel': selected?.has(r.id) }"
    >
      <KinCheckbox
        v-if="selectable"
        class="sel"
        :model-value="selected?.has(r.id) ?? false"
        :aria-label="`${t.tree.select}: ${r.title}`"
        @update:model-value="v => emit('toggle', r.id, v)"
      />
      <RouterLink
        class="row"
        :to="{ name: 'record', params: { d: dashboard, id: r.id } }"
      >
        <RecordAvatar
          :dashboard="dashboard"
          :photo="r.photo"
          :title="r.title"
        />
        <span
          v-if="r.rel"
          class="title parts"
        >
          <span
            v-for="(o, i) in r.rel.leads"
            :key="'l' + i"
            class="part lead"
          >{{ o }}</span>
          <span
            v-for="(p, i) in r.rel.people"
            :key="'p' + i"
            class="part"
          >{{ p.title }} <span class="role">— {{ p.label }}</span></span>
          <span
            v-if="r.more"
            class="kin-muted more"
          >{{ r.more }}</span>
          <span
            v-if="!r.rel.leads.length && !r.rel.people.length"
            class="kin-muted more"
          >{{ t.dashboard.noParticipants }}</span>
        </span>
        <span
          v-else
          class="title"
        >{{ r.title }}</span>
        <span
          v-for="g in r.groups"
          :key="g"
          class="chip kin-chip"
        >{{ g }}</span>
        <span class="kin-muted template">{{ r.template }}</span>
      </RouterLink>
    </li>
  </ul>
</template>

<style scoped>
.record-list {
  margin: 0;
  padding: 6px;
  list-style: none;
}

.item {
  display: flex;
  gap: 6px;
  align-items: center;
}

.item.is-sel {
  background: var(--kin-color-accent-soft);
  border-radius: 12px;
}

.sel {
  flex: none;
  margin-left: 8px;
}

.row {
  flex: 1;
  min-width: 0;
  display: flex;
  box-sizing: border-box;
  gap: 12px;
  align-items: center;
  min-height: 48px;
  padding: 6px 10px;
  color: var(--kin-color-text);
  border-radius: 12px;
  text-decoration: none;
}

.row:hover {
  background: var(--kin-color-surface-raised);
  text-decoration: none;
}

.row :deep(.avatar) {
  width: 34px;
  height: 34px;
  font-size: var(--kin-font-tiny);
}

.title {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  font-weight: 700;
  text-overflow: ellipsis;
}

.part:not(:last-child)::after {
  content: ", ";
}

.role {
  font-weight: 400;
}

.more {
  font-weight: 400;
}

.template {
  font-size: var(--kin-font-tiny);
}

.empty {
  margin: 0;
  padding: var(--kin-space-4) var(--kin-space-3);
  text-align: center;
}

@media (max-width: 560px) {
  .row {
    flex-wrap: wrap;
  }

  .title {
    flex-basis: calc(100% - 50px);
  }
}
</style>
