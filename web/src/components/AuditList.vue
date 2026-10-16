<script setup lang="ts">
// Журнал (F-08, F-16): операции сгруппированы по operation_id, новые сверху;
// в операции — «запись · что · действие» и изменения «было → стало». Для
// истории записи — фильтр entity; на странице журнала — фильтры по записи,
// автору и действию. Окончательно удалённая запись — её последнее название
// по журналу (F-94). Страницы — по курсору сервера.
import { PopoverContent, PopoverPortal, PopoverRoot, PopoverTrigger } from 'reka-ui'
import { computed, ref, shallowRef, watch } from 'vue'
import { RouterLink } from 'vue-router'

import { api } from '@/api/client'
import type { AuditOperationDTO, GroupDTO, RelatedDTO } from '@/api/types'
import { errorText, t } from '@/i18n'
import { useDashboardsStore } from '@/stores/dashboards'
import { useSessionStore } from '@/stores/session'
import { auditCtx, operationView } from '@/ui/audit'
import { makeCtx } from '@/ui/ctx'
import type { Registry } from '@/core/registry'
import KinIcon from './KinIcon.vue'
import RecordPicker from './RecordPicker.vue'

const props = defineProps<{
  dashboard: string
  /** история одной записи */
  entity?: string
}>()

const dashboards = useDashboardsStore()
const session = useSessionStore()
const reg = shallowRef<Registry | null>(null)
const groups = shallowRef<GroupDTO[]>([])
const ops = shallowRef<AuditOperationDTO[]>([])
const records = shallowRef<RelatedDTO>({})
const next = shallowRef<string | null>(null)
const loading = shallowRef(false)
const error = shallowRef('')
const purged = shallowRef<RelatedDTO>({})

/** фильтры страницы журнала (в истории записи — только entity) */
const byRecord = shallowRef<{ id: string; title: string } | null>(null)
const pickerOpen = ref(false)
const actor = ref('')
const op = ref('')
const OPS = ['create', 'update', 'delete', 'restore', 'purge', 'move'] as const
const opNames = t.audit.ops as Record<string, string | undefined>
const actors = computed(() => [
  { value: '', name: t.audit.anyActor },
  ...(session.account ? [{ value: session.account.id, name: t.audit.actorMe }] : []),
  { value: 'system', name: t.audit.actorSystem },
])

function pickRecord(id: string, title: string) {
  byRecord.value = { id, title }
  pickerOpen.value = false
}

const views = computed(() => {
  if (!reg.value) return []
  const a = auditCtx(makeCtx(reg.value, [], records.value), t, ops.value, groups.value, purged.value)
  return ops.value.map(o => operationView(a, o))
})

let seq = 0
async function load(more = false) {
  const my = ++seq
  loading.value = true
  error.value = ''
  try {
    const d = props.dashboard
    const [r, g, out] = await Promise.all([
      reg.value && more ? reg.value : dashboards.registry(d),
      more ? groups.value : api.groups(d),
      api.audit(d, {
        entity: props.entity ?? byRecord.value?.id,
        actor: props.entity ? undefined : actor.value || undefined,
        op: props.entity ? undefined : op.value || undefined,
        before: more ? (next.value ?? undefined) : undefined,
      }),
    ])
    if (my !== seq) return
    reg.value = r
    groups.value = g
    ops.value = more ? [...ops.value, ...out.operations] : out.operations
    records.value = more
      ? { ...records.value, ...out.related, ...out.records }
      : { ...out.related, ...out.records }
    purged.value = more ? { ...purged.value, ...out.purged } : { ...out.purged }
    next.value = out.next
  } catch (e) {
    if (my === seq) error.value = errorText(e)
  } finally {
    if (my === seq) loading.value = false
  }
}

// другой дашборд — фильтры прежнего к нему не относятся
watch(() => props.dashboard, () => {
  byRecord.value = null
  actor.value = ''
  op.value = ''
}, { flush: 'sync' })

watch(() => [props.dashboard, props.entity, byRecord.value, actor.value, op.value], () => void load(), { immediate: true })

defineExpose({ reload: () => load() })

const timeFmt = new Intl.DateTimeFormat(document.documentElement.lang || undefined, {
  dateStyle: 'medium', timeStyle: 'short',
})
function when(at: string): string {
  return timeFmt.format(new Date(at))
}
</script>

