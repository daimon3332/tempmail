import { useState } from 'react'
import { LayoutDashboard, Mail, Users, Send, Activity, Database, Settings, LogOut, ScrollText } from 'lucide-react'
import Layout from '../components/Layout'
import { Button, Input } from '../components/ui'
import { useApp } from '../context/AppContext'
import api, { setAdmin, state, clearUser, clearAddress } from '../lib/api'
import { sha256 } from '../lib/utils'
import StatsPage from './admin/StatsPage'
import AddressPage from './admin/AddressPage'
import UsersPage from './admin/UserPage'
import MailsPage from './admin/MailsPage'
import TelegramPage from './admin/TelegramPage'
import MaintenancePage from './admin/MaintenancePage'
import ConfigPage from './admin/ConfigPage'
import OperationLogPage from './admin/OperationLogPage'

export default function AdminPage({ initialTab = 'stats' }) {
  const { t } = useApp()
  const [authed, setAuthed] = useState(!!sessionStorage.getItem('tm_admin_auth') || state.isAdmin)
  const [tab, setTab] = useState(initialTab)
  if (!authed) return <AdminLogin onOk={() => setAuthed(true)} t={t} />
  const nav = [
    ['stats', t('adminStats'), LayoutDashboard],
    ['address', t('adminAddress'), Mail],
    ['users', t('adminUsers'), Users],
    ['mails', t('adminMails'), Send],
    ['telegram', 'Telegram 机器人', Activity],
    ['maintenance', '数据维护', Database],
    ['config', '系统设置', Settings],
    ['logs', '操作日志', ScrollText],
  ]
  return (
    <Layout>
      <div className="admin-workspace">
        <aside className="admin-sidebar">
          <div className="mb-3 px-2 text-xs font-semibold text-muted-foreground">管理中心</div>
          <div className="space-y-1">
            {nav.map(([k, label, Icon]) => (
              <button key={k} onClick={() => setTab(k)} className={`admin-nav ${tab === k ? 'is-active' : ''}`}>
                <Icon className="h-4 w-4" /> {label}
              </button>
            ))}
          </div>
          <div className="mt-4 border-t border-border pt-3">
            <Button size="sm" variant="outline" className="w-full" onClick={() => { setAdmin(''); clearUser(); clearAddress(); setAuthed(false); location.href = '/login' }}><LogOut className="h-4 w-4" /> {t('logout')}</Button>
          </div>
        </aside>
        <main className="admin-content">
          {tab === 'stats' && <StatsPage t={t} />}
          {tab === 'address' && <AddressPage t={t} />}
          {tab === 'users' && <UsersPage t={t} />}
          {tab === 'mails' && <MailsPage t={t} />}
          {tab === 'telegram' && <TelegramPage t={t} />}
          {tab === 'maintenance' && <MaintenancePage t={t} />}
          {tab === 'config' && <ConfigPage t={t} />}
          {tab === 'logs' && <OperationLogPage />}
        </main>
      </div>
    </Layout>
  )
}

function AdminLogin({ onOk, t }) {
  const [pw, setPw] = useState('')
  const [err, setErr] = useState('')
  const doLogin = async () => {
    const r = await api('/open_api/admin_login', 'POST', { password: await sha256(pw), cf_token: '' })
    if (r.status === 200) { setAdmin(pw); onOk() }
    else setErr(r.data || 'Invalid')
  }
  return (
    <Layout>
      <div className="mx-auto mt-24 max-w-sm rounded-xl border border-border bg-card p-6">
        <h2 className="mb-1 text-lg font-semibold">{t('admin')}</h2>
        <p className="mb-4 text-sm text-muted-foreground">管理员密码</p>
        <div className="space-y-3">
          <Input type="password" value={pw} onChange={(e) => setPw(e.target.value)} autoFocus onKeyDown={(e) => e.key === 'Enter' && doLogin()} />
          {err && <p className="text-xs text-destructive">{err}</p>}
          <Button className="w-full" onClick={doLogin}>{t('login')}</Button>
        </div>
      </div>
    </Layout>
  )
}
