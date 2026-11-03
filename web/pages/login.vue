<script setup lang="ts">
definePageMeta({ layout: false })

const { apiBase } = useRuntimeConfig().public
const route = useRoute()
const { load } = useMe()

const login = ref('')
const password = ref('')
const error = ref('')
const busy = ref(false)

async function submit() {
  error.value = ''
  busy.value = true
  try {
    await $fetch(`${apiBase}/auth/login`, { method: 'POST', body: { login: login.value, password: password.value } })
    const me = await load()
    const next = typeof route.query.next === 'string' && route.query.next.startsWith('/') ? route.query.next : null
    await navigateTo(next || firstPage(me))
  } catch (e: any) {
    error.value = e?.data?.error || (e?.statusCode ? `Ошибка ${e.statusCode}` : 'Сервис недоступен')
    password.value = ''
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <main class="login">
    <form class="login-card" @submit.prevent="submit">
      <h1>Отчёты Adata</h1>
      <label>Логин<input v-model="login" autocomplete="username" required autofocus></label>
      <label>Пароль<input v-model="password" type="password" autocomplete="current-password" required></label>
      <p v-if="error" class="form-error" role="alert">{{ error }}</p>
      <button class="btn primary" type="submit" :disabled="busy">{{ busy ? 'Вход…' : 'Войти' }}</button>
      <p class="sub">Нет доступа — попросите администратора создать пользователя.</p>
    </form>
  </main>
</template>
