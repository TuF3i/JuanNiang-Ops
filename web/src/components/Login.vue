<script setup>
import { ref, computed } from 'vue'
import logoImg from '../assets/logo.jpg'
import logo2Img from '../assets/logo-2.png'

const emit = defineEmits(['login-success'])

// ---------- 表单状态 ----------
const username = ref('')
const password = ref('')
const showPassword = ref(false)
const loading = ref(false)
const errorMsg = ref('')
const formRef = ref(null)

// 字段级校验错误
const fieldErrors = ref({ username: '', password: '' })

const canSubmit = computed(() => username.value.trim() !== '' && password.value !== '' && !loading.value)

// ---------- Mock 登录 ----------
// 模拟后端账号表; 实际接入后端时替换为 API 调用
const MOCK_USERS = [
  { username: 'admin', password: 'admin123', nickname: '管理员' },
  { username: 'juan', password: 'juan123', nickname: '卷娘' },
]

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms))

async function handleSubmit() {
  errorMsg.value = ''
  if (!validate()) return

  loading.value = true
  try {
    // Mock: 模拟网络延迟 + 账号校验
    await sleep(800)
    const user = MOCK_USERS.find(
      (u) => u.username === username.value.trim() && u.password === password.value,
    )
    if (!user) {
      errorMsg.value = '用户名或密码错误（试试 admin/admin123）'
      return
    }
    // Mock: 登录态存 sessionStorage (会话内有效, 关闭标签页即退出)
    sessionStorage.setItem('mock-login-user', JSON.stringify({ username: user.username, nickname: user.nickname }))
    emit('login-success', { username: user.username, nickname: user.nickname })
  } finally {
    loading.value = false
  }
}

// ---------- 校验 ----------
function validate() {
  fieldErrors.value = { username: '', password: '' }
  let ok = true
  if (!username.value.trim()) {
    fieldErrors.value.username = '请输入用户名'
    ok = false
  }
  if (!password.value) {
    fieldErrors.value.password = '请输入密码'
    ok = false
  }
  return ok
}

function clearError(field) {
  fieldErrors.value[field] = ''
  errorMsg.value = ''
}
</script>

<template>
  <div class="login-page">
    <!-- 品牌: 页面右上角 (不在登录卡片内) -->
    <div class="brand">
      <img class="brand-logo" :src="logoImg" alt="JuanNiang logo" />
      <span class="brand-name">JuanNiang</span>
    </div>

    <div class="login-card">
      <!-- 左侧: 登录表单 -->
      <div class="login-left">
        <div class="login-header">
          <h1 class="login-title">登录</h1>
          <button class="lang-switch" type="button" title="语言">
            <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="12" r="10" />
              <path d="M2 12h20M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z" />
            </svg>
            中文(简体)
            <span class="lang-caret">▾</span>
          </button>
        </div>

        <form ref="formRef" class="login-form" @submit.prevent="handleSubmit">
          <!-- 用户名 -->
          <div class="field" :class="{ 'field-error': fieldErrors.username }">
            <input
              v-model="username"
              type="text"
              placeholder="用户名"
              autocomplete="username"
              @input="clearError('username')"
            />
            <p v-if="fieldErrors.username" class="field-msg">{{ fieldErrors.username }}</p>
          </div>

          <!-- 密码 -->
          <div class="field" :class="{ 'field-error': fieldErrors.password }">
            <div class="password-wrap">
              <input
                v-model="password"
                :type="showPassword ? 'text' : 'password'"
                placeholder="密码"
                autocomplete="current-password"
                @input="clearError('password')"
              />
              <button
                class="password-toggle"
                type="button"
                tabindex="-1"
                :title="showPassword ? '隐藏密码' : '显示密码'"
                @click="showPassword = !showPassword"
              >
                <!-- 眼睛图标 -->
                <svg v-if="!showPassword" viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
                  <circle cx="12" cy="12" r="3" />
                </svg>
                <svg v-else viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94" />
                  <path d="M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19" />
                  <path d="M14.12 14.12a3 3 0 1 1-4.24-4.24" />
                  <line x1="1" y1="1" x2="23" y2="23" />
                </svg>
              </button>
            </div>
            <p v-if="fieldErrors.password" class="field-msg">{{ fieldErrors.password }}</p>
          </div>

          <!-- 错误提示 -->
          <transition name="fade">
            <div v-if="errorMsg" class="alert-error" role="alert">{{ errorMsg }}</div>
          </transition>

          <!-- 登录按钮 -->
          <button class="btn-login" type="submit" :disabled="!canSubmit">
            <span v-if="loading" class="spinner" aria-hidden="true"></span>
            <span>{{ loading ? '登录中...' : '登录' }}</span>
          </button>
        </form>

        <p class="login-hint">Mock 模式: admin / admin123 或 juan / juan123</p>
      </div>

      <!-- 右侧: 品牌插画区 -->
      <div class="login-right">
        <!-- 主插画 -->
        <div class="illustration" aria-hidden="true">
          <img class="illustration-img" :src="logo2Img" alt="" draggable="false" />
          <div class="iso-glow"></div>
        </div>
      </div>
    </div>

    <footer class="login-footer">JuanNiang-Ops · 卷娘巡检姬</footer>
  </div>
