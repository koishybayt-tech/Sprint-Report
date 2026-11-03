<script setup lang="ts">
const { me, logout } = useMe()
const route = useRoute()
const { theme, next: nextTheme } = useTheme()
</script>

<template>
  <div>
    <div class="appbar">
      <NuxtLink to="/" class="brand">Отчёты Adata</NuxtLink>
      <nav class="appnav" aria-label="Разделы">
        <NuxtLink v-if="canSeeSprints(me)" to="/sprints" :class="{ on: route.path.startsWith('/sprints') }">Спринты</NuxtLink>
        <NuxtLink v-if="canSeeLeadControl(me)" to="/lead-control" :class="{ on: route.path.startsWith('/lead-control') }">Контроль спринта</NuxtLink>
        <NuxtLink v-if="canSeeAttendance(me)" to="/attendance" :class="{ on: route.path.startsWith('/attendance') }">Посещаемость</NuxtLink>
        <NuxtLink v-if="canSeeEmployees(me)" :to="me.role === 'employee' && me.employee_id ? `/employee/${me.employee_id}` : '/employee'" :class="{ on: route.path.startsWith('/employee') }">Сотрудники</NuxtLink>
        <NuxtLink to="/settings" :class="{ on: route.path.startsWith('/settings') }">Настройки</NuxtLink>
      </nav>
      <NotifyBell v-if="canSeeLeadControl(me)" />
      <button class="theme-btn" type="button" :title="`${THEME_TITLES[theme]} — нажмите, чтобы сменить`" :aria-label="THEME_TITLES[theme]" @click="nextTheme">
        <span aria-hidden="true">{{ theme === 'dark' ? '☾' : theme === 'light' ? '☀' : '◐' }}</span>
        {{ theme === 'dark' ? 'Тёмная' : theme === 'light' ? 'Светлая' : 'Авто' }}
      </button>
      <div v-if="me" class="who-am-i">
        <span>{{ me.full_name }}</span>
        <span class="sub">{{ ROLE_TITLES[me.role] }}</span>
        <button class="linkbtn" @click="logout">Выйти</button>
      </div>
    </div>
    <slot />
  </div>
</template>
