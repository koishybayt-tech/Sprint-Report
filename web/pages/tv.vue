<script setup lang="ts">
// Экран для ТВ: все важные сигналы по спринтам на одной странице, без прокрутки. Обновляется сам (раз в 5 минут).
// Данные — GET /tv/report и /tv/lead-control (доступ по модулю «ТВ»); расчёты — utils/tv.ts. Двойной клик — полный экран.
import { type ApiReport, buildModel } from '~/utils/sprints'
import { defaultCtl, peopleLogic } from '~/utils/sprintsPeople'
import { type LcReport, TV, buildTv, daysLevel, workDays } from '~/utils/tv'

definePageMeta({ layout: 'tv' })
useHead({ title: 'ТВ — Отчёты Adata' })

const { apiBase } = useRuntimeConfig().public
const REFRESH_MS = 5 * 60 * 1000
const report = ref<ApiReport | null>(null)
const lc = ref<LcReport[]>([])
const loadedAt = ref<Date | null>(null)
const err = ref('')

async function load() {
  try {
    const [r, reps] = await Promise.all([
      $fetch<ApiReport>(`${apiBase}/tv/report`),
      $fetch<LcReport[]>(`${apiBase}/tv/lead-control`),
    ])
    report.value = r; lc.value = reps; loadedAt.value = new Date(); err.value = ''
  } catch (e: any) {
    // Старые данные остаются на экране, пока не получится обновить.
    err.value = e?.statusCode ? `не удалось обновить (${e.statusCode})` : 'нет связи с сервером'
  }
}

const now = ref(new Date())
const root = ref<HTMLElement | null>(null)

// Подбор числа строк под высоту панели: прячем последнюю строку самого длинного списка, пока панель не влезет.
// «+N ещё» — сколько строк не поместилось (с учётом тех, что не рисовались вовсе).
function fit() {
  root.value?.querySelectorAll<HTMLElement>('.tv-p').forEach((p) => {
    const uls = [...p.querySelectorAll<HTMLElement>('ul[data-fit]')]
    const rows = (u: HTMLElement) => [...u.querySelectorAll<HTMLElement>('li:not(.tv-ok)')]
    const shown = (u: HTMLElement) => rows(u).filter(li => !li.hidden)
    uls.forEach(u => rows(u).forEach((li) => { li.hidden = false }))
    // «+N ещё» обновляем вместе со скрытием строк: подпись сама занимает место и влияет на подбор
    const marks = () => uls.forEach((u) => {
      const hidden = Math.max(0, Number(u.dataset.total || 0) - shown(u).length)
      const more = u.nextElementSibling
      if (more?.classList.contains('tv-more')) more.textContent = hidden > 0 ? `+${hidden} ещё` : ''
    })
    marks()
    for (let guard = 300; guard > 0 && p.scrollHeight > p.clientHeight + 1; guard--) {
      const u = uls.filter(x => shown(x).length > 1).sort((x, y) => shown(y).length - shown(x).length)[0]
      if (!u) break
      shown(u).at(-1)!.hidden = true
      marks()
    }
  })
}
let fitRaf = 0
const scheduleFit = () => { cancelAnimationFrame(fitRaf); fitRaf = requestAnimationFrame(fit) }
let ro: ResizeObserver | undefined
const idle = ref(false)
let timers: ReturnType<typeof setInterval>[] = []
let idleT: ReturnType<typeof setTimeout> | undefined
function wake() { idle.value = false; clearTimeout(idleT); idleT = setTimeout(() => { idle.value = true }, 5000) }
function fullscreen() { document.fullscreenElement ? document.exitFullscreen() : document.documentElement.requestFullscreen?.() }
onMounted(() => {
  load()
  timers = [setInterval(load, REFRESH_MS), setInterval(() => { now.value = new Date() }, 15000)]
  wake()
  ro = new ResizeObserver(scheduleFit)
  if (root.value) ro.observe(root.value)
})
onBeforeUnmount(() => { timers.forEach(clearInterval); clearTimeout(idleT); ro?.disconnect(); cancelAnimationFrame(fitRaf) })

const m = computed(() => report.value?.updated_at ? buildModel(report.value) : null)
const tv = computed(() => {
  if (!m.value) return null
  const pl = peopleLogic(m.value, defaultCtl(), () => true, () => true)
  return buildTv(m.value, pl, lc.value)
})
watch(tv, () => nextTick(scheduleFit), { flush: 'post' })

