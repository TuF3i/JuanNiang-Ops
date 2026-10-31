import { useMemo, useState } from 'react'
import logoImg from '../assets/logo.jpg'
import logo2Img from '../assets/logo-2.png'
import './Login.css'

// 字段级校验错误初始值
const INITIAL_FIELD_ERRORS = { username: '', password: '' }

// 模拟后端账号表; 实际接入后端时替换为 API 调用
const MOCK_USERS = [
  { username: 'admin', password: 'admin123', nickname: '管理员' },
  { username: 'juan', password: 'juan123', nickname: '卷娘' },
]

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms))

export default function Login({ onLoginSuccess }) {
  // ---------- 表单状态 ----------
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [loading, setLoading] = useState(false)
  const [errorMsg, setErrorMsg] = useState('')
  const [fieldErrors, setFieldErrors] = useState(INITIAL_FIELD_ERRORS)

  const canSubmit = useMemo(
    () => username.trim() !== '' && password !== '' && !loading,
    [username, password, loading],
  )

  // ---------- Mock 登录 ----------
  async function handleSubmit(e) {
    e.preventDefault()
    setErrorMsg('')
    if (!validate()) return

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

  // ---------- 校验 ----------
  function validate() {
    const next = { ...INITIAL_FIELD_ERRORS }
    let ok = true
    if (!username.trim()) {
      next.username = '请输入用户名'
      ok = false
    }
    if (!password) {
      next.password = '请输入密码'
      ok = false
    }
    setFieldErrors(next)
    return ok
  }

  function clearError(field) {
    setFieldErrors((prev) => ({ ...prev, [field]: '' }))
    setErrorMsg('')
  }

  return (
    <div className="login-page">
      {/* 品牌: 页面右上角 (不在登录卡片内) */}
      <div className="brand">
        <img className="brand-logo" src={logoImg} alt="JuanNiang logo" />
        <span className="brand-name">JuanNiang</span>
      </div>

      <div className="login-card">
        {/* 左侧: 登录表单 */}
        <div className="login-left">
          <div className="login-header">
            <h1 className="login-title">登录</h1>
            <button className="lang-switch" type="button" title="语言">
              <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="2">
                <circle cx="12" cy="12" r="10" />
                <path d="M2 12h20M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z" />
              </svg>
              中文(简体)
              <span className="lang-caret">▾</span>
            </button>
          </div>

          <form className="login-form" onSubmit={handleSubmit}>
            {/* 用户名 */}
            <div className={'field' + (fieldErrors.username ? ' field-error' : '')}>
              <input
                type="text"
                placeholder="用户名"
                autoComplete="username"
                value={username}
                onChange={(e) => {
                  setUsername(e.target.value)
                  clearError('username')
                }}
              />
              {fieldErrors.username && <p className="field-msg">{fieldErrors.username}</p>}
            </div>

            {/* 密码 */}
            <div className={'field' + (fieldErrors.password ? ' field-error' : '')}>
              <div className="password-wrap">
                <input
                  type={showPassword ? 'text' : 'password'}
                  placeholder="密码"
                  autoComplete="current-password"
                  value={password}
                  onChange={(e) => {
                    setPassword(e.target.value)
                    clearError('password')
                  }}
                />
                <button
                  className="password-toggle"
                  type="button"
                  tabIndex={-1}
                  title={showPassword ? '隐藏密码' : '显示密码'}
                  onClick={() => setShowPassword((v) => !v)}
                >
                  {/* 眼睛图标 */}
                  {!showPassword ? (
                    <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="2">
                      <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
                      <circle cx="12" cy="12" r="3" />
                    </svg>
                  ) : (
                    <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="2">
                      <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94" />
                      <path d="M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19" />
                      <path d="M14.12 14.12a3 3 0 1 1-4.24-4.24" />
                      <line x1="1" y1="1" x2="23" y2="23" />
                    </svg>
                  )}
                </button>
              </div>
              {fieldErrors.password && <p className="field-msg">{fieldErrors.password}</p>}
            </div>

            {/* 错误提示 */}
            {errorMsg && (
              <div className="alert-error" role="alert">
                {errorMsg}
              </div>
            )}

            {/* 登录按钮 */}
            <button className="btn-login" type="submit" disabled={!canSubmit}>
              {loading && <span className="spinner" aria-hidden="true" />}
              <span>{loading ? '登录中...' : '登录'}</span>
            </button>
          </form>

          <p className="login-hint">Mock 模式: admin / admin123 或 juan / juan123</p>
        </div>

        {/* 右侧: 品牌插画区 */}
        <div className="login-right">
          {/* 主插画 */}
          <div className="illustration" aria-hidden="true">
            <img className="illustration-img" src={logo2Img} alt="" draggable="false" />
            <div className="iso-glow" />
          </div>
        </div>
      </div>

      <footer className="login-footer">JuanNiang-Ops · 卷娘巡检姬</footer>
    </div>
  )
}
