import { useState } from 'react'
import { LayoutDashboard, Mail, Users, Send, Shield, Activity, Database, Settings, BookOpen, LogOut } from 'lucide-react'
import Layout from '../components/Layout'
import { Button, Input, Card } from '../components/ui'
import { useApp } from '../context/AppContext'
import api, { setAdmin } from '../lib/api'
import { sha256 } from '../lib/utils'
import StatsPage from './admin/StatsPage'
import AddressPage from './admin/AddressPage'
import UsersPage from './admin/UserPage'
import MailsPage from './admin/MailsPage'
import RolesPage from './admin/RolesPage'
import TelegramPage from './admin/TelegramPage'
import MaintenancePage from './admin/MaintenancePage'
import ConfigPage from './admin/ConfigPage'

export default function AdminPage() {
  const { t } = useApp()
  const [authed, setAuthed] = useState(!!localStorage.getItem('tm_admin_auth'))
  const [tab, setTab] = useState('stats')
  if (!authed) return <AdminLogin onOk={() => setAuthed(true)} t={t} />
  const nav = [
    ['stats', t('adminStats'), LayoutDashboard],
    ['address', t('adminAddress'), Mail],
    ['users', t('adminUsers'), Users],
    ['mails', t('adminMails'), Send],
    ['roles', t('adminRoles'), Shield],
    ['telegram', '电报机器人', Activity],
    ['maintenance', t('adminCleanup'), Database],
    ['config', '配置', Settings],
    ['docs', 'API 文档', BookOpen],
  ]
  return (
    <Layout>
      <div className="flex h-[calc(100vh-3.5rem)]">
        <aside className="w-52 shrink-0 border-r border-border p-3">
          <div className="space-y-1">
            {nav.map(([k, label, Icon]) => (
              <button key={k} onClick={() => setTab(k)} className={`flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm ${tab === k ? 'bg-accent' : 'hover:bg-accent'}`}>
                <Icon className="h-4 w-4" /> {label}
              </button>
            ))}
          </div>
          <div className="mt-4 border-t border-border pt-3">
            <Button size="sm" variant="outline" className="w-full" onClick={() => { setAdmin(''); setAuthed(false) }}><LogOut className="h-4 w-4" /> {t('logout')}</Button>
          </div>
        </aside>
        <main className="min-w-0 flex-1 overflow-auto p-6">
          {tab === 'stats' && <StatsPage t={t} />}
          {tab === 'address' && <AddressPage t={t} />}
          {tab === 'users' && <UsersPage t={t} />}
          {tab === 'mails' && <MailsPage t={t} />}
          {tab === 'roles' && <RolesPage t={t} />}
          {tab === 'telegram' && <TelegramPage t={t} />}
          {tab === 'maintenance' && <MaintenancePage t={t} />}
          {tab === 'config' && <ConfigPage t={t} />}
          {tab === 'docs' && <ApiDocs />}
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

function ApiDocs() {
  return (
    <div className="space-y-3">
      <h2 className="text-xl font-semibold">API 文档</h2>
      <Card className="p-6 text-center">
        <p className="mb-3 text-muted-foreground">完整 API 用法（认证、请求/响应约定、curl 示例、各端点说明与注意事项）。</p>
        <a href="/docs/api" target="_blank" rel="noreferrer"><Button>打开 API 文档 <BookOpen className="ml-1 h-4 w-4" /></Button></a>
      </Card>
    </div>
  )
}
