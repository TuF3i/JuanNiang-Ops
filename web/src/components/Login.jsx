import { useState } from 'react'
import { Button, Spinner, TextField, Input, Label, FieldError } from '@heroui/react'
import logoImg from '../assets/logo.jpg'
import logo2Img from '../assets/logo-2.png'

// 模拟后端账号表; 实际接入后端时替换为 API 调用
const MOCK_USERS = [
  { username: 'admin', password: 'admin123', nickname: '管理员' },
  { username: 'juan', password: 'juan123', nickname: '卷娘' },
]

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms))

// 眼睛图标 (密码可见切换)
function EyeIcon({ off }) {
  return off ? (
    <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="2">
      <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94" />
      <path d="M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19" />
      <path d="M14.12 14.12a3 3 0 1 1-4.24-4.24" />
      <line x1="1" y1="1" x2="23" y2="23" />
    </svg>
  ) : (
    <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="2">
      <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  )
}

export default function Login({ onLoginSuccess }) {
  // ---------- 表单状态 ----------
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [loading, setLoading] = useState(false)
  const [errorMsg, setErrorMsg] = useState('')

  const canSubmit = username.trim() !== '' && password !== '' && !loading

  // ---------- Mock 登录 ----------
  async function handleSubmit(e) {
    e.preventDefault()
    setErrorMsg('')
    if (!username.trim() || !password) return

    setLoading(true)
    try {
      // Mock: 模拟网络延迟 + 账号校验
      await sleep(800)
      const user = MOCK_USERS.find(
        (u) => u.username === username.trim() && u.password === password,
      )
      if (!user) {
        setErrorMsg('用户名或密码错误（试试 admin/admin123）')
        return
      }
      // Mock: 登录态存 sessionStorage (会话内有效, 关闭标签页即退出)
      const loggedIn = { username: user.username, nickname: user.nickname }
      sessionStorage.setItem('mock-login-user', JSON.stringify(loggedIn))
      onLoginSuccess(loggedIn)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="relative flex min-h-screen flex-col items-center justify-center gap-6 overflow-hidden bg-background px-4 py-8 font-sans">
      {/* 品牌光晕背景 (Tailwind + 内联渐变) */}
      <div
        aria-hidden
        className="pointer-events-none absolute inset-0"
        style={{
          background:
            'radial-gradient(640px 460px at 14% 16%, rgba(168,85,247,0.16), transparent 62%), radial-gradient(720px 520px at 86% 84%, rgba(236,72,153,0.14), transparent 62%)',
        }}
      />

      {/* 品牌: 页面右上角 */}
      <div className="fixed top-6 right-8 z-10 flex items-center gap-3">
        <img
          className="h-12 w-12 rounded-xl bg-white/95 object-contain p-0.5 shadow-[0_6px_18px_-6px_rgba(168,85,247,0.55)]"
          src={logoImg}
          alt="JuanNiang logo"
        />
        <span className="bg-gradient-to-r from-primary to-pink-500 bg-clip-text text-2xl font-extrabold text-transparent">
          JuanNiang
        </span>
      </div>

      <div className="z-10 flex w-full max-w-[1000px] min-h-[520px] flex-col overflow-hidden rounded-2xl border border-default-100 bg-content1 shadow-2xl md:flex-row">
        {/* 左侧: 登录表单 */}
        <div className="flex flex-1 basis-[46%] flex-col px-8 pt-10 pb-8 sm:px-14">
          <div className="mb-8 flex items-center justify-between">
            <h1 className="bg-gradient-to-r from-primary to-pink-500 bg-clip-text text-3xl font-bold tracking-wide text-transparent">
              登录
            </h1>
            <Button variant="ghost" size="sm" className="text-default-500">
              中文(简体) ▾
            </Button>
          </div>

          <form className="flex flex-col gap-5" onSubmit={handleSubmit}>
            {/* 用户名 */}
            <TextField name="username" value={username} onChange={setUsername}>
              <Label>用户名</Label>
              <Input placeholder="用户名" autoComplete="username" />
            </TextField>

            {/* 密码 (自绘可见切换) */}
            <TextField name="password" value={password} onChange={setPassword}>
              <Label>密码</Label>
              <div className="relative flex w-full">
                <Input
                  type={showPassword ? 'text' : 'password'}
                  placeholder="密码"
                  autoComplete="current-password"
                  className="w-full pr-11"
                />
                <Button
                  isIconOnly
                  variant="ghost"
                  size="sm"
                  aria-label={showPassword ? '隐藏密码' : '显示密码'}
                  tabIndex={-1}
                  className="absolute top-1/2 right-1.5 h-8 w-8 -translate-y-1/2 text-default-400"
                  onPress={() => setShowPassword((v) => !v)}
                >
                  <EyeIcon off={showPassword} />
                </Button>
              </div>
            </TextField>

            {/* 错误提示 */}
            {errorMsg && (
              <div
                role="alert"
                className="rounded-lg border border-pink-500/30 bg-pink-500/10 px-3.5 py-2.5 text-[13px] text-pink-400"
              >
                {errorMsg}
              </div>
            )}

            {/* 登录按钮 (HeroUI isPending 自带 Spinner + 禁用) */}
            <Button
              type="submit"
              fullWidth
              size="lg"
              isPending={loading}
              isDisabled={!canSubmit}
              className="mt-1 h-12 text-base font-semibold tracking-[4px] text-white"
              style={{ background: 'linear-gradient(135deg, #a855f7 0%, #d946ef 55%, #ec4899 100%)' }}
            >
              {loading ? '登录中...' : '登录'}
            </Button>
          </form>

          <p className="mt-auto pt-7 text-center text-xs text-default-400">
            Mock 模式: admin / admin123 或 juan / juan123
          </p>
        </div>

        {/* 右侧: 品牌插画区 */}
        <div
          className="relative hidden flex-1 basis-[54%] flex-col items-center justify-center gap-2 border-l border-default-100 md:flex"
          style={{
            background:
              'radial-gradient(460px 340px at 30% 28%, rgba(168,85,247,0.18), transparent 62%), radial-gradient(520px 400px at 74% 76%, rgba(236,72,153,0.14), transparent 60%)',
          }}
        >
          <div className="relative flex w-full max-w-[400px] justify-center p-6" aria-hidden>
            <img
              className="w-full select-none drop-shadow-[0_24px_48px_rgba(0,0,0,0.6)]"
              src={logo2Img}
              alt=""
              draggable="false"
            />
            <div
              className="absolute bottom-[8%] left-1/2 h-8 w-3/5 -translate-x-1/2 rounded-[50%] blur-[8px]"
              style={{ background: 'radial-gradient(ellipse at center, rgba(168,85,247,0.4), transparent 70%)' }}
            />
          </div>
        </div>
      </div>

      <footer className="z-10 text-xs tracking-wider text-default-400">
        JuanNiang-Ops · 卷娘巡检姬
      </footer>
    </div>
  )
}
