<script setup lang="ts">
// Карточка сотрудника (v-one в reference/employee/page_template.html): вывод, показатели против медианы роли,
// графики по месяцам, таблица месяцев, задачи года, журнал отметок.
import { type Col, esc } from '~/utils/sprints'
import { type Alog, type Card, type EMP, type Sum, type Task, IX, bars, empMeta, fmtD, hm, mlab, roleMedian, roleSize, summarize } from '~/utils/employee'

const route = useRoute()
const id = Number(route.params.id)
const { apiBase, jiraBase } = useRuntimeConfig().public
const JB = jiraBase.replace(/\/$/, '') + '/'
const { me } = useMe()
const [{ data: E, error: e1 }, { data: C, error: e2 }] = await Promise.all([
  useFetch<EMP>(`${apiBase}/employees`),
  useFetch<Card>(`${apiBase}/employees/${id}`),
])
const err = computed(() => e1.value || e2.value)
const p = computed(() => E.value?.ppl.find(x => x.id === id))
const sums = computed(() => new Map<number, Sum>((E.value?.ppl || []).map(x => [x.id, summarize(E.value!, x)])))
const s = computed(() => p.value ? sums.value.get(p.value.id)! : null)
const MONS = computed(() => E.value?.mon || [])
const year = computed(() => E.value?.stamp.slice(0, 4) || '')
const showTasks = computed(() => !!E.value?.tasks)
const canList = computed(() => me.value?.role !== 'employee')

const jq = (q: string) => JB + 'issues/?jql=' + encodeURIComponent(q)
const chips = computed(() => (p.value?.proj || []).slice(0, 12))

// Показатели против медианы роли.
const kpis = computed(() => {
  if (!p.value || !s.value) return []
  const S = s.value, P = p.value
  const K: [string, number | null, keyof Sum, string, boolean, boolean][] = [
    ['Закрыто задач', S.done, 'done', '', false, true], ['В месяц (с первой активности)', +S.perM.toFixed(1), 'perM', '', false, true],
    ['Выполнение плана спринта', S.planPct, 'planPct', '%', false, true], ['Возвраты на доработку', S.rwPct, 'rwPct', '%', true, true],
    ['Медиана цикла', S.cyc == null ? null : Math.round(S.cyc * 10) / 10, 'cyc', ' дн.', true, true], ['Переносы между спринтами', S.carryPct, 'carryPct', '%', true, true],
    ['Опозданий', S.late, 'late', '', true, false], ['Пропущено времени', S.mins, 'mins', '', true, false],
  ]
  return K.filter(k => showTasks.value || !k[5]).map(([l, v, f, u, inv]) => {
    const m = roleMedian(E.value!, sums.value, P, f)
    let c = 'sub', t = ''
    if (v != null && m != null && m !== 0) { const better = inv ? v < m : v > m, far = inv ? v > m * 1.3 : v < m * 0.7; c = better ? 'good' : far ? 'hi' : 'sub'; t = `медиана роли ${f === 'mins' ? hm(Math.round(m)) : Math.round(m * 10) / 10 + (u || '')}` }
    else if (m === 0 && v) { c = inv ? 'hi' : 'good'; t = 'у роли обычно 0' }
    return { l, val: v == null ? '—' : f === 'mins' ? (v ? hm(v) : '0') : v + (u || ''), c, t }
  })
})