</template>

<style scoped>
/* ---------- 主题变量 (黑色风格, 紫粉点缀) ---------- */
.login-page {
  --primary: #a855f7;          /* 点缀: 紫 */
  --primary-pink: #ec4899;     /* 点缀: 粉 */
  --primary-grad: linear-gradient(135deg, #a855f7 0%, #d946ef 55%, #ec4899 100%);
  --bg-deep: #0c0a12;          /* 页面深黑 */
  --card-bg: #14111c;          /* 卡片黑 */
  --card-right-bg: #191423;    /* 右半区稍亮 */
  --border-dark: #2a2438;      /* 暗边框 */
  --text-main: #efeaf6;        /* 主文字 */
  --text-sub: #9a8fb0;         /* 次文字 */
  --input-bg: #1d1828;         /* 输入框底 */
  --input-border: #322a44;
  --error: #f472b6;

  min-height: 100vh;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 24px;
  padding: 32px 16px;
  font-family: system-ui, 'Segoe UI', 'Microsoft YaHei', sans-serif;
  color: var(--text-main);
  /* 深黑背景 + 紫粉光晕 */
  background:
    radial-gradient(640px 460px at 14% 16%, rgba(168, 85, 247, 0.16), transparent 62%),
    radial-gradient(720px 520px at 86% 84%, rgba(236, 72, 153, 0.14), transparent 62%),
    var(--bg-deep);
}

/* ---------- 卡片 ---------- */
.login-card {
  display: flex;
  width: min(1000px, 94vw);
  min-height: 520px;
  background: var(--card-bg);
  border: 1px solid var(--border-dark);
  border-radius: 16px;
  overflow: hidden;
  box-shadow:
    0 24px 60px -20px rgba(0, 0, 0, 0.8),
    0 0 80px -30px rgba(168, 85, 247, 0.35);
}

/* ---------- 左侧表单 ---------- */
.login-left {
  flex: 1 1 46%;
  padding: 56px 56px 32px;
  display: flex;
  flex-direction: column;
}

.login-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 36px;
}

.login-title {
  margin: 0;
  font-size: 30px;
  font-weight: 700;
  letter-spacing: 1px;
  background: var(--primary-grad);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
}

.lang-switch {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border: none;
  background: none;
  color: var(--text-sub);
  font-size: 13px;
  cursor: pointer;
  padding: 6px 8px;
  border-radius: 8px;
  transition: background 0.2s, color 0.2s;
}
.lang-switch:hover {
  background: rgba(168, 85, 247, 0.12);
  color: var(--primary);
}
.lang-caret { font-size: 10px; }

.login-form { display: flex; flex-direction: column; gap: 18px; }

.field { display: flex; flex-direction: column; gap: 6px; }

