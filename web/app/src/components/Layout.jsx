import { Link, useLocation, useNavigate } from 'react-router-dom'
import { Sun, Moon, Mail, Users, LogOut, Settings, BookOpen } from 'lucide-react'
import { useApp } from '../context/AppContext'
import { state, clearAddress, clearUser, clearAdmin } from '../lib/api'

export default function Layout({ children }) {
  const { t, dark, setDark, locale, setLocale } = useApp()
  const nav = useNavigate()
  const location = useLocation()
  const items = [['/', t('mail'), Mail], ...(state.userJwt && state.isAdmin ? [['/user', '用户与权限', Users], ['/settings', '系统设置', Settings]] : [])]
  const logout = () => { clearAddress(); clearUser(); clearAdmin(); nav('/login', { replace: true }) }
  return (
    <div className="app-shell min-h-screen">
      <header className="app-header">
        <div className="flex min-w-0 items-center gap-3">
          <Link to="/" className="brand-mark"><span className="brand-icon"><Mail className="h-4 w-4" /></span><span>Temp Mail</span></Link>
          <nav className="hidden items-center gap-1 sm:flex">{items.map(([href, label, Icon]) => <Link key={href} to={href} className={`top-nav ${location.pathname === href || (href !== '/' && location.pathname.startsWith(href)) ? 'is-active' : ''}`}><Icon className="h-3.5 w-3.5" />{label}</Link>)}</nav>
        </div>
        <div className="ml-auto flex items-center gap-3">
          <a className="top-nav" href="/docs/api" target="_blank" rel="noreferrer"><BookOpen className="h-3.5 w-3.5" />API 文档</a>
          {state.address && <span className="hidden max-w-[220px] truncate rounded-md bg-muted px-2 py-1 text-xs text-muted-foreground lg:block">{state.address}</span>}
          <button className="icon-button" onClick={() => setDark(!dark)} title={dark ? t('light') : t('dark')}>{dark ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}</button>
          <select
            id="language" name="language" aria-label="Language" className="h-8 rounded-md border border-border bg-background px-2 text-xs"
            value={locale}
            onChange={(e) => setLocale(e.target.value)}
          >
            <option value="zh">中文</option>
            <option value="en">English</option>
          </select>
          <button className="icon-button" onClick={logout} title={t('logout')}><LogOut className="h-4 w-4" /></button>
        </div>
      </header>
      <div className="mobile-nav sm:hidden">{items.map(([href, label, Icon]) => <Link key={href} to={href} className={location.pathname === href ? 'is-active' : ''}><Icon className="h-4 w-4" />{label}</Link>)}</div>
      <main className="app-main">{children}</main>
    </div>
  )
}
