import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { PagedTable, Input, Select } from '../../components/Table'
import { Button, Card, Badge, Modal } from '../../components/ui'
import api, { download } from '../../lib/api'
import { fmtTime } from '../../lib/utils'

export default function AddressPage({ t }) {
  const qc = useQueryClient()
  const [sel, setSel] = useState(new Set())
  const [showCreate, setShowCreate] = useState(false)
  const [showPwd, setShowPwd] = useState(null)
  const [showMails, setShowMails] = useState(null)
  const refresh = () => qc.invalidateQueries({ queryKey: ['admin_address'] })

  const del = useMutation({ mutationFn: (ids) => api(`/admin/delete_address/${ids}`, 'DELETE'), onSuccess: () => refresh() })
  const clear = useMutation({ mutationFn: ({ kind, id }) => api(`/admin/${kind}/${id}`, 'DELETE'), onSuccess: () => refresh() })
  const bulkDel = useMutation({ mutationFn: (ids) => Promise.all([...ids].map(id => api(`/admin/delete_address/${id}`, 'DELETE'))), onSuccess: () => { setSel(new Set()); refresh() } })

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-xl font-semibold">{t('adminAddress')}</h2>
        <div className="flex gap-2">
          <Button size="sm" variant="outline" onClick={() => download('/admin/address/export', 'addresses.csv').catch(e => alert(e.message))}>导出 CSV</Button>
          <Button size="sm" onClick={() => setShowCreate(true)}>+ 新建邮箱</Button>
          {sel.size > 0 && <Button size="sm" variant="destructive" onClick={() => { if (confirm(`删除 ${sel.size} 个邮箱？`)) bulkDel.mutate(sel) }}>删除选中 ({sel.size})</Button>}
        </div>
      </div>
      <PagedTable
        path="/admin/address"
        queryKey="admin_address"
        t={t}
        filters={({ setExtra, extra }) => (
          <>
            <Input className="max-w-xs" placeholder="搜索邮箱" value={extra.query || ''} onChange={e => setExtra(x => ({ ...x, query: e.target.value }))} />
            <Select value={extra.sort_by || 'id'} onChange={e => setExtra(x => ({ ...x, sort_by: e.target.value }))}>
              <option value="id">ID 排序</option><option value="created_at">时间</option><option value="mail_count">邮件数</option>
            </Select>
          </>
        )}
        columns={[
          { key: 'id', title: 'ID' },
          { key: 'name', title: '邮箱', render: r => <span className="font-medium">{r.name}</span> },
          { key: 'created_at', title: t('created_at'), render: r => fmtTime(r.created_at) },
          { key: 'source_meta', title: '来源' },
          { key: 'mail_count', title: t('mailCount'), render: r => <Badge variant="green">{r.mail_count}</Badge> },
          { key: 'send_count', title: '发送' },
          {
            key: 'op', title: '操作', render: r => (
              <div className="flex gap-1">
                <Button size="sm" variant="ghost" onClick={() => setShowMails(r)}>{t('inbox')}</Button>
                <Button size="sm" variant="ghost" onClick={() => setShowPwd(r)}>凭据</Button>
                <Button size="sm" variant="ghost" onClick={() => { if (confirm('清空收件箱？')) clear.mutate({ kind: 'clear_inbox', id: r.id }) }}>清收</Button>
                <Button size="sm" variant="ghost" onClick={() => { if (confirm('删除？')) del.mutate(r.id) }}>删</Button>
              </div>
            ),
          },
        ]}
      />
      {showCreate && <CreateAddress t={t} onClose={() => { setShowCreate(false); refresh() }} />}
      {showPwd && <ShowPwd addr={showPwd} onClose={() => setShowPwd(null)} />}
      {showMails && <ViewMails addr={showMails} onClose={() => setShowMails(null)} />}
    </div>
  )
}

