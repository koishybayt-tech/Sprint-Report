<script setup lang="ts">
const { me, logout } = useMe()
const route = useRoute()
</script>

<template>
  <div>
    <div class="appbar">
      <NuxtLink to="/" class="brand">Отчёты Adata</NuxtLink>
      <nav class="appnav" aria-label="Разделы">
        <NuxtLink v-if="canSeeSprints(me)" to="/sprints" :class="{ on: route.path.startsWith('/sprints') }">Спринты</NuxtLink>
        <NuxtLink v-if="me" to="/attendance" :class="{ on: route.path.startsWith('/attendance') }">Посещаемость</NuxtLink>
        <NuxtLink to="/settings" :class="{ on: route.path.startsWith('/settings') }">Настройки</NuxtLink>
      </nav>
      <div v-if="me" class="who-am-i">
        <span>{{ me.full_name }}</span>
        <span class="sub">{{ ROLE_TITLES[me.role] }}</span>
        <button class="linkbtn" @click="logout">Выйти</button>
      </div>
    </div>
    <slot />
  </div>
</template>
