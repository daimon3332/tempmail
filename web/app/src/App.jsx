import { Routes, Route, Navigate } from 'react-router-dom'
import MailPage from './pages/MailPage'
import UserPage from './pages/UserPage'
import LoginPage from './pages/LoginPage'
import WorkspacePage from './pages/WorkspacePage'
import SettingsPage from './pages/SettingsPage'
import { state } from './lib/api'

function Home() {
  if (!state.userJwt && !state.addressJwt) return <Navigate to="/login" replace />
  return state.userJwt ? <WorkspacePage /> : <MailPage />
}

function Protected({ children, addressOnly = false, userOnly = false, adminOnly = false }) {
  if (!state.userJwt && !state.addressJwt) return <Navigate to="/login" replace />
  if (userOnly && !state.userJwt) return <Navigate to="/login" replace />
  if (addressOnly && !state.addressJwt) return <Navigate to="/" replace />
  if (adminOnly && !state.isAdmin && !state.adminAuth) return <Navigate to="/" replace />
  return children
}

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<Home />} />
      <Route path="/login" element={<LoginPage />} />
      <Route path="/mail" element={<Protected addressOnly><MailPage /></Protected>} />
      <Route path="/settings" element={<Protected adminOnly><SettingsPage /></Protected>} />
      <Route path="/user" element={<Protected adminOnly><UserPage /></Protected>} />
      <Route path="/user/oauth2/callback" element={<Protected userOnly><UserPage /></Protected>} />
      <Route path="/admin" element={<Protected adminOnly><Navigate to="/settings" replace /></Protected>} />
      <Route path="/telegram_mail" element={<Protected addressOnly><MailPage /></Protected>} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
