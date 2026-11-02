<script setup lang="ts">
// Настройки: смена своего пароля (все), «Telegram: кто это?» (admin, hr), пользователи сервиса (только admin).
import type { Me } from '~/composables/useMe'

type User = Me & { active: boolean; created_at: string; last_login_at: string | null }

const { apiBase } = useRuntimeConfig().public
const { me } = useMe()
const isAdmin = computed(() => me.value?.role === 'admin')
const errText = (e: any) => e?.data?.error || (e?.statusCode ? `Ошибка ${e.statusCode}` : 'Сервис недоступен')
const dt = (s: string | null) => s ? new Date(s).toLocaleString('ru', { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' }) : '—'

// ---------- мой пароль ----------
const pw = reactive({ current: '', next: '', repeat: '', msg: '', err: '', busy: false })
async function changePassword() {
  pw.msg = pw.err = ''
  if (pw.next !== pw.repeat) { pw.err = 'Новые пароли не совпадают'; return }
  pw.busy = true
  try {
    await $fetch(`${apiBase}/me/password`, { method: 'POST', body: { current: pw.current, new: pw.next } })
    pw.current = pw.next = pw.repeat = ''
    pw.msg = 'Пароль изменён. Другие ваши сессии закрыты.'
  } catch (e) { pw.err = errText(e) } finally { pw.busy = false }
}

// ---------- Telegram: кто это? (admin, hr) ----------
type TgAuthor = { from_id: number | null; from_name: string; messages: number; last_at: string; sample: string }
type JiraUser = { id: number; full_name: string; jira_login: string; active: boolean }
const canMapTg = computed(() => me.value?.role === 'admin' || me.value?.role === 'hr')
const tgAuthors = ref<TgAuthor[]>([])
const tgUsers = ref<JiraUser[]>([])
const tgErr = ref('')
const tgPick = ref<Record<string, number | ''>>({})
const tgBusy = ref<Record<string, boolean>>({})
const tgRowErr = ref<Record<string, string>>({})
const tgDone = ref('')
const tgOptions = computed(() => tgUsers.value.map(u => ({ value: u.id, label: u.full_name, hint: u.active ? u.jira_login : `${u.jira_login} · неактивен в Jira` })))
const authorKey = (a: TgAuthor) => (a.from_id ?? '') + '|' + a.from_name
async function loadTg() {
  if (!canMapTg.value) return
  try {
    const r = await $fetch<{ authors: TgAuthor[]; employees: JiraUser[] }>(`${apiBase}/admin/tg/unknown`)
    tgAuthors.value = r.authors; tgUsers.value = r.employees; tgErr.value = ''
  } catch (e) { tgErr.value = errText(e) }
}
onMounted(loadTg)
async function mapAuthor(a: TgAuthor) {
  const k = authorKey(a), emp = tgPick.value[k]
  if (!emp) { tgRowErr.value = { ...tgRowErr.value, [k]: 'Выберите пользователя Jira' }; return }
  tgBusy.value = { ...tgBusy.value, [k]: true }; tgRowErr.value = { ...tgRowErr.value, [k]: '' }
  try {
    const r = await $fetch<{ messages: number; events: number }>(`${apiBase}/admin/tg/aliases`, {
      method: 'POST', body: { from_id: a.from_id, from_name: a.from_name, employee_id: emp },
    })
    const u = tgUsers.value.find(x => x.id === emp)
    tgDone.value = `«${a.from_name}» → ${u?.full_name ?? 'сотрудник'}: разобрано сообщений ${r.messages}, событий посещаемости ${r.events}.`
    await loadTg()
  } catch (e) { tgRowErr.value = { ...tgRowErr.value, [k]: errText(e) } } finally { tgBusy.value = { ...tgBusy.value, [k]: false } }
}

// ---------- пользователи ----------
const users = ref<User[]>([])
const listErr = ref('')
async function loadUsers() {
  if (!isAdmin.value) return
  try { users.value = await $fetch<User[]>(`${apiBase}/admin/users`); listErr.value = '' } catch (e) { listErr.value = errText(e) }
}
onMounted(loadUsers)

// Пароль, выданный один раз (новый пользователь или сброс) — показываем админу, чтобы передал человеку.
const issued = ref<{ login: string; password: string } | null>(null)
const copied = ref(false)
async function copyIssued() {
  if (!issued.value) return
  try { await navigator.clipboard.writeText(issued.value.password); copied.value = true } catch { copied.value = false }
}

const form = reactive({ login: '', full_name: '', role: 'employee', employee_login: '', err: '', busy: false })
async function createUser() {
  form.err = ''; form.busy = true
  try {
    const r = await $fetch<{ user: User; password?: string }>(`${apiBase}/admin/users`, {
      method: 'POST', body: { login: form.login, full_name: form.full_name, role: form.role, employee_login: form.employee_login },
    })
    issued.value = r.password ? { login: r.user.login, password: r.password } : null; copied.value = false
    form.login = form.full_name = form.employee_login = ''; form.role = 'employee'
    await loadUsers()
  } catch (e) { form.err = errText(e) } finally { form.busy = false }
}

const rowErr = ref<Record<number, string>>({})
async function patchUser(u: User, body: Partial<Pick<User, 'role' | 'active' | 'full_name'>>) {
  rowErr.value = { ...rowErr.value, [u.id]: '' }
  try {
    const nu = await $fetch<User>(`${apiBase}/admin/users/${u.id}`, { method: 'PATCH', body })
    users.value = users.value.map(x => x.id === nu.id ? nu : x)
  } catch (e) {
    rowErr.value = { ...rowErr.value, [u.id]: errText(e) }
    await loadUsers() // вернуть значение в селекте/переключателе
  }
}
async function resetPassword(u: User) {
  if (!confirm(`Сбросить пароль пользователю ${u.full_name}? Текущие сессии будут закрыты.`)) return
  try {
    const r = await $fetch<{ password: string }>(`${apiBase}/admin/users/${u.id}/password`, { method: 'POST', body: {} })
    issued.value = { login: u.login, password: r.password }; copied.value = false
  } catch (e) { rowErr.value = { ...rowErr.value, [u.id]: errText(e) } }
}
</script>

<template>
  <main class="page-narrow">
    <h1>Настройки</h1>

    <section class="card">
      <h2>Мой пароль</h2>
      <form class="form-grid" @submit.prevent="changePassword">
        <label>Текущий пароль<input v-model="pw.current" type="password" autocomplete="current-password" required></label>
        <label>Новый пароль<input v-model="pw.next" type="password" autocomplete="new-password" minlength="10" required></label>
        <label>Ещё раз<input v-model="pw.repeat" type="password" autocomplete="new-password" minlength="10" required></label>
        <div class="form-actions">
          <button class="btn primary" type="submit" :disabled="pw.busy">Сменить пароль</button>
          <span v-if="pw.msg" class="ok-text">{{ pw.msg }}</span>
          <span v-if="pw.err" class="form-error">{{ pw.err }}</span>
        </div>
      </form>
      <p class="sub">Не короче 10 символов.</p>
    </section>

    <section v-if="canMapTg" class="card">
      <h2>Telegram: кто это?</h2>
      <p class="lead">Аккаунты в группе «Adata Group», чьи сообщения о посещаемости не удалось связать с сотрудником. Выберите, кто это в Jira, — все прошлые сообщения этого человека разберутся заново и появятся в отчётах. В списке Jira — только пользователи, ещё не связанные с Telegram.</p>
      <p v-if="tgDone" class="ok-text" role="status">{{ tgDone }}</p>
      <p v-if="tgErr" class="form-error">{{ tgErr }}</p>
      <div class="tbl">
        <table>
          <thead><tr><th>Аккаунт Telegram</th><th class="n">Сообщений</th><th>Последнее</th><th>Начало сообщения</th><th>Пользователь Jira</th><th /></tr></thead>
          <tbody>
            <tr v-for="a in tgAuthors" :key="authorKey(a)">
              <td><b>{{ a.from_name || 'без имени' }}</b><div v-if="!a.from_id" class="sub">без Telegram ID (из HTML-экспорта)</div></td>
              <td class="n">{{ a.messages }}</td>
              <td class="sub">{{ dt(a.last_at) }}</td>
              <td class="wrap"><span class="sub">{{ a.sample || '—' }}</span></td>
              <td>
                <SearchSelect v-model="tgPick[authorKey(a)]" :options="tgOptions" placeholder="Найти по ФИО или логину" :label="`Пользователь Jira для ${a.from_name}`" />
                <div v-if="tgRowErr[authorKey(a)]" class="form-error">{{ tgRowErr[authorKey(a)] }}</div>
              </td>
              <td><button class="btn" :disabled="tgBusy[authorKey(a)] || !tgPick[authorKey(a)]" @click="mapAuthor(a)">{{ tgBusy[authorKey(a)] ? 'Сохраняю…' : 'Сохранить' }}</button></td>
            </tr>
            <tr v-if="!tgAuthors.length && !tgErr"><td colspan="6" class="sub empty">Все авторы сообщений о посещаемости опознаны</td></tr>
          </tbody>
        </table>
      </div>
      <p class="sub">Свободных пользователей Jira: {{ tgUsers.length }}.</p>
    </section>

    <template v-if="isAdmin">
      <section class="card">
        <h2>Пользователи</h2>
        <p class="lead">Кто может входить в сервис и что видит. «Спринты» доступны всем ролям, кроме HR; пользователями управляет только администратор.</p>

        <div v-if="issued" class="issued" role="status">
          <div>Пароль для <b>{{ issued.login }}</b> — передайте его человеку лично. Больше он показан не будет.</div>
          <code>{{ issued.password }}</code>
          <button class="btn" @click="copyIssued">{{ copied ? 'Скопировано' : 'Скопировать' }}</button>
          <button class="linkbtn" @click="issued = null">Скрыть</button>
        </div>

        <form class="form-grid cols" @submit.prevent="createUser">
          <label>Логин<input v-model="form.login" autocomplete="off" required placeholder="ivanov"></label>
          <label>ФИО<input v-model="form.full_name" autocomplete="off" required placeholder="Иванов Иван"></label>
          <label>Роль
            <select v-model="form.role">
              <option v-for="(t, r) in ROLE_TITLES" :key="r" :value="r">{{ t }}</option>
            </select>
          </label>
          <label>Логин в Jira <span class="sub">(необязательно)</span><input v-model="form.employee_login" autocomplete="off" placeholder="для связи с карточкой"></label>
          <div class="form-actions">
            <button class="btn primary" type="submit" :disabled="form.busy">Создать — пароль сгенерируется</button>
            <span v-if="form.err" class="form-error">{{ form.err }}</span>
          </div>
        </form>

        <p v-if="listErr" class="form-error">{{ listErr }}</p>
        <div class="tbl">
          <table>
            <thead><tr><th>Пользователь</th><th>Логин</th><th>Роль</th><th>Активен</th><th>Последний вход</th><th /></tr></thead>
            <tbody>
              <tr v-for="u in users" :key="u.id" :class="{ inactive: !u.active }">
                <td>{{ u.full_name }}<span v-if="u.id === me?.id" class="pill mute" style="margin-left:6px">это вы</span>
                  <div v-if="rowErr[u.id]" class="form-error">{{ rowErr[u.id] }}</div></td>
                <td><span class="key">{{ u.login }}</span></td>
                <td>
                  <select :value="u.role" :aria-label="`Роль ${u.full_name}`" @change="patchUser(u, { role: ($event.target as HTMLSelectElement).value as User['role'] })">
                    <option v-for="(t, r) in ROLE_TITLES" :key="r" :value="r">{{ t }}</option>
                  </select>
                </td>
                <td><label class="chk"><input type="checkbox" :checked="u.active" @change="patchUser(u, { active: ($event.target as HTMLInputElement).checked })"> {{ u.active ? 'да' : 'заблокирован' }}</label></td>
                <td class="sub">{{ dt(u.last_login_at) }}</td>
                <td><button class="linkbtn" @click="resetPassword(u)">Сбросить пароль</button></td>
              </tr>
              <tr v-if="!users.length"><td colspan="6" class="sub empty">Пользователей пока нет</td></tr>
            </tbody>
          </table>
        </div>
      </section>
    </template>
  </main>
</template>
