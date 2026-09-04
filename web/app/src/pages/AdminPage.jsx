import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { LayoutDashboard, Mail, Users as UsersIcon, Shield, Save, Trash2, Plus } from 'lucide-react'
import Layout from '../components/Layout'
import { Button, Input, Spinner, Badge, Card, Select } from '../components/ui'
import { useApp } from '../context/AppContext'
import api, { setAdmin } from '../lib/api'
import { sha256 } from '../lib/utils'

export default function AdminPage() {
  const { t } = useApp()
  const [authed, setAuthed] = useState(!!localStorage.getItem('tm_admin_auth'))
  const [tab, setTab] = useState('stats')
  if (!authed) return <AdminLogin onOk={() => setAuthed(true)} t={t} />

  return (
    <Layout>
      <div className="flex h-[calc(100vh-3.5rem)]">
        <aside className="w-52 border-r border-border p-3">
          <div className="space-y-1">
            {[
              ['stats', t('adminStats'), LayoutDashboard],
              ['address', t('adminAddress'), Mail],
              ['users', t('adminUsers'), UsersIcon],
              ['roles', t('adminRoles'), Shield],
            ].map(([k, label, Icon]) => (
              <button key={k} onClick={() => setTab(k)} className={`flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm ${tab === k ? 'bg-accent' : 'hover:bg-accent'}`}>
                <Icon className="h-4 w-4" /> {label}
              </button>
            ))}
          </div>
          <div className="mt-4 border-t border-border pt-3">
            <Button size="sm" variant="outline" className="w-full" onClick={() => { setAdmin(''); setAuthed(false) }}>{t('logout')}</Button>
          </div>
        </aside>
        <main className="flex-1 overflow-auto p-6">
          {tab === 'stats' && <Stats t={t} />}
          {tab === 'address' && <Addresses t={t} />}
          {tab === 'users' && <UsersTable t={t} />}
          {tab === 'roles' && <Roles t={t} />}
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
        <p className="mb-4 text-sm text-muted-foreground">Administrator password</p>
        <div className="space-y-3">
          <Input type="password" value={pw} onChange={(e) => setPw(e.target.value)} placeholder="Password" onKeyDown={(e) => e.key === 'Enter' && doLogin()} />
          {err && <p className="text-xs text-destructive">{err}</p>}
          <Button className="w-full" onClick={doLogin}>{t('login')}</Button>
        </div>
      </div>
    </Layout>
  )
}

function Stats({ t }) {
  const q = useQuery({ queryKey: ['admin_stats'], queryFn: () => api('/admin/stats').then(r => r.data) })
  if (q.isLoading) return <Spinner />
  const s = q.data || {}
  const items = [
    [t('addressCount'), s.address_count],
    [t('mailCount'), s.mail_count],
    [t('userCount'), s.user_count],
    ['Active', s.active_address_count],
    ['Today', s.today_mail_count],
    ['Unread', s.unread_mail_count],
  ]
  return (
    <div>
      <h2 className="mb-4 text-xl font-semibold">{t('adminStats')}</h2>
      <div className="grid grid-cols-2 gap-4 md:grid-cols-3">
        {items.map(([label, v]) => (
          <Card key={label} className="p-4">
            <div className="text-sm text-muted-foreground">{label}</div>
            <div className="mt-1 text-3xl font-bold">{v ?? 0}</div>
          </Card>
        ))}
      </div>
    </div>
  )
}

