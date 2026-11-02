<script setup lang="ts">
// Снепшоты к созвонам (reference/snapshot/sprint_port.js): по проектам — ход спринта по дням (срез 18:00),
// возвраты, переназначения, бэклог вне спринта; по людям — простой. Кнопка замораживает расчёт на сегодня.
import { type Col, esc } from '~/utils/sprints'

type Proj = {
  key: string; name: string; lead: string; sprint: string; start?: string; end?: string; total: number; dn: number
  ended: boolean; overdue: number; time: number; ready: number; n_issues: number; rw_ev: number; gave: number
  ser: [string, number, number, number, number, number][]; bl_start: [number, number, number]; bl_now: [number, number, number]
}
type Person = {
  name: string; pos: string; main: string; sprint: string; from: string; to: string; days: number; idle: number; none: number; quiet: number; cur: number
  t: number; dn: number; todo: number; wk: number; rw: number; ts_cr: number; left: number; abs_now: boolean; projects: string[]
}
type Snap = { today: string; hist_end: string; cur: Record<string, Proj>; people: Person[]; unas: Record<string, [number, number]> }
type Info = { taken_for: string; history_end: string; note: string; created_by: string; created_at: string }

const { apiBase, jiraBase } = useRuntimeConfig().public
const JB = jiraBase.replace(/\/$/, '') + '/'
const route = useRoute()
const sel = computed(() => typeof route.query.date === 'string' ? route.query.date : '')

const { data: list, refresh: refreshList } = await useFetch<Info[]>(`${apiBase}/snapshots`)
const { data, error, pending, refresh } = await useFetch<{ info: Info | null; snapshot: Snap }>(
  () => `${apiBase}/snapshots/${sel.value || 'live'}`, { watch: [sel] })
const S = computed(() => data.value?.snapshot)
const info = computed(() => data.value?.info)

