<script setup lang="ts">
// Выбор групп (F-02, F-04): выбранные — метками, все группы — плоским списком
// во всплывающем окне (F-105); предложения — группы связанных записей и
// последние назначенные этим пользователем (F-81, с сервера). Тот же выбор —
// «Входит в» и «Включает в себя» в форме группы (без предложений).
import { PopoverContent, PopoverPortal, PopoverRoot, PopoverTrigger } from 'reka-ui'
import { computed, onMounted, ref } from 'vue'

import { api } from '@/api/client'

import { t } from '@/i18n'
import { suggestGroups } from '@/ui/groups'
import { useGroups } from '@/ui/useGroups'
import KinCheckbox from './KinCheckbox.vue'
import KinIcon from './KinIcon.vue'

import { errorText } from '@/i18n'

const props = defineProps<{
  dashboard: string
  /** группы связанных записей — предлагаются первыми */
  suggest?: string[]
  /** группы, которые выбрать нельзя (цикл во вложенности, F-105) */
  exclude?: string[]
  /** подпись кнопки выбора */
  label?: string
  /** без предложений (форма группы) */
  noOffers?: boolean
}>()
const model = defineModel<string[]>({ required: true })

const { groups, reload } = useGroups(props.dashboard)
const choices = computed(() => groups.value.filter(g => !props.exclude?.includes(g.id)))
const byId = computed(() => new Map(groups.value.map(g => [g.id, g])))
const recent = ref<string[]>([])
onMounted(async () => {
  if (props.noOffers) return
  try {
    recent.value = await api.recentGroups(props.dashboard)
  } catch {
    recent.value = [] // предложения необязательны
  }
})
const offers = computed(() => props.noOffers
  ? []
  : suggestGroups(choices.value, props.suggest ?? [], recent.value, model.value))

function toggle(id: string, on: boolean) {
  model.value = on ? [...model.value.filter(x => x !== id), id] : model.value.filter(x => x !== id)
}

/** Новая группа прямо из выбора (F-122): создаётся и сразу выбирается. */
const newName = ref('')
const creating = ref(false)
const createError = ref('')
async function createGroup() {
  const name = newName.value.trim()
  if (!name || creating.value) return
  creating.value = true
  createError.value = ''
  try {
    const g = await api.createGroup(props.dashboard, { name })
    await reload()
    toggle(g.id, true)
    newName.value = ''
  } catch (e) {
    createError.value = errorText(e)
  } finally {
    creating.value = false
  }
}
</script>

<template>
  <div class="group-picker">
    <div class="kin-row">
      <span
        v-for="id in model"
        :key="id"
        class="chip"
      >
        {{ byId.get(id)?.name ?? '…' }}
        <button
          class="kin-icon-btn"
          type="button"
          :aria-label="`${t.common.delete}: ${byId.get(id)?.name ?? ''}`"
          :title="`${t.common.delete}: ${byId.get(id)?.name ?? ''}`"
          @click="toggle(id, false)"
        >
          <KinIcon name="close" />
        </button>
      </span>
      <PopoverRoot>
        <PopoverTrigger
          class="kin-ibtn kin-ibtn--sm kin-ibtn--dashed choose"
          :aria-label="label ?? t.groups.choose"
          :title="label ?? t.groups.choose"
        >
          <KinIcon name="plus" />
        </PopoverTrigger>
        <PopoverPortal>
          <PopoverContent
            class="kin-popover"
            side="right"
            align="start"
            :side-offset="4"
          >
            <ul class="tree">
              <li
                v-for="g in choices"
                :key="g.id"
              >
                <KinCheckbox
                  :model-value="model.includes(g.id)"
                  @update:model-value="v => toggle(g.id, v)"
                >
                  {{ g.name }}
                </KinCheckbox>
              </li>
            </ul>
            <form
              class="kin-row new-group"
              @submit.prevent="createGroup"
            >
              <input
                v-model="newName"
                class="kin-input"
                name="new-group"
                :placeholder="t.groups.newGroup"
                :aria-label="t.groups.newGroupName"
                @keydown.stop
              >
              <button
                class="kin-ibtn kin-ibtn--sm kin-ibtn--primary"
                type="submit"
                :disabled="creating || !newName.trim()"
                :aria-label="t.common.create"
                :title="t.common.create"
              >
                <KinIcon name="plus" />
              </button>
            </form>
            <p
              v-if="createError"
              class="kin-notice kin-notice--error"
              role="alert"
            >
              {{ createError }}
            </p>
          </PopoverContent>
        </PopoverPortal>
      </PopoverRoot>
    </div>
    <div
      v-if="offers.length"
      class="kin-row kin-muted offers"
    >
      {{ t.groups.suggested }}:
      <button
        v-for="g in offers"
        :key="g.id"
        class="kin-btn kin-btn--small offer"
        type="button"
        @click="toggle(g.id, true)"
      >
        <KinIcon name="plus" />
        {{ g.name }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.group-picker {
  display: flex;
  flex-direction: column;
  gap: var(--kin-space-1);
}

.chip {
  display: inline-flex;
  gap: 2px;
  align-items: center;
  padding: 0 2px 0 10px;
  color: var(--kin-color-accent-text);
  background: var(--kin-color-accent-soft);
  border-radius: var(--kin-radius-pill);
  font-size: var(--kin-font-tiny);
  font-weight: 600;
}

.chip .kin-icon-btn {
  min-width: 20px;
  height: 20px;
  color: inherit;
  border-radius: 50%;
}

.tree {
  display: flex;
  flex-direction: column;
  gap: var(--kin-space-1);
  max-height: 16rem;
  margin: 0;
  padding: 0;
  overflow: auto;
  list-style: none;
}

.offers {
  font-size: var(--kin-font-tiny);
}

.new-group {
  margin-top: var(--kin-space-2);
}
</style>