function CreateAddress({ onClose, t }) {
  const [form, setForm] = useState({ name: '', domain: '', enablePrefix: false, enableRandomSubdomain: false })
  const [res, setRes] = useState(null)
  const [err, setErr] = useState('')
  const domainsQ = useQuery({ queryKey: ['open-domains'], queryFn: () => api('/open_api/domains').then(r => r.data) })
  const domains = domainsQ.data?.domains || []
  useEffect(() => { if (!form.domain && domains[0]) setForm(x => ({ ...x, domain: domains[0] })) }, [domains, form.domain])
  const doCreate = async () => {
    if (!form.domain || !domains.includes(form.domain)) { setErr('请选择有效域名'); return }
    const r = await api('/admin/new_address', 'POST', form)
    if (r.status === 200 || r.status === 201) setRes(r.data)
    else setErr(r.data || '失败')
  }
  return (
    <Modal title="新建邮箱" onClose={onClose}>
      {res ? (
        <div className="space-y-3">
          <div className="rounded-lg border border-border p-3 text-sm">
            <div className="break-all">{res.address}</div>
            <div className="mt-1 break-all text-xs text-muted-foreground">JWT: {res.jwt}</div>
            {res.password && <div className="text-xs">密码: {res.password}</div>}
          </div>
          <Button className="w-full" onClick={onClose}>{res.password ? '复制并关闭' : '完成'}</Button>
        </div>
      ) : (
        <div className="space-y-3">
          <div className="flex gap-2"><Input placeholder="邮箱前缀" value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} /><span className="self-center text-muted-foreground">@</span><Select className="min-w-[12rem]" value={form.domain} onChange={e => setForm({ ...form, domain: e.target.value })} disabled={domainsQ.isLoading || !domains.length}><option value="">选择域名</option>{domains.map(d => <option key={d} value={d}>{d}</option>)}</Select></div>
          <div className="flex gap-4 text-sm"><label><input type="checkbox" checked={form.enablePrefix} onChange={e => setForm({ ...form, enablePrefix: e.target.checked })} /> 自动加前缀</label><label><input type="checkbox" checked={form.enableRandomSubdomain} onChange={e => setForm({ ...form, enableRandomSubdomain: e.target.checked })} /> 随机子域</label></div>
          {err && <p className="text-xs text-destructive">{err}</p>}
          <Button className="w-full" onClick={doCreate}>{t('create')}</Button>
        </div>
      )}
    </Modal>
  )
}

function ShowPwd({ addr, onClose }) {
  const [token, setToken] = useState('')
  useEffect(() => { api(`/admin/show_password/${addr.id}`).then(r => setToken(r.data?.jwt || '')) }, [addr.id])
  return (
    <Modal title={`邮箱凭据 · ${addr.name}`} onClose={onClose}>
      <div className="space-y-3">
        <div className="break-all rounded-lg border border-border p-3 text-xs">{token || '加载中...'}</div>
        <Button className="w-full" onClick={() => { navigator.clipboard.writeText(token); alert('已复制') }}>复制 JWT</Button>
      </div>
    </Modal>
  )
}

function ViewMails({ addr, onClose }) {
  const [mails, setMails] = useState(null)
  const [cur, setCur] = useState(null)
  useEffect(() => { api(`/admin/mails?address=${encodeURIComponent(addr.name)}&limit=20&offset=0`).then(r => setMails(r.data)) }, [addr.name])
  return (
    <Modal title={`邮件 · ${addr.name}`} onClose={onClose}>
      {cur ? (
        <div className="space-y-2">
          <div className="text-sm font-medium">{cur.subject || '(无主题)'}</div>
          <div className="whitespace-pre-wrap text-xs text-muted-foreground">{cur.raw ? cur.raw.slice(0, 1500) : '...'}</div>
        </div>
      ) : (
        <div className="max-h-[50vh] overflow-y-auto">
          {(mails?.results || []).map(m => (
            <div key={m.id} onClick={() => setCur(m)} className="cursor-pointer border-b border-border py-2 text-sm hover:bg-accent">
              <div className="flex justify-between"><span className="truncate">{m.raw?.slice(0, 80)}</span><span className="text-xs text-muted-foreground">{m.created_at}</span></div>
            </div>
          ))}
          {!mails?.results?.length && <div className="p-4 text-center text-sm text-muted-foreground">无邮件</div>}
        </div>
      )}
    </Modal>
  )
}
