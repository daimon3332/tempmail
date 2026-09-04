import { useEffect, useState } from 'react'
import { PagedTable, Input } from '../../components/Table'
import { Button, Card, Badge, Modal } from '../../components/ui'
import api from '../../lib/api'
import { sha256 } from '../../lib/utils'

export default function UsersPage({ t }) {
  const [showCreate, setShowCreate] = useState(false)
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">{t('adminUsers')}</h2>
        <div className="flex gap-2">
          <Button size="sm" onClick={() => setShowCreate(true)}>+ 创建用户</Button>
        </div>
      </div>
      {showCreate && <CreateUser onClose={() => { setShowCreate(false); window.location.reload() }} />}
      <PagedTable
        path="/admin/users"
        queryKey="admin_users"
        filters={({ setExtra, extra }) => <Input className="max-w-xs" placeholder="搜索用户名" value={extra.query || ''} onChange={e => setExtra(x => ({ ...x, query: e.target.value }))} />}
        columns={[
          { key: 'id', title: 'ID' },
          { key: 'username', title: '用户名', render: r => <span className="font-medium">{r.username || r.user_email}</span> },
          { key: 'created_at', title: t('created_at') },
          { key: 'password', title: '密码', render: r => r.password ? <code className="select-all text-xs">{r.password}</code> : <Badge variant="muted">管理员不展示</Badge> },
          { key: 'address_count', title: '邮箱数' },
          { key: 'mail_count', title: '邮件数' },
          { key: 'op', title: '操作', render: r => <RowActions r={r} t={t} /> },
        ]}
      />
    </div>
  )
}

function RowActions({ r, t }) {
  const [show, setShow] = useState(null)
  const refresh = () => window.location.reload()
  const edit = async (body) => {
    const res = await api(`/admin/users/${r.id}/reset_password`, 'POST', body)
    if (res.status === 200) { setShow(null); refresh() } else alert(res.data || '操作失败')
  }
  const del = async () => { if (confirm(`删除用户 ${r.user_email}？`)) { await api(`/admin/users/${r.id}`, 'DELETE'); refresh() } }
  return (
    <div className="flex gap-1">
      <Button size="sm" variant="ghost" onClick={() => setShow('pwd')}>重置密码</Button>
      <Button size="sm" variant="ghost" onClick={() => setShow('limits')}>配额</Button>
      <Button size="sm" variant="ghost" onClick={del} disabled={r.username === 'admin' || r.user_email === 'admin'}>删除</Button>
      {show === 'pwd' && <ResetPwdModal r={r} onClose={() => setShow(null)} onSubmit={edit} />}
      {show === 'limits' && <LimitsModal r={r} onClose={() => setShow(null)} />}
    </div>
  )
}

function LimitsModal({ r, onClose }) {
  const [form, setForm] = useState(null)
  const [err, setErr] = useState('')
  const [saving, setSaving] = useState(false)
  useEffect(() => {
    api(`/admin/user_limits/${r.id}`).then(res => {
      if (res.status === 200) setForm(res.data)
      else setErr(res.data || '读取失败')
    })
  }, [r.id])
  const set = (key, value) => setForm(x => ({ ...x, [key]: Math.max(-1, Number.parseInt(value, 10) || 0) }))
  const save = async () => {
    setSaving(true); setErr('')
    const res = await api(`/admin/user_limits/${r.id}`, 'PATCH', form)
    setSaving(false)
    if (res.status === 200) onClose()
    else setErr(res.data || '保存失败')
  }
  return <Modal title={`用户配额 · ${r.user_email}`} onClose={onClose}>
    <div className="space-y-3">
      {form ? <>
        <LimitField label="邮箱总数" value={form.max_address_count} onChange={v => set('max_address_count', v)} />
        <LimitField label="邮件总数" value={form.max_mail_count} onChange={v => set('max_mail_count', v)} />
        <LimitField label="每月创建邮箱" value={form.monthly_address_quota} onChange={v => set('monthly_address_quota', v)} />
        <LimitField label="每月接收邮件" value={form.monthly_receive_quota} onChange={v => set('monthly_receive_quota', v)} />
        <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={!!form.can_send_mail} onChange={e => setForm(x => ({ ...x, can_send_mail: e.target.checked }))} />允许发信</label>
        <p className="text-xs text-muted-foreground">数值为 -1 表示无限。</p>
        {err && <p className="text-xs text-destructive">{err}</p>}
        <Button className="w-full" disabled={saving} onClick={save}>{saving ? '保存中…' : '保存配额'}</Button>
      </> : <SpinnerText error={err} />}
    </div>
  </Modal>
}

function LimitField({ label, value, onChange }) {
  return <label className="flex items-center gap-3 text-sm"><span className="w-32 shrink-0">{label}</span><Input type="number" value={value ?? -1} onChange={e => onChange(e.target.value)} /></label>
}

function SpinnerText({ error }) { return <p className={`text-sm ${error ? 'text-destructive' : 'text-muted-foreground'}`}>{error || '加载中…'}</p> }

function ResetPwdModal({ r, onClose, onSubmit }) {
  const [pw, setPw] = useState('')
  return (
    <Modal title={`重置密码 · ${r.user_email}`} onClose={onClose}>
      <div className="space-y-3">
        <Input type="password" placeholder="新密码" value={pw} onChange={e => setPw(e.target.value)} />
        <Button className="w-full" onClick={async () => onSubmit({ password: await sha256(pw), password_plain: pw })}>保存</Button>
      </div>
    </Modal>
  )
}

function CreateUser({ onClose }) {
  const [f, setF] = useState({ username: '', password: '', max_address_count: -1, max_mail_count: -1, monthly_address_quota: -1, monthly_receive_quota: -1, can_send_mail: false })
  const [err, setErr] = useState('')
  const doCreate = async () => {
    if (!f.username || !f.password) { setErr('用户名和密码不能为空'); return }
    const r = await api('/admin/users', 'POST', { username: f.username, email: f.username, password: await sha256(f.password), password_plain: f.password, limits: { max_address_count: Number(f.max_address_count), max_mail_count: Number(f.max_mail_count), monthly_address_quota: Number(f.monthly_address_quota), monthly_receive_quota: Number(f.monthly_receive_quota), can_send_mail: !!f.can_send_mail } })
    if (r.status === 200) onClose()
    else setErr(r.data || '创建失败')
  }
  return (
    <Modal title="创建用户" onClose={onClose}>
      <div className="space-y-3">
        <Input placeholder="用户名" value={f.username} onChange={e => setF({ ...f, username: e.target.value })} />
        <Input type="password" placeholder="密码" value={f.password} onChange={e => setF({ ...f, password: e.target.value })} />
        <div className="grid grid-cols-2 gap-2">{[['max_address_count','邮箱总数'],['max_mail_count','邮件总数'],['monthly_address_quota','每月创建邮箱'],['monthly_receive_quota','每月接收邮件']].map(([k,l]) => <label key={k} className="text-xs text-muted-foreground">{l}<Input type="number" value={f[k]} onChange={e => setF({ ...f, [k]: e.target.value })} /></label>)}</div>
        <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={f.can_send_mail} onChange={e => setF({ ...f, can_send_mail: e.target.checked })} />允许发信</label>
        {err && <p className="text-xs text-destructive">{err}</p>}
        <Button className="w-full" onClick={doCreate}>创建</Button>
      </div>
    </Modal>
  )
}