// Вывод.
const verdict = computed(() => {
  if (!p.value || !s.value) return []
  const S = s.value, P = p.value, V: [string, string][] = []
  if (showTasks.value) {
    const r3 = S.r3, rp = S.rPrev
    if (S.done === 0 && !S.plan) V.push(['', `В ${year.value} году закрытых задач в Jira нет — сотрудник не ведёт задачи в Jira или работает под другим исполнителем.`])
    else {
      V.push([r3 < rp * 0.7 ? 'bad' : r3 > rp * 1.3 ? 'ok' : '', `За последние 3 месяца закрывает ${r3.toFixed(1)} задач в месяц, раньше в среднем ${rp.toFixed(1)} — ${r3 < rp * 0.7 ? 'заметный спад' : r3 > rp * 1.3 ? 'рост' : 'без больших изменений'}.`])
      const rm = roleMedian(E.value!, sums.value, P, 'perM')
      if (rm) V.push([S.perM < rm * 0.7 ? 'warn' : S.perM > rm * 1.3 ? 'ok' : '', `Темп ${S.perM.toFixed(1)} задач в месяц против медианы роли ${rm.toFixed(1)}.`])
      if (S.planPct != null) V.push([S.planPct < 50 ? 'bad' : S.planPct < 70 ? 'warn' : 'ok', `Закрывает в срок спринта ${S.planPct}% своих задач (${S.pd} из ${S.plan}).`])
      if (S.rwPct != null && S.done >= 5) V.push([S.rwPct >= 30 ? 'bad' : S.rwPct >= 15 ? 'warn' : 'ok', `Возвратов на доработку: ${S.rw} (${S.rwPct} на 100 закрытых задач).`])
    }
  }
  if (S.late || S.off || S.day) {
    const lm = S.l3, lp = S.lPrev
    V.push([lm >= 4 ? 'bad' : lm >= 1.5 ? 'warn' : 'ok', `Опозданий за год: ${S.late}; за последние 3 месяца в среднем ${lm.toFixed(1)} в месяц (раньше ${lp.toFixed(1)}) — ${lm > lp * 1.3 && lm >= 1 ? 'опаздывает чаще' : lm < lp * 0.7 ? 'опаздывает реже' : 'примерно так же'}.`])
    const rl = roleMedian(E.value!, sums.value, P, 'late')
    if (rl != null) V.push([S.late > rl * 1.5 && S.late > 3 ? 'warn' : '', `Медиана опозданий по роли за год — ${Math.round(rl)}.`])
    V.push(['', `Отпрашивался ${S.off + S.day} раз (из них целый день — ${S.day}), всего пропущено ${hm(S.mins)}; больничных по сообщениям — ${S.sick}.`])
  } else V.push(['ok', `Опозданий и пропусков в группе «Adata Group» за ${year.value} год нет.`])
  return V
})

// Графики — ширина по контейнеру, перерисовка при ресайзе.
const chW = ref(520)
const chEl = ref<HTMLElement | null>(null)
function measure() { const c = chEl.value?.firstElementChild as HTMLElement | null; if (c) chW.value = c.clientWidth - 28 || 520 }
onMounted(() => { measure(); addEventListener('resize', measure) })
onBeforeUnmount(() => removeEventListener('resize', measure))
watch(chEl, measure)
const rowsM = computed(() => MONS.value.map(m => p.value?.m[m] || []) as number[][])
const chWork = computed(() => bars(MONS.value, chW.value, [{ l: 'закрыто', c: 'var(--c-done)', v: rowsM.value.map(r => r[IX.done] || 0) }],
  { c: 'var(--c-plan)', v: rowsM.value.map(r => r[IX.plan] ? Math.round(r[IX.planDone] * 100 / r[IX.plan]) : null) }, 'Закрыто задач по месяцам'))
const chAtt = computed(() => bars(MONS.value, chW.value, [{ l: 'опоздания', c: 'var(--c-late)', v: rowsM.value.map(r => r[IX.late] || 0) },
  { l: 'отпрашивался', c: 'var(--c-off)', v: rowsM.value.map(r => (r[IX.early] || 0) + (r[IX.mid] || 0) + (r[IX.day] || 0)) }], null, 'Опоздания по месяцам'))

type MR = { m: string; r: (number | null)[] }
const monRows = computed<MR[]>(() => MONS.value.map(m => ({ m, r: p.value?.m[m] || [] })))
const n0 = (v: number | null | undefined) => (v as number) || 0
const monCols = computed<Col<MR>[]>(() => {
  const t: Col<MR>[] = [
    { h: 'Закрыто', v: x => n0(x.r[IX.done]), num: true },
    { h: 'Ошибок', v: x => n0(x.r[IX.bug]), num: true },
    { h: 'План спринта', v: x => x.r[IX.plan] ? n0(x.r[IX.planDone]) / n0(x.r[IX.plan]) : -1, num: true, f: x => x.r[IX.plan] ? `${Math.round(n0(x.r[IX.planDone]) * 100 / n0(x.r[IX.plan]))}% <span class="sub">${x.r[IX.planDone]}/${x.r[IX.plan]}</span>` : '—' },
    { h: 'Возвраты', v: x => n0(x.r[IX.rw]), num: true },
    { h: 'Переносы', v: x => n0(x.r[IX.carry]), num: true },
    { h: 'Цикл, дн.', v: x => x.r[IX.cyc] ?? -1, num: true, f: x => x.r[IX.cyc] == null ? '—' : String(x.r[IX.cyc]) },
    { h: 'Часы', v: x => n0(x.r[IX.h]), num: true },
    { h: 'Создал задач', v: x => n0(x.r[IX.rep]), num: true },
    { h: 'Протестировал', v: x => n0(x.r[IX.qa]), num: true },
  ]
  const a: Col<MR>[] = [
    { h: 'Опоздания', v: x => n0(x.r[IX.late]), num: true, f: x => x.r[IX.late] ? `<span class="mid">${x.r[IX.late]}</span>` : '0' },
    { h: 'Ушёл раньше', v: x => n0(x.r[IX.early]), num: true },
    { h: 'Отлучки', v: x => n0(x.r[IX.mid]), num: true },
    { h: 'Целый день', v: x => n0(x.r[IX.day]), num: true },
    { h: 'Больничный', v: x => n0(x.r[IX.sick]), num: true },
    { h: 'Пропущено', v: x => n0(x.r[IX.min]), num: true, f: x => x.r[IX.min] ? hm(n0(x.r[IX.min])) : '—' },
  ]
  return [{ h: 'Месяц', v: x => x.m, f: x => mlab(x.m) + ' ' + x.m.slice(0, 4) }, ...(showTasks.value ? t : []), ...a]
})

