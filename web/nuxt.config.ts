// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  compatibilityDate: '2025-07-01',
  devtools: { enabled: false },
  // Внутренний инструмент: страницы рендерятся только на клиенте.
  routeRules: { '/**': { ssr: false } },
  css: ['~/assets/main.css'],
  app: {
    head: {
      htmlAttrs: { lang: 'ru' },
      title: 'Отчёты Adata',
      link: [
        { rel: 'preconnect', href: 'https://fonts.googleapis.com' },
        { rel: 'stylesheet', href: 'https://fonts.googleapis.com/css2?family=IBM+Plex+Sans:wght@400;500;600;700&family=IBM+Plex+Mono:wght@400;500&display=swap' },
      ],
    },
  },
  runtimeConfig: {
    // Куда web-сервер проксирует /api/** (локально без Caddy). Env: NUXT_API_UPSTREAM.
    apiUpstream: 'http://api:8080',
    public: {
      apiBase: '/api/v1',                 // NUXT_PUBLIC_API_BASE
      tgBotUsername: '',                  // NUXT_PUBLIC_TG_BOT_USERNAME
      jiraBase: 'https://jira.adata.kz',  // NUXT_PUBLIC_JIRA_BASE
    },
  },
})
