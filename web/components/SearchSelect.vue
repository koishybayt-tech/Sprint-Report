<script setup lang="ts">
// Выпадающий список с поиском (combobox): печатаете часть ФИО или логина — список сужается.
// Клавиатура: ↑/↓ — выбор, Enter — подтвердить, Esc — закрыть.
type Option = { value: number | string; label: string; hint?: string }

const props = defineProps<{ options: Option[]; placeholder?: string; label?: string }>()
const model = defineModel<number | string | ''>({ default: '' })

const open = ref(false)
const query = ref('')
const active = ref(0)
const root = ref<HTMLElement | null>(null)
const pos = ref({ top: 0, left: 0, width: 0 })
// Список — position: fixed по координатам поля: внутри таблиц с прокруткой (.tbl) его иначе обрезало бы.
function place() {
  const r = root.value?.getBoundingClientRect()
  if (!r) return
  const below = innerHeight - r.bottom
  pos.value = { top: below < 270 && r.top > below ? Math.max(8, r.top - 268) : r.bottom + 4, left: r.left, width: Math.max(r.width, 300) }
}
const listId = `ss-${Math.random().toString(36).slice(2, 9)}`

// Регистр, ё/е и казахские буквы не мешают искать: «асанали» найдёт «Асанәлі».
const fold = (s: string) => s.toLowerCase().replace(/ё/g, 'е').replace(/[әіүұқғңөһ]/g, c => ({ ә: 'а', і: 'и', ү: 'у', ұ: 'у', қ: 'к', ғ: 'г', ң: 'н', ө: 'о', һ: 'х' } as Record<string, string>)[c])
const selected = computed(() => props.options.find(o => o.value === model.value))
const filtered = computed(() => {
  const q = fold(query.value.trim())
  if (!q) return props.options
  const words = q.split(/\s+/)
  return props.options.filter(o => { const t = fold(o.label + ' ' + (o.hint ?? '')); return words.every(w => t.includes(w)) })
})
watch(filtered, () => { active.value = 0 })

function show() { place(); open.value = true; query.value = ''; active.value = Math.max(0, filtered.value.findIndex(o => o.value === model.value)) }
function pick(o: Option) { model.value = o.value; open.value = false; query.value = '' }
function clear() { model.value = ''; query.value = ''; open.value = true }
function onKey(e: KeyboardEvent) {
  if (!open.value && (e.key === 'ArrowDown' || e.key === 'Enter')) { show(); e.preventDefault(); return }
  if (e.key === 'ArrowDown') { active.value = Math.min(active.value + 1, filtered.value.length - 1); e.preventDefault() }
  else if (e.key === 'ArrowUp') { active.value = Math.max(active.value - 1, 0); e.preventDefault() }
  else if (e.key === 'Enter') { const o = filtered.value[active.value]; if (o) pick(o); e.preventDefault() }
  else if (e.key === 'Escape') { open.value = false; query.value = '' }
}
watch(active, i => nextTick(() => root.value?.querySelector(`#${listId}-${i}`)?.scrollIntoView({ block: 'nearest' })))
const onDoc = (e: MouseEvent) => { if (open.value && root.value && !root.value.contains(e.target as Node)) { open.value = false; query.value = '' } }
const onMove = () => { if (open.value) place() }
onMounted(() => { document.addEventListener('mousedown', onDoc); addEventListener('scroll', onMove, true); addEventListener('resize', onMove) })
onBeforeUnmount(() => { document.removeEventListener('mousedown', onDoc); removeEventListener('scroll', onMove, true); removeEventListener('resize', onMove) })
</script>

<template>
  <div ref="root" class="ss" :class="{ open }">
    <input
      class="ss-input" type="text" role="combobox" autocomplete="off" :aria-label="label" aria-autocomplete="list"
      :aria-expanded="open" :aria-controls="listId" :aria-activedescendant="open && filtered.length ? `${listId}-${active}` : undefined"
      :placeholder="selected ? '' : (placeholder || 'Начните вводить…')"
      :value="open ? query : (selected?.label ?? '')"
      @focus="show" @click="!open && show()" @input="query = ($event.target as HTMLInputElement).value; open = true" @keydown="onKey"
    >
    <button v-if="selected && !open" type="button" class="ss-clear" aria-label="Очистить выбор" @click="clear">×</button>
    <ul v-if="open" :id="listId" class="ss-list" role="listbox" :style="{ top: pos.top + 'px', left: pos.left + 'px', width: pos.width + 'px' }">
      <li
        v-for="(o, i) in filtered" :id="`${listId}-${i}`" :key="o.value" role="option" :aria-selected="o.value === model"
        :class="{ active: i === active, chosen: o.value === model }" @mousedown.prevent="pick(o)" @mousemove="active = i"
      >
        <span>{{ o.label }}</span><span v-if="o.hint" class="sub">{{ o.hint }}</span>
      </li>
      <li v-if="!filtered.length" class="ss-empty sub">Никого не нашлось</li>
    </ul>
  </div>
</template>