const p2 = (n: number) => String(n).padStart(2, '0')
const clock = computed(() => `${p2(now.value.getHours())}:${p2(now.value.getMinutes())}`)
const dateTxt = computed(() => now.value.toLocaleDateString('ru-RU', { weekday: 'long', day: 'numeric', month: 'long' }))
const updTxt = computed(() => {
  const u = report.value?.updated_at ? new Date(report.value.updated_at) : null
  return u ? `Jira: ${p2(u.getHours())}:${p2(u.getMinutes())}` : ''
})
const CAP = 14 // сколько строк рисуем с запасом; по высоте экрана их подрезает fit()
const dnoun = (n: number) => { const a = n % 10, h = n % 100; return a === 1 && h !== 11 ? 'день' : a >= 2 && a <= 4 && (h < 12 || h > 14) ? 'дня' : 'дней' }
const short = (s: string, n = 46) => s.length > n ? s.slice(0, n - 1) + '…' : s

const kpis = computed(() => {
  const k = tv.value!.kpi
  return [
    { v: k.tasks, l: 'задач в спринтах', c: '' }, { v: k.done + '%', l: 'готово', c: '' },
    { v: k.stuck, l: 'спринтов не закрыто', c: k.stuck ? 'bad' : '' },
    { v: k.absentNow, l: 'отсутствуют сегодня', c: k.absentNow ? 'warn' : '' },
    { v: k.need, l: 'нужны задачи', c: k.need ? 'warn' : '' },
    { v: k.blocked, l: 'в Blocked', c: k.blocked ? 'bad' : '' },
    { v: k.longWork, l: `в работе ≥ ${TV.longWork} дн.`, c: k.longWork ? 'warn' : '' },
    { v: k.longTest, l: `на тесте ≥ ${TV.longTest} дн.`, c: k.longTest ? 'warn' : '' },
  ]
})
</script>

