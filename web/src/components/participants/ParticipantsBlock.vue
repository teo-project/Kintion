<script setup lang="ts">
// Участники записи-контейнера по ролям шаблона (F-10, F-39): подпись — форма
// роли со стороны другого участника (этап — на сегодня), период и пометка —
// во всплывающей форме (F-69). Добавить / убрать с учётом min / max (Д-22):
// кнопки недоступны, сервер тоже откажет.
import { PopoverContent, PopoverPortal, PopoverRoot, PopoverTrigger } from 'reka-ui'
import { computed, ref } from 'vue'
import { RouterLink } from 'vue-router'

import { api } from '@/api/client'
import type { RoleView } from '@/core/card'
import { canJoin, limits, participantsRev } from '@/core/participants'
import type { Ctx } from '@/core/resolver'
import type { RecordDTO } from '@/core/types'
import { fmt, t } from '@/i18n'
import { useRecord } from '@/ui/record'

import RecordPicker from '../RecordPicker.vue'

import ParticipantForm from './ParticipantForm.vue'
import KinIcon from '../KinIcon.vue'

defineProps<{ roles: RoleView[]; dashboard: string }>()

const s = useRecord()
const adding = ref<Record<string, boolean>>({})

const tpl = computed(() => s.card.value?.template)
const lim = computed(() => {
  const rec = s.rec.value
  return tpl.value && rec ? limits(tpl.value, rec.participants.map(p => p.role)) : null
})

function limitOf(role: string) {
  return lim.value?.roles.find(r => r.role.key === role)
}

function acceptFor(role: string) {
  return (cand: RecordDTO, ctx: Ctx) => {
    const r = tpl.value?.roles?.find(x => x.key === role)
    return !!r && !!tpl.value && canJoin(ctx, tpl.value, r, cand, s.rec.value?.id)
  }
}

async function add(role: string, entity: string) {
  const rec = s.rec.value
  if (!rec) return
  const ok = await s.save(participantsRev(role), rev => api.addParticipant(rec.id, { if_rev: rev, entity, role }))
  if (ok) adding.value = { ...adding.value, [role]: false }
}

function dto(pid: string) {
  return s.rec.value?.participants.find(p => p.id === pid)
}
</script>

<template>
  <section class="participants">
    <div
      v-for="r in roles"
      :key="r.role"
      class="role"
    >
      <div class="kin-row role-head">
        <span class="kin-muted role-name">{{ r.name }}</span>
        <span
          v-if="limitOf(r.role)?.missing"
          class="kin-error need"
        >{{ fmt(t.participants.need, { n: limitOf(r.role)!.missing }) }}</span>
        <PopoverRoot
          :open="adding[r.role] ?? false"
          @update:open="v => (adding = { ...adding, [r.role]: v })"
        >
          <PopoverTrigger
            class="kin-icon-btn add"
            :disabled="!limitOf(r.role)?.canAdd"
            :aria-label="`${t.participants.add}: ${r.name}`"
            :title="limitOf(r.role)?.canAdd ? t.participants.add : t.participants.full"
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
              <RecordPicker
                v-if="s.reg.value"
                :dashboard="dashboard"
                :reg="s.reg.value"
                :accept="acceptFor(r.role)"
                @pick="id => add(r.role, id)"
              />
            </PopoverContent>
          </PopoverPortal>
        </PopoverRoot>
      </div>
      <ul class="items">
        <li
          v-for="p in r.items"
          :key="p.participation"
          class="kin-row kin-hover-row"
        >
          <RouterLink :to="{ name: 'record', params: { d: dashboard, id: p.record } }">
            {{ p.title }}
          </RouterLink>
          <span class="kin-muted">{{ p.label }}</span>
          <span
            v-if="p.period"
            class="kin-muted small"
          >{{ p.period }}</span>
          <span
            v-if="dto(p.participation)?.note"
            class="kin-muted small"
          >· {{ dto(p.participation)?.note }}</span>
          <ParticipantForm
            v-if="dto(p.participation)"
            :participant="dto(p.participation)!"
            :title="p.title"
            :can-remove="limitOf(r.role)?.canRemove ?? true"
          />
        </li>
      </ul>
      <p
        v-if="s.fieldErrors[participantsRev(r.role)]"
        class="kin-error"
        role="alert"
      >
        {{ s.fieldErrors[participantsRev(r.role)] }}
      </p>
    </div>
  </section>
</template>

<style scoped>
.participants {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.role + .role {
  padding-top: 10px;
  border-top: 1px solid var(--kin-color-border);
}

.role-name {
  min-width: 130px;
  font-size: var(--kin-font-tiny);
}

.need,
.small {
  font-size: var(--kin-font-tiny);
}

.items {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 4px 0 0;
  padding: 0;
  list-style: none;
}

.items a {
  font-weight: 700;
}
</style>
