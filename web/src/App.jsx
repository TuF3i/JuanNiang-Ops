import { useEffect, useState } from 'react'
import Login from './components/Login.jsx'
import Welcome from './components/Welcome.jsx'

// Mock 登录态: sessionStorage (会话内有效, 关闭标签页即退出)
const STORAGE_KEY = 'mock-login-user'

function readStoredUser() {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY)
    return raw ? JSON.parse(raw) : null
  } catch {
    return null // 忽略损坏的存储数据
  }
}

export default function App() {
  const [user, setUser] = useState(null)

  useEffect(() => {
    setUser(readStoredUser())
  }, [])

  function handleLoginSuccess(loggedIn) {
    setUser(loggedIn)
  }

  function handleLogout() {
    sessionStorage.removeItem(STORAGE_KEY)
    setUser(null)
  }

  if (user) return <Welcome user={user} onLogout={handleLogout} />
  return <Login onLoginSuccess={handleLoginSuccess} />
}
