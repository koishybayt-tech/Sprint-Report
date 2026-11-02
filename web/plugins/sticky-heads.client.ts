// Заголовки таблиц не уезжают при прокрутке страницы.
// Таблицы лежат в контейнерах с собственной прокруткой (.tbl, .lc-scroll) — без них широкие таблицы нельзя листать вбок,
// а position: sticky у <th> работает только внутри такого контейнера. Поэтому при прокрутке страницы строка заголовков
// сдвигается (--head-shift) так, чтобы стоять под закреплённой шапкой страницы (.stick) или у верхнего края окна,
// пока видна сама таблица. Прокрутку внутри контейнера по-прежнему держит sticky.
export default defineNuxtPlugin(() => {
  function update() {
    const stick = document.querySelector<HTMLElement>('.stick')
    const top = stick ? Math.max(0, stick.getBoundingClientRect().bottom) : 0
    document.querySelectorAll<HTMLElement>('.tbl, .lc-scroll').forEach((box) => {
      const r = box.getBoundingClientRect()
      const head = box.querySelector('thead')
      if (!head || r.height === 0) return
      const max = Math.max(0, r.height - head.getBoundingClientRect().height - 4)
      const shift = Math.min(max, Math.max(0, top - r.top))
      const v = shift ? `${Math.round(shift)}px` : ''
      if (box.style.getPropertyValue('--head-shift') !== v) box.style.setProperty('--head-shift', v)
    })
  }
  // Пересчёт дешёвый (несколько таблиц) — прямо в обработчике прокрутки, без requestAnimationFrame:
  // его нет на фоновых вкладках, а заголовок должен стоять на месте сразу после возврата.
  let timer: ReturnType<typeof setTimeout> | undefined
  const schedule = () => {
    clearTimeout(timer)
    timer = setTimeout(update, 50)
  }
  window.addEventListener('scroll', update, { passive: true })
  window.addEventListener('resize', schedule)
  // Таблицы появляются и меняются (вкладки, фильтры, загрузка) — пересчитать после изменений DOM.
  new MutationObserver(schedule).observe(document.body, { childList: true, subtree: true })
})
