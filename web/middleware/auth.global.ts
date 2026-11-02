// Без входа доступны только /login и /status; остальное — после входа. /settings для всех, кто вошёл.
const PUBLIC = ['/login', '/status']

export default defineNuxtRouteMiddleware(async (to) => {
  if (PUBLIC.includes(to.path)) return
  const { me, load } = useMe()
  if (!me.value) await load()
  if (!me.value) return navigateTo({ path: '/login', query: to.fullPath !== '/' ? { next: to.fullPath } : {} })
  // HR не видит «Спринты» (docs/security.md) — ведём в «Посещаемость».
  if (to.path.startsWith('/sprints') && !canSeeSprints(me.value)) return navigateTo('/attendance')
  // Отдельной страницы снепшотов больше нет — они открываются в «Спринтах».
  if (to.path.startsWith('/snapshots')) return navigateTo({ path: '/sprints', query: typeof to.query.date === 'string' ? { snap: to.query.date } : {} })
  if (to.path.startsWith('/lead-control') && !canSeeLeadControl(me.value)) return navigateTo('/attendance')
})