.field input {
  width: 100%;
  height: 46px;
  padding: 0 14px;
  font-size: 15px;
  color: var(--text-main);
  background: var(--input-bg);
  border: 1px solid var(--input-border);
  border-radius: 10px;
  outline: none;
  transition: border-color 0.2s, box-shadow 0.2s;
}
.field input::placeholder { color: #6d6182; }
.field input:focus {
  border-color: var(--primary);
  box-shadow: 0 0 0 3px rgba(168, 85, 247, 0.2);
}
.field-error input {
  border-color: var(--error);
  box-shadow: 0 0 0 3px rgba(244, 114, 182, 0.16);
}
.field-msg {
  margin: 0;
  font-size: 12px;
  color: var(--error);
}

/* 密码框 + 眼睛 */
.password-wrap { position: relative; }
.password-wrap input { padding-right: 44px; }
.password-toggle {
  position: absolute;
  top: 0;
  right: 0;
  height: 46px;
  width: 44px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: none;
  background: none;
  color: #6d6182;
  cursor: pointer;
  border-radius: 0 10px 10px 0;
  transition: color 0.2s;
}
.password-toggle:hover { color: var(--primary); }

/* 错误提示条 */
.alert-error {
  padding: 10px 14px;
  font-size: 13px;
  color: var(--error);
  background: rgba(244, 114, 182, 0.08);
  border: 1px solid rgba(244, 114, 182, 0.3);
  border-radius: 10px;
}
.fade-enter-active, .fade-leave-active { transition: opacity 0.25s; }
.fade-enter-from, .fade-leave-to { opacity: 0; }

/* 登录按钮 (紫粉渐变) */
.btn-login {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  height: 48px;
  border: none;
  border-radius: 10px;
  font-size: 16px;
  font-weight: 600;
  letter-spacing: 4px;
  color: #fff;
  background: var(--primary-grad);
  cursor: pointer;
  transition: transform 0.15s, box-shadow 0.2s, filter 0.2s;
  box-shadow: 0 8px 24px -8px rgba(168, 85, 247, 0.65);
}
.btn-login:hover:not(:disabled) {
  filter: brightness(1.1);
  box-shadow: 0 10px 28px -8px rgba(236, 72, 153, 0.7);
  transform: translateY(-1px);
}
.btn-login:active:not(:disabled) { transform: translateY(0); }
.btn-login:disabled {
  opacity: 0.45;
  cursor: not-allowed;
  box-shadow: none;
}

.spinner {
  width: 16px;
  height: 16px;
  border: 2px solid rgba(255, 255, 255, 0.4);
  border-top-color: #fff;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}
@keyframes spin { to { transform: rotate(360deg); } }

.login-hint {
  margin-top: auto;
  padding-top: 28px;
  font-size: 12px;
  color: #6d6182;
  text-align: center;
}

/* ---------- 右侧品牌插画 ---------- */
.login-right {
  flex: 1 1 54%;
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  background:
    radial-gradient(460px 340px at 30% 28%, rgba(168, 85, 247, 0.18), transparent 62%),
    radial-gradient(520px 400px at 74% 76%, rgba(236, 72, 153, 0.14), transparent 60%),
    var(--card-right-bg);
  border-left: 1px solid var(--border-dark);
}

/* ---------- 品牌 (页面右上角, fixed 于视口) ---------- */
.brand {
  position: fixed;
  top: 24px;
  right: 32px;
  display: flex;
  align-items: center;
  gap: 12px;
  z-index: 10;
}
/* logo 圆角小图: contain 完整显示 (不裁切) */
.brand-logo {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  object-fit: contain;
  background: rgba(255, 255, 255, 0.96); /* jpg 白底与圆角底融合 */
  padding: 2px;
  box-shadow: 0 6px 18px -6px rgba(168, 85, 247, 0.55);
}
.brand-name {
  font-size: 24px;
  font-weight: 800;
  letter-spacing: 0.5px;
  background: var(--primary-grad);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
}

/* 主插画 */
.illustration {
  position: relative;
  width: 100%;
  max-width: 400px;
  padding: 24px;
  display: flex;
  justify-content: center;
}
.illustration-img {
  width: 100%;
  height: auto;
  user-select: none;
  filter:
    drop-shadow(0 24px 48px rgba(0, 0, 0, 0.6))
    drop-shadow(0 0 36px rgba(168, 85, 247, 0.35));
}
/* 底部光晕 */
.iso-glow {
  position: absolute;
  bottom: 8%;
  left: 50%;
  translate: -50% 0;
  width: 62%;
  height: 34px;
  border-radius: 50%;
  background: radial-gradient(ellipse at center, rgba(168, 85, 247, 0.4), transparent 70%);
  filter: blur(8px);
  animation: glow-pulse 4.2s ease-in-out infinite;
}
@keyframes glow-pulse {
  0%, 100% { opacity: 0.7; transform: scaleX(1); }
  50% { opacity: 1; transform: scaleX(0.88); }
}

/* ---------- 底部 ---------- */
.login-footer {
  font-size: 12px;
  color: #6d6182;
  letter-spacing: 1px;
}

/* ---------- 响应式 ---------- */
@media (max-width: 860px) {
  .login-card { flex-direction: column; min-height: unset; }
  .login-right { display: none; }
  .login-left { padding: 40px 28px 24px; }
}
</style>
