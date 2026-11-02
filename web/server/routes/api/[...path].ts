// Прокси /api/** → api-сервис. В проде /api перехватывает Caddy раньше, сюда запросы не доходят;
// нужен для локального запуска без Caddy (docker-compose.local.yml, make dev).
export default defineEventHandler((event) => {
  const upstream = useRuntimeConfig(event).apiUpstream.replace(/\/$/, '')
  return proxyRequest(event, upstream + event.path)
})
