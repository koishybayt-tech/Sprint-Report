// Тема оформления: «авто» (как в системе), светлая или тёмная. Выбор — удобство конкретного браузера,
// поэтому хранится в localStorage (ключ «theme»); до отрисовки его применяет скрипт в <head> (nuxt.config.ts).
export type Theme = 'auto' | 'light' | 'dark'

export const THEME_TITLES: Record<Theme, string> = { auto: 'Тема: авто', light: 'Тема: светлая', dark: 'Тема: тёмная' }

export function useTheme() {
  const theme = useState<Theme>('theme', () => {
    try {
      const t = localStorage.getItem('theme')
      return t === 'light' || t === 'dark' ? t : 'auto'
    } catch { return 'auto' }
  })
  function setTheme(t: Theme) {
    theme.value = t
    const root = document.documentElement
    if (t === 'auto') root.removeAttribute('data-theme')
    else root.setAttribute('data-theme', t)
    try { t === 'auto' ? localStorage.removeItem('theme') : localStorage.setItem('theme', t) } catch { /* приватный режим */ }
  }
  // По кругу: авто → светлая → тёмная.
  const next = () => setTheme(theme.value === 'auto' ? 'light' : theme.value === 'light' ? 'dark' : 'auto')
  return { theme, setTheme, next }
}