<template>
  <main ref="root" class="tv" :class="{ idle }" @mousemove="wake" @dblclick="fullscreen">
    <header class="tv-head">
      <h1>Спринты Adata</h1>
      <div class="tv-meta">
        <span v-if="err" class="tv-err">{{ err }}</span>
        <span class="tv-sub">{{ updTxt }}</span>
        <span class="tv-date">{{ dateTxt }}</span>
        <b class="tv-clock">{{ clock }}</b>
      </div>
    </header>

    <p v-if="!tv" class="tv-wait">{{ err || 'Загружаю данные…' }}</p>
    <template v-else>
      <div class="tv-kpi">
        <div v-for="k in kpis" :key="k.l" class="tv-k" :class="k.c"><b>{{ k.v }}</b><span>{{ k.l }}</span></div>
      </div>

      <div class="tv-grid">
        <!-- 1. отсутствуют -->
        <section class="tv-p">
          <h2>Отсутствуют <small>сегодня и ближайшие {{ TV.absentDays }} дн.</small><i>{{ tv.absent.length }}</i></h2>
          <ul data-fit :data-total="tv.absent.length">
            <li v-for="a in tv.absent.slice(0, CAP)" :key="a.n + a.text">
              <b>{{ a.n }}</b><span class="tv-pos">{{ a.pos }}</span>
              <em :class="a.now ? 'bad' : 'warn'">{{ a.text }}</em>
              <span v-if="a.q" class="tv-tag bad">{{ a.q }} задач висит</span>
            </li>
            <li v-if="!tv.absent.length" class="tv-ok">Все на месте</li>
          </ul>
          <p class="tv-more" />
                  </section>

        <!-- 2. нужны задачи -->
        <section class="tv-p">
          <h2>Нужны задачи <small>≤ {{ defaultCtl().nTh }} задач в очереди</small><i>{{ tv.need.length }}</i></h2>
          <ul data-fit :data-total="tv.need.length">
            <li v-for="x in tv.need.slice(0, CAP)" :key="x.n">
              <b>{{ x.n }}</b><span class="tv-pos">{{ x.pos }}</span>
              <em :class="x.q === 0 ? 'bad' : 'warn'">{{ x.q === 0 ? 'нет задач' : x.q + ' в очереди' }}</em>
            </li>
            <li v-if="!tv.need.length" class="tv-ok">Задач хватает всем</li>
          </ul>
          <p class="tv-more" />
                  </section>

        <!-- 3. риски проектов -->
        <section class="tv-p">
          <h2>Риски проектов<i>{{ tv.risks.length }}</i></h2>
          <ul data-fit :data-total="tv.risks.length">
            <li v-for="r in tv.risks.slice(0, CAP)" :key="r.k" class="tv-risk">
              <b>{{ r.k }}</b><span class="tv-pos">{{ short(r.name, 22) }}</span>
              <span class="tv-chips"><em v-for="x in r.reasons" :key="x.text" :class="x.sev >= 2 ? 'bad' : 'warn'">{{ x.text }}</em></span>
            </li>
            <li v-if="!tv.risks.length" class="tv-ok">Рисков нет</li>
          </ul>
          <p class="tv-more" />
                  </section>

        <!-- 4. тестирование -->
        <section class="tv-p">
          <h2>Тестирование<i>{{ tv.kpi.longTest }}</i></h2>
          <h3>Задач в Test по проектам</h3>
          <ul class="tv-bars" data-fit :data-total="tv.tests.length">
            <li v-for="t in tv.tests.slice(0, CAP)" :key="t.k">
              <b>{{ t.k }}</b>
              <div class="tv-bar"><i :class="t.level" :style="{ width: Math.min(100, t.tst / Math.max(TV.testHigh, tv.tests[0].tst) * 100) + '%' }" /></div>
              <em :class="t.level">{{ t.tst }}</em>
            </li>
            <li v-if="!tv.tests.length" class="tv-ok">В Test пусто</li>
          </ul>
          <p class="tv-more" />
          <h3>Не успевают тестировать <small>≥ {{ TV.longTest }} дн. на тесте</small></h3>
          <ul data-fit :data-total="tv.slowTesters.length">
            <li v-for="w in tv.slowTesters.slice(0, CAP)" :key="w.name">
              <b>{{ w.name }}</b>
              <em :class="daysLevel(w.max, TV.longTest, 7)">{{ w.count }} задач · до {{ w.max }} {{ dnoun(w.max) }}</em>
            </li>
            <li v-if="!tv.slowTesters.length" class="tv-ok">Тест идёт в срок</li>
          </ul>
          <p class="tv-more" />
                  </section>

        <!-- 5. долго в работе -->
        <section class="tv-p">
          <h2>Долго в работе <small>≥ {{ TV.longWork }} дн. с момента взятия</small><i>{{ tv.slowWork.length }}</i></h2>
          <h3>Задачи</h3>
          <ul data-fit :data-total="tv.slowWork.length">
            <li v-for="x in tv.slowWork.slice(0, CAP)" :key="x.key">
              <b>{{ x.key }}</b><span class="tv-pos">{{ short(x.assignee || 'не назначен', 22) }} · {{ x.status }}</span>
              <em :class="daysLevel(workDays(x), TV.longWork, 10)">{{ workDays(x) }} {{ dnoun(workDays(x)) }}</em>
            </li>
            <li v-if="!tv.slowWork.length" class="tv-ok">Долгих задач нет</li>
          </ul>
          <p class="tv-more" />
          <h3>Кто дольше всех</h3>
          <ul data-fit :data-total="tv.slowDevs.length">
            <li v-for="w in tv.slowDevs.slice(0, CAP)" :key="w.name">
              <b>{{ w.name }}</b>
              <em :class="daysLevel(w.max, TV.longWork, 10)">{{ w.count }} задач · до {{ w.max }} {{ dnoun(w.max) }}</em>
            </li>
          </ul>
          <p class="tv-more" />
                  </section>

        <!-- 6. Blocked -->
        <section class="tv-p">
          <h2>Заблокированные задачи<i>{{ tv.blocked.length }}</i></h2>
          <ul data-fit :data-total="tv.blocked.length">
            <li v-for="x in tv.blocked.slice(0, CAP)" :key="x.key" class="tv-two">
              <div>
                <b>{{ x.key }}</b><span class="tv-pos">{{ short(x.assignee || 'не назначен', 24) }}</span>
                <em :class="daysLevel(x.days_blocked, 3, 7)">{{ x.days_blocked }} {{ dnoun(x.days_blocked) }}</em>
              </div>
              <div class="tv-chips"><em :class="x.block_state.startsWith('Можно снимать') ? 'ok' : 'warn'">{{ short(x.block_state, 60) }}</em></div>
            </li>
            <li v-if="!tv.blocked.length" class="tv-ok">Блоков нет</li>
          </ul>
          <p class="tv-more" />
                  </section>
      </div>
    </template>
  </main>
</template>

