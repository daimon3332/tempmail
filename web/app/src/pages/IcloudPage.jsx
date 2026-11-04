import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Cloud, Plus, RefreshCw, Copy, ExternalLink, Check, Recycle, Pin } from 'lucide-react'
import Layout from '../components/Layout'
import { PagedTable } from '../components/Table'
import { Badge, Button, Card, Input, Modal, Select, Spinner, Textarea } from '../components/ui'
import api from '../lib/api'
import { fmtTime } from '../lib/utils'

const statusBadge = {
  active: ['正常', 'green'],
  pending: ['待校验', 'muted'],
  expired: ['登录失效', 'red'],
}
const errText = (r, fallback = '操作失败') => (typeof r.data === 'string' && r.data) || fallback
const utc = (s) => (s ? new Date(s.endsWith('Z') ? s : s.replace(' ', 'T') + 'Z') : null)
const copy = (s) => navigator.clipboard?.writeText(s)
const FILL_BUDGET = 4

export default function IcloudPage() {
  const qc = useQueryClient()
  const [modal, setModal] = useState(null)
  const accountsQ = useQuery({ queryKey: ['hme-accounts'], queryFn: () => api('/admin/hme/accounts').then(r => r.data?.results || []) })
  const accounts = accountsQ.data || []
  const refresh = () => { qc.invalidateQueries({ queryKey: ['hme-accounts'] }); qc.invalidateQueries({ queryKey: ['hme-aliases'] }) }
  const close = () => { setModal(null); refresh() }

  return <Layout><div className="mx-auto max-w-[1400px] px-4 py-5 sm:px-6">
    <div className="mb-5 flex flex-wrap items-end justify-between gap-3">
      <div>
        <h1 className="text-2xl font-semibold">iCloud 隐藏邮箱</h1>
        <p className="mt-1 text-sm text-muted-foreground">用 iCloud+ 账号生成 @icloud.com 别名，别名收到的邮件经转发地址直接进入本系统。</p>
      </div>
      <div className="flex gap-2">
        <Button variant="outline" size="sm" onClick={refresh}><RefreshCw className="h-4 w-4" />刷新</Button>
        <Button variant="outline" size="sm" onClick={() => setModal({ type: 'recycle' })}><Recycle className="h-4 w-4" />名额回收</Button>
        <Button size="sm" onClick={() => setModal({ type: 'add' })}><Plus className="h-4 w-4" />添加账号</Button>
      </div>
    </div>

    {accountsQ.isLoading && <div className="flex justify-center p-16"><Spinner /></div>}
    {!accountsQ.isLoading && !accounts.length && <Card className="p-8 text-center">
      <Cloud className="mx-auto h-8 w-8 text-muted-foreground" />
      <p className="mt-3 font-medium">还没有 iCloud 账号</p>
      <p className="mx-auto mt-1 max-w-md text-sm text-muted-foreground">需要开通 iCloud+ 的 Apple 账号。可以用账号密码 + 双重认证登录，或导入 icloud.com 的浏览器 Cookie。</p>
      <Button className="mt-4" size="sm" onClick={() => setModal({ type: 'add' })}><Plus className="h-4 w-4" />添加账号</Button>
    </Card>}
    {accounts.length > 0 && <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
      {accounts.map(acc => <AccountCard key={acc.id} acc={acc} onAction={(type) => setModal({ type, acc })} onChanged={refresh} />)}
    </div>}

    {accounts.length > 0 && <div className="mt-8">
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-lg font-semibold">别名</h2>
        <Button size="sm" onClick={() => setModal({ type: 'create' })}><Plus className="h-4 w-4" />新建别名</Button>
      </div>
      <AliasTable accounts={accounts} onChanged={refresh} />
    </div>}

    {modal?.type === 'add' && <AddAccountModal onClose={close} />}
    {modal?.type === 'login' && <LoginModal acc={modal.acc} onClose={close} />}
    {modal?.type === 'cookies' && <CookieModal acc={modal.acc} onClose={close} />}
    {modal?.type === 'edit' && <EditModal acc={modal.acc} onClose={close} />}
    {modal?.type === 'forward' && <ForwardModal acc={modal.acc} onClose={close} />}
    {modal?.type === 'create' && <CreateAliasModal accounts={accounts} onClose={close} />}
    {modal?.type === 'recycle' && <RecycleModal onClose={close} />}
  </div></Layout>
}

