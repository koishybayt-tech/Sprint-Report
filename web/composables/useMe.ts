// Текущий пользователь (GET /api/v1/me). null — не вошёл.
export type Me = { id: number; login: string; full_name: string; role: 'admin' | 'lead' | 'hr' | 'employee'; employee_id: number | null }

export const ROLE_TITLES: Record<Me['role'], string> = {
  admin: 'Администратор',
  lead: 'Руководитель',
  hr: 'HR',
  employee: 'Сотрудник',
}

// Кто какие отчёты видит — docs/security.md.
export const canSeeSprints = (me: Me | null) => !!me && me.role !== 'hr'
// Сохранять и удалять снепшоты «Спринтов» — admin и lead (docs/security.md).
export const canSeeSnapshots = (me: Me | null) => !!me && (me.role === 'admin' || me.role === 'lead')
// «Контроль спринта руководителей» — admin и lead (docs/lead-control.md).
export const canSeeLeadControl = canSeeSnapshots

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
