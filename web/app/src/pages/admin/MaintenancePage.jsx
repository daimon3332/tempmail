import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button, Card, Input, Badge } from '../../components/ui'
import api, { download } from '../../lib/api'

const CLEAN_TYPES = [
  { type: 'mails', label: '邮件（按接收时间）', enabled: 'enableMailsAutoCleanup', days: 'cleanMailsDays', defaultEnabled: false },
  { type: 'mails_unknow', label: '未知邮件', enabled: 'enableUnknowMailsAutoCleanup', days: 'cleanUnknowMailsDays', defaultEnabled: false },
  { type: 'sendbox', label: '发件箱', enabled: 'enableSendBoxAutoCleanup', days: 'cleanSendBoxDays', defaultEnabled: false },
  { type: 'addressCreated', label: '邮箱（按创建时间）', enabled: 'enableAddressAutoCleanup', days: 'cleanAddressDays', defaultEnabled: false },
  { type: 'inactiveAddress', label: '非活跃邮箱', enabled: 'enableInactiveAddressAutoCleanup', days: 'cleanInactiveAddressDays', defaultEnabled: false },
  { type: 'unboundAddress', label: '未绑定邮箱', enabled: 'enableUnboundAddressAutoCleanup', days: 'cleanUnboundAddressDays', defaultEnabled: false },
  { type: 'emptyAddress', label: '空邮箱', enabled: 'enableEmptyAddressAutoCleanup', days: 'cleanEmptyAddressDays', defaultEnabled: true },
]

export default function MaintenancePage({ t }) {
  const [tab, setTab] = useState('cleanup')
  return (
    <div className="space-y-3">
      <h2 className="text-xl font-semibold">{t('adminCleanup')}</h2>
      <div className="flex gap-2 text-sm">
        <TabBtn active={tab === 'cleanup'} onClick={() => setTab('cleanup')}>清理策略</TabBtn>
        <TabBtn active={tab === 'db'} onClick={() => setTab('db')}>数据库管理</TabBtn>
      </div>
      {tab === 'cleanup' && <Cleanup />}
      {tab === 'db' && <Database />}
    </div>
  )
}

function TabBtn({ active, onClick, children }) {
  return <button className={`rounded-lg px-3 py-1.5 ${active ? 'bg-primary text-primary-foreground' : 'bg-muted hover:bg-accent'}`} onClick={onClick}>{children}</button>
}

function Cleanup() {
  const q = useQuery({ queryKey: ['auto_cleanup'], queryFn: () => api('/admin/auto_cleanup').then(r => r.data) })
  const [manual, setManual] = useState({ cleanTypes: ['mails'], cleanDays: 30 })
  const [sql, setSql] = useState('')
  const save = async (cfg) => { const r = await api('/admin/auto_cleanup', 'POST', cfg); if (r.status === 200) q.refetch(); else alert(r.data || '保存失败') }
  const runNow = async () => {
    if (!manual.cleanTypes.length) { alert('请至少选择一项清理内容'); return }
    const r = await api('/admin/cleanup', 'POST', manual)
    alert(r.status === 200 ? `已执行 ${manual.cleanTypes.length} 项清理` : (r.data || '失败'))
  }
  const execSql = async () => { if (!sql.trim()) return; const r = await api('/admin/auto_cleanup', 'POST', { ...(q.data || {}), customSqlCleanupList: [...((q.data?.customSqlCleanupList) || []), { id: 'custom-' + Date.now(), name: '手动SQL', sql, enabled: true }] }); if (r.status === 200) { await api('/admin/cleanup', 'POST', { cleanType: 'custom', cleanDays: 0 }); setSql(''); alert('已执行') } else alert(r.data || 'SQL 校验失败') }
  const cfg = q.data || {}
  return (
    <div className="space-y-4">
      <Card className="p-5">
        <h3 className="mb-3 font-semibold">自动清理策略</h3>
        <div className="space-y-3">
          {CLEAN_TYPES.map(item => <AutoRow key={item.type} item={item} cfg={cfg} onSave={save} />)}
        </div>
      </Card>
      <Card className="p-5">
        <h3 className="mb-3 font-semibold">立即执行清理</h3>
        <p className="mb-3 text-sm text-muted-foreground">可同时选择多项，按统一保留天数清理。</p>
        <div className="grid gap-2 sm:grid-cols-2">
          {CLEAN_TYPES.map(item => {
            const checked = manual.cleanTypes.includes(item.type)
            return <label key={item.type} className={`flex cursor-pointer items-start gap-3 rounded-md border px-3 py-2 text-sm transition-colors ${checked ? 'border-primary bg-primary/5' : 'border-border hover:bg-accent'}`}>
              <input className="mt-0.5 h-4 w-4 shrink-0 accent-[hsl(var(--primary))]" type="checkbox" checked={checked} onChange={() => setManual(x => ({ ...x, cleanTypes: checked ? x.cleanTypes.filter(t => t !== item.type) : [...x.cleanTypes, item.type] }))} />
              <span className="break-words">{item.label}</span>
            </label>
          })}
        </div>
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <Input aria-label="立即清理保留天数" type="number" min="0" className="w-24" value={manual.cleanDays} onChange={e => setManual({ ...manual, cleanDays: Math.max(0, parseInt(e.target.value, 10) || 0) })} />
          <span className="text-sm text-muted-foreground">天前</span>
          <Button size="sm" onClick={runNow}>执行清理</Button>
        </div>
      </Card>
      <Card className="p-5">
        <h3 className="mb-3 font-semibold">自定义清理 SQL（仅 DELETE，单条，无注释）</h3>
        <Input placeholder="DELETE FROM raw_mails WHERE created_at < datetime('now','-30 day')" value={sql} onChange={e => setSql(e.target.value)} />
        <Button size="sm" className="mt-2" onClick={execSql}>执行 SQL</Button>
      </Card>
    </div>
  )
}

