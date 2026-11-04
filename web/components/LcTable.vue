<script setup lang="ts">
// Таблица «Контроля спринта руководителей» (drawTable в design/prototypes/lead-control.html):
// сортировка по клику (первый клик — по убыванию), пустые значения всегда в конце, подсветка строки, ячейки — HTML.
import { sortRows } from '~/utils/leadcontrol'

export type LcCol = { f: string; label: string; cls?: string; html?: (r: any) => string; tip?: string }
const props = defineProps<{ cols: LcCol[]; rows: any[]; sort: [string, number]; rowClass?: (r: any) => string; rowKey: (r: any) => string; empty: string; fit?: boolean }>()
const emit = defineEmits<{ sort: [[string, number]] }>()

const sorted = computed(() => sortRows(props.rows, props.sort))
function click(f: string) { emit('sort', [f, props.sort[0] === f ? -props.sort[1] : -1]) }
</script>

<template>
  <div class="lc-scroll" :class="{ 'lc-fit': fit }">
    <table class="lc-t">
      <thead>
        <tr>
          <th v-for="c in cols" :key="c.f" tabindex="0" :title="c.tip || undefined"
              :aria-sort="c.f === sort[0] ? (sort[1] > 0 ? 'ascending' : 'descending') : undefined"
              @click="click(c.f)" @keydown.enter.prevent="click(c.f)">{{ c.label }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="r in sorted" :key="rowKey(r)" :class="rowClass ? rowClass(r) : ''">
          <td v-for="c in cols" :key="c.f" :class="c.cls" v-html="c.html ? c.html(r) : (r[c.f] === null || r[c.f] === undefined || r[c.f] === '' ? '—' : String(r[c.f]).replace(/[&<>]/g, ch => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' }[ch]!)))" />
        </tr>
      </tbody>
    </table>
    <div v-if="!sorted.length" class="lc-empty">{{ empty }}</div>
  </div>
</template>
