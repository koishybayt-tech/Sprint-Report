<script setup lang="ts">
// Отчёт «Спринты» — повторяет reference/sprints/page_template.html. Пока только вкладка «Статус спринтов».
type Oldest = { key: string; age_days: number }
type Project = {
  key: string; name: string; lead: string; sprint: string; start?: string; end?: string; left_days: number | null
  total: number; todo: number; wk: number; cr: number; rw: number; ts: number; dn: number
  blocked: number; carry_open: number; people: number; ready_pct: number; oldest_open: Oldest | null
}
type Next = { project_key: string; next_id: number; next_name: string; next_total: number }

const { apiBase, jiraBase } = useRuntimeConfig().public
const JB = jiraBase.replace(/\/$/, '') + '/'
const route = useRoute()
const router = useRouter()

const { data, error, pending } = await useFetch<{ updated_at: string | null; projects: Project[] }>(`${apiBase}/sprints`)
const { data: nextData } = await useFetch<Next[]>(`${apiBase}/sprints/next`)

const all = computed(() => data.value?.projects ?? [])
const NSD = computed(() => Object.fromEntries((nextData.value ?? []).map(n => [n.project_key, n])))

// ---------- фильтр по руководителю (общий для всех вкладок, хранится в ?lead=) ----------
const lead = ref(String(route.query.lead ?? ''))
watch(lead, l => router.replace({ query: { ...route.query, lead: l || undefined } }))
const leads = computed(() => [...new Set(all.value.map(p => p.lead))].sort((a, b) => a.localeCompare(b, 'ru'))
  .map(l => ({ l, n: all.value.filter(p => p.lead === l).length })))
const scoped = computed(() => lead.value ? all.value.filter(p => p.lead === lead.value) : all.value)

