<script setup>
import { ref, onMounted } from 'vue'
import Login from './components/Login.vue'
import Welcome from './components/Welcome.vue'

// Mock 登录态: sessionStorage (会话内有效, 关闭标签页即退出)
const user = ref(null)
const STORAGE_KEY = 'mock-login-user'

onMounted(() => {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY)
    if (raw) user.value = JSON.parse(raw)
  } catch {
    /* 忽略损坏的存储数据 */
  }
})

function handleLoginSuccess(loggedIn) {
  user.value = loggedIn
}

function handleLogout() {
  sessionStorage.removeItem(STORAGE_KEY)
  user.value = null
}
</script>

<template>
  <Welcome v-if="user" :user="user" @logout="handleLogout" />
  <Login v-else @login-success="handleLoginSuccess" />
</template>
