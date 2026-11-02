<script setup lang="ts">
// «Сотрудник Adata» — список (v-list в reference/employee/page_template.html). Клик по строке — карточка.
import { type Col, esc } from '~/utils/sprints'
import { type EMP, type EmpRow, type Sum, IX, active, cls, empMeta, hm, spark, summarize } from '~/utils/employee'

const { apiBase } = useRuntimeConfig().public
const { data: E, error, pending } = await useFetch<EMP>(`${apiBase}/employees`)

const Q = ref('')
const POS = ref('')
const ACT = ref(true)

const sums = computed(() => new Map((E.value?.ppl || []).map(p => [p.id, summarize(E.value!, p)])))
// Сотрудник видит только себя — сразу в карточку.
watch(E, e => { if (e && e.ppl.length === 1 && useMe().me.value?.role === 'employee') navigateTo(`/employee/${e.ppl[0].id}`, { replace: true }) }, { immediate: true })

const roles = computed(() => [...new Set((E.value?.ppl || []).map(p => p.pos || '—'))].sort())
type R = { p: EmpRow; s: Sum }
const rows = computed<R[]>(() => {
  const q = Q.value.trim().toLowerCase()
  return (E.value?.ppl || []).filter(p => (!q || p.n.toLowerCase().includes(q)) && (!POS.value || (p.pos || '—') === POS.value) && (!ACT.value || active(sums.value.get(p.id)!)))
    .map(p => ({ p, s: sums.value.get(p.id)! }))
})
const NIL = -1e9 // пустые значения — в конец при сортировке по убыванию, как в шаблоне
const cols = computed<Col<R>[]>(() => {
  const MONS = E.value!.mon
  const all: (Col<R> & { task?: boolean })[] = [
    { h: 'Сотрудник', v: x => x.p.n, f: x => `<b>${esc(x.p.n)}</b>${x.p.lead ? ' <span class="pill mute">рук.</span>' : ''}${x.p.tl ? ' <span class="pill mute">тимлид</span>' : ''}<div class="sub">${esc(x.p.pos || 'роль не указана')}</div>` },
    { h: 'Закрыто за год', v: x => x.s.done, num: true, task: true },
    { h: 'Динамика', v: x => x.s.r3 - x.s.rPrev, tip: 'Закрыто задач по месяцам', task: true, f: x => spark(MONS.map(m => (x.p.m[m]?.[IX.done] as number) || 0)) },
    { h: 'В месяц, последние 3', v: x => x.s.r3, num: true, task: true, f: x => x.s.r3.toFixed(1) + (x.s.rPrev ? ` <span class="${x.s.r3 < x.s.rPrev * 0.7 ? 'hi' : x.s.r3 > x.s.rPrev * 1.3 ? 'good' : 'sub'}">${x.s.r3 >= x.s.rPrev ? '↑' : '↓'}</span>` : '') },
    { h: 'План спринта', v: x => x.s.planPct ?? NIL, num: true, task: true, tip: 'Доля задач в закрытых спринтах, закрытых в срок спринта', f: x => x.s.planPct == null ? '—' : `<span class="${cls(x.s.planPct, 70, 50)}">${x.s.planPct}%</span>` },
    { h: 'Возвраты', v: x => x.s.rwPct ?? NIL, num: true, task: true, tip: 'Возвратов на доработку на 100 закрытых задач', f: x => x.s.rwPct == null ? '—' : `<span class="${cls(x.s.rwPct, 15, 30, true)}">${x.s.rwPct}%</span>` },
    { h: 'Цикл, дн.', v: x => x.s.cyc ?? NIL, num: true, task: true, f: x => x.s.cyc == null ? '—' : String(Math.round(x.s.cyc * 10) / 10) },
    { h: 'Опозданий', v: x => x.s.late, num: true, f: x => x.s.late ? `<span class="${cls(x.s.l3, 1, 4, true)}">${x.s.late}</span>` : '0' },
    { h: 'Опозд. в мес., посл. 3', v: x => x.s.l3, num: true, f: x => x.s.l3.toFixed(1) + (x.s.lPrev || x.s.l3 ? ` <span class="${x.s.l3 > x.s.lPrev * 1.3 && x.s.l3 >= 1 ? 'hi' : x.s.l3 < x.s.lPrev * 0.7 ? 'good' : 'sub'}">${x.s.l3 > x.s.lPrev ? '↑' : x.s.l3 < x.s.lPrev ? '↓' : '='}</span>` : '') },
    { h: 'Отпросился', v: x => x.s.off + x.s.day, num: true, tip: 'Уходы раньше, отлучки и целые дни', f: x => (x.s.off + x.s.day) + (x.s.day ? ` <span class="sub">(${x.s.day} дн.)</span>` : '') },
    { h: 'Пропущено', v: x => x.s.mins, num: true, f: x => x.s.mins ? hm(x.s.mins) : '—' },
    { h: 'Больничные', v: x => x.s.sick, num: true },
    { h: 'Открыто сейчас', v: x => x.s.open, num: true, task: true },
  ]
  return all.filter(c => E.value!.tasks || !c.task)
})
const year = computed(() => E.value?.stamp.slice(0, 4) || '')
</script>

<template>
  <div class="emp">
    <header>
      <h1>Сотрудник Adata</h1>
      <span v-if="E" class="meta">{{ empMeta(E) }}</span>
    </header>
    <p v-if="pending" class="sub">Загружаю…</p>
    <div v-else-if="error" class="notice">Не удалось получить данные: {{ error.statusCode ? `HTTP ${error.statusCode}` : error.message }}</div>
    <template v-else-if="E">
      <div v-if="E.tasks && !E.history" class="notice">История задач Jira за год ещё загружается (первая загрузка — около 20 минут). Пока задачные показатели неполные.</div>
      <div class="tools">
        <input v-model="Q" type="search" placeholder="Найти сотрудника" aria-label="Сотрудник">
        <label>Роль <select v-model="POS"><option value="">Все роли</option><option v-for="r in roles" :key="r">{{ r }}</option></select></label>
        <label><input v-model="ACT" type="checkbox"> Только с задачами или отметками в {{ year }}</label>
      </div>
      <section>
        <h2>Все сотрудники за {{ year }} год</h2>
        <p class="lead">
          <template v-if="E.tasks">Задачи — из Jira (закрытые в {{ year }}, по текущему исполнителю), посещаемость — из группы «Adata Group». </template>
          <template v-else>Посещаемость — из группы «Adata Group»; задачи Jira для вашей роли не показываются. </template>
          Нажмите на строку, чтобы открыть карточку сотрудника.<template v-if="E.tasks"> «План спринта» — какая доля задач сотрудника в закрытых спринтах была закрыта в срок спринта.</template>
        </p>
        <SortTable :cols="cols" :rows="rows" :init="[E.tasks ? 1 : 7, 'desc']" :row="(x: R) => navigateTo(`/employee/${x.p.id}`)" empty="Ничего не найдено" />
      </section>
    </template>
  </div>
</template>
