import { Routes, Route, Navigate } from 'react-router-dom'
import MailPage from './pages/MailPage'
import UserPage from './pages/UserPage'
import AdminPage from './pages/AdminPage'

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<MailPage />} />
      <Route path="/user" element={<UserPage />} />
      <Route path="/user/oauth2/callback" element={<UserPage />} />
      <Route path="/admin" element={<AdminPage />} />
      <Route path="/telegram_mail" element={<MailPage />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
