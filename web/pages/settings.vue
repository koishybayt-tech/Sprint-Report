<script setup lang="ts">
// Настройки: смена своего пароля (все), «Telegram: кто это?» (admin, hr), пользователи сервиса (только admin).
import type { Me } from '~/composables/useMe'

type User = Me & { active: boolean; created_at: string; last_login_at: string | null; employee_login: string | null }

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

// ---------- уведомления «Контроля спринта руководителей» (admin, lead) ----------
type NotifySettings = { on_blocked: boolean; on_unblock: boolean; on_gap: boolean; telegram: { available: boolean; configured: boolean; connected: boolean; bot: string; link: string } }
const canNotify = computed(() => canSeeLeadControl(me.value))
const ns = ref<NotifySettings | null>(null)
const nsErr = ref('')
const nsMsg = ref('')
const botToken = ref('')
const nsBusy = ref(false)
async function loadNotify() {
  if (!canNotify.value) return
  try { ns.value = await $fetch<NotifySettings>(`${apiBase}/notifications/settings`); nsErr.value = '' } catch (e) { nsErr.value = errText(e) }
}
onMounted(loadNotify)
async function saveFlag(f: 'on_blocked' | 'on_unblock' | 'on_gap', v: boolean) {
  try { ns.value = await $fetch<NotifySettings>(`${apiBase}/notifications/settings`, { method: 'PUT', body: { [f]: v } }) } catch (e) { nsErr.value = errText(e) }
}
async function connectBot() {
  nsBusy.value = true; nsErr.value = ''; nsMsg.value = ''
  try {
    ns.value = await $fetch<NotifySettings>(`${apiBase}/notifications/telegram`, { method: 'POST', body: { token: botToken.value.trim() } })
    botToken.value = ''
    nsMsg.value = 'Бот подключён — откройте ссылку и нажмите «Старт», чтобы привязать его к себе.'
  } catch (e) { nsErr.value = errText(e) } finally { nsBusy.value = false }
}
// «Проверить уведомление» — пример выбранного события: в колокольчик всегда, в бота — если привязан.
const testKind = ref<'blocked' | 'unblock' | 'gap'>('gap')
const testBusy = ref(false)
const TG_RESULT: Record<string, string> = {
  sent: 'и в ваш Telegram-бот', not_connected: 'в Telegram не отправлено — бот не подключён или не привязан',
  unavailable: 'в Telegram не отправлено — на сервере нет SECRETS_KEY', error: 'в Telegram не дошло',
}
async function testNotify() {
  testBusy.value = true; nsErr.value = ''; nsMsg.value = ''
  try {
    const r = await $fetch<{ bell: boolean; telegram: string; error?: string }>(`${apiBase}/notifications/test`, { method: 'POST', body: { kind: testKind.value } })
    nsMsg.value = `Пример отправлен в колокольчик ${r.telegram === 'sent' ? '' : '· '}${TG_RESULT[r.telegram] || ''}${r.error ? ': ' + r.error : ''}.`
    window.dispatchEvent(new Event('notify:refresh'))
  } catch (e) { nsErr.value = errText(e) } finally { testBusy.value = false }
  await loadNotify()
}
async function disconnectBot() {
  if (!confirm('Отключить бота? Уведомления останутся только в колокольчике.')) return
  try { await $fetch(`${apiBase}/notifications/telegram`, { method: 'DELETE' }); await loadNotify() } catch (e) { nsErr.value = errText(e) }
}
// Пока привязка не завершена — тихо проверяем раз в 5 секунд, чтобы статус сменился сам.
let nsTimer: ReturnType<typeof setInterval> | undefined
watch(ns, v => {
  clearInterval(nsTimer)
  if (v?.telegram.configured && !v.telegram.connected) nsTimer = setInterval(loadNotify, 5000)
})
onBeforeUnmount(() => clearInterval(nsTimer))

