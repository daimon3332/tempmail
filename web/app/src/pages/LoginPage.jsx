import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { KeyRound, UserRound } from 'lucide-react'
import { Button, Input } from '../components/ui'
import api, { setAdmin, setUser, setAddress, setAccessToken } from '../lib/api'
import { sha256 } from '../lib/utils'

export default function LoginPage() {
  const nav = useNavigate()
  const [mode, setMode] = useState('user')
  const [identifier, setIdentifier] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const submit = async () => {
    if (submitting) return
    setError('')
    setSubmitting(true)
    try {
      if (mode === 'address') {
        const r = await api('/open_api/credential_login', 'POST', { credential: password.trim() })
        if (r.status !== 200) { setError(typeof r.data === 'string' ? r.data : '邮箱 Token 无效'); return }
        try {
          const p = JSON.parse(atob(password.trim().split('.')[1].replace(/-/g, '+').replace(/_/g, '/')))
          setAddress(p.address, password.trim()); nav('/')
        } catch { setError('邮箱 Token 无效') }
        return
      }
      if (!identifier.trim() || !password) { setError('请输入用户名和密码'); return }
      const r = await api('/user_api/login', 'POST', { username: identifier.trim(), email: identifier.trim(), password: await sha256(password), cf_token: '' })
      if (r.status === 200 && r.data?.jwt) { setUser(r.data.jwt, !!r.data.is_admin); setAccessToken(r.data.access_token || ''); if (r.data.is_admin) setAdmin(password); nav('/') } else setError(typeof r.data === 'string' ? r.data : '用户名或密码错误')
    } catch { setError('网络请求失败，请稍后重试') } finally { setSubmitting(false) }
  }
  return <div className="min-h-screen bg-muted/30 px-4"><div className="mx-auto flex min-h-screen max-w-md items-center"><div className="w-full rounded-2xl border border-border bg-background p-7 shadow-sm">
    <div className="mb-6"><p className="text-xs font-semibold uppercase tracking-[0.18em] text-primary">Temp Mail</p><h1 className="mt-2 text-2xl font-semibold">登录工作台</h1><p className="mt-1 text-sm text-muted-foreground">管理邮箱、邮件和账户设置</p></div>
    <div className="mb-5 grid grid-cols-2 gap-1 rounded-lg bg-muted p-1"><button className={`flex items-center justify-center gap-2 rounded-md px-3 py-2 text-sm ${mode === 'user' ? 'bg-background font-medium shadow-sm' : 'text-muted-foreground'}`} onClick={() => setMode('user')}><UserRound className="h-4 w-4" />用户登录</button><button className={`flex items-center justify-center gap-2 rounded-md px-3 py-2 text-sm ${mode === 'address' ? 'bg-background font-medium shadow-sm' : 'text-muted-foreground'}`} onClick={() => setMode('address')}><KeyRound className="h-4 w-4" />邮箱 Token</button></div>
    <form className="space-y-3" onSubmit={e => { e.preventDefault(); submit() }}><Input autoFocus value={identifier} onChange={e => setIdentifier(e.target.value)} placeholder="用户名或邮箱" className={mode === 'address' ? 'hidden' : ''} /><Input type={mode === 'address' ? 'text' : 'password'} value={password} onChange={e => setPassword(e.target.value)} placeholder={mode === 'address' ? '粘贴 Address JWT' : '密码'} /><p className={`text-sm text-destructive ${error ? '' : 'invisible'}`}>{error || ' '}</p><Button type="submit" disabled={submitting} className="w-full">{submitting ? '登录中…' : '登录'}</Button></form>
  </div></div></div>
}