// Задачи.
const QT = ref('')
const FT = ref('')
const tasks = computed(() => {
  const q = QT.value.toLowerCase(), f = FT.value
  return (C.value?.t || []).filter(t => (!q || (t[0] + ' ' + t[1]).toLowerCase().includes(q)) &&
    (!f || (f === 'done' ? !!t[5] : f === 'open' ? !t[5] : f === 'rw' ? t[7] > 0 : f === 'carry' ? t[6] > 1 : t[2] === 'Ошибка')))
})
const tCols: Col<Task>[] = [
  { h: 'Задача', v: t => t[0], f: t => `<a class="key" href="${JB}browse/${esc(t[0])}" target="_blank" rel="noopener">${esc(t[0])}</a>` },
  { h: 'Название', v: t => t[1], cls: 'wrap' },
  { h: 'Тип', v: t => t[2] },
  { h: 'Статус', v: t => t[3], f: t => `<span class="pill ${t[5] ? 'ok' : 'mute'}">${esc(t[3])}</span>` },
  { h: 'Создана', v: t => t[4], f: t => fmtD(t[4]) },
  { h: 'Закрыта', v: t => t[5] || '', f: t => fmtD(t[5]) },
  { h: 'Спринтов', v: t => t[6], num: true, f: t => t[6] > 1 ? `<span class="mid">${t[6]}</span>` : String(t[6]) },
  { h: 'Возвратов', v: t => t[7], num: true, f: t => t[7] ? `<span class="hi">${t[7]}</span>` : '0' },
  { h: 'Цикл, дн.', v: t => t[8] ?? -1e9, num: true, f: t => t[8] == null ? '—' : String(t[8]) },
  { h: 'Часы', v: t => t[9], num: true, f: t => t[9] ? String(t[9]) : '—' },
]
const aCols: Col<Alog>[] = [
  { h: 'Дата', v: a => a[0], f: a => fmtD(a[0]) + '.' + a[0].slice(0, 4) },
  { h: 'Что', v: a => a[1], f: a => `<span class="pill ${a[1] === 'Опоздание' ? 'warn' : a[1] === 'Весь день' || a[1] === 'Больничный' ? 'bad' : 'mute'}">${esc(a[1])}</span>` },
  { h: 'Время', v: a => a[2], num: true, f: a => a[2] ? hm(a[2]) : '—' },
  { h: 'Подробности', v: a => a[3], cls: 'wrap' },
]
useHead({ title: computed(() => p.value ? `${p.value.n} — Сотрудник Adata` : 'Сотрудник Adata') })
</script>