function AccountCard({ acc, onAction, onChanged }) {
  const [busy, setBusy] = useState(false)
  const [label, variant] = statusBadge[acc.status] || [acc.status, 'muted']
  const cooldown = utc(acc.cooldown_until)
  const cooling = cooldown && cooldown > new Date()
  const full = acc.observed_limit && acc.alias_total >= acc.observed_limit
  const sync = async () => {
    setBusy(true)
    const r = await api(`/admin/hme/accounts/${acc.id}/sync`, 'POST')
    setBusy(false)
    if (r.status !== 200) alert(errText(r, '同步失败'))
    onChanged()
  }
  const remove = async () => {
    if (!confirm(`删除账号「${acc.name}」？\n别名邮箱和邮件会保留，但不再自动分发新邮件。iCloud 端的别名不受影响。`)) return
    const r = await api(`/admin/hme/accounts/${acc.id}`, 'DELETE')
    if (r.status !== 200) alert(errText(r))
    onChanged()
  }
  return <Card className="flex flex-col gap-3 p-4">
    <div className="flex items-start justify-between gap-2">
      <div className="min-w-0">
        <div className="truncate font-semibold">{acc.name}</div>
        <div className="truncate text-xs text-muted-foreground">{acc.apple_id || '未校验'} · {acc.host}{acc.has_proxy ? ' · 代理' : ''}</div>
      </div>
      <div className="flex shrink-0 gap-1">
        {!acc.enabled && <Badge variant="muted">已停用</Badge>}
        <Badge variant={variant}>{label}</Badge>
      </div>
    </div>
    <div className="grid grid-cols-3 gap-2 text-center">
      <Stat label={acc.observed_limit ? '已用 / 实测上限' : '已用 / 上限≈750'} value={`${acc.alias_total}/${acc.observed_limit || '≈750'}`} hint="停用的别名也占名额；上限以 Apple 实际拒绝时的数量为准" />
      <Stat label="启用" value={acc.alias_active} />
      <Stat label="未使用" value={acc.auto_fill ? acc.alias_pooled : `${acc.alias_pooled}/${acc.pool_size}`} hint="还没收到过邮件、也没被「新建」分配过的别名" />
    </div>
    <div className="rounded-md bg-muted/50 px-3 py-2 text-xs">
      <div className="flex items-center justify-between gap-2">
        <span className="text-muted-foreground">转发到</span>
        {acc.forward_local && acc.forward_mailbox ? <Badge variant="green">推送已接入</Badge> : <Badge variant="amber">未接入本系统</Badge>}
      </div>
      <div className="mt-1 break-all font-medium">{acc.forward_to || '—'}</div>
    </div>
    <p className="text-xs text-muted-foreground">{acc.auto_fill
      ? (full ? `已达 Apple 上限（${acc.observed_limit}），自动补满已停止` : `自动补满中：每 30 分钟约 ${FILL_BUDGET} 个，直到 Apple 上限`)
      : `保持 ${acc.pool_size} 个未使用别名`}</p>
    {cooling && <p className="text-xs text-amber-600 dark:text-amber-400">Apple 限速冷却中，{cooldown.toLocaleTimeString()} 后继续创建</p>}
    {acc.last_error && <p className="break-words text-xs text-destructive">{acc.last_error}</p>}
    <div className="text-xs text-muted-foreground">同步于 {acc.last_synced_at ? fmtTime(acc.last_synced_at) : '—'} · 校验于 {acc.last_validated_at ? fmtTime(acc.last_validated_at) : '—'}</div>
    <div className="mt-auto flex flex-wrap gap-1 border-t border-border pt-3">
      <Button size="sm" variant="outline" disabled={busy} onClick={sync}><RefreshCw className={`h-3.5 w-3.5 ${busy ? 'animate-spin' : ''}`} />同步</Button>
      <Button size="sm" variant={acc.forward_local && acc.forward_mailbox ? 'ghost' : 'default'} onClick={() => onAction('forward')}>转发设置</Button>
      <Button size="sm" variant={acc.status === 'expired' ? 'default' : 'ghost'} onClick={() => onAction('login')}>重新登录</Button>
      <Button size="sm" variant="ghost" onClick={() => onAction('cookies')}>Cookie</Button>
      <Button size="sm" variant="ghost" onClick={() => onAction('edit')}>编辑</Button>
      <Button size="sm" variant="ghost" className="text-destructive" onClick={remove}>删除</Button>
    </div>
  </Card>
}