function AutoRow({ item, cfg, onSave }) {
  const enabled = cfg[item.enabled] == null ? item.defaultEnabled : !!cfg[item.enabled]
  const days = cfg[item.days] ?? 30
  const [draftDays, setDraftDays] = useState(String(days))
  useEffect(() => setDraftDays(String(days)), [days])
  const toggle = () => onSave({ ...cfg, [item.enabled]: !enabled, [item.days]: days })
  const commitDays = () => { const value = Math.max(1, Number.parseInt(draftDays, 10) || 30); setDraftDays(String(value)); if (value !== days) onSave({ ...cfg, [item.days]: value }) }
  return (
    <div className="grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-3 border-b border-border pb-2 text-sm">
      <label className="flex min-w-0 items-center gap-2">
        <button type="button" role="switch" aria-checked={enabled} onClick={toggle} className={`relative h-6 w-11 shrink-0 rounded-full transition-colors ${enabled ? 'bg-primary' : 'bg-muted-foreground/30'}`}><span className={`absolute left-0 top-1 h-4 w-4 rounded-full bg-white transition-transform ${enabled ? 'translate-x-6' : 'translate-x-1'}`} /></button>
        <span className="min-w-0 break-words">{item.label}</span>
      </label>
      <div className="flex shrink-0 items-center gap-1">
        <input aria-label={`${item.label}保留天数`} className="w-16 rounded border border-border bg-background px-2 py-1" type="number" min="1" value={draftDays} onChange={e => setDraftDays(e.target.value)} onBlur={commitDays} onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); commitDays(); e.currentTarget.blur() } }} /> 天
      </div>
    </div>
  )
}
function Database() {
  const v = useQuery({ queryKey: ['db_version'], queryFn: () => api('/admin/db_version').then(r => r.data) })
  const [merge, setMerge] = useState(false)
  const doBackup = () => download('/admin/db_backup', `tempmail-${new Date().toISOString().slice(0, 10)}.db`).catch(e => alert(e.message))
  const doImport = async (file) => {
    const data = await file.text()
    const r = await api(`/admin/db_import?merge=${merge ? 1 : 0}`, 'POST', data, { headers: { 'Content-Type': 'application/octet-stream' } })
    alert(r.status === 200 ? `导入完成：${r.data.executed} 语句` : (r.data || '导入失败'))
  }
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <Card className="p-5 lg:col-span-2">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div><h3 className="font-semibold">数据库状态</h3><p className="mt-1 text-sm text-muted-foreground">用于确认当前数据版本与程序是否匹配。</p></div>
          <Badge variant={v.data?.need_migration ? 'default' : 'green'}>{v.data?.need_migration ? '需要迁移' : '已是最新'}</Badge>
        </div>
        <div className="mt-4 grid gap-3 sm:grid-cols-3">
          <div className="rounded-md bg-muted/50 px-3 py-2"><div className="text-xs text-muted-foreground">当前数据库版本</div><div className="mt-1 font-medium">{v.data?.current_db_version || '—'}</div></div>
          <div className="rounded-md bg-muted/50 px-3 py-2"><div className="text-xs text-muted-foreground">程序支持版本</div><div className="mt-1 font-medium">{v.data?.code_db_version || 'v0.0.8'}</div></div>
          <div className="rounded-md bg-muted/50 px-3 py-2"><div className="text-xs text-muted-foreground">兼容导出</div><div className="mt-1 font-medium">Cloudflare Temp Mail / cf-teml-mail SQLite SQL</div></div>
        </div>
      </Card>
      <Card className="p-5"><h3 className="font-semibold">备份数据</h3><p className="mt-1 text-sm text-muted-foreground">下载当前 SQLite 数据库文件，导入或迁移前建议先备份。</p><Button className="mt-4" variant="outline" onClick={doBackup}>下载数据库备份</Button></Card>
      <Card className="p-5"><h3 className="font-semibold">导入数据</h3><p className="mt-1 text-sm text-muted-foreground">支持 Cloudflare Temp Mail / cf-teml-mail 的 SQL 导出文件。</p><label className="mt-4 flex items-center gap-2 text-sm"><input type="checkbox" checked={merge} onChange={e => setMerge(e.target.checked)} />保留现有数据并合并冲突</label><input className="mt-3 block w-full text-sm" type="file" accept=".sql" onChange={e => e.target.files[0] && doImport(e.target.files[0])} /><p className="mt-2 text-xs text-destructive">未勾选合并时会覆盖导入，请确认已下载备份。</p></Card>
      <Card className="p-5 lg:col-span-2"><h3 className="font-semibold">迁移说明</h3><p className="mt-1 text-sm text-muted-foreground">导入后程序会按当前数据库版本执行兼容迁移。不会删除现有服务器邮箱或邮件；覆盖导入只作用于你选择的 SQL 文件。</p></Card>
    </div>
  )
}