<style>
/* ТВ всегда тёмный и не зависит от выбранной темы; размеры от ширины экрана — читается с расстояния */
.tvroot{position:fixed;inset:0;background:#0b1220;overflow:hidden}
.tv{--bg:#0b1220;--card:#131d31;--line:#243049;--fg:#e9eff8;--mute:#8d9bb2;--bad:#ff6b6b;--warn:#ffb84d;--ok:#4ade80;
  height:100%;display:flex;flex-direction:column;gap:1.1vh;padding:1.6vh 1.4vw;color:var(--fg);font:500 clamp(13px,.98vw,32px)/1.3 var(--f-body,system-ui,sans-serif)}
.tv.idle{cursor:none}
.tv-head{display:flex;align-items:baseline;justify-content:space-between;gap:1vw}
.tv-head h1{margin:0;font-size:2.2em;letter-spacing:-.01em}
.tv-meta{display:flex;align-items:baseline;gap:1.2vw;color:var(--mute)}
.tv-clock{font-size:2.4em;color:var(--fg);font-variant-numeric:tabular-nums}
.tv-date{text-transform:capitalize}
.tv-err{color:var(--warn)}
.tv-wait{margin:auto;font-size:1.6em;color:var(--mute)}
.tv-kpi{display:grid;grid-template-columns:repeat(8,1fr);gap:.7vw}
.tv-k{background:var(--card);border:1px solid var(--line);border-radius:.6em;padding:.6vh .8vw;display:flex;flex-direction:column}
.tv-k b{font-size:2.3em;line-height:1.05;font-variant-numeric:tabular-nums}
.tv-k span{color:var(--mute);font-size:.85em}
.tv-k.bad{border-color:var(--bad)}.tv-k.bad b{color:var(--bad)}
.tv-k.warn{border-color:var(--warn)}.tv-k.warn b{color:var(--warn)}
.tv-grid{flex:1;min-height:0;display:grid;grid-template-columns:repeat(3,1fr);grid-template-rows:1fr 1fr;gap:.9vw}
.tv-p{min-height:0;overflow:hidden;background:var(--card);border:1px solid var(--line);border-radius:.7em;padding:1vh 1vw;display:flex;flex-direction:column}
.tv-p h2{margin:0 0 .5vh;font-size:1.3em;display:flex;align-items:baseline;gap:.6vw}
.tv-p h2 small{font-size:.62em;font-weight:500;color:var(--mute)}
.tv-p h2 i{margin-left:auto;font-style:normal;font-size:1.3em;color:var(--warn);font-variant-numeric:tabular-nums}
.tv-p h3{margin:.6vh 0 .2vh;font-size:.82em;font-weight:600;color:var(--mute);text-transform:uppercase;letter-spacing:.04em}
.tv-p h3 small{text-transform:none;font-weight:500;letter-spacing:0}
.tv-p ul{list-style:none;margin:0;padding:0}
.tv-p li{display:flex;align-items:baseline;gap:.6vw;padding:.45vh 0;border-bottom:1px solid var(--line);min-width:0}
.tv-p li:last-child{border-bottom:0}
.tv-p li[hidden]{display:none}
.tv-p li b{white-space:nowrap}
.tv-pos{color:var(--mute);font-size:.85em;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;min-width:0}
.tv-p li em{margin-left:auto;font-style:normal;white-space:nowrap;font-weight:600}
.tv-p em.bad,.tv-tag.bad{color:var(--bad)}.tv-p em.warn{color:var(--warn)}.tv-p em.ok{color:var(--ok)}
.tv-tag{font-size:.8em;white-space:nowrap}
.tv-ok{color:var(--ok);padding:.8vh 0}
.tv-more{margin:0;color:var(--mute);font-size:.8em;text-align:right}
.tv-more:empty{display:none}
.tv-risk{flex-wrap:wrap}
.tv-two{flex-direction:column;align-items:stretch;gap:.1vh}
.tv-two>div:first-child{display:flex;align-items:baseline;gap:.6vw;min-width:0}
.tv-chips{margin-left:auto;display:flex;flex-wrap:wrap;justify-content:flex-end;gap:0 .8vw;font-size:.86em}
.tv-two .tv-chips{margin-left:0;justify-content:flex-start}
.tv-chips em{margin-left:0!important;font-weight:500!important}
.tv-bars li{gap:.8vw}
.tv-bars b{width:4.6em}
.tv-bar{flex:1;height:.8em;background:var(--line);border-radius:.4em;overflow:hidden}
.tv-bar i{display:block;height:100%;background:#4aa3ff;border-radius:.4em}
.tv-bar i.warn{background:var(--warn)}.tv-bar i.bad{background:var(--bad)}
.tv-bars em{margin-left:0!important;min-width:1.6em;text-align:right}
</style>