function Stat({ label, value, hint }) {
  return <div className="rounded-md border border-border px-2 py-2" title={hint}>
    <div className="text-base font-semibold">{value}</div>
    <div className="text-[11px] text-muted-foreground">{label}</div>
  </div>
}

function Field({ label, hint, children }) {
  return <label className="block space-y-1">
    <span className="text-xs font-medium">{label}</span>
    {children}
    {hint && <span className="block text-[11px] text-muted-foreground">{hint}</span>}
  </label>
}

function AddAccountModal({ onClose }) {
  const [mode, setMode] = useState('login')
  return <Modal title="添加 iCloud 账号" onClose={onClose}>
    <div className="mb-4 flex rounded-md bg-muted p-1 text-sm">
      {[['login', '账号密码登录'], ['cookies', '导入 Cookie']].map(([k, l]) => <button key={k} className={`flex-1 rounded px-3 py-1.5 ${mode === k ? 'bg-background font-medium shadow-sm' : 'text-muted-foreground'}`} onClick={() => setMode(k)}>{l}</button>)}
    </div>
    {mode === 'login' ? <LoginForm onDone={onClose} /> : <CookieForm onDone={onClose} />}
  </Modal>
}

function LoginModal({ acc, onClose }) {
  return <Modal title={`重新登录 · ${acc.name}`} onClose={onClose}><LoginForm acc={acc} onDone={onClose} /></Modal>
}

function CookieModal({ acc, onClose }) {
  return <Modal title={`更新 Cookie · ${acc.name}`} onClose={onClose}><CookieForm acc={acc} onDone={onClose} /></Modal>
}

function AccountFields({ f, set }) {
  return <>
    <Field label="名称"><Input value={f.name} onChange={e => set('name', e.target.value)} placeholder="例如：主号" /></Field>
    <div className="grid grid-cols-2 gap-2">
      <Field label="区域"><Select className="w-full" value={f.host} onChange={e => set('host', e.target.value)}><option value="icloud.com">icloud.com</option><option value="icloud.com.cn">icloud.com.cn（云上贵州）</option></Select></Field>
      <Field label="代理（可选）"><Input value={f.proxy} onChange={e => set('proxy', e.target.value)} placeholder="socks5://host:port" /></Field>
    </div>
  </>
}

function LoginForm({ acc, onDone }) {
  const [f, setF] = useState({ name: '', host: 'icloud.com', proxy: '', apple_id: acc?.apple_id || '', password: '' })
  const [step, setStep] = useState(null)
  const [code, setCode] = useState('')
  const [phone, setPhone] = useState(0)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const set = (k, v) => setF(x => ({ ...x, [k]: v }))
  const run = async (fn) => { setBusy(true); setErr(''); try { await fn() } finally { setBusy(false) } }
  const start = () => run(async () => {
    const r = await api('/admin/hme/login', 'POST', { ...f, account_id: acc?.id || 0 }, { timeout: 90000 })
    if (r.status !== 200) return setErr(errText(r, '登录失败'))
    if (r.data?.need_code) setStep(r.data)
    else onDone()
  })
  const sms = () => run(async () => {
    const r = await api(`/admin/hme/login/${step.login_id}/sms`, 'POST', { phone_id: Number(phone) })
    if (r.status !== 200) setErr(errText(r, '短信发送失败'))
  })
  const verify = () => run(async () => {
    const r = await api(`/admin/hme/login/${step.login_id}/code`, 'POST', { code, phone_id: Number(phone) }, { timeout: 90000 })
    if (r.status !== 200) return setErr(errText(r, '验证失败'))
    onDone()
  })
  if (step) return <div className="space-y-3">
    <p className="text-sm text-muted-foreground">验证码已推送到你的受信任 Apple 设备。{step.phones?.length ? '收不到时可改用短信。' : ''}</p>
    {step.phones?.length > 0 && <div className="flex gap-2">
      <Select className="flex-1" value={phone} onChange={e => setPhone(e.target.value)}>
        <option value={0}>受信任设备</option>
        {step.phones.map(p => <option key={p.id} value={p.id}>短信 {p.numberWithDialCode}</option>)}
      </Select>
      {Number(phone) > 0 && <Button variant="outline" disabled={busy} onClick={sms}>发送短信</Button>}
    </div>}
    <Input autoFocus inputMode="numeric" maxLength={6} placeholder="6 位验证码" value={code} onChange={e => setCode(e.target.value.replace(/\D/g, ''))} onKeyDown={e => e.key === 'Enter' && code.length === 6 && verify()} />
    {err && <p className="text-sm text-destructive">{err}</p>}
    <Button className="w-full" disabled={busy || code.length !== 6} onClick={verify}>{busy ? '验证中…' : '验证并完成登录'}</Button>
  </div>
  return <div className="space-y-3">
    {!acc && <AccountFields f={f} set={set} />}
    <Field label="Apple ID"><Input autoComplete="username" value={f.apple_id} onChange={e => set('apple_id', e.target.value)} placeholder="name@example.com" /></Field>
    <Field label="密码" hint="密码仅用于本次登录，不会保存；登录后只保存加密的会话 Cookie。"><Input type="password" autoComplete="current-password" value={f.password} onChange={e => set('password', e.target.value)} onKeyDown={e => e.key === 'Enter' && start()} /></Field>
    {err && <p className="text-sm text-destructive">{err}</p>}
    <Button className="w-full" disabled={busy || !f.apple_id || !f.password} onClick={start}>{busy ? '登录中…' : '登录'}</Button>
  </div>
}

