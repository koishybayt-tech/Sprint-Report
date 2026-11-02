<script setup lang="ts">
type Health = { status: string; db: string }

const { apiBase } = useRuntimeConfig().public
const health = ref<Health | null>(null)
const error = ref('')
const loading = ref(true)

async function check() {
  loading.value = true
  error.value = ''
  try {
    health.value = await $fetch<Health>(`${apiBase}/healthz`)
  } catch (e: any) {
    health.value = e?.data ?? null
    error.value = e?.statusCode ? `HTTP ${e.statusCode}` : String(e?.message ?? e)
  } finally {
    loading.value = false
  }
}

onMounted(check)
</script>

<template>
  <main class="page">
    <h1>Adata Reports</h1>
    <p v-if="loading">Проверяю API…</p>
    <template v-else>
      <p :class="error ? 'bad' : 'ok'">
        API: <b>{{ error ? 'недоступен' : health?.status }}</b>
        <span v-if="error"> ({{ error }})</span>
      </p>
      <p v-if="health">База: <b>{{ health.db }}</b></p>
    </template>
    <button :disabled="loading" @click="check">Проверить ещё раз</button>
  </main>
</template>

<style>
body { margin: 0; font-family: system-ui, sans-serif; background: #f6f7f9; color: #1d2330; }
.page { max-width: 480px; margin: 64px auto; padding: 0 16px; }
.ok b { color: #16803c; }
.bad b { color: #c0392b; }
button { padding: 6px 14px; border: 1px solid #c7ccd6; border-radius: 6px; background: #fff; cursor: pointer; }
</style>
