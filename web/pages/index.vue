<script setup lang="ts">
const { me, load } = useMe()
if (!me.value) await load()
await navigateTo(firstPage(me.value), { replace: true })
</script>

<template>
  <div />
</template>