function CookieForm({ acc, onDone }) {
  const [f, setF] = useState({ name: '', host: 'icloud.com', proxy: '', cookies: '' })
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const set = (k, v) => setF(x => ({ ...x, [k]: v }))
  const submit = async () => {
    setBusy(true); setErr('')
    const r = acc ? await api(`/admin/hme/accounts/${acc.id}/cookies`, 'PUT', { cookies: f.cookies }, { timeout: 90000 }) : await api('/admin/hme/accounts', 'POST', f, { timeout: 90000 })
    setBusy(false)
    if (r.status !== 200) return setErr(errText(r, '导入失败'))
    if (r.data?.last_error) return setErr(`已保存，但校验失败：${r.data.last_error}`)
    onDone()
  }
  return <div className="space-y-3">
    {!acc && <AccountFields f={f} set={set} />}
    <Field label="Cookie" hint="在浏览器登录 icloud.com，从开发者工具 › Application › Cookies 复制，或用扩展导出 JSON。支持 JSON 对象、JSON 数组或 Cookie 请求头格式。">
      <Textarea rows={6} className="font-mono text-xs" value={f.cookies} onChange={e => set('cookies', e.target.value)} placeholder='{"X-APPLE-WEBAUTH-TOKEN":"…", …}' />
    </Field>
    {err && <p className="text-sm text-destructive">{err}</p>}
    <Button className="w-full" disabled={busy || !f.cookies.trim()} onClick={submit}>{busy ? '校验中…' : '保存并校验'}</Button>
  </div>
}

function EditModal({ acc, onClose }) {
  const [f, setF] = useState({ name: acc.name, auto_fill: acc.auto_fill, pool_size: acc.pool_size || 0, enabled: acc.enabled, proxy: '', clearProxy: false })
  const [err, setErr] = useState('')
  const set = (k, v) => setF(x => ({ ...x, [k]: v }))
  const save = async () => {
    const body = { name: f.name, auto_fill: f.auto_fill, pool_size: Number(f.pool_size), enabled: f.enabled }
    if (f.clearProxy) body.proxy = ''
    else if (f.proxy.trim()) body.proxy = f.proxy.trim()
    const r = await api(`/admin/hme/accounts/${acc.id}`, 'PATCH', body)
    if (r.status !== 200) return setErr(errText(r, '保存失败'))
    onClose()
  }
  return <Modal title={`编辑 · ${acc.name}`} onClose={onClose}><div className="space-y-3">
    <Field label="名称"><Input value={f.name} onChange={e => set('name', e.target.value)} /></Field>
    <Field label="代理" hint={acc.has_proxy ? '已设置代理，留空保持不变。' : '留空表示直连。'}>
      <Input value={f.proxy} disabled={f.clearProxy} onChange={e => set('proxy', e.target.value)} placeholder="http://user:pass@host:port 或 socks5://…" />
    </Field>
    {acc.has_proxy && <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={f.clearProxy} onChange={e => set('clearProxy', e.target.checked)} />清除代理</label>}
    <label className="flex items-start gap-2 text-sm"><input className="mt-1" type="checkbox" checked={f.auto_fill} onChange={e => set('auto_fill', e.target.checked)} />
      <span>自动补满<span className="block text-[11px] text-muted-foreground">冷却结束后持续创建别名，每 30 分钟约 {FILL_BUDGET} 个，直到 Apple 提示数量已满。每个别名创建后立即可收信；「新建」时优先分配未使用的别名，无需等待限速。补满不会删除任何已有别名。</span></span></label>
    {!f.auto_fill && <Field label="未使用别名数量（0–20）" hint="后台按限速补足到该数量，遇到 Apple 限速自动冷却。">
      <Input type="number" min={0} max={20} value={f.pool_size} onChange={e => set('pool_size', e.target.value)} />
    </Field>}
    <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={f.enabled} onChange={e => set('enabled', e.target.checked)} />启用（停用后不再保活、同步和创建）</label>
    {err && <p className="text-sm text-destructive">{err}</p>}
    <Button className="w-full" onClick={save}>保存</Button>
  </div></Modal>
}