// ---------- кнопка ----------
const note = ref('')
const busy = ref(false)
const msg = ref('')
const todayIso = () => { const d = new Date(); return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0') }
const savedToday = computed(() => (list.value || []).some(x => x.taken_for === todayIso()))
async function create() {
  busy.value = true; msg.value = ''
  try {
    const r = await $fetch<{ date: string }>(`${apiBase}/snapshots`, { method: 'POST', body: { note: note.value.trim() } })
    note.value = ''
    await refreshList()
    await navigateTo({ query: { date: r.date } })
    msg.value = `Снепшот на ${fmt(r.date)} сохранён`
  } catch (e: any) {
    msg.value = e?.data?.error || (e?.statusCode ? `Ошибка ${e.statusCode}` : 'Сервис недоступен')
  } finally { busy.value = false }
}

// ---------- помощники ----------
const D = (s: string) => new Date(s + 'T00:00:00')
const WD = ['вс', 'пн', 'вт', 'ср', 'чт', 'пт', 'сб']
const fmt = (s?: string) => s ? String(D(s).getDate()).padStart(2, '0') + '.' + String(D(s).getMonth() + 1).padStart(2, '0') : '—'
const fmtW = (s: string) => WD[D(s).getDay()] + ' ' + fmt(s)
const projects = computed(() => Object.values(S.value?.cur || {}).filter(p => p.start).sort((a, b) => a.key.localeCompare(b.key)))

// Ход спринта: столбики по дням (todo у руководителя / без исполнителя / у других / в работе / готово).
const SER = [
  { l: 'В бэклоге у руководителя', c: 'var(--warn)' }, { l: 'В бэклоге без исполнителя', c: 'var(--bad)' },
  { l: 'В бэклоге у исполнителей', c: 'var(--muted)' }, { l: 'В работе, ревью, тесте', c: 'var(--accent)' }, { l: 'Готово', c: 'var(--ok)' },
]
function chart(p: Proj) {
  const W = 420, H = 150, L = 26, R = 6, T = 8, B = 20, iw = W - L - R, ih = H - T - B, n = Math.max(1, p.ser.length), bw = iw / n
  const mx = Math.max(1, ...p.ser.map(r => r[1] + r[2] + r[3] + r[4] + r[5]))
  let g = `<line x1="${L}" x2="${W - R}" y1="${T + ih}" y2="${T + ih}" stroke="var(--line)"/><text x="${L - 4}" y="${T + 9}" text-anchor="end">${mx}</text>`
  p.ser.forEach((r, i) => {
    let acc = 0
    const x = L + i * bw + bw * 0.15
    for (let k = 1; k <= 5; k++) {
      const v = r[k] as number; if (!v) continue
      const h = v / mx * ih
      g += `<rect x="${x.toFixed(1)}" y="${(T + ih - (acc + v) / mx * ih).toFixed(1)}" width="${(bw * 0.7).toFixed(1)}" height="${h.toFixed(1)}" fill="${SER[k - 1].c}"><title>${fmtW(r[0])}: ${SER[k - 1].l.toLowerCase()} — ${v}</title></rect>`
      acc += v
    }
    if (n <= 12 || i % 2 === 0) g += `<text x="${(L + i * bw + bw / 2).toFixed(1)}" y="${H - 6}" text-anchor="middle">${fmt(r[0]).slice(0, 2)}</text>`
  })
  return `<svg width="100%" viewBox="0 0 ${W} ${H}" role="img" aria-label="Ход спринта ${esc(p.key)} по дням">${g}</svg>`
}

const pCols: Col<Proj>[] = [
  { h: 'Проект', v: p => p.key, f: p => `<b>${esc(p.key)}</b><div class="sub">${esc(p.name)}</div>` },
  { h: 'Руководитель', v: p => p.lead, f: p => esc(p.lead) },
  { h: 'Спринт', v: p => p.sprint, f: p => `${esc(p.sprint)}<div class="sub">${fmt(p.start)} — ${fmt(p.end)}</div>` },
  { h: 'Время', v: p => p.time, num: true, tip: 'Сколько времени спринта прошло', f: p => p.ended ? `<span class="hi">просрочен ${p.overdue} дн.</span>` : p.time + '%' },
  { h: 'Готово', v: p => p.ready, num: true, tip: 'Доля готовых задач спринта', f: p => `<span class="${p.ready + 20 < p.time ? 'hi' : p.ready < p.time ? 'mid' : ''}">${p.ready}%</span>` },
  { h: 'Задач', v: p => p.n_issues, num: true },
  { h: 'Возвраты', v: p => p.rw_ev, num: true, tip: 'Задачи спринта, уходившие в Need Rework после старта', f: p => p.rw_ev ? `<span class="mid">${p.rw_ev}</span>` : '0' },
  { h: 'Отдал с себя', v: p => p.gave, num: true, tip: 'Сколько раз руководитель переназначил задачу спринта с себя на другого после старта' },
  { h: 'Бэклог вне спринта', v: p => p.bl_now[0], num: true, tip: 'Открытые задачи не в спринте и не запланированные на ближайшие 14 дней: на старте → сейчас (у руководителя / без исполнителя)',
    f: p => `${p.bl_start[0]} → <b class="${p.bl_now[0] > p.bl_start[0] ? 'mid' : ''}">${p.bl_now[0]}</b><div class="sub">рук. ${p.bl_now[1]} · без исп. ${p.bl_now[2]}</div>` },
  { h: 'Без исполнителя', v: p => S.value?.unas[p.key]?.[0] || 0, num: true, tip: 'Задачи активного спринта без исполнителя (из них в бэклоге)',
    f: p => { const u = S.value?.unas[p.key]; return u ? `${u[0]} <span class="sub">(${u[1]})</span>` : '0' } },
]
const jq = (n: string) => JB + 'issues/?jql=' + encodeURIComponent(`cf[10104] in openSprints() AND assignee = "${n}" ORDER BY status ASC`)
const peCols: Col<Person>[] = [
  { h: 'Сотрудник', v: p => p.name, f: p => `<b>${esc(p.name)}</b>${p.abs_now ? ' <span class="pill mute">отсутствует</span>' : ''}<div class="sub">${esc(p.pos || 'роль не указана')}</div>` },
  { h: 'Проект', v: p => p.main, f: p => `${esc(p.main || '—')}<div class="sub">${esc(p.sprint)}</div>` },
  { h: 'Простой, дн.', v: p => p.idle, num: true, tip: 'Рабочие дни без отсутствий, когда на нём не было открытой задачи в спринте вне статусов ожидания (Test…Done)',
    f: p => `<span class="${p.idle >= 3 ? 'hi' : p.idle ? 'mid' : ''}">${p.idle}</span> <span class="sub">из ${p.days}</span>` },
  { h: 'Подряд сейчас', v: p => p.cur, num: true, tip: 'Текущая серия дней простоя', f: p => p.cur ? `<span class="hi">${p.cur}</span>` : '0' },
  { h: 'Без задач', v: p => p.none, num: true, tip: 'Из дней простоя — вообще без открытых задач' },
  { h: 'Без активности', v: p => p.quiet, num: true, tip: 'Из дней простоя — без изменений в Jira' },
  { h: 'Осталось задач', v: p => p.left, num: true, tip: 'Бэклог + в работе + реворк в открытых спринтах',
    f: p => p.left ? `<a href="${jq(p.name)}" target="_blank" rel="noopener">${p.left}</a>` : '<span class="hi">0</span>' },
  { h: 'Бэклог', v: p => p.todo, num: true },
  { h: 'В работе', v: p => p.wk, num: true },
  { h: 'Реворк', v: p => p.rw, num: true },
  { h: 'Ревью и тест', v: p => p.ts_cr, num: true },
  { h: 'Готово', v: p => p.dn, num: true },
  { h: 'Окно', v: p => p.from, f: p => `<span class="sub">${fmt(p.from)} — ${fmt(p.to)}</span>` },
]
const idlePeople = computed(() => (S.value?.people || []))
const kpis = computed(() => {
  const ps = projects.value, pe = idlePeople.value
  return [
    [ps.length, 'спринтов', ''], [ps.filter(p => p.ended).length, 'просрочено', 'bad'], [ps.filter(p => !p.ended && p.ready + 20 < p.time).length, 'отстают от времени', 'warn'],
    [pe.filter(p => p.cur > 0).length, 'сейчас в простое', 'bad'], [pe.filter(p => !p.left).length, 'без задач в спринте', 'warn'],
  ] as [number, string, string][]
})
</script>

<template>
  <div>
    <header>
      <h1>Снепшоты к созвонам</h1>
      <span v-if="S" class="meta">{{ info ? `Снепшот на ${fmtW(info.taken_for)}` : 'Живой расчёт' }} · история по {{ fmtW(S.hist_end) }}, срез 18:00</span>
    </header>
    <p class="lead">Ход каждого спринта по дням, возвраты, переназначения и бэклог вне спринта, плюс простой разработчиков. Сформированный снепшот больше не пересчитывается — его можно открыть на созвоне и через неделю сравнить.</p>

    <div class="tools">
      <input v-model="note" type="search" placeholder="Комментарий к снепшоту (необязательно)" aria-label="Комментарий" maxlength="200" :disabled="savedToday">
      <button type="button" class="btn primary" :disabled="busy || savedToday" @click="create">{{ busy ? 'Формирую…' : savedToday ? 'На сегодня уже сформирован' : 'Сформировать снепшот' }}</button>
      <span v-if="msg" class="sub" role="status">{{ msg }}</span>
      <span style="flex:1" />
      <label>Показать
        <select :value="sel" aria-label="Снепшот" @change="navigateTo({ query: ($event.target as HTMLSelectElement).value ? { date: ($event.target as HTMLSelectElement).value } : {} })">
          <option value="">Сейчас (живой расчёт)</option>
          <option v-for="s in list || []" :key="s.taken_for" :value="s.taken_for">{{ fmtW(s.taken_for) }}{{ s.note ? ' — ' + s.note : '' }}</option>
        </select>
      </label>
      <button v-if="!sel" type="button" class="btn" :disabled="pending" @click="refresh()">Обновить</button>
    </div>
    <p v-if="info" class="sub">Сформировал {{ info.created_by || 'администратор' }} · {{ new Date(info.created_at).toLocaleString('ru-RU', { timeZone: 'Asia/Almaty' }) }}{{ info.note ? ' · ' + info.note : '' }}</p>

    <p v-if="pending" class="sub">Считаю… (по истории задач за год, несколько секунд)</p>
    <div v-else-if="error" class="notice">{{ (error.data as { error?: string } | undefined)?.error || (error.statusCode ? `HTTP ${error.statusCode}` : error.message) }}</div>
    <template v-else-if="S">
      <div class="kpis"><div v-for="[v, l, c] in kpis" :key="l" class="kpi" :class="c"><b>{{ v }}</b><span>{{ l }}</span></div></div>

      <section>
        <h2>Спринты</h2>
        <SortTable :cols="pCols" :rows="projects" :init="[0, 'asc']" empty="Активных спринтов нет" />
      </section>

      <section>
        <h2>Ход спринтов по дням</h2>
        <div class="legend"><span v-for="s in SER" :key="s.l"><i :style="{ background: s.c }" />{{ s.l }}</span></div>
        <div class="snap-grid">
          <div v-for="p in projects" :key="p.key" class="card">
            <h3>{{ p.key }} · {{ p.sprint }} <span class="sub">{{ fmt(p.start) }} — {{ fmt(p.end) }} · {{ p.lead }}</span></h3>
            <div v-if="p.ser.length" class="snap-chart" v-html="chart(p)" />
            <p v-else class="sub">Нет рабочих дней с данными.</p>
          </div>
        </div>
      </section>

      <section>
        <h2>Простой разработчиков</h2>
        <p class="lead">Окно — с начала спринта основного проекта (по задачам в открытых спринтах). Руководители, ПМ и тестировщики не входят. Дни отпусков, больничных и отгулов не считаются.</p>
        <SortTable :cols="peCols" :rows="idlePeople" :init="[2, 'desc']" empty="Простоя нет" />
      </section>
    </template>
  </div>
</template>

<style>
.snap-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(min(100%,380px),1fr));gap:12px}
.snap-grid .card{padding:12px 14px;gap:6px}
.snap-grid h3{font-size:13px;margin:0}
.snap-chart svg text{fill:var(--muted);font:10px var(--f-body)}
</style>
