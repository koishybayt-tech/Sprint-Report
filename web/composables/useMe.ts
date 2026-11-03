// Текущий пользователь (GET /api/v1/me). null — не вошёл.
export type Me = { id: number; login: string; full_name: string; role: 'admin' | 'lead' | 'hr' | 'employee'; employee_id: number | null; modules: string[]; has_projects?: boolean }

// Модули шапки: ключ совпадает с users.modules на сервере; у админа в me.modules всегда все.
export const MODULES = [
  { key: 'sprints', title: 'Спринты' },
  { key: 'lead_control', title: 'Контроль спринта' },
  { key: 'attendance', title: 'Посещаемость' },
  { key: 'employees', title: 'Сотрудники' },
] as const
export const hasModule = (me: Me | null, key: string) => !!me && (me.modules ?? []).includes(key)

export const ROLE_TITLES: Record<Me['role'], string> = {
  admin: 'Администратор',
  lead: 'Руководитель',
  hr: 'HR',
  employee: 'Сотрудник',
}

// Кто какие отчёты видит — docs/security.md.
// Роль ограничивает, модуль (Настройки → Пользователи) выдаёт; сервер проверяет оба условия.
export const canSeeSprints = (me: Me | null) => !!me && me.role !== 'hr' && hasModule(me, 'sprints')
// Сохранять и удалять снепшоты «Спринтов» — admin и lead (docs/security.md).
export const canSeeSnapshots = (me: Me | null) => !!me && (me.role === 'admin' || me.role === 'lead')
// «Контроль спринта руководителей» — admin и lead (docs/lead-control.md).
// Плюс любой пользователь, которому админ назначил проекты («Настройки → Руководители и проекты»), например ПМ.
export const canSeeLeadControl = (me: Me | null) => (canSeeSnapshots(me) || !!me?.has_projects) && hasModule(me, 'lead_control')
// Экран для ТВ (/tv) — пока только администратор; расширять доступ — здесь и в middleware.
export const canSeeTv = (me: Me | null) => !!me && me.role === 'admin'
export const canSeeAttendance =(me: Me | null) => hasModule(me, 'attendance')
export const canSeeEmployees = (me: Me | null) => hasModule(me, 'employees')

// Первая доступная страница: после входа и при переходе на закрытый модуль. «Настройки» открыты всем.
export function firstPage(me: Me | null): string {
  if (canSeeSprints(me)) return '/sprints'
  if (canSeeLeadControl(me)) return '/lead-control'
  if (canSeeAttendance(me)) return '/attendance'
  if (canSeeEmployees(me)) return me && me.role === 'employee' && me.employee_id ? `/employee/${me.employee_id}` : '/employee'
  return '/settings'
}

export function useMe() {
  const me = useState<Me | null>('me', () => null)
  const { apiBase } = useRuntimeConfig().public

  async function load() {
    try {
      me.value = await $fetch<Me>(`${apiBase}/me`)
    } catch {
      me.value = null
    }
    return me.value
  }

  async function logout() {
    await $fetch(`${apiBase}/auth/logout`, { method: 'POST' }).catch(() => {})
    me.value = null
    await navigateTo('/login')
  }

  return { me, load, logout }
}