// ---------- руководители и проекты («Контроль спринта руководителей») ----------
type LeadProject = { lead: string; project_key: string; title: string; position: number }
const leadProjects = ref<LeadProject[]>([])
const lpErr = ref('')
async function loadLeadProjects() {
  if (!isAdmin.value) return
  try { leadProjects.value = await $fetch<LeadProject[]>(`${apiBase}/admin/lead-projects`); lpErr.value = '' } catch (e) { lpErr.value = errText(e) }
}
onMounted(loadLeadProjects)
const lpForm = reactive({ lead: '', project_key: '', title: '', position: 1, busy: false })
const lpLeads = computed(() => [...new Set(leadProjects.value.map(x => x.lead))])
async function saveLeadProject(x?: LeadProject) {
  lpErr.value = ''; lpForm.busy = true
  try {
    const body = x || { lead: lpForm.lead, project_key: lpForm.project_key, title: lpForm.title, position: Number(lpForm.position) || 0 }
    await $fetch(`${apiBase}/admin/lead-projects`, { method: 'POST', body })
    if (!x) { lpForm.project_key = ''; lpForm.title = ''; lpForm.position = (Number(lpForm.position) || 0) + 1 }
    await loadLeadProjects()
  } catch (e) { lpErr.value = errText(e) } finally { lpForm.busy = false }
}
async function deleteLeadProject(x: LeadProject) {
  if (!confirm(`Убрать проект ${x.project_key} у руководителя ${x.lead}?`)) return
  lpErr.value = ''
  try {
    await $fetch(`${apiBase}/admin/lead-projects`, { method: 'DELETE', query: { lead: x.lead, project: x.project_key } })
    await loadLeadProjects()
  } catch (e) { lpErr.value = errText(e) }
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

const allModules = () => MODULES.map(m => m.key as string)
const form = reactive({ login: '', full_name: '', role: 'employee', employee_login: '', modules: allModules(), err: '', busy: false })

// Галочка модуля у существующего пользователя сохраняется сразу; админу доступны все модули всегда.
function toggleModule(u: User, key: string, on: boolean) {
  const cur = new Set(u.modules)
  if (on) cur.add(key); else cur.delete(key)
  return patchUser(u, { modules: MODULES.map(m => m.key as string).filter(k => cur.has(k)) })
}
async function createUser() {
  form.err = ''; form.busy = true
  try {
    const r = await $fetch<{ user: User; password?: string }>(`${apiBase}/admin/users`, {
      method: 'POST', body: { login: form.login, full_name: form.full_name, role: form.role, employee_login: form.employee_login, modules: form.modules },
    })
    issued.value = r.password ? { login: r.user.login, password: r.password } : null; copied.value = false
    form.login = form.full_name = form.employee_login = ''; form.role = 'employee'; form.modules = allModules()
    await loadUsers()
  } catch (e) { form.err = errText(e) } finally { form.busy = false }
}

const rowErr = ref<Record<number, string>>({})
// Правка пользователя: ФИО, логин входа и связь с сотрудником (логин Jira; пусто — отвязать).
const editing = ref<number | null>(null)
const edit = reactive({ full_name: '', login: '', employee_login: '', busy: false })
function startEdit(u: User) {
  editing.value = u.id
  Object.assign(edit, { full_name: u.full_name, login: u.login, employee_login: u.employee_login || '', busy: false })
  rowErr.value = { ...rowErr.value, [u.id]: '' }
}
async function saveEdit(u: User) {
  edit.busy = true
  const ok = await patchUser(u, { full_name: edit.full_name, login: edit.login, employee_login: edit.employee_login.trim() })
  edit.busy = false
  if (ok) editing.value = null
}

async function patchUser(u: User, body: Partial<Pick<User, 'role' | 'active' | 'full_name' | 'login' | 'modules'>> & { employee_login?: string }): Promise<boolean> {
  rowErr.value = { ...rowErr.value, [u.id]: '' }
  try {
    const nu = await $fetch<User>(`${apiBase}/admin/users/${u.id}`, { method: 'PATCH', body })
    users.value = users.value.map(x => x.id === nu.id ? nu : x)
    if (nu.id === me.value?.id) await useMe().load() // своё имя в шапке
    return true
  } catch (e) {
    rowErr.value = { ...rowErr.value, [u.id]: errText(e) }
    if (editing.value !== u.id) await loadUsers() // вернуть значение в селекте/переключателе
    return false
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
  <main class="page-wide">
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

    <section v-if="canNotify" id="notify" class="card">
      <h2>Уведомления</h2>
      <p class="lead">События «Контроля спринта руководителей» по вашим проектам приходят в колокольчик вверху страницы и, если подключить, в ваш личный Telegram-бот. Проверяются после каждого обновления данных Jira (каждые 30 минут с 08:30 до 20:00).</p>
      <p v-if="nsErr" class="form-error">{{ nsErr }}</p>
      <div v-if="ns" class="notify-grid">
        <label class="chk"><input type="checkbox" :checked="ns.on_blocked" @change="saveFlag('on_blocked', ($event.target as HTMLInputElement).checked)"> Задача попала в блок</label>
        <label class="chk"><input type="checkbox" :checked="ns.on_unblock" @change="saveFlag('on_unblock', ($event.target as HTMLInputElement).checked)"> Задачу можно вытащить из блока (всё, что она ждала, уже на тесте)</label>
        <label class="chk"><input type="checkbox" :checked="ns.on_gap" @change="saveFlag('on_gap', ($event.target as HTMLInputElement).checked)"> Задача завершилась без продолжения — создать продолжение или игнорировать</label>

        <h3>Личный Telegram-бот</h3>
        <p v-if="!ns.telegram.available" class="sub">Недоступно: на сервере не задан ключ шифрования SECRETS_KEY (deploy/.env). Пока уведомления — только в колокольчике.</p>
        <template v-else-if="!ns.telegram.configured">
          <ol class="notify-steps">
            <li>Откройте в Telegram <a href="https://t.me/BotFather" target="_blank" rel="noopener">@BotFather</a>, отправьте <code>/newbot</code>, придумайте имя и адрес бота.</li>
            <li>Скопируйте токен, который пришлёт BotFather, и вставьте сюда. Токен хранится зашифрованным и больше нигде не показывается.</li>
            <li>Откройте появившуюся ссылку и нажмите «Старт» — бот привяжется к вам.</li>
          </ol>
          <form class="form-grid cols" @submit.prevent="connectBot">
            <label>Токен бота<input v-model="botToken" type="password" autocomplete="off" required placeholder="123456789:AA…"></label>
            <div class="form-actions"><button class="btn primary" type="submit" :disabled="nsBusy">{{ nsBusy ? 'Проверяю…' : 'Подключить' }}</button></div>
          </form>
        </template>
        <template v-else>
          <p v-if="ns.telegram.connected">✅ Подключён бот <b>@{{ ns.telegram.bot }}</b> — уведомления приходят вам в личные сообщения.</p>
          <p v-else>Бот <b>@{{ ns.telegram.bot }}</b> подключён, осталось привязать: <a :href="ns.telegram.link" target="_blank" rel="noopener">открыть бота и нажать «Старт»</a>.</p>
          <div class="form-actions">
            <button class="linkbtn" type="button" @click="disconnectBot">Отключить бота</button>
          </div>
        </template>
        <h3>Проверить уведомление</h3>
        <p class="sub">Пришлёт пример события: в колокольчик вверху страницы и, если бот привязан, в Telegram — с теми же кнопками, что у настоящих уведомлений.</p>
        <div class="form-actions">
          <select v-model="testKind" aria-label="Какое событие проверить">
            <option value="blocked">Задача попала в блок</option>
            <option value="unblock">Задачу можно вытащить из блока</option>
            <option value="gap">Задача завершилась без продолжения</option>
          </select>
          <button class="btn primary" type="button" :disabled="testBusy" @click="testNotify">{{ testBusy ? 'Отправляю…' : 'Проверить уведомление' }}</button>
        </div>
        <p v-if="nsMsg" class="sub" role="status">{{ nsMsg }}</p>
      </div>
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
        <p class="lead">Кто может входить в сервис и какие модули шапки видит. Модуль нужно выдать галочкой, и при этом роль должна его допускать (например, «Спринты» недоступны HR). Администратору доступны все модули. Пользователями управляет только администратор.</p>

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
          <fieldset class="mods">
            <legend>Модули в шапке</legend>
            <label v-for="m in MODULES" :key="m.key" class="chk"><input v-model="form.modules" type="checkbox" :value="m.key"> {{ m.title }}</label>
            <span v-if="form.role === 'admin'" class="sub">администратору доступны все</span>
          </fieldset>
          <div class="form-actions">
            <button class="btn primary" type="submit" :disabled="form.busy">Создать — пароль сгенерируется</button>
            <span v-if="form.err" class="form-error">{{ form.err }}</span>
          </div>
        </form>

        <p v-if="listErr" class="form-error">{{ listErr }}</p>
        <div class="tbl">
          <table>
            <thead><tr><th>Пользователь</th><th>Логин</th><th>Логин в Jira</th><th>Роль</th><th>Модули</th><th>Активен</th><th>Последний вход</th><th /></tr></thead>
            <tbody>
              <tr v-for="u in users" :key="u.id" :class="{ inactive: !u.active }">
                <template v-if="editing === u.id">
                  <td><input v-model="edit.full_name" :aria-label="`ФИО ${u.full_name}`" required>
                    <div v-if="rowErr[u.id]" class="form-error">{{ rowErr[u.id] }}</div></td>
                  <td><input v-model="edit.login" autocomplete="off" :aria-label="`Логин ${u.full_name}`" style="width:9em" required></td>
                  <td><input v-model="edit.employee_login" autocomplete="off" placeholder="не связан" :aria-label="`Логин в Jira ${u.full_name}`" style="width:9em"></td>
                </template>
                <template v-else>
                  <td>{{ u.full_name }}<span v-if="u.id === me?.id" class="pill mute" style="margin-left:6px">это вы</span>
                    <div v-if="rowErr[u.id]" class="form-error">{{ rowErr[u.id] }}</div></td>
                  <td><span class="key">{{ u.login }}</span></td>
                  <td><span v-if="u.employee_login" class="key">{{ u.employee_login }}</span><span v-else class="sub">не связан</span></td>
                </template>
                <td>
                  <select :value="u.role" :aria-label="`Роль ${u.full_name}`" @change="patchUser(u, { role: ($event.target as HTMLSelectElement).value as User['role'] })">
                    <option v-for="(t, r) in ROLE_TITLES" :key="r" :value="r">{{ t }}</option>
                  </select>
                </td>
                <td class="mods">
                  <label v-for="m in MODULES" :key="m.key" class="chk">
                    <input type="checkbox" :checked="u.modules.includes(m.key)" :disabled="u.role === 'admin'" :aria-label="`${m.title}: ${u.full_name}`"
                      @change="toggleModule(u, m.key, ($event.target as HTMLInputElement).checked)"> {{ m.title }}
                  </label>
                </td>
                <td><label class="chk"><input type="checkbox" :checked="u.active" @change="patchUser(u, { active: ($event.target as HTMLInputElement).checked })"> {{ u.active ? 'да' : 'заблокирован' }}</label></td>
                <td class="sub">{{ dt(u.last_login_at) }}</td>
                <td class="actions">
                  <template v-if="editing === u.id">
                    <button class="btn primary" :disabled="edit.busy" @click="saveEdit(u)">Сохранить</button>
                    <button class="linkbtn" @click="editing = null; rowErr = { ...rowErr, [u.id]: '' }">Отмена</button>
                  </template>
                  <template v-else>
                    <button class="linkbtn" @click="startEdit(u)">Изменить</button>
                    <button class="linkbtn" @click="resetPassword(u)">Сбросить пароль</button>
                  </template>
                </td>
              </tr>
              <tr v-if="!users.length"><td colspan="8" class="sub empty">Пользователей пока нет</td></tr>
            </tbody>
          </table>
        </div>
      </section>

      <section class="card">
        <h2>Руководители и проекты</h2>
        <p class="lead">Чьи проекты показывает «Контроль спринта руководителей». ФИО руководителя — как в Jira; ключ проекта — латиницей. Изменения попадут в отчёт после следующей синхронизации Jira.</p>
        <form class="form-grid cols" @submit.prevent="saveLeadProject()">
          <label>Руководитель<input v-model="lpForm.lead" list="lp-leads" autocomplete="off" required placeholder="Қойшыбай Темірлан"></label>
          <datalist id="lp-leads"><option v-for="l in lpLeads" :key="l" :value="l" /></datalist>
          <label>Ключ проекта<input v-model="lpForm.project_key" autocomplete="off" required placeholder="AC" style="text-transform:uppercase"></label>
          <label>Название для чипа<input v-model="lpForm.title" autocomplete="off" placeholder="Комплаенс"></label>
          <label>Порядок<input v-model.number="lpForm.position" type="number" min="0"></label>
          <div class="form-actions">
            <button class="btn primary" type="submit" :disabled="lpForm.busy">Добавить</button>
            <span v-if="lpErr" class="form-error">{{ lpErr }}</span>
          </div>
        </form>
        <div class="tbl">
          <table>
            <thead><tr><th>Руководитель</th><th>Проект</th><th>Название</th><th>Порядок</th><th /></tr></thead>
            <tbody>
              <tr v-for="x in leadProjects" :key="x.lead + x.project_key">
                <td>{{ x.lead }}</td>
                <td><span class="key">{{ x.project_key }}</span></td>
                <td><input :value="x.title" :aria-label="`Название ${x.project_key}`" @change="saveLeadProject({ ...x, title: ($event.target as HTMLInputElement).value })"></td>
                <td><input :value="x.position" type="number" min="0" style="width:5em" :aria-label="`Порядок ${x.project_key}`" @change="saveLeadProject({ ...x, position: Number(($event.target as HTMLInputElement).value) || 0 })"></td>
                <td><button class="linkbtn" @click="deleteLeadProject(x)">Убрать</button></td>
              </tr>
              <tr v-if="!leadProjects.length"><td colspan="5" class="sub empty">Руководителей пока нет — выполните сид или добавьте выше</td></tr>
            </tbody>
          </table>
        </div>
      </section>
    </template>
  </main>
</template>
