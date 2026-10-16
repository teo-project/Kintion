<script setup lang="ts">
// Группы записи в карточке (F-02, F-04): метки; правка — во всплывающей
// форме (F-69), все метки одним запросом с версией (F-45).
import { PopoverContent, PopoverPortal, PopoverRoot, PopoverTrigger } from 'reka-ui'
import { computed, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'

import { api } from '@/api/client'
import { t } from '@/i18n'
import { relatedGroups } from '@/ui/groups'
import { useRecord } from '@/ui/record'
import { useGroups } from '@/ui/useGroups'

import GroupPicker from '../GroupPicker.vue'
import KinIcon from '../KinIcon.vue'

const props = defineProps<{ label: string; dashboard: string }>()

/** ключ версии меток записи (план §4) */
const KEY = 'groups'

const s = useRecord()
const { groups } = useGroups(props.dashboard)
const open = ref(false)
const draft = ref<string[]>([])

const names = computed(() => {
  const byId = new Map(groups.value.map(g => [g.id, g.name]))
  return (s.rec.value?.groups ?? []).map(id => ({ id, name: byId.get(id) ?? '…' }))
})
const suggest = computed(() => (s.rec.value && s.ctx.value ? relatedGroups(s.rec.value, s.ctx.value.records) : []))

watch(open, v => {
  if (v) draft.value = [...(s.rec.value?.groups ?? [])]
})

async function save() {
  const rec = s.rec.value
  if (!rec) return
  const chosen = draft.value
  if (s.draft.value) {
    s.setDraftGroups(chosen) // черновик (F-88): группы уйдут с первым значением
    open.value = false
    return
  }
  const ok = await s.save(KEY, rev => api.setGroups(rec.id, rev, chosen))
  if (ok || !s.fieldErrors[KEY]) open.value = false
}
</script>

<template>
  <div
    v-if="groups.length || names.length || s.editing.value"
    class="groups-row kin-hover-row"
  >
    <span class="kin-muted label">{{ label }}</span>
    <span class="chips">
      <RouterLink
        v-for="g in names"
        :key="g.id"
        class="chip kin-chip"
        :to="{ name: 'dashboard', params: { d: dashboard }, query: { g: g.id } }"
      >{{ g.name }}</RouterLink>
      <span
        v-if="!names.length"
        class="kin-muted"
      >{{ t.common.empty }}</span>
    </span>
    <PopoverRoot
      v-if="s.editing.value"
      v-model:open="open"
    >
      <PopoverTrigger
        class="kin-icon-btn kin-semi"
        :aria-label="`${t.common.edit}: ${label}`"
        :title="`${t.common.edit}: ${label}`"
      >
        <KinIcon name="edit" />
      </PopoverTrigger>
      <PopoverPortal>
        <PopoverContent
          class="kin-popover"
          side="bottom"
          align="start"
          :side-offset="4"
        >
          <form
            class="kin-form"
            @submit.prevent="save"
          >
            <GroupPicker
              v-model="draft"
              :dashboard="dashboard"
              :suggest="suggest"
            />
            <p
              v-if="s.fieldErrors[KEY]"
              class="kin-error"
              role="alert"
            >
              {{ s.fieldErrors[KEY] }}
            </p>
            <div class="kin-actions">
              <button
                class="kin-ibtn"
                type="button"
                :aria-label="t.common.cancel"
                :title="t.common.cancel"
                @click="open = false"
              >
                <KinIcon name="close" />
              </button>
              <button
                class="kin-ibtn kin-ibtn--primary save"
                type="submit"
                :disabled="s.saving[KEY]"
                :aria-label="t.common.save"
                :title="t.common.save"
              >
                <KinIcon name="check" />
              </button>
            </div>
          </form>
        </PopoverContent>
      </PopoverPortal>
    </PopoverRoot>
  </div>
</template>

<style scoped>
.groups-row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kin-space-2) 12px;
  align-items: center;
}

.label {
  flex: none;
  width: 130px;
  font-size: var(--kin-font-tiny);
}

.chips {
  display: inline-flex;
  flex-wrap: wrap;
  gap: var(--kin-space-1);
}

.chip:hover {
  text-decoration: none;
  box-shadow: inset 0 0 0 1px var(--kin-color-accent);
}
</style>
