import { useState, useEffect, useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Plus, RefreshCw, Copy, Mail, Inbox, Trash2, X, Send } from 'lucide-react'
import Layout from '../components/Layout'
import { Button, Input, Spinner, Badge, Select, Modal } from '../components/ui'
import { useApp } from '../context/AppContext'
import api, { state, setAddress, clearAddress } from '../lib/api'
import { fmtTime } from '../lib/utils'

export default function MailPage() {
  const { t } = useApp()
  const [address, setAddr] = useState(state.address)
  const [tab, setTab] = useState('inbox')
  const [selected, setSelected] = useState(null)
  const [selectedBody, setSelectedBody] = useState(null)
  const [showNew, setShowNew] = useState(false)
  const [showLogin, setShowLogin] = useState(!state.address)
  const [showSend, setShowSend] = useState(false)
  const [page, setPage] = useState(0)
  const pageSize = 100

  const settingsQ = useQuery({ queryKey: ['open_settings'], queryFn: () => api('/open_api/settings').then(r => r.data) })

  const mailsQ = useQuery({
    queryKey: ['mails', address, tab, page],
    enabled: !!address,
    // If we have no address JWT but an address is set, prompt login instead.
    refetchInterval: tab === 'inbox' ? 30000 : false,
    queryFn: async () => {
      const path = tab === 'inbox' ? '/api/parsed_mails' : '/api/sendbox'
      const r = await api(`${path}?limit=${pageSize}&offset=${page * pageSize}`)
      if (r.status === 401) { setShowLogin(true); throw new Error('unauthorized') }
      if (r.status === 400 && path === '/api/parsed_mails') { setShowLogin(true); throw new Error('not-logged') }
      return r.data
    },
  })

  useEffect(() => {
    if (!address || !state.addressJwt) return
    const es = new EventSource(`/events?address=${encodeURIComponent(address)}&token=${encodeURIComponent(state.addressJwt)}`)
    es.addEventListener('mail', () => mailsQ.refetch())
    es.onerror = () => { es.close(); }
    return () => es.close()
  }, [address])
  useEffect(() => { setSelected(null); setSelectedBody(null); setPage(0) }, [address, tab])

  const openMail = async (id) => {
    const r = await api(`/api/parsed_mail/${id}`)
    if (r.status === 200) {
      setSelected(id); setSelectedBody(r.data)
      if (r.data?.is_unread) api(`/api/mails/${id}/read`, 'POST', { isUnread: false })
    }
  }

  return (
    <Layout>
      <div className={`mail-workspace ${selectedBody ? 'has-detail' : ''}`}>
        {/* sidebar */}
        <aside className="mail-sidebar">
          <div className="mb-3 flex items-center gap-2">
            <Inbox className="h-4 w-4 text-primary" />
            <span className="font-semibold">{t('mail')}</span>
          </div>
          <div className="grid grid-cols-2 gap-1 text-sm">
            <Button size="sm" variant={tab === 'inbox' ? 'default' : 'ghost'} className="flex-1" onClick={() => setTab('inbox')}>{t('inbox')}</Button>
            <Button size="sm" variant={tab === 'sent' ? 'default' : 'ghost'} className="flex-1" onClick={() => setTab('sent')}>{t('sent')}</Button>
          </div>
          <div className="mt-3 space-y-2">
            {state.address && (
              <div className="rounded-md border border-border bg-background p-3 text-xs">
                <div className="mb-1 flex items-center justify-between">
                  <span className="font-medium">{t('address')}</span>
                  <button className="text-muted-foreground hover:text-destructive" onClick={() => { clearAddress(); setShowLogin(true) }}>
                    <X className="h-3.5 w-3.5" />
                  </button>
                </div>
                <div className="break-all text-muted-foreground">{address}</div>
                <div className="mt-2 flex gap-1">
                  <Button size="sm" variant="outline" onClick={() => setShowSend(true)}><Send className="h-3.5 w-3.5" />{t('send')}</Button>
                  <Button size="sm" variant="ghost" title={t('refresh')} onClick={() => mailsQ.refetch()}><RefreshCw className="h-3.5 w-3.5" /></Button>
                </div>
              </div>
            )}
          </div>
        </aside>

        {/* mail list */}
        <section className="mail-list">
          <div className="flex items-center justify-between border-b border-border px-4 py-2">
            <span className="text-sm font-medium">{t(tab === 'inbox' ? 'inbox' : 'sent')}</span>
            <Button size="sm" variant="ghost" onClick={() => mailsQ.refetch()}><RefreshCw className="h-4 w-4" /></Button>
          </div>
          <div className="flex-1 overflow-y-auto scrollbar-thin">
            {mailsQ.isLoading && <div className="flex justify-center p-6"><Spinner /></div>}
            {!mailsQ.isLoading && mailsQ.data?.count === 0 && (
              <div className="p-6 text-center text-sm text-muted-foreground">{t('empty')}</div>
            )}
            {(mailsQ.data?.results || []).map((m) => (
              <MailRow key={m.id} m={m} active={selected === m.id} onClick={() => openMail(m.id)} t={t} />
            ))}
          </div>
          {!mailsQ.isLoading && <div className="flex flex-wrap items-center justify-between gap-2 border-t border-border px-4 py-2 text-xs text-muted-foreground"><span>共 {mailsQ.data?.count || 0} 条 · 第 {mailsQ.data?.count ? page + 1 : 0} / {mailsQ.data?.count ? Math.ceil(mailsQ.data.count / pageSize) : 0} 页</span><div className="flex gap-1"><Button size="sm" variant="outline" disabled={page <= 0} onClick={() => setPage(0)}>首页</Button><Button size="sm" variant="outline" disabled={page <= 0} onClick={() => setPage(p => p - 1)}>上一页</Button><Button size="sm" variant="outline" disabled={!mailsQ.data?.count || page >= Math.ceil(mailsQ.data.count / pageSize) - 1} onClick={() => setPage(p => p + 1)}>下一页</Button><Button size="sm" variant="outline" disabled={!mailsQ.data?.count || page >= Math.ceil(mailsQ.data.count / pageSize) - 1} onClick={() => setPage(Math.max(0, Math.ceil(mailsQ.data.count / pageSize) - 1))}>末页</Button></div></div>}
        </section>

        {/* mail detail */}
        <section className="mail-detail">
          {!selectedBody && <EmptyDetail t={t} />}
          {selectedBody && <MailDetail m={selectedBody} onClose={() => { setSelected(null); setSelectedBody(null) }} t={t} />}
        </section>
      </div>

      {showLogin && <LoginModal onClose={() => setShowLogin(false)} setAddr={setAddr} t={t} />}
      {showNew && <NewAddressModal settings={settingsQ.data} setAddr={setAddr} onClose={() => setShowNew(false)} t={t} />}
      {showSend && address && <SendModal address={address} onClose={() => setShowSend(false)} t={t} />}
    </Layout>
  )
}