function ForwardModal({ acc, onClose }) {
  const settingsQ = useQuery({ queryKey: ['workspace-domains'], queryFn: () => api('/open_api/settings').then(r => r.data) })
  const domains = settingsQ.data?.domains || []
  const [cur, setCur] = useState(acc)
  const [domain, setDomain] = useState('')
  const [relay, setRelay] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const isLocal = (e) => domains.some(d => e.toLowerCase().endsWith('@' + d) || e.toLowerCase().endsWith('.' + d))
  const target = relay || (cur.forward_options || []).find(isLocal) || ''
  const verified = target && (cur.forward_options || []).some(o => o.toLowerCase() === target.toLowerCase())
  const done = cur.forward_local && cur.forward_mailbox
  const act = async (fn) => { setBusy(true); setErr(''); try { await fn() } finally { setBusy(false) } }
  const createRelay = () => act(async () => {
    const r = await api('/admin/hme/relay', 'POST', { domain: domain || domains[0] })
    if (r.status !== 200) return setErr(errText(r, '创建失败'))
    setRelay(r.data.address)
  })
  const sync = () => act(async () => {
    const r = await api(`/admin/hme/accounts/${cur.id}/sync`, 'POST')
    if (r.status !== 200) return setErr(errText(r, '同步失败'))
    setCur(r.data)
  })
  const apply = (email) => act(async () => {
    const r = await api(`/admin/hme/accounts/${cur.id}/forward`, 'POST', { email })
    if (r.status !== 200) return setErr(errText(r, '设置失败'))
    setCur(r.data)
  })
  return <Modal title={`转发设置 · ${acc.name}`} onClose={onClose}><div className="space-y-4 text-sm">
    <p className="text-muted-foreground">Apple 会把所有别名的邮件转发到同一个地址。把它设为本系统的中转邮箱后，邮件会按别名实时分发，无需 App 专用密码或轮询。</p>
    {done && <div className="flex items-center gap-2 rounded-md bg-emerald-500/10 px-3 py-2 text-emerald-700 dark:text-emerald-400"><Check className="h-4 w-4" />已接入：{cur.forward_to}</div>}
    <Step n={1} title="准备中转邮箱">
      {target ? <div className="flex items-center gap-2"><code className="break-all rounded bg-muted px-2 py-1">{target}</code><button className="icon-button" title="复制" onClick={() => copy(target)}><Copy className="h-4 w-4" /></button></div>
        : <div className="flex gap-2">
          <Select className="flex-1" value={domain} onChange={e => setDomain(e.target.value)}>{domains.map(d => <option key={d} value={d}>{d}</option>)}</Select>
          <Button variant="outline" disabled={busy || !domains.length} onClick={createRelay}>创建中转邮箱</Button>
        </div>}
    </Step>
    <Step n={2} title="添加到 Apple 账户">
      <p className="text-muted-foreground">在 Apple 账户 › 个人信息 › 电子邮件 中添加上面的地址。Apple 发来的验证码会出现在下方。</p>
      <a className="mt-1 inline-flex items-center gap-1 text-primary hover:underline" href="https://account.apple.com/account/manage" target="_blank" rel="noreferrer">打开 account.apple.com<ExternalLink className="h-3.5 w-3.5" /></a>
      {target && <RelayInbox address={target} />}
    </Step>
    <Step n={3} title="同步并设为转发地址">
      <div className="flex flex-wrap items-center gap-2">
        <Button variant="outline" size="sm" disabled={busy} onClick={sync}><RefreshCw className="h-3.5 w-3.5" />同步</Button>
        {target && <Button size="sm" disabled={busy || !verified || cur.forward_to === target.toLowerCase()} onClick={() => apply(target)}>设为转发地址</Button>}
        {target && !verified && <span className="text-xs text-muted-foreground">验证完成后点同步，地址出现在 Apple 可选列表中即可设置</span>}
      </div>
      <div className="mt-2 text-xs text-muted-foreground">Apple 可选地址：{(cur.forward_options || []).join('、') || '—'}</div>
    </Step>
    {err && <p className="text-destructive">{err}</p>}
  </div></Modal>
}