function Addresses({ t }) {
  const [query, setQuery] = useState('')
  const q = useQuery({ queryKey: ['admin_address', query], queryFn: () => api(`/admin/address?limit=20&offset=0${query ? '&query=' + encodeURIComponent(query) : ''}`).then(r => r.data) })
  return (
    <div>
      <h2 className="mb-4 text-xl font-semibold">{t('adminAddress')}</h2>
      <Input className="mb-4 max-w-sm" placeholder="Search" value={query} onChange={(e) => { setQuery(e.target.value); q.refetch() }} />
      <div className="overflow-hidden rounded-xl border border-border">
        <table className="w-full text-sm">
          <thead className="bg-muted text-left text-xs uppercase text-muted-foreground">
            <tr><th className="px-3 py-2">ID</th><th className="px-3 py-2">{t('address')}</th><th className="px-3 py-2">{t('created_at')}</th><th className="px-3 py-2">{t('mailCount')}</th></tr>
          </thead>
          <tbody>
            {(q.data?.results || []).map((a) => (
              <tr key={a.id} className="border-t border-border"><td className="px-3 py-2 text-muted-foreground">{a.id}</td><td className="px-3 py-2 font-medium">{a.name}</td><td className="px-3 py-2 text-muted-foreground">{a.created_at}</td><td className="px-3 py-2"><Badge variant="green">{a.mail_count}</Badge></td></tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function UsersTable({ t }) {
  const q = useQuery({ queryKey: ['admin_users'], queryFn: () => api('/admin/users?limit=50&offset=0').then(r => r.data) })
  return (
    <div>
      <h2 className="mb-4 text-xl font-semibold">{t('adminUsers')}</h2>
      <div className="overflow-hidden rounded-xl border border-border">
        <table className="w-full text-sm">
          <thead className="bg-muted text-left text-xs uppercase text-muted-foreground">
            <tr><th className="px-3 py-2">ID</th><th className="px-3 py-2">Email</th><th className="px-3 py-2">{t('role')}</th><th className="px-3 py-2">{t('used')}</th></tr>
          </thead>
          <tbody>
            {(q.data?.results || []).map((u) => (
              <tr key={u.id} className="border-t border-border"><td className="px-3 py-2 text-muted-foreground">{u.id}</td><td className="px-3 py-2 font-medium">{u.user_email}</td><td className="px-3 py-2">{u.role_text ? <Badge variant="green">{u.role_text}</Badge> : <Badge variant="muted">—</Badge>}</td><td className="px-3 py-2">{u.address_count}</td></tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function Roles({ t }) {
  const q = useQuery({ queryKey: ['admin_roles'], queryFn: () => api('/admin/roles').then(r => r.data?.results || r.data) })
  const [showCreate, setShowCreate] = useState(false)
  return (
    <div>
      <div className="mb-4 flex items-center justify-between">
        <h2 className="text-xl font-semibold">{t('adminRoles')}</h2>
        <Button size="sm" onClick={() => setShowCreate(true)}><Plus className="h-4 w-4" /> {t('newRole')}</Button>
      </div>
      <div className="space-y-3">
        {(q.data || []).length === 0 && <Card className="p-6 text-center text-sm text-muted-foreground">{t('empty')}</Card>}
        {(q.data || []).map((r) => (
          <Card key={r.role} className="p-4">
            <div className="flex items-center justify-between">
              <span className="font-medium">{r.role} {r.name && <span className="ml-1 text-sm text-muted-foreground">{r.name}</span>}</span>
              <Badge variant={r.source === 'db' ? 'green' : 'muted'}>{r.source}</Badge>
            </div>
            <div className="mt-2 flex flex-wrap gap-4 text-xs text-muted-foreground">
              <span>{t('max')}: {r.max_address_count < 0 ? '∞' : r.max_address_count}</span>
              <span>Monthly: {r.monthly_address_quota < 0 ? '∞' : r.monthly_address_quota}</span>
              <span>Custom name: {r.can_custom_name ? '✓' : '✗'}</span>
              <span>Send mail: {r.can_send_mail ? '✓' : '✗'}</span>
              {(r.domains || []).length > 0 && <span>{r.domains.join(', ')}</span>}
            </div>
          </Card>
        ))}
      </div>
      {showCreate && <CreateRole onClose={() => setShowCreate(false)} t={t} />}
    </div>
  )
}

function CreateRole({ onClose, t }) {
  const [role, setRole] = useState('')
  const [max, setMax] = useState('-1')
  const [monthly, setMonthly] = useState('-1')
  const [custom, setCustom] = useState(true)
  const [send, setSend] = useState(true)
  const [err, setErr] = useState('')
  const save = async () => {
    const r = await api('/admin/roles', 'POST', { role, name: '', domains: [], max_address_count: parseInt(max), monthly_address_quota: parseInt(monthly), can_custom_name: custom, can_send_mail: send })
    if (r.status === 200) { onClose(); window.location.reload() }
    else setErr(r.data)
  }
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={onClose}>
      <div className="w-full max-w-md rounded-xl border border-border bg-card p-5" onClick={(e) => e.stopPropagation()}>
        <h3 className="mb-4 text-lg font-semibold">{t('newRole')}</h3>
        <div className="space-y-3">
          <Input placeholder="Role code (e.g. premium)" value={role} onChange={(e) => setRole(e.target.value)} />
          <div className="grid grid-cols-2 gap-3">
            <div><label className="text-xs text-muted-foreground">Max address (-1=∞)</label><Input value={max} onChange={(e) => setMax(e.target.value)} /></div>
            <div><label className="text-xs text-muted-foreground">Monthly (-1=∞)</label><Input value={monthly} onChange={(e) => setMonthly(e.target.value)} /></div>
          </div>
          <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={custom} onChange={(e) => setCustom(e.target.checked)} /> Custom name</label>
          <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={send} onChange={(e) => setSend(e.target.checked)} /> Send mail</label>
          {err && <p className="text-xs text-destructive">{err}</p>}
          <Button className="w-full" onClick={save}><Save className="h-4 w-4" /> {t('save')}</Button>
        </div>
      </div>
    </div>
  )
}
