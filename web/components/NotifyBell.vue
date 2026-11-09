<script setup lang="ts">
// Колокольчик: уведомления «Контроля спринта руководителей» (в блоке, можно снимать блок, без продолжения, назначена на вас).
// Опрос раз в минуту; список — последние 50, «Прочитать все».
import { linkKeys } from '~/utils/leadcontrol'

type N = { id: number; kind: string; text: string; url: string; create_url: string; created_at: string; read: boolean }
const { apiBase, jiraBase } = useRuntimeConfig().public
const data = ref<{ unread: number; items: N[] }>({ unread: 0, items: [] })
const open = ref(false)
const root = ref<HTMLElement | null>(null)

async function load() {
  try { data.value = await $fetch(`${apiBase}/notifications`) } catch (e: any) {
    // Сессия кончилась (выход, блокировка) — перестаём опрашивать до следующего входа; сеть — попробуем позже.
    if (e?.statusCode === 401) clearInterval(timer)
  }
}
async function readAll() {
  await $fetch(`${apiBase}/notifications/read`, { method: 'POST', body: { all: true } }).catch(() => {})
  await load()
}
async function readOne(n: N) {
  if (n.read) return
  await $fetch(`${apiBase}/notifications/read`, { method: 'POST', body: { ids: [n.id] } }).catch(() => {})
  n.read = true
  data.value.unread = Math.max(0, data.value.unread - 1)
}
let timer: ReturnType<typeof setInterval> | undefined
function outside(e: MouseEvent) { if (open.value && root.value && e.target instanceof Node && !root.value.contains(e.target)) open.value = false }
// notify:refresh — обновить сразу (например, после «Проверить уведомление» в настройках).
onMounted(() => { load(); timer = setInterval(load, 60_000); document.addEventListener('click', outside); window.addEventListener('notify:refresh', load) })
onBeforeUnmount(() => { clearInterval(timer); document.removeEventListener('click', outside); window.removeEventListener('notify:refresh', load) })

const when = (s: string) => new Date(s).toLocaleString('ru-RU', { timeZone: 'Asia/Almaty', day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })
const html = (n: N) => linkKeys(n.text, jiraBase)
const ICON: Record<string, string> = { blocked: 'В блоке', unblock: 'Можно снимать блок', gap: 'Без продолжения', assigned: 'Назначена на вас', test: 'Тест' }
</script>

<template>
  <div ref="root" class="bell">
    <button type="button" class="bell-btn" :aria-label="`Уведомления: непрочитанных ${data.unread}`" :aria-expanded="open" @click="open = !open; if (open) load()">
      <span aria-hidden="true">🔔</span><span v-if="data.unread" class="bell-n">{{ data.unread > 99 ? '99+' : data.unread }}</span>
    </button>
    <div v-if="open" class="bell-pop" role="dialog" aria-label="Уведомления">
      <div class="bell-head">
        <b>Уведомления</b>
        <button v-if="data.unread" type="button" class="linkbtn" @click="readAll">Прочитать все</button>
        <NuxtLink to="/settings#notify" class="linkbtn" @click="open = false">Настроить</NuxtLink>
      </div>
      <p v-if="!data.items.length" class="sub bell-empty">Уведомлений пока нет. Руководитель получает их по своим проектам из «Контроля спринта»; что присылать — в «Настройках».</p>
      <ul v-else class="bell-list">
        <li v-for="n in data.items" :key="n.id" :class="{ unread: !n.read }" @click="readOne(n)">
          <div class="bell-meta"><span class="pill mute">{{ ICON[n.kind] || n.kind }}</span><span class="sub">{{ when(n.created_at) }}</span></div>
          <div class="bell-text" v-html="html(n)" />
          <div v-if="n.create_url" class="bell-act"><a :href="n.create_url" target="_blank" rel="noopener">Создать продолжение в Jira</a>
            · <NuxtLink :to="`/lead-control?tab=gaps&q=${encodeURIComponent(n.text.match(/[A-Z][A-Z0-9]+-\d+/)?.[0] || '')}`" @click="open = false">Игнорировать — в «Контроле спринта»</NuxtLink></div>
        </li>
      </ul>
    </div>
  </div>
</template>