function Step({ n, title, children }) {
  return <div className="flex gap-3">
    <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-primary text-xs font-semibold text-primary-foreground">{n}</div>
    <div className="min-w-0 flex-1 space-y-2"><div className="font-medium">{title}</div>{children}</div>
  </div>
}

function RelayInbox({ address }) {
  const q = useQuery({
    queryKey: ['hme-relay', address], refetchInterval: 10000,
    queryFn: () => api(`/admin/workspace_mails?${new URLSearchParams({ address, limit: '5', offset: '0' })}`).then(r => r.data?.results || []),
  })
  const mails = q.data || []
  return <div className="mt-2 rounded-md border border-border">
    <div className="flex items-center justify-between border-b border-border px-3 py-1.5 text-xs text-muted-foreground"><span>中转邮箱最新邮件（每 10 秒刷新）</span>{q.isFetching && <Spinner className="h-3 w-3" />}</div>
    {!mails.length && <div className="px-3 py-3 text-xs text-muted-foreground">暂无邮件</div>}
    {mails.map(m => {
      const code = `${m.subject} ${m.text}`.match(/\b\d{6}\b/)?.[0]
      return <div key={m.id} className="flex items-center justify-between gap-2 border-b border-border px-3 py-2 text-xs last:border-0">
        <div className="min-w-0"><div className="truncate font-medium">{m.subject || '(无主题)'}</div><div className="truncate text-muted-foreground">{m.sender} · {fmtTime(m.created_at)}</div></div>
        {code && <button className="shrink-0 rounded bg-primary/10 px-2 py-1 font-mono text-sm font-semibold text-primary" title="复制验证码" onClick={() => copy(code)}>{code}</button>}
      </div>
    })}
  </div>
}

export function CreateAliasForm({ accounts, onCreated }) {
  const [f, setF] = useState({ account_id: '', label: '', note: '' })
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const set = (k, v) => setF(x => ({ ...x, [k]: v }))
  const usable = accounts.filter(a => a.enabled && a.status === 'active')
  const submit = async () => {
    setBusy(true); setErr('')
    const r = await api('/admin/hme/aliases', 'POST', { ...f, account_id: Number(f.account_id) || 0 }, { timeout: 90000 })
    setBusy(false)
    if (r.status !== 200) return setErr(r.status === 429 ? `Apple 限速：${errText(r)}` : errText(r, '创建失败'))
    onCreated(r.data)
  }
  return <div className="space-y-3">
    <Field label="账号"><Select className="w-full" value={f.account_id} onChange={e => set('account_id', e.target.value)}>
      <option value="">自动选择（优先未使用的别名，其次别名最少的账号）</option>
      {usable.map(a => <option key={a.id} value={a.id}>{a.name}（{a.alias_active} 个启用）</option>)}
    </Select></Field>
    <Field label="标签" hint="显示在 iCloud 的别名列表中，便于识别用途。"><Input value={f.label} onChange={e => set('label', e.target.value)} placeholder="例如：注册 GitHub" /></Field>
    <Field label="备注（可选）"><Input value={f.note} onChange={e => set('note', e.target.value)} /></Field>
    {!usable.length && <p className="text-sm text-muted-foreground">没有可用的 iCloud 账号，请先在「iCloud」页添加并完成登录。</p>}
    {err && <p className="text-sm text-destructive">{err}</p>}
    <Button className="w-full" disabled={busy || !usable.length} onClick={submit}><Plus className="h-4 w-4" />{busy ? '创建中…' : '创建 iCloud 别名'}</Button>
  </div>
}

