// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  compatibilityDate: '2025-07-01',
  devtools: { enabled: false },
  // Внутренний инструмент: страницы рендерятся только на клиенте.
  routeRules: { '/**': { ssr: false } },
  runtimeConfig: {
    // Куда web-сервер проксирует /api/** (локально без Caddy). Env: NUXT_API_UPSTREAM.
    apiUpstream: 'http://api:8080',
    public: {
      apiBase: '/api/v1',   // NUXT_PUBLIC_API_BASE
      tgBotUsername: '',    // NUXT_PUBLIC_TG_BOT_USERNAME
      jiraBase: '',         // NUXT_PUBLIC_JIRA_BASE
    },
  },
})
