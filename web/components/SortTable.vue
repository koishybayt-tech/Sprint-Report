<script setup lang="ts">
// Сортируемая таблица как table() в reference/sprints/page_template.html:
// колонка = { h, v, f?, num?, tip?, cls? }; f возвращает HTML (данные внутри экранируются через esc).
import { type Col, esc } from '~/utils/sprints'

const props = defineProps<{ cols: Col[]; rows: any[]; init?: [number, 'asc' | 'desc']; empty?: string }>()
const sk = ref<number | null>(props.init ? props.init[0] : null)
const dir = ref<'asc' | 'desc'>(props.init ? props.init[1] : 'desc')

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
        <tr v-for="(r, ri) in sorted" :key="ri">
          <td v-for="c in cols" :key="c.h" :class="[c.num ? 'n' : '', c.cls || '']" v-html="c.f ? c.f(r) : esc(c.v(r))" />
        </tr>
        <tr v-if="!sorted.length"><td :colspan="cols.length" class="sub empty">{{ empty || 'Никого нет по этому фильтру' }}</td></tr>
      </tbody>
    </table>
  </div>
</template>