function CreateAliasModal({ accounts, onClose }) {
  const [res, setRes] = useState(null)
  return <Modal title="新建 iCloud 别名" onClose={onClose}>
    {res ? <div className="space-y-3">
      <div className="flex items-center justify-between gap-2 rounded-md border border-border p-3"><span className="break-all font-medium">{res.address}</span><button className="icon-button" title="复制" onClick={() => copy(res.address)}><Copy className="h-4 w-4" /></button></div>
      <Button className="w-full" onClick={onClose}>完成</Button>
    </div> : <CreateAliasForm accounts={accounts} onCreated={setRes} />}
  </Modal>
}

function AliasTable({ accounts, onChanged }) {
  const [removing, setRemoving] = useState(null)
  const call = async (path, method = 'POST', body) => {
    const r = await api(path, method, body)
    if (r.status !== 200) alert(errText(r))
    onChanged()
  }
  const enc = (e) => encodeURIComponent(e)
  const relabel = (a) => {
    const label = prompt('新标签', a.label || '')
    if (label?.trim()) call(`/admin/hme/aliases/${enc(a.email)}`, 'PATCH', { label: label.trim(), note: a.note || '' })
  }
  const columns = [
    { key: 'email', title: '别名', render: a => <div className="min-w-0">
      <div className="flex items-center gap-1 font-medium"><span className="break-all">{a.email}</span><button className="icon-button h-6 w-6" title="复制" onClick={() => copy(a.email)}><Copy className="h-3.5 w-3.5" /></button></div>
      <div className="text-xs text-muted-foreground">{a.label || '—'}</div>
    </div> },
    { key: 'account_name', title: '账号', render: a => <span className="text-xs">{a.account_name}</span> },
    { key: 'state', title: '状态', render: a => <div className="flex flex-wrap gap-1">
      {a.active ? <Badge variant="green">启用</Badge> : <Badge variant="amber">已停用</Badge>}
      {a.pooled ? <Badge variant="muted" title="还没收到过邮件，「新建」时会优先分配">未使用</Badge> : null}
      {a.pinned ? <Badge>保留</Badge> : null}
      {!a.managed ? <Badge variant="muted" title="非本系统创建（如 iPhone），不会被自动回收">外部</Badge> : null}
    </div> },
    { key: 'mailbox', title: '邮件', render: a => <a className="text-xs text-primary hover:underline" href={`/?address=${enc(a.email)}`}>{a.mail_count} 封</a> },
    { key: 'last_mail', title: '最后收信', render: a => <span className="text-xs text-muted-foreground">{a.last_mail_at ? fmtTime(a.last_mail_at) : '从未'}</span> },
    { key: 'created', title: '创建', render: a => <span className="text-xs text-muted-foreground">{fmtTime(a.apple_created_at || a.created_at)}</span> },
    { key: 'actions', title: '', render: a => <div className="flex flex-wrap justify-end gap-1">
      <Button size="sm" variant="ghost" title={a.pinned ? '取消保留' : '保留：永不自动回收'} onClick={() => call(`/admin/hme/aliases/${enc(a.email)}/pin`, 'POST', { pinned: !a.pinned })}><Pin className={`h-3.5 w-3.5 ${a.pinned ? 'fill-current' : ''}`} /></Button>
      <Button size="sm" variant="ghost" onClick={() => relabel(a)}>标签</Button>
      {a.active ? <Button size="sm" variant="ghost" onClick={() => confirm(`停用 ${a.email}？停用后发往该地址的邮件会被退回。`) && call(`/admin/hme/aliases/${enc(a.email)}/deactivate`)}>停用</Button>
        : <Button size="sm" variant="ghost" onClick={() => call(`/admin/hme/aliases/${enc(a.email)}/reactivate`)}>启用</Button>}
      <Button size="sm" variant="ghost" className="text-destructive" onClick={() => setRemoving(a)}>删除</Button>
    </div> },
  ]
  return <>
    <PagedTable path="/admin/hme/aliases" queryKey="hme-aliases" columns={columns} pageSize={25}
      filters={({ setExtra, extra, q, setQ }) => <>
        <Input className="w-56" placeholder="搜索别名或标签" value={q} onChange={e => setQ(e.target.value)} />
        <Select value={extra.account_id || ''} onChange={e => setExtra(x => ({ ...x, account_id: e.target.value }))}>
          <option value="">全部账号</option>{accounts.map(a => <option key={a.id} value={a.id}>{a.name}</option>)}
        </Select>
        <Select value={extra.state || ''} onChange={e => setExtra(x => ({ ...x, state: e.target.value }))}>
          <option value="">全部状态</option><option value="active">启用</option><option value="inactive">已停用</option>
          <option value="unused">未使用</option><option value="used">已使用</option>
        </Select>
      </>} />
    {removing && <Modal title={`删除 ${removing.email}`} onClose={() => setRemoving(null)}><div className="space-y-3 text-sm">
      <p className="text-muted-foreground">将在 iCloud 永久删除该别名（释放名额）并删除本地邮箱和其中的邮件，无法恢复。</p>
      <Button variant="destructive" className="w-full" onClick={() => { call(`/admin/hme/aliases/${enc(removing.email)}`, 'DELETE'); setRemoving(null) }}>永久删除</Button>
    </div></Modal>}
  </>
}

