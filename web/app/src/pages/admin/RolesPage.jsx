import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button, Card, Badge, Modal, Input } from '../../components/ui'
import api from '../../lib/api'

export default function RolesPage({ t }) {
  const q = useQuery({ queryKey: ['admin_roles'], queryFn: () => api('/admin/roles').then(r => r.data?.results || r.data) })
  const [show, setShow] = useState(false)
  const refresh = () => q.refetch()
  const del = async (role) => { if (confirm(`删除角色 ${role}？`)) { await api(`/admin/roles/${role}`, 'DELETE'); refresh() } }
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">{t('adminRoles')}</h2>
        <Button size="sm" onClick={() => setShow(true)}>+ {t('newRole')}</Button>
      </div>
      {(q.data || []).length === 0 && <Card className="p-6 text-center text-sm text-muted-foreground">暂无角色，点击右上角新建</Card>}
      {(q.data || []).map(r => (
        <Card key={r.role} className="p-4">
          <div className="flex items-center justify-between">
            <span className="font-medium">{r.role}{r.name && <span className="ml-1 text-sm text-muted-foreground">{r.name}</span>}</span>
            <div className="flex items-center gap-2">
              <Badge variant={r.source === 'db' ? 'green' : 'muted'}>{r.source}</Badge>
              <Button size="sm" variant="ghost" onClick={() => del(r.role)}>删除</Button>
            </div>
          </div>
          <div className="mt-2 flex flex-wrap gap-4 text-xs text-muted-foreground">
            <span>{t('max')}: {r.max_address_count < 0 ? '∞' : r.max_address_count}</span>
            <span>Monthly: {r.monthly_address_quota < 0 ? '∞' : r.monthly_address_quota}</span>
            <span>自定义名: {r.can_custom_name ? '✓' : '✗'}</span>
            <span>发信: {r.can_send_mail ? '✓' : '✗'}</span>
            {(r.domains || []).length > 0 && <span>{r.domains.join(', ')}</span>}
          </div>
        </Card>
      ))}
      {show && <CreateRole t={t} onClose={() => { setShow(false); refresh() }} />}
    </div>
  )
}

function CreateRole({ onClose, t }) {
  const [f, setF] = useState({ role: '', name: '', max_address_count: -1, monthly_address_quota: -1, can_custom_name: true, can_send_mail: true, domains: [] })
  const [err, setErr] = useState('')
  const save = async () => {
    const r = await api('/admin/roles', 'POST', { ...f, domains: f.domains[0] ? f.domains[0].split(',').map(s => s.trim()).filter(Boolean) : [] })
    if (r.status === 200) { onClose() } else { setErr(r.data) }
  }
  return (
    <Modal title={t('newRole')} onClose={onClose}>
      <div className="space-y-3">
        <Input placeholder="角色码（如 premium）" value={f.role} onChange={e => setF({ ...f, role: e.target.value })} />
        <Input placeholder="显示名（可选）" value={f.name} onChange={e => setF({ ...f, name: e.target.value })} />
        <div className="grid grid-cols-2 gap-3">
          <div><label className="text-xs text-muted-foreground">Max 绑定数 (-1=∞)</label><Input type="number" value={f.max_address_count} onChange={e => setF({ ...f, max_address_count: parseInt(e.target.value) })} /></div>
          <div><label className="text-xs text-muted-foreground">每月创建 (-1=∞)</label><Input type="number" value={f.monthly_address_quota} onChange={e => setF({ ...f, monthly_address_quota: parseInt(e.target.value) })} /></div>
        </div>
        <Input placeholder="域名白名单（逗号分隔，可空）" onChange={e => setF({ ...f, domains: [e.target.value] })} />
        <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={f.can_custom_name} onChange={e => setF({ ...f, can_custom_name: e.target.checked })} /> 允许自定义名</label>
        <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={f.can_send_mail} onChange={e => setF({ ...f, can_send_mail: e.target.checked })} /> 允许发信</label>
        {err && <p className="text-xs text-destructive">{err}</p>}
        <Button className="w-full" onClick={save}>保存</Button>
      </div>
    </Modal>
  )
}
