import { Link, useNavigate } from 'react-router-dom'
import { Sun, Moon, Mail, User, Settings, LogOut } from 'lucide-react'
import { useApp } from '../context/AppContext'
import { state } from '../lib/api'

export default function Layout({ children }) {
  const { t, dark, setDark, locale, setLocale } = useApp()
  const nav = useNavigate()
  return (
    <div className="flex min-h-screen flex-col">
      <header className="flex h-14 items-center gap-6 border-b border-border px-6">
        <Link to="/" className="flex items-center gap-2 font-bold text-primary">
          <Mail className="h-6 w-6" />
          <span>Temp Mail</span>
        </Link>
        <nav className="flex items-center gap-4 text-sm">
          <Link to="/" className="text-muted-foreground hover:text-foreground">{t('mail')}</Link>
          <Link to="/user" className="text-muted-foreground hover:text-foreground">{t('userCenter')}</Link>
          <Link to="/admin" className="text-muted-foreground hover:text-foreground">{t('admin')}</Link>
        </nav>
        <div className="ml-auto flex items-center gap-3">
          <span className="hidden max-w-[240px] truncate text-xs text-muted-foreground md:block">{state.address}</span>
          <button className="rounded-lg p-2 hover:bg-accent" onClick={() => setDark(!dark)} title={dark ? t('light') : t('dark')}>
            {dark ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
          </button>
          <select
            className="rounded-lg border border-border bg-background px-2 py-1 text-sm"
            value={locale}
            onChange={(e) => setLocale(e.target.value)}
          >
            <option value="zh">中文</option>
            <option value="en">English</option>
          </select>
          <button className="rounded-lg p-2 hover:bg-accent" onClick={() => { localStorage.removeItem('tm_address'); localStorage.removeItem('tm_address_jwt'); nav('/') }}>
            <LogOut className="h-4 w-4" />
          </button>
        </div>
      </header>
      <main className="flex-1">{children}</main>
    </div>
  )
}
