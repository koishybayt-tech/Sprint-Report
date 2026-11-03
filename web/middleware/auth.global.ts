// Без входа доступны только /login и /status; остальное — после входа. /settings для всех, кто вошёл.
const PUBLIC = ['/login', '/status']

export default defineNuxtRouteMiddleware(async (to) => {
  if (PUBLIC.includes(to.path)) return
  const { me, load } = useMe()
  if (!me.value) await load()
  if (!me.value) return navigateTo({ path: '/login', query: to.fullPath !== '/' ? { next: to.fullPath } : {} })
  // Нет роли или модуля (HR не видит «Спринты», docs/security.md) — ведём на первую доступную страницу.
  if (to.path.startsWith('/sprints') && !canSeeSprints(me.value)) return navigateTo(firstPage(me.value))
  // Отдельной страницы снепшотов больше нет — они открываются в «Спринтах».
  if (to.path.startsWith('/snapshots')) return navigateTo({ path: '/sprints', query: typeof to.query.date === 'string' ? { snap: to.query.date } : {} })
  if (to.path.startsWith('/lead-control') && !canSeeLeadControl(me.value)) return navigateTo(firstPage(me.value))
  if (to.path.startsWith('/tv') && !canSeeTv(me.value)) return navigateTo(firstPage(me.value))
  if (to.path.startsWith('/attendance') &&!canSeeAttendance(me.value)) return navigateTo(firstPage(me.value))
  if (to.path.startsWith('/employee') && !canSeeEmployees(me.value)) return navigateTo(firstPage(me.value))
})
