import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { PagedTable, Input, Select } from '../../components/Table'
import { Button, Card, Badge, Modal, Spinner } from '../../components/ui'
import api from '../../lib/api'
import { fmtTime } from '../../lib/utils'

export default function MailsPage({ t }) {
  const [tab, setTab] = useState('mails')
  const [showSend, setShowSend] = useState(false)
  const [showWebhook, setShowWebhook] = useState(false)
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">{t('adminMails')}</h2>
        <div className="flex gap-2">
          <Button size="sm" variant="outline" onClick={() => setShowWebhook(true)}>邮件 Webhook</Button>
          <Button size="sm" onClick={() => setShowSend(true)}>{t('send')}</Button>
        </div>
      </div>
      <div className="flex gap-2 text-sm">
        <TabBtn active={tab === 'mails'} onClick={() => setTab('mails')}>邮件</TabBtn>
        <TabBtn active={tab === 'unknow'} onClick={() => setTab('unknow')}>未知邮件</TabBtn>
        <TabBtn active={tab === 'sendbox'} onClick={() => setTab('sendbox')}>发件箱</TabBtn>
      </div>
      {tab === 'mails' && <MailList t={t} />}
      {tab === 'unknow' && <MailList unknow t={t} />}
      {tab === 'sendbox' && <Sendbox t={t} />}
      {showSend && <SendMail onClose={() => setShowSend(false)} t={t} />}
      {showWebhook && <MailWebhook onClose={() => setShowWebhook(false)} t={t} />}
    </div>
  )
}

function TabBtn({ active, onClick, children }) { return <button className={`rounded-lg px-3 py-1.5 ${active ? 'bg-primary text-primary-foreground' : 'bg-muted hover:bg-accent'}`} onClick={onClick}>{children}</button> }

function MailList({ unknow, t }) {
  const [cur, setCur] = useState(null)
  const path = unknow ? '/admin/mails_unknow' : '/admin/mails'
  const filters = unknow ? undefined : ({ setExtra, extra }) => <Input className="max-w-xs" placeholder="按地址过滤" value={extra.address || ''} onChange={e => setExtra(x => ({ ...x, address: e.target.value }))} />
  const del = (id) => api(`/admin/mails/${id}`, 'DELETE')
  return (
    <div className="space-y-3">
      <PagedTable path={path} queryKey={unknow ? 'unknown_mails' : 'admin_mails'} pageSize={20} filters={filters}
        columns={[
          { key: 'id', title: 'ID' },
          { key: 'address', title: '地址', render: r => <span className="break-all text-xs">{r.address}</span> },
          { key: 'source', title: '发件人', render: r => <span className="break-all text-xs">{r.source}</span> },
          { key: 'created_at', title: t('created_at'), render: r => fmtTime(r.created_at) },
          { key: 'op', title: '操作', render: r => <div className="flex gap-1"><Button size="sm" variant="ghost" onClick={() => setCur(r)}>查看</Button><Button size="sm" variant="ghost" onClick={() => { if (confirm('删除？')) { del(r.id); window.location.reload() } }}>删除</Button></div> },
        ]}
      />
      {cur && <RawMail m={cur} onClose={() => setCur(null)} />}
    </div>
  )
}

function RawMail({ m, onClose }) {
  return (
    <Modal title={`邮件 #${m.id}`} onClose={onClose}>
      <pre className="max-h-[60vh] overflow-auto whitespace-pre-wrap text-xs">{m.raw || m.message_id || JSON.stringify(m)}</pre>
    </Modal>
  )
}

function Sendbox({ t }) {
  const del = (id) => api(`/admin/sendbox/${id}`, 'DELETE')
  return (
    <PagedTable path="/admin/sendbox" queryKey="admin_sendbox" pageSize={20}
      filters={({ setExtra, extra }) => <Input className="max-w-xs" placeholder="按地址过滤" value={extra.address || ''} onChange={e => setExtra(x => ({ ...x, address: e.target.value }))} />}
      columns={[
        { key: 'id', title: 'ID' },
        { key: 'address', title: '地址', render: r => <span className="break-all text-xs">{r.address}</span> },
        { key: 'created_at', title: t('created_at'), render: r => fmtTime(r.created_at) },
        { key: 'op', title: '操作', render: r => <Button size="sm" variant="ghost" onClick={() => { if (confirm('删除？')) { del(r.id); window.location.reload() } }}>删除</Button> },
      ]}
    />
  )
}

function SendMail({ onClose, t }) {
  const [f, setF] = useState({ from_mail: '', to_mail: '', subject: '', content: '', is_html: false })
  const [err, setErr] = useState('')
  const send = async () => {
    const r = await api('/admin/send_mail', 'POST', f)
    if (r.status === 200) onClose()
    else setErr(r.data || '发送失败')
  }
  return (
    <Modal title={`${t('send')} 邮件`} onClose={onClose}>
      <div className="space-y-3">
        <Input placeholder="发件地址（如 a@333186.xyz）" value={f.from_mail} onChange={e => setF({ ...f, from_mail: e.target.value })} />
        <Input placeholder="收件人" value={f.to_mail} onChange={e => setF({ ...f, to_mail: e.target.value })} />
        <Input placeholder="主题" value={f.subject} onChange={e => setF({ ...f, subject: e.target.value })} />
        <textarea className="min-h-[100px] w-full rounded-lg border border-input p-3 text-sm" placeholder="内容" value={f.content} onChange={e => setF({ ...f, content: e.target.value })} />
        <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={f.is_html} onChange={e => setF({ ...f, is_html: e.target.checked })} /> HTML</label>
        {err && <p className="text-xs text-destructive">{err}</p>}
        <Button className="w-full" onClick={send}>{t('send')}</Button>
      </div>
    </Modal>
  )
}

function MailWebhook({ onClose, t }) {
  const q = useQuery({ queryKey: ['mail_webhook'], queryFn: () => api('/admin/mail_webhook/settings').then(r => r.data) })
  const [f, setF] = useState(null)
  const s = f || q.data || {}
  if (q.data && f === null) setF(q.data)
  const up = (k, v) => setF(x => ({ ...x, [k]: v }))
  const save = async () => { await api('/admin/mail_webhook/settings', 'POST', s); onClose() }
  const test = async () => { const r = await api('/admin/mail_webhook/test', 'POST', s); alert(r.status === 200 ? '成功' : (r.data || '失败')) }
  return (
    <Modal title="邮件 Webhook（管理级）" onClose={onClose}>
      <div className="space-y-3">
        <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={!!s.enabled} onChange={e => up('enabled', e.target.checked)} /> 启用</label>
        <Input placeholder="URL" value={s.url || ''} onChange={e => up('url', e.target.value)} />
        <Input placeholder="Method（POST）" value={s.method || 'POST'} onChange={e => up('method', e.target.value)} />
        <Input placeholder="Headers JSON" value={s.headers || ''} onChange={e => up('headers', e.target.value)} />
        <textarea className="min-h-[100px] w-full rounded-lg border border-input p-3 text-sm" placeholder="Body 模板" value={s.body || ''} onChange={e => up('body', e.target.value)} />
        <div className="flex gap-2"><Button size="sm" variant="outline" className="flex-1" onClick={test}>测试</Button><Button className="flex-1" onClick={save}>保存</Button></div>
      </div>
    </Modal>
  )
}