const reasonText = { inactive: '已停用', never_received: '从未收信', idle: '长期未收信' }

function RecycleModal({ onClose }) {
  const q = useQuery({ queryKey: ['hme-recycle'], queryFn: () => api('/admin/hme/recycle').then(r => r.data) })
  const [f, setF] = useState(null)
  const [err, setErr] = useState('')
  const s = f || q.data?.settings
  const set = (k, v) => setF({ ...s, [k]: v })
  const save = async () => {
    const r = await api('/admin/hme/recycle', 'POST', { enabled: s.enabled, unusedDays: Number(s.unusedDays), idleDays: Number(s.idleDays) })
    if (r.status !== 200) return setErr(errText(r, '保存失败'))
    onClose()
  }
  return <Modal title="名额回收" onClose={onClose}>{!s ? <div className="flex justify-center p-6"><Spinner /></div> : <div className="space-y-3 text-sm">
    <p className="text-muted-foreground">Apple 提示数量已满、且没有未使用的别名时，新建邮箱会自动在 iCloud 永久删除 1 个代价最小的别名再创建。本地邮件保留。按顺序挑选：</p>
    <ol className="list-decimal space-y-1 pl-5 text-muted-foreground">
      <li>已停用的别名</li>
      <li>创建超过 <b className="text-foreground">{s.unusedDays}</b> 天且从未收到邮件</li>
      <li>最后收信超过 <b className="text-foreground">{s.idleDays}</b> 天中最久未用的</li>
    </ol>
    <p className="text-xs text-muted-foreground">永不回收：标记「保留」的、已分配给用户的、未使用的、非本系统创建的（如 iPhone 上创建的）别名。</p>
    <label className="flex items-center gap-2"><input type="checkbox" checked={s.enabled} onChange={e => set('enabled', e.target.checked)} />启用自动回收</label>
    <div className="grid grid-cols-2 gap-2">
      <Field label="从未收信（天）"><Input type="number" min={1} value={s.unusedDays} onChange={e => set('unusedDays', e.target.value)} /></Field>
      <Field label="长期未收信（天）"><Input type="number" min={1} value={s.idleDays} onChange={e => set('idleDays', e.target.value)} /></Field>
    </div>
    <div className="rounded-md border border-border">
      <div className="border-b border-border px-3 py-1.5 text-xs text-muted-foreground">下一个会被回收的别名（按当前已保存的设置）</div>
      {(q.data?.preview || []).map(p => <div key={p.account_id} className="flex items-center justify-between gap-2 border-b border-border px-3 py-2 text-xs last:border-0">
        <span className="text-muted-foreground">{p.account_name}</span>
        {p.candidate ? <span className="min-w-0 truncate">{p.candidate.email} · {reasonText[p.candidate.reason]}</span> : <span className="text-muted-foreground">无可回收别名</span>}
      </div>)}
    </div>
    {err && <p className="text-destructive">{err}</p>}
    <Button className="w-full" onClick={save}>保存</Button>
  </div>}</Modal>
}