// ---------- помощники как в шаблоне ----------
const fmt = (d?: string | null) => d ? `${d.slice(8, 10)}.${d.slice(5, 7)}` : '—'
const pct = (a: number, b: number) => b ? Math.round(a / b * 100) : 0
const lvl = (v: number, w: number, b: number) => v >= b ? 'hi' : v >= w ? 'mid' : ''
const sum = (f: keyof Project) => scoped.value.reduce((a, p) => a + (p[f] as number), 0)
const updated = computed(() => {
  const u = data.value?.updated_at
  if (!u) return ''
  const d = new Date(u)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(d.getDate())}.${p(d.getMonth() + 1)}.${d.getFullYear()} ${p(d.getHours())}:${p(d.getMinutes())}`
})

// ---------- ссылки в Jira ----------
const BASE = 'cf[10104] in openSprints()'
const ST = { open: 'status != 10001' }
const jq = (parts: string[]) => JB + 'issues/?jql=' + encodeURIComponent([BASE, ...parts.filter(Boolean)].join(' AND ') + ' ORDER BY status ASC, updated DESC')
const projQ = (k: string) => `project = "${k}"`
const tasksLink = (k: string) => jq([projQ(k)])
const carryLink = (k: string) => jq([projQ(k), 'cf[10104] in closedSprints()', ST.open])
const nextLink = (id: number) => `${JB}issues/?jql=${encodeURIComponent('cf[10104] = ' + id)}`

// ---------- KPI (только те, что считаются по проектам; остальные — с вкладками людей) ----------
const kpis = computed(() => {
  const P = scoped.value, T = sum('total')
  const stuck = P.filter(p => (p.left_days ?? 0) < 0).length
  const noNext = P.filter(p => !NSD.value[p.key]?.next_total)
  return [
    { v: T, l: 'задач в спринтах', c: '' },
    { v: P.length, l: 'проектов', c: '' },
    { v: pct(sum('dn'), T) + '%', l: 'готово', c: '' },
    { v: stuck, l: 'спринтов не закрыто', c: stuck ? 'bad' : '' },
    { v: noNext.length, l: 'нет следующего спринта (или пустой)', c: noNext.some(p => (p.left_days ?? 99) <= 3) ? 'bad' : '' },
    { v: sum('blocked'), l: 'в Blocked', c: 'bad' },
    { v: sum('todo'), l: 'в бэклоге', c: '' },
  ]
})

// ---------- таблица «Статус спринтов» ----------
const SEG = [['dn', 'Готово'], ['ts', 'Тестируется'], ['cr', 'Код ревью'], ['rw', 'Реворк'], ['wk', 'В работе'], ['todo', 'Бэклог']] as const
const segVar = (k: string) => `var(--s-${k === 'todo' ? 'bl' : k})`
type Col = { h: string; v: (p: Project) => string | number; num?: boolean; tip?: string; cls?: string }
const cols: Col[] = [
  { h: 'Проект', v: p => p.key },
  { h: 'Руководитель', v: p => p.lead },
  { h: 'Спринт', v: p => p.sprint },
  { h: 'Даты', v: p => p.end ?? '' },
  { h: 'Срок', v: p => p.left_days ?? Number.POSITIVE_INFINITY, num: true },
  { h: 'Следующий спринт', v: p => NSD.value[p.key] ? (NSD.value[p.key].next_total ? 2 : 1) : 0, cls: 'wrap',
    tip: 'Есть ли в Jira следующий спринт (номер +1 к текущему) и сколько в нём задач. Подробнее — вкладка «Следующий спринт»' },
  { h: 'Задач', v: p => p.total, num: true },
  { h: 'Прогресс', v: p => p.ready_pct },
  { h: 'Готово', v: p => p.ready_pct, num: true },
  { h: 'Самая старая задача', v: p => p.oldest_open?.age_days ?? 0, num: true,
    tip: 'Самая давно созданная незакрытая задача спринта: сколько дней назад создана' },
  { h: 'Переходят из спринта в спринт', v: p => p.carry_open, num: true,
    tip: 'Незакрытые задачи, которые были минимум в 2 спринтах (перенесены из закрытого спринта)' },
  { h: 'Люди', v: p => p.people, num: true },
]
const sortKey = ref(4)
const sortDir = ref<'asc' | 'desc'>('asc')
function sortBy(i: number) {
  if (sortKey.value === i) sortDir.value = sortDir.value === 'asc' ? 'desc' : 'asc'
  else { sortKey.value = i; sortDir.value = cols[i].num ? 'desc' : 'asc' }
}
const rows = computed(() => {
  const c = cols[sortKey.value], k = sortDir.value === 'asc' ? 1 : -1
  return [...scoped.value].sort((a, b) => {
    const x = c.v(a), y = c.v(b)
    return (typeof x === 'string' ? x.localeCompare(String(y), 'ru') : (x as number) - (y as number)) * k
  })
})
</script>

<template>
  <div>
    <div class="stick">
      <header>
        <h1>Спринты Adata</h1>
        <span class="meta">{{ updated ? `Данные Jira на ${updated}` : '' }}</span>
      </header>
      <div class="gbar">
        <label class="gf"><span>Руководитель проекта</span>
          <select v-model="lead">
            <option value="">Все руководители</option>
            <option v-for="o in leads" :key="o.l" :value="o.l">{{ o.l }} ({{ o.n }})</option>
          </select>
        </label>
        <span v-if="lead" class="scope">В фильтре: {{ scoped.length }} проектов · {{ sum('total') }} задач</span>
      </div>
      <nav role="tablist">
        <button role="tab" aria-selected="true">Статус спринтов</button>
      </nav>
    </div>

    <p v-if="pending" class="sub">Загружаю…</p>
    <div v-else-if="error" class="notice">Не удалось получить данные: {{ error.statusCode ? `HTTP ${error.statusCode}` : error.message }}</div>
    <div v-else-if="!data?.updated_at" class="notice">
      Данных Jira ещё нет. Запустите синхронизацию: <code>reports sync jira --full</code>
    </div>
    <template v-else>
      <div class="kpis">
        <div v-for="k in kpis" :key="k.l" class="kpi" :class="k.c"><b>{{ k.v }}</b><span>{{ k.l }}</span></div>
      </div>

      <section>
        <h2>Статус спринтов</h2>
        <p class="lead">Прогресс каждого активного спринта по статусам. Спринт, чья дата окончания прошла, но он не закрыт в Jira, отмечен «Не закрыт».</p>
        <div class="legend">
          <span v-for="[k, l] in SEG" :key="k"><i :style="{ background: segVar(k) }" />{{ l }}</span>
        </div>
        <div class="tbl">
          <table>
            <thead>
              <tr>
                <th v-for="(c, i) in cols" :key="c.h" tabindex="0" :class="{ n: c.num }" :title="c.tip"
                    :data-dir="i === sortKey ? sortDir : undefined" @click="sortBy(i)" @keydown.enter.prevent="sortBy(i)">{{ c.h }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="p in rows" :key="p.key">
                <td><b>{{ p.key }}</b> <span class="sub">{{ p.name }}</span></td>
                <td><span class="sub">{{ p.lead }}</span></td>
                <td>{{ p.sprint }}</td>
                <td>{{ fmt(p.start) }} – {{ fmt(p.end) }}</td>
                <td class="n">
                  <span v-if="p.left_days === null" class="sub">—</span>
                  <span v-else-if="p.left_days < 0" class="pill bad">Не закрыт · {{ -p.left_days }} дн.</span>
                  <span v-else-if="p.left_days <= 3" class="pill warn">{{ p.left_days }} дн.</span>
                  <span v-else class="pill mute">{{ p.left_days }} дн.</span>
                </td>
                <td class="wrap">
                  <span v-if="!NSD[p.key]" class="pill" :class="(p.left_days ?? 99) <= 3 ? 'bad' : 'mute'">не создан</span>
                  <template v-else>
                    <template v-if="NSD[p.key].next_total">
                      <span class="pill ok">есть</span>{{ ' ' }}
                      <a class="nl" :href="nextLink(NSD[p.key].next_id)"
                         target="_blank" rel="noopener" :title="NSD[p.key].next_name">{{ NSD[p.key].next_total }} задач</a>
                    </template>
                    <span v-else class="pill" :class="(p.left_days ?? 99) <= 3 ? 'bad' : 'warn'">пустой</span>
                    <div class="sub">{{ NSD[p.key].next_name }}</div>
                  </template>
                </td>
                <td class="n">
                  <a v-if="p.total" class="nl" :href="tasksLink(p.key)"
                     target="_blank" rel="noopener" title="Открыть в Jira">{{ p.total }}</a>
                  <span v-else class="sub">0</span>
                </td>
                <td>
                  <div class="bar" :title="SEG.map(([k, l]) => `${l} ${p[k]}`).join(' · ')">
                    <i v-for="[k] in SEG" :key="k" :style="{ width: `${p[k] / p.total * 100}%`, background: segVar(k) }" />
                  </div>
                </td>
                <td class="n">{{ p.ready_pct }}%</td>
                <td class="n">
                  <template v-if="p.oldest_open">
                    <a class="key" :href="`${JB}browse/${p.oldest_open.key}`" target="_blank" rel="noopener">{{ p.oldest_open.key }}</a>{{ ' ' }}
                    <span :class="lvl(p.oldest_open.age_days, 30, 90)">{{ p.oldest_open.age_days }} дн.</span>
                  </template>
                  <span v-else class="sub">—</span>
                </td>
                <td class="n">
                  <a v-if="p.carry_open" class="nl" :class="lvl(pct(p.carry_open, p.total), 20, 40)"
                     :href="carryLink(p.key)"
                     target="_blank" rel="noopener" title="Открыть в Jira">{{ p.carry_open }}</a>
                  <span v-else class="sub">0</span>
                  <span v-if="p.carry_open" class="sub"> {{ pct(p.carry_open, p.total) }}%</span>
                </td>
                <td class="n">{{ p.people }}</td>
              </tr>
              <tr v-if="!rows.length"><td :colspan="cols.length" class="sub empty">Никого нет по этому фильтру</td></tr>
            </tbody>
          </table>
        </div>
      </section>
    </template>
  </div>
</template>
