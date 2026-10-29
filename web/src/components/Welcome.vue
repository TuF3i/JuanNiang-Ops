<script setup>
// 登录成功后的占位页: Mock 登录态展示 (后端就绪后替换为真实工作台)
defineProps({
  user: { type: Object, required: true },
})
const emit = defineEmits(['logout'])
</script>

<template>
  <div class="welcome-page">
    <div class="welcome-card">
      <div class="welcome-icon">🎀</div>
      <h1>欢迎回来, {{ user.nickname }}!</h1>
      <p class="welcome-sub">已通过 Mock 登录 (<code>{{ user.username }}</code>)，工作台建设中…</p>
      <button class="btn-logout" type="button" @click="emit('logout')">退出登录</button>
    </div>
  </div>
</template>

<style scoped>
.welcome-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  font-family: system-ui, 'Segoe UI', 'Microsoft YaHei', sans-serif;
  background:
    radial-gradient(640px 460px at 14% 16%, rgba(168, 85, 247, 0.16), transparent 62%),
    radial-gradient(720px 520px at 86% 84%, rgba(236, 72, 153, 0.14), transparent 62%),
    #0c0a12;
}
.welcome-card {
  text-align: center;
  padding: 64px 80px;
  background: #14111c;
  border: 1px solid #2a2438;
  border-radius: 16px;
  box-shadow: 0 24px 60px -20px rgba(0, 0, 0, 0.8), 0 0 80px -30px rgba(168, 85, 247, 0.35);
}
.welcome-icon { font-size: 56px; margin-bottom: 12px; }
h1 {
  margin: 0 0 8px;
  font-size: 26px;
  background: linear-gradient(135deg, #a855f7, #ec4899);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
}
.welcome-sub { margin: 0 0 28px; color: #9a8fb0; font-size: 14px; }
.welcome-sub code { color: #a855f7; background: rgba(168, 85, 247, 0.1); padding: 2px 6px; border-radius: 6px; }
.btn-logout {
  height: 40px;
  padding: 0 28px;
  border: none;
  border-radius: 10px;
  font-size: 14px;
  color: #fff;
  cursor: pointer;
  background: linear-gradient(135deg, #a855f7, #ec4899);
  transition: filter 0.2s;
}
.btn-logout:hover { filter: brightness(1.1); }
</style>