function MailRow({ m, active, onClick, t }) {
  const unread = m.is_unread === 1
  return (
    <div
      onClick={onClick}
      className={`flex cursor-pointer flex-col gap-0.5 border-b border-border px-4 py-3 hover:bg-accent ${active ? 'bg-accent' : ''}`}
    >
      <div className="flex items-center justify-between">
        <span className="truncate text-sm font-medium">{m.sender || t('from')}</span>
        <span className="shrink-0 text-xs text-muted-foreground">{fmtTime(m.created_at || m.received_at)}</span>
      </div>
      <div className="truncate text-sm text-muted-foreground">{m.subject || t('noSubject')}</div>
      <div className="truncate text-xs text-muted-foreground">{m.text}</div>
      <div className="mt-0.5 flex items-center gap-1">
        {unread && <Badge variant="primary" className="h-1.5 w-1.5 rounded-full p-0" />}
        {(m.attachments && m.attachments.length > 0) && <Badge variant="muted">{m.attachments.length} 📎</Badge>}
      </div>
    </div>
  )
}

function EmptyDetail({ t }) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-3 text-muted-foreground">
      <Mail className="h-10 w-10 opacity-30" />
      <span className="text-sm">{t('empty')}</span>
    </div>
  )
}

function MailDetail({ m, onClose, t }) {
  const bodyRef = useRef(null)
  const copy = async () => {
    const text = m.text || bodyRef.current?.innerText || ''
    try { await navigator.clipboard.writeText(text); alert(t('copied')) } catch { alert(t('copyFailed')) }
  }
  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center gap-2 border-b border-border px-5 py-3">
        <span className="truncate text-lg font-semibold">{m.subject || t('noSubject')}</span>
        <div className="ml-auto flex gap-1">
          <Button size="sm" variant="ghost" onClick={copy}><Copy className="h-4 w-4" /> {t('copy')}</Button>
          <Button size="sm" variant="ghost" onClick={() => window.print()}>{t('textOnly')}</Button>
          <Button size="sm" variant="ghost" title={t('cancel')} onClick={onClose}><X className="h-4 w-4" /></Button>
          <Button size="sm" variant="ghost" title={t('delete')} onClick={async () => { if (confirm(t('deleteMail'))) { await api(`/api/mails/${m.id}`, 'DELETE'); onClose() } }}><Trash2 className="h-4 w-4" /></Button>
        </div>
      </div>
      <div className="flex flex-wrap gap-x-6 gap-y-1 border-b border-border px-5 py-2 text-xs text-muted-foreground">
        <span><b>{t('from')}:</b> {m.sender}</span>
        <span><b>{t('date')}:</b> {m.created_at || m.received_at}</span>
      </div>
      <div className="flex-1 overflow-auto p-5">
        {m.html && m.html.trim() ? (
          <iframe title={m.subject || t('mail')} sandbox="allow-popups" srcDoc={`<meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data: cid:; style-src 'unsafe-inline'; font-src data:"><base target="_blank">${m.html}`} className="mail-frame" />
        ) : (
          <div ref={bodyRef} className="whitespace-pre-wrap text-sm">{m.text}</div>
        )}
        {(m.attachments && m.attachments.length > 0) && (
          <div className="mt-4 space-y-2">
            <div className="text-sm font-medium">{t('attachments') || 'Attachments'}</div>
            {m.attachments.map((a, i) => (
              <div key={i} className="flex items-center gap-2 rounded-lg border border-border p-2 text-sm">
                <span className="truncate">{a.filename}</span>
                <span className="text-xs text-muted-foreground">{(a.size / 1024).toFixed(1)} KB</span>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

function LoginModal({ onClose, setAddr, t }) {
  const [value, setValue] = useState('')
  const [err, setErr] = useState('')
  const submit = async () => {
    const r = await api('/open_api/credential_login', 'POST', { credential: value.trim() })
    if (r.status === 200) {
      try {
        const payload = JSON.parse(atob(value.trim().split('.')[1].replace(/-/g, '+').replace(/_/g, '/')))
        setAddress(payload.address, value.trim())
        setAddr(payload.address)
        onClose()
      } catch { setErr('Invalid credential') }
    } else { setErr('Invalid credential') }
  }
  return (
    <Modal title={t('login')} onClose={onClose} t={t}>
      <div className="space-y-3">
        <Input placeholder={t('credential')} value={value} onChange={(e) => setValue(e.target.value)} />
        {err && <p className="text-xs text-destructive">{err}</p>}
        <Button className="w-full" onClick={submit}>{t('login')}</Button>
      </div>
    </Modal>
  )
}

function NewAddressModal({ settings, setAddr, onClose, t }) {
  const [name, setName] = useState('')
  const [domain, setDomain] = useState(settings?.defaultDomains?.[0] || settings?.domains?.[0] || '')
  const [err, setErr] = useState('')
  const submit = async () => {
    const r = await api('/api/new_address', 'POST', { name, domain, cf_token: '' })
    if (r.status === 200 || r.status === 201) {
      setAddress(r.data.address, r.data.jwt)
      setAddr(r.data.address)
      onClose()
    } else { setErr(r.data || 'Failed') }
  }
  return (
    <Modal title={t('newAddress')} onClose={onClose} t={t}>
      <div className="space-y-3">
        <div className="flex items-center gap-2">
          <div className="flex flex-1 items-center rounded-lg border border-input bg-background">
            <span className="pl-3 text-sm text-muted-foreground">{settings?.prefix || ''}</span>
            <Input className="border-0 focus-visible:ring-0" value={name} onChange={(e) => setName(e.target.value)} placeholder={t('address')} />
          </div>
          <span className="text-muted-foreground">@</span>
          <Select value={domain} onChange={(e) => setDomain(e.target.value)}>
            {(settings?.defaultDomains || settings?.domains || []).map((d) => <option key={d} value={d}>{d}</option>)}
          </Select>
        </div>
        {err && <p className="text-xs text-destructive">{err}</p>}
        <Button className="w-full" onClick={submit}>{t('create')}</Button>
      </div>
    </Modal>
  )
}

function SendModal({ address, onClose, t }) {
  const [to, setTo] = useState('')
  const [subject, setSubject] = useState('')
  const [content, setContent] = useState('')
  const [err, setErr] = useState('')
  const submit = async () => {
    const r = await api('/api/send_mail', 'POST', { to_mail: to, from_name: '', to_name: '', subject, content, is_html: false })
    if (r.status === 200) onClose()
    else setErr(r.data || 'Failed')
  }
  return (
    <Modal title={t('send')} onClose={onClose} t={t}>
      <div className="space-y-3">
        <Input placeholder={t('sendTo')} value={to} onChange={(e) => setTo(e.target.value)} />
        <Input placeholder={t('subject')} value={subject} onChange={(e) => setSubject(e.target.value)} />
        <textarea className="min-h-[120px] w-full rounded-lg border border-input bg-background p-3 text-sm" placeholder={t('content')} value={content} onChange={(e) => setContent(e.target.value)} />
        {err && <p className="text-xs text-destructive">{err}</p>}
        <Button className="w-full" onClick={submit}>{t('send')}</Button>
      </div>
    </Modal>
  )
}
