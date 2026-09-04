import { useQuery } from '@tanstack/react-query'
import { Card, Spinner } from '../../components/ui'
import api from '../../lib/api'

export default function StatsPage({ t }) {
  const stats = useQuery({ queryKey: ['admin_stats'], queryFn: () => api('/admin/stats').then(r => r.data) })
  const sys = useQuery({ queryKey: ['system_status'], queryFn: () => api('/admin/system_status').then(r => r.data), retry: false })
  const domains = useQuery({ queryKey: ['domain_status'], queryFn: () => api('/admin/domain_status').then(r => r.data), retry: false })
  if (stats.isLoading) return <Spinner />
  const s = stats.data || {}
  const items = [
    [t('addressCount'), s.address_count], [t('mailCount'), s.mail_count], [t('userCount'), s.user_count],
    ['Active', s.active_address_count], ['Today', s.today_mail_count], ['Unread', s.unread_mail_count],
  ]
  const channels = sys.data ? [
    ['SMTP :25', sys.data.smtp], ['Ingest', sys.data.ingestToken], ['API Key', sys.data.apiKey],
    ['AI 提取', sys.data.aiEnabled], ['Telegram', sys.data.telegram],
  ] : []
  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">{t('adminStats')}</h2>
      <div className="grid grid-cols-2 gap-4 md:grid-cols-3">
        {items.map(([label, v]) => (
          <Card key={label} className="p-4"><div className="text-sm text-muted-foreground">{label}</div><div className="mt-1 text-3xl font-bold">{v ?? 0}</div></Card>
        ))}
        <Card className="p-4"><div className="text-sm text-muted-foreground">发件箱</div><div className="mt-1 text-3xl font-bold">{s.sendbox_count ?? 0}</div></Card>
      </div>
      <Card className="p-4">
        <div className="mb-2 text-sm font-medium">系统状态</div>
        <div className="flex flex-wrap gap-2 text-sm">
          {channels.map(([k, v]) => <span key={k} className={`rounded-full px-2 py-0.5 ${v ? 'bg-emerald-500/10 text-emerald-600' : 'bg-muted text-muted-foreground'}`}>{k}: {v ? '✔' : '—'}</span>)}
        </div>
      </Card>
      <Card className="p-4">
        <div className="mb-2 text-sm font-medium">域名 (MX)</div>
        <div className="flex flex-wrap gap-2 text-sm">
          {(domains.data?.domains || []).map((d) => <span key={d.name} className={`rounded-full px-2 py-0.5 ${d.mx_ok ? 'bg-emerald-500/10 text-emerald-600' : 'bg-destructive/10 text-destructive'}`}>{d.name}</span>)}
        </div>
      </Card>
    </div>
  )
}
