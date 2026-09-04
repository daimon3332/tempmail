import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Key, Fingerprint, Link2, Plus } from 'lucide-react'
import Layout from '../components/Layout'
import { Modal } from '../components/ui'
import { Button, Input, Spinner, Badge } from '../components/ui'
import { useApp } from '../context/AppContext'
import api, { state, setUser, setAddress } from '../lib/api'
import { sha256 } from '../lib/utils'

export default function UserPage() {
  const { t } = useApp()
  const [mode, setMode] = useState('login')
  const userQ = useQuery({ queryKey: ['user_settings'], enabled: !!state.userJwt, queryFn: () => api('/user_api/settings').then(r => r.data) })
  const bindQ = useQuery({ queryKey: ['bind_address'], enabled: !!state.userJwt, queryFn: () => api('/user_api/bind_address?limit=50&offset=0').then(r => r.data) })

  return (
    <Layout>
      <div className="mx-auto max-w-3xl p-6">
        {!state.userJwt ? (
          <div className="mx-auto max-w-md rounded-xl border border-border bg-card p-6">
            <div className="mb-4 flex rounded-lg bg-muted p-1">
              {['login', 'register', 'passkey'].map(m => (
                <button key={m} className={`flex-1 rounded-md py-2 text-sm ${mode === m ? 'bg-background shadow' : ''}`} onClick={() => setMode(m)}>
                  {t(m === 'passkey' ? 'passkeyLogin' : m)}
                </button>
              ))}
            </div>
            {mode === 'login' && <LoginForm />}
            {mode === 'register' && <RegisterForm />}
            {mode === 'passkey' && <PasskeyLogin />}
          </div>
        ) : (
          <div className="space-y-4">
            <div className="rounded-xl border border-border bg-card p-5">
              <h2 className="mb-1 font-semibold">{state.userJwt ? String(userQ.data?.user_email || '') : ''}</h2>
              <div className="flex flex-wrap gap-2 text-sm">
                <span>{t('userCenter')}</span>
                {userQ.data?.user_role && <Badge variant="green">{userQ.data.user_role.role}</Badge>}
                {userQ.data?.is_admin && <Badge variant="primary">admin</Badge>}
              </div>
            </div>
            <div className="rounded-xl border border-border bg-card p-5">
              <div className="mb-3 flex items-center justify-between">
                <h3 className="font-semibold">{t('bindAddr')}</h3>
                <button className="flex items-center gap-1 text-sm text-primary" onClick={() => setMode('bind')}>
                  <Plus className="h-4 w-4" /> {t('bindAddr')}
                </button>
              </div>
              <div className="space-y-2">
                {(bindQ.data?.results || []).map((a) => <BoundRow key={a.id} a={a} t={t} />)}
              </div>
            </div>
          </div>
        )}
      </div>
      {mode === 'bind' && <BindModal onClose={() => setMode('')} t={t} />}
      <div className="hidden"></div>
    </Layout>
  )
}

function LoginForm() {
  const [email, setEmail] = useState('')
  const [pw, setPw] = useState('')
  const [err, setErr] = useState('')
  const doLogin = async () => {
    const r = await api('/user_api/login', 'POST', { email, password: await sha256(pw), cf_token: '' })
    if (r.status === 200) { setUser(r.data.jwt); window.location.reload() }
    else setErr(r.data)
  }
  return (
    <div className="space-y-3">
      <Input placeholder="Email" value={email} onChange={(e) => setEmail(e.target.value)} />
      <Input type="password" placeholder="Password" value={pw} onChange={(e) => setPw(e.target.value)} />
      {err && <p className="text-xs text-destructive">{err}</p>}
      <Button className="w-full" onClick={doLogin}>登录</Button>
    </div>
  )
}

function RegisterForm() {
  const [email, setEmail] = useState('')
  const [pw, setPw] = useState('')
  const [err, setErr] = useState('')
  const doRegister = async () => {
    const r = await api('/user_api/register', 'POST', { email, password: await sha256(pw), code: '', cf_token: '' })
    if (r.status === 200) { setErr(''); alert('注册成功，请登录'); setUser(null) }
    else setErr(r.data)
  }
  return (
    <div className="space-y-3">
      <Input placeholder="Email" value={email} onChange={(e) => setEmail(e.target.value)} />
      <Input type="password" placeholder="Password" value={pw} onChange={(e) => setPw(e.target.value)} />
      {err && <p className="text-xs text-destructive">{err}</p>}
      <Button className="w-full" onClick={doRegister}>注册</Button>
    </div>
  )
}

function PasskeyLogin() {
  const [err, setErr] = useState('')
  const doLogin = async () => {
    setErr('')
    try {
      const opts = (await api('/user_api/passkey/authenticate_request', 'POST', {})).data
      const cred = await window.PublicKeyCredential.get({ publicKey: sanitizeOptions(opts) })
      const resp = await api('/user_api/passkey/authenticate_response', 'POST', { ...cred, challenge: opts.challenge })
      if (resp.status === 200) { setUser(resp.data.jwt); window.location.reload() }
      else setErr(resp.data)
    } catch (e) { setErr(String(e)) }
  }
  return (
    <div className="space-y-3">
      {err && <p className="text-xs text-destructive">{err}</p>}
      <Button className="w-full" onClick={doLogin}><Fingerprint className="h-4 w-4" /> 使用密钥登录</Button>
    </div>
  )
}

function sanitizeOptions(o) {
  if (o.challenge) o.challenge = Uint8Array.from(atob(o.challenge), c => c.charCodeAt(0)).buffer
  return o
}

function BoundRow({ a, t }) {
  const [loading, setLoading] = useState(false)
  const useIt = () => setAddress(a.name, '')
  const getJwt = async () => {
    setLoading(true)
    const r = await api(`/user_api/bind_address_jwt/${a.id}`)
    if (r.status === 200) setAddress(a.name, r.data.jwt)
    setLoading(false)
  }
  return (
    <div className="flex items-center justify-between rounded-lg border border-border p-3">
      <div className="min-w-0">
        <div className="truncate text-sm">{a.name}</div>
        <div className="text-xs text-muted-foreground">{a.mail_count || 0} msgs</div>
      </div>
      <div className="flex gap-1">
        <Button size="sm" variant="ghost" onClick={getJwt}>{loading ? <Spinner className="h-4 w-4" /> : <Key className="h-4 w-4" />}</Button>
        <Button size="sm" variant="ghost" onClick={useIt}><Link2 className="h-4 w-4" /></Button>
      </div>
    </div>
  )
}

function BindModal({ onClose, t }) {
  const [email, setEmail] = useState('')
  const [err, setErr] = useState('')
  const admin = async () => {
    // For the logged-in user, bind by their current address JWT via /user_api/bind_address
    const r = await api('/user_api/bind_address', 'POST', {})
    if (r.status === 200) { onClose(); window.location.reload() }
    else setErr(r.data)
  }
  return (
    <Modal title={t('bindAddr')} onClose={onClose} t={t}>
      <div className="space-y-3">
        <p className="text-sm text-muted-foreground">{t('credential')}: {state.address}</p>
        {err && <p className="text-xs text-destructive">{err}</p>}
        <Button className="w-full" onClick={admin}>{t('bindAddr')}</Button>
      </div>
    </Modal>
  )
}

