<script setup lang="ts">
// Сортируемая таблица как table() в reference/sprints/page_template.html:
// колонка = { h, v, f?, sf?, num?, tip?, cls? }; f возвращает HTML (данные внутри экранируются через esc).
// ex — раскрываемые строки: key(строка) и sub(строка) → подстроки, их ячейки рисует sf (или f).
import { type Col, esc } from '~/utils/sprints'

const props = defineProps<{
  cols: Col[]; rows: any[]; init?: [number, 'asc' | 'desc']; empty?: string
  ex?: { key: (r: any) => string; sub: (r: any) => any[] }
}>()
const sk = ref<number | null>(props.init ? props.init[0] : null)
const dir = ref<'asc' | 'desc'>(props.init ? props.init[1] : 'desc')
const open = ref(new Set<string>())

const sorted = computed(() => {
  const r = [...props.rows]
  if (sk.value == null) return r
  const v = props.cols[sk.value].v, k = dir.value === 'asc' ? 1 : -1
  return r.sort((a, b) => {
    const x = v(a), y = v(b)
    return (typeof x === 'string' ? x.localeCompare(String(y), 'ru') : (x as number) - (y as number)) * k
  })
})
function go(i: number) {
  if (sk.value === i) dir.value = dir.value === 'asc' ? 'desc' : 'asc'
  else { sk.value = i; dir.value = props.cols[i].num ? 'desc' : 'asc' }
}
function toggle(r: any, e: Event) {
  if (!props.ex || (e.target as HTMLElement).closest('a')) return
  const k = props.ex.key(r), s = new Set(open.value)
  s.has(k) ? s.delete(k) : s.add(k)
  open.value = s
}
const isOpen = (r: any) => !!props.ex && open.value.has(props.ex.key(r))
const cell = (c: Col, r: any) => c.f ? c.f(r) : esc(c.v(r))
const subCell = (c: Col, r: any) => c.sf ? c.sf(r) : cell(c, r)
</script>

<template>
  <div class="tbl">
    <table>
      <thead>
        <tr>
          <th v-for="(c, i) in cols" :key="c.h" tabindex="0" :class="{ n: c.num && !c.cls }" :title="c.tip || ''"
              :data-dir="i === sk ? dir : undefined" @click="go(i)" @keydown.enter.prevent="go(i)" @keydown.space.prevent="go(i)">{{ c.h }}</th>
        </tr>
      </thead>
      <tbody>
        <template v-for="(r, ri) in sorted" :key="ex ? ex.key(r) : ri">
          <tr :class="{ xrow: ex, open: isOpen(r) }" :tabindex="ex ? 0 : undefined"
              :title="ex ? `Нажмите, чтобы ${isOpen(r) ? 'свернуть' : 'раскрыть'}` : undefined"
              @click="toggle(r, $event)" @keydown.enter="toggle(r, $event)">
            <td v-for="(c, ci) in cols" :key="c.h" :class="[c.num ? 'n' : '', c.cls || '']"
                v-html="(ex && ci === 0 ? `<span class='xc'>${isOpen(r) ? '▾' : '▸'}</span>` : '') + cell(c, r)" />
          </tr>
          <template v-if="isOpen(r)">
            <tr v-for="(s, si) in ex!.sub(r)" :key="si" class="xsub">
              <td v-for="c in cols" :key="c.h" :class="[c.num ? 'n' : '', c.cls || '']" v-html="subCell(c, s)" />
            </tr>
          </template>
        </template>
        <tr v-if="!sorted.length"><td :colspan="cols.length" class="sub empty">{{ empty || 'Никого нет по этому фильтру' }}</td></tr>
      </tbody>
    </table>
  </div>
</template>