<template>
  <div class="emp">
    <header>
      <h1>Сотрудник Adata</h1>
      <div style="display:flex;gap:10px;flex-wrap:wrap;align-items:baseline">
        <span v-if="E" class="meta">{{ empMeta(E) }}</span>
        <NuxtLink v-if="canList" class="xlink" to="/employee">← Все сотрудники</NuxtLink>
      </div>
    </header>
    <div v-if="err" class="notice">{{ err.statusCode === 404 ? 'Сотрудник не найден или у вас нет доступа к его карточке.' : `Не удалось получить данные: ${err.statusCode ? 'HTTP ' + err.statusCode : err.message}` }}</div>
    <p v-else-if="!E || !C" class="sub">Загружаю…</p>
    <div v-else-if="!p || !s" class="notice">Сотрудник не найден или у вас нет доступа к его карточке.</div>
    <template v-else>
      <div v-if="showTasks && !E.history" class="notice">История задач Jira за год ещё загружается — задачные показатели пока неполные.</div>
      <section>
        <div class="card prof">
          <div class="who">
            <b>{{ p.n }}</b>
            <span class="sub">{{ p.pos || 'роль в Jira не указана' }}{{ p.tl ? ' · тимлид' : '' }}{{ p.lead ? ' · руководство' : '' }}{{ p.login ? ' · ' + p.login : '' }}</span>
            <div v-if="showTasks" class="chips">
              <a v-for="[k, c] in chips" :key="k" class="chip" target="_blank" rel="noopener" :title="`${c} задач в ${year}`"
                 :href="jq(`project = ${k} AND assignee = &quot;${p.login || p.n}&quot; AND updated >= ${year}-01-01 ORDER BY updated DESC`)">{{ k }} <span class="sub">{{ c }}</span></a>
            </div>
          </div>
          <div style="display:flex;flex-direction:column;gap:6px;align-items:flex-end">
            <a v-if="showTasks && p.login" class="xlink" target="_blank" rel="noopener" :href="jq(`assignee = &quot;${p.login}&quot; AND statusCategory != Done ORDER BY updated DESC`)">Открытые задачи в Jira: {{ s.open }}</a>
          </div>
        </div>
      </section>
      <section>
        <h2>Вывод</h2>
        <ul class="verdict card"><li v-for="([c, t], i) in verdict" :key="i" :class="c">{{ t }}</li></ul>
      </section>
      <section>
        <h2>Показатели за {{ year }} год</h2>
        <p class="lead">Сравнение с медианой роли «{{ p.pos || '—' }}» ({{ roleSize(E, p) }} чел.). Зелёным — лучше медианы, красным — заметно хуже.</p>
        <div class="kpis">
          <div v-for="k in kpis" :key="k.l" class="card kpi"><span class="sub">{{ k.l }}</span><b>{{ k.val }}</b><span class="cmp" :class="k.c">{{ k.t || '&nbsp;' }}</span></div>
        </div>
      </section>
      <section>
        <h2>Динамика по месяцам</h2>
        <div ref="chEl" class="charts">
          <div v-if="showTasks" class="card"><h3>Закрыто задач и выполнение плана спринта</h3>
            <div class="legend"><span><i style="background:var(--c-done)" />Закрыто задач</span><span><i style="background:var(--c-plan)" />План спринта, %</span></div>
            <div v-html="chWork" />
          </div>
          <div class="card"><h3>Опоздания и отсутствия</h3>
            <div class="legend"><span><i style="background:var(--c-late)" />Опоздания</span><span><i style="background:var(--c-off)" />Уходы, отлучки, целый день</span></div>
            <div v-html="chAtt" />
          </div>
        </div>
        <SortTable :cols="monCols" :rows="monRows" :init="[0, 'asc']" />
      </section>
      <section v-if="showTasks">
        <h2>Задачи {{ year }}</h2>
        <div class="tools" style="border:0;padding:0">
          <input v-model="QT" type="search" placeholder="Поиск по задаче или проекту" aria-label="Поиск по задачам">
          <select v-model="FT" aria-label="Показать">
            <option value="">Все задачи</option><option value="done">Закрытые</option><option value="open">Открытые</option>
            <option value="rw">Возвращались на доработку</option><option value="carry">Переносились между спринтами</option><option value="bug">Ошибки</option>
          </select>
          <span class="sub">{{ tasks.length }} задач</span>
        </div>
        <SortTable :cols="tCols" :rows="tasks" :init="[5, 'desc']" empty="Ничего не найдено" />
      </section>
      <section>
        <h2>Посещаемость: все отметки</h2>
        <SortTable :cols="aCols" :rows="C.alog" :init="[0, 'desc']" empty="Отметок нет" />
      </section>
      <section>
        <h2>Как считается</h2>
        <p class="lead">Задачи {{ year }} — все задачи Jira, обновлявшиеся с 01.01.{{ year }}; закрытая задача засчитывается текущему исполнителю в месяц закрытия. «Возвраты» — переходы в Need Rework, засчитываются тому, кто был исполнителем в момент возврата. «Цикл» — медиана дней от начала работы до закрытия. «Переносы» — закрытые задачи, побывавшие больше чем в одном спринте. Опоздание — пропуск, начинающийся в 9:00; «целый день» — пропуск на весь рабочий день; больничные и отпуска по сообщениям в группе. Сравнение — с медианой сотрудников той же роли.</p>
      </section>
    </template>
  </div>
</template>
