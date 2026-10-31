import './Welcome.css'

// 登录成功后的占位页: Mock 登录态展示 (后端就绪后替换为真实工作台)
export default function Welcome({ user, onLogout }) {
  return (
    <div className="welcome-page">
      <div className="welcome-card">
        <div className="welcome-icon">🎀</div>
        <h1>欢迎回来, {user.nickname}!</h1>
        <p className="welcome-sub">
          已通过 Mock 登录 (<code>{user.username}</code>)，工作台建设中…
        </p>
        <button className="btn-logout" type="button" onClick={onLogout}>
          退出登录
        </button>
      </div>
    </div>
  )
}
