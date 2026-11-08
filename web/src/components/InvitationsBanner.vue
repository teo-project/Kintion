<script setup lang="ts">
// Приглашения в чужие дашборды, ждущие ответа (F-03): принять — дашборд
// появляется в переключателе; отказаться — приглашение и права удаляются.
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'

import { api } from '@/api/client'
import type { InvitationDTO } from '@/api/types'
import { errorText, fmt, t } from '@/i18n'
import { useDashboardsStore } from '@/stores/dashboards'

const dashboards = useDashboardsStore()
const router = useRouter()
const list = ref<InvitationDTO[]>([])
const error = ref('')

onMounted(async () => {
  try {
    list.value = await api.invitations()
  } catch {
    // приглашения — не главное на экране: без них экран работает
  }
})

async function answer(inv: InvitationDTO, accept: boolean) {
  error.value = ''
  try {
    if (accept) await api.acceptInvitation(inv.dashboard_id)
    else await api.declineInvitation(inv.dashboard_id)
    list.value = list.value.filter(x => x.dashboard_id !== inv.dashboard_id)
    await dashboards.load(true)
    if (accept) await router.push({ name: 'dashboard', params: { d: inv.dashboard_id } })
  } catch (e) {
    error.value = errorText(e)
  }
}
</script>

<template>
  <div
    v-if="list.length || error"
    class="invitations"
  >
    <p
      v-for="inv in list"
      :key="inv.dashboard_id"
      class="kin-notice kin-row"
    >
      <span class="grow">{{ fmt(t.sharing.invitation, { owner: inv.owner_nick ?? t.sharing.someone, name: inv.name }) }}</span>
      <button
        class="kin-btn kin-btn--small kin-btn--primary"
        type="button"
        @click="answer(inv, true)"
      >
        {{ t.sharing.accept }}
      </button>
      <button
        class="kin-btn kin-btn--small"
        type="button"
        @click="answer(inv, false)"
      >
        {{ t.sharing.decline }}
      </button>
    </p>
    <p
      v-if="error"
      class="kin-error"
      role="alert"
    >
      {{ error }}
    </p>
  </div>
</template>

<style scoped>
.invitations {
  display: grid;
  gap: 8px;
  margin-bottom: var(--kin-gap-islands);
}
.grow {
  flex: 1;
  min-width: 0;
}
</style>
