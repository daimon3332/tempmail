import { useState } from 'react'
import { PagedTable, Input } from '../../components/Table'
import { Button, Card, Badge, Modal } from '../../components/ui'
import api from '../../lib/api'

export default function UsersPage({ t }) {
  const [showCreate, setShowCreate] = useState(false)
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">{t('adminUsers')}</h2>
        <div className="flex gap-2">
          <Button size="sm" onClick={() => setShowCreate(true)}>+ {t('create')}</Button>
        </div>
      </div>
      {showCreate && <CreateUser onClose={() => { setShowCreate(false); window.location.reload() }} />}
      <PagedTable
        path="/admin/users"
        queryKey="admin_users"
        filters={({ setExtra, extra }) => <Input className="max-w-xs" placeholder="搜索邮箱" value={extra.query || ''} onChange={e => setExtra(x => ({ ...x, query: e.target.value }))} />}
        columns={[
          { key: 'id', title: 'ID' },
          { key: 'user_email', title: '邮箱', render: r => <span className="font-medium">{r.user_email}</span> },
          { key: 'created_at', title: t('created_at') },
          { key: 'role_text', title: t('role'), render: r => r.role_text ? <Badge variant="green">{r.role_text}</Badge> : <Badge variant="muted">—</Badge> },
          { key: 'address_count', title: t('used') },
          { key: 'op', title: '操作', render: r => <RowActions r={r} /> },
        ]}
      />
    </div>
  )
}

function RowActions({ r }) {
  const [show, setShow] = useState(null)
  const refresh = () => window.location.reload()
  const edit = async (body) => {
    const res = await api(`/admin/users/${r.id}/reset_password`, 'POST', body)
    if (res.status === 200) { setShow(null); refresh() }
  }
  const del = async () => { if (confirm(`删除用户 ${r.user_email}？`)) { await api(`/admin/users/${r.id}`, 'DELETE'); refresh() } }
  const role = async (role_text) => { await api('/admin/user_roles', 'POST', { user_id: r.id, role_text }); refresh() }
  return (
    <div className="flex gap-1">
      <Button size="sm" variant="ghost" onClick={() => setShow('pwd')}>重置密码</Button>
      <Button size="sm" variant="ghost" onClick={() => setShow('bind')}>绑定地址</Button>
      <Button size="sm" variant="ghost" onClick={del}>删除</Button>
      {show === 'pwd' && <ResetPwdModal r={r} onClose={() => setShow(null)} onSubmit={edit} />}
      {show === 'bind' && <BindModal r={r} onClose={() => setShow(null)} role={role} t={t} />}
    </div>
  )
}

function ResetPwdModal({ r, onClose, onSubmit }) {
  const [pw, setPw] = useState('')
  return (
    <Modal title={`重置密码 · ${r.user_email}`} onClose={onClose}>
      <div className="space-y-3">
        <Input type="password" placeholder="新密码（SHA-256）" value={pw} onChange={e => setPw(e.target.value)} />
        <Button className="w-full" onClick={() => onSubmit({ password: pw })}>保存</Button>
      </div>
    </Modal>
  )
}

function BindModal({ r, onClose, role, t }) {
  const roles = null
  const [addr, setAddr] = useState('')
  const [roleText, setRoleText] = useState('')
  const doBind = async () => {
    if (addr) await api('/admin/users/bind_address', 'POST', { user_email: r.user_email, address: addr })
  }
  return (
    <Modal title={`绑定地址 · ${r.user_email}`} onClose={onClose}>
      <div className="space-y-3">
        <Input placeholder="地址名（如 a@333186.xyz）" value={addr} onChange={e => setAddr(e.target.value)} />
        <Button className="w-full" onClick={doBind}>绑定</Button>
        <div className="border-t border-border pt-3">
          <div className="mb-2 text-sm">{t('role')}</div>
          <div className="flex gap-2">
            <Input placeholder="角色码（如 premium）" value={roleText} onChange={e => setRoleText(e.target.value)} />
            <Button size="sm" onClick={() => role(roleText)}>设置角色</Button>
          </div>
        </div>
      </div>
    </Modal>
  )
}

function CreateUser({ onClose }) {
  const [f, setF] = useState({ email: '', password: '' })
  const [err, setErr] = useState('')
  const doCreate = async () => {
    const r = await api('/admin/users', 'POST', f)
    if (r.status === 200) onClose()
    else setErr(r.data || '创建失败')
  }
  return (
    <Modal title="创建用户" onClose={onClose}>
      <div className="space-y-3">
        <Input placeholder="邮箱" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} />
        <Input type="password" placeholder="密码（SHA-256）" value={f.password} onChange={e => setF({ ...f, password: e.target.value })} />
        {err && <p className="text-xs text-destructive">{err}</p>}
        <Button className="w-full" onClick={doCreate}>创建</Button>
      </div>
    </Modal>
  )
}