<template>
  <div class="audit">
    <div
      v-if="!entity && reg"
      class="filters"
      role="group"
      :aria-label="t.audit.filters"
    >
      <span
        v-if="byRecord"
        class="kin-chip record-chip"
      >
        {{ t.audit.record }}: {{ byRecord.title }}
        <button
          class="kin-icon-btn"
          type="button"
          :aria-label="t.audit.clearRecord"
          :title="t.audit.clearRecord"
          @click="byRecord = null"
        >
          <KinIcon name="close" />
        </button>
      </span>
      <PopoverRoot
        v-else
        v-model:open="pickerOpen"
      >
        <PopoverTrigger
          class="kin-input record-filter"
          :aria-label="t.audit.record"
        >
          {{ t.audit.anyRecord }}
          <KinIcon name="more" />
        </PopoverTrigger>
        <PopoverPortal>
          <PopoverContent
            class="kin-popover"
            side="bottom"
            align="start"
            :side-offset="4"
          >
            <RecordPicker
              :dashboard="dashboard"
              :reg="reg"
              @pick="pickRecord"
            />
          </PopoverContent>
        </PopoverPortal>
      </PopoverRoot>
      <select
        v-model="actor"
        class="kin-input"
        name="audit-actor"
        :aria-label="t.audit.actor"
      >
        <option
          v-for="x in actors"
          :key="x.value"
          :value="x.value"
        >
          {{ x.name }}
        </option>
      </select>
      <select
        v-model="op"
        class="kin-input"
        name="audit-op"
        :aria-label="t.audit.op"
      >
        <option value="">
          {{ t.audit.anyOp }}
        </option>
        <option
          v-for="x in OPS"
          :key="x"
          :value="x"
        >
          {{ opNames[x] ?? x }}
        </option>
      </select>
    </div>
    <p
      v-if="error"
      class="kin-notice kin-notice--error"
      role="alert"
    >
      {{ error }}
    </p>
    <p
      v-if="!loading && !error && views.length === 0"
      class="kin-muted"
    >
      {{ t.audit.empty }}
    </p>
    <ol class="ops">
      <li
        v-for="o in views"
        :key="o.id"
        class="op"
        data-test="audit-op"
      >
        <div
          class="op-head kin-muted"
          :title="o.details"
        >
          <time :datetime="o.at">{{ when(o.at) }}</time>
          · {{ o.actor }}
        </div>
        <ul class="entries">
          <li
            v-for="e in o.entries"
            :key="e.id"
            class="entry"
          >
            <span class="what">
              <RouterLink
                v-if="e.recordId && !entity && !e.purged"
                :to="{ name: 'record', params: { d: dashboard, id: e.recordId } }"
              >{{ e.record }}</RouterLink>
              <span v-else-if="e.record && !entity">{{ e.record }}<span
                v-if="e.purged && e.record !== t.audit.unknownRecord"
                class="kin-muted"
              > ({{ t.audit.purged }})</span></span>
              <span class="kin-muted">{{ e.record && !entity ? ' ' : '' }}{{ e.object }} · {{ e.op }}</span>
            </span>
            <ul
              v-if="e.changes.length"
              class="changes"
            >
              <li
                v-for="(c, i) in e.changes"
                :key="i"
              >
                <span class="kin-muted">{{ c.label }}:</span>
                <del v-if="c.before">{{ c.before }}</del>
                <span
                  v-if="c.before || c.after"
                  aria-hidden="true"
                > → </span>
                <ins v-if="c.after">{{ c.after }}</ins>
                <span
                  v-else-if="c.before"
                  class="kin-muted"
                >{{ t.common.empty }}</span>
              </li>
            </ul>
          </li>
        </ul>
      </li>
    </ol>
    <p
      v-if="loading"
      class="kin-muted"
    >
      {{ t.common.loading }}
    </p>
    <button
      v-else-if="next"
      class="kin-btn"
      type="button"
      @click="load(true)"
    >
      {{ t.audit.more }}
    </button>
  </div>
</template>

<style scoped>
.filters {
  display: flex;
  flex-wrap: wrap;
  gap: var(--kin-space-2);
  align-items: center;
  margin-bottom: var(--kin-space-3);
}

.filters .kin-input {
  width: auto;
  min-width: 0;
  max-width: 100%;
}

.record-filter {
  display: inline-flex;
  gap: var(--kin-space-1);
  align-items: center;
  cursor: pointer;
}

.record-chip {
  display: inline-flex;
  gap: var(--kin-space-1);
  align-items: center;
  max-width: 100%;
}
.ops,
.entries,
.changes {
  margin: 0;
  padding: 0;
  list-style: none;
}

.op {
  padding: 10px 0;
  border-bottom: 1px solid var(--kin-color-border);
}

.op:first-child {
  padding-top: 0;
}

.op:last-child {
  border-bottom: none;
}

.op-head {
  font-size: var(--kin-font-tiny);
}

.what {
  font-weight: 600;
}

.entry {
  margin-top: var(--kin-space-1);
}

.changes {
  padding-left: var(--kin-space-3);
  font-size: var(--kin-font-tiny);
  overflow-wrap: anywhere;
}

.entry {
  min-width: 0;
}

del {
  color: var(--kin-color-text-muted);
}

ins {
  text-decoration: none;
}
</style>
