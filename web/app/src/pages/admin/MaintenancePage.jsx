import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button, Card, Input, Select, Badge } from '../../components/ui'
import api from '../../lib/api'
import OperationLogPage from './OperationLogPage'

const CLEAN_TYPES = [
  ['mails', '邮件（按接收时间）'], ['mails_unknow', '未知邮件'], ['sendbox', '发件箱'],
  ['addressCreated', '地址（按创建时间）'], ['inactiveAddress', '非活跃地址'],
  ['unboundAddress', '未绑定地址'], ['emptyAddress', '空地址'],
]

export default function MaintenancePage({ t }) {
  const [tab, setTab] = useState('cleanup')
  return (
    <div className="space-y-3">
      <h2 className="text-xl font-semibold">{t('adminCleanup')}</h2>
      <div className="flex gap-2 text-sm">
        <TabBtn active={tab === 'cleanup'} onClick={() => setTab('cleanup')}>清理策略</TabBtn>
        <TabBtn active={tab === 'db'} onClick={() => setTab('db')}>数据库管理</TabBtn>
        <TabBtn active={tab === 'oplog'} onClick={() => setTab('oplog')}>操作日志</TabBtn>
      </div>
      {tab === 'cleanup' && <Cleanup />}
      {tab === 'db' && <Database />}
      {tab === 'oplog' && <OperationLogPage />}
    </div>
  )
}

function TabBtn({ active, onClick, children }) {
  return <button className={`rounded-lg px-3 py-1.5 ${active ? 'bg-primary text-primary-foreground' : 'bg-muted hover:bg-accent'}`} onClick={onClick}>{children}</button>
}

function Cleanup() {
  const q = useQuery({ queryKey: ['auto_cleanup'], queryFn: () => api('/admin/auto_cleanup').then(r => r.data) })
  const [manual, setManual] = useState({ cleanType: 'mails', cleanDays: 7 })
  const [sql, setSql] = useState('')
  const save = async (cfg) => { await api('/admin/auto_cleanup', 'POST', cfg); q.refetch() }
  const runNow = async () => { const r = await api('/admin/cleanup', 'POST', manual); alert(r.status === 200 ? '已执行' : (r.data || '失败')) }
  const execSql = async () => { if (!sql.trim()) return; const r = await api('/admin/auto_cleanup', 'POST', { ...(q.data || {}), customSqlCleanupList: [...((q.data?.customSqlCleanupList) || []), { id: 'custom-' + Date.now(), name: '手动SQL', sql, enabled: true }] }); if (r.status === 200) { await api('/admin/cleanup', 'POST', { cleanType: 'custom', cleanDays: 0 }); setSql(''); alert('已执行') } else alert(r.data || 'SQL 校验失败') }
  const cfg = q.data || {}
  const On = (k, d) => ({ enabled: !!cfg[k], days: cfg[d] || 7 })
  return (
    <div className="space-y-4">
      <Card className="p-5">
        <h3 className="mb-3 font-semibold">自动清理策略</h3>
        <div className="space-y-3">
          {CLEAN_TYPES.map(([type, label]) => <AutoRow key={type} type={type} label={label} cfg={cfg} onSave={save} />)}
        </div>
      </Card>
      <Card className="p-5">
        <h3 className="mb-3 font-semibold">立即执行清理</h3>
        <div className="flex gap-2">
          <Select value={manual.cleanType} onChange={e => setManual({ ...manual, cleanType: e.target.value })}>
            {CLEAN_TYPES.map(([k, v]) => <option key={k} value={k}>{v}</option>)}
          </Select>
          <Input type="number" className="w-24" value={manual.cleanDays} onChange={e => setManual({ ...manual, cleanDays: parseInt(e.target.value) })} />
          <Button size="sm" onClick={runNow}>执行</Button>
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

function AutoRow({ type, label, cfg, onSave }) {
  const enabled = !!cfg[`enable${upper(type)}AutoCleanup`]
  const days = cfg[`clean${upper(type)}Days`] ?? 7
  const toggle = async () => { const k = `enable${upper(type)}AutoCleanup`; await api('/admin/auto_cleanup', 'POST', { ...cfg, [k]: !enabled }); onSave({ ...cfg, [k]: !enabled }) }
  const setDays = async (d) => { const k = `clean${upper(type)}Days`; onSave({ ...cfg, [k]: d }) }
  return (
    <div className="flex items-center gap-3 border-b border-border pb-2 text-sm">
      <label className="flex items-center gap-2"><input type="checkbox" checked={enabled} onChange={toggle} /> {label}</label>
      <div className="ml-auto flex items-center gap-1">
        <input className="w-16 rounded border border-border bg-background px-2 py-1" type="number" value={days} onChange={e => setDays(parseInt(e.target.value))} /> 天
      </div>
    </div>
  )
}
function upper(s) { return s.charAt(0).toUpperCase() + s.slice(1) }

function Database() {
  const v = useQuery({ queryKey: ['db_version'], queryFn: () => api('/admin/db_version').then(r => r.data) })
  const [merge, setMerge] = useState(false)
  const doBackup = () => window.location.assign('/admin/db_backup')
  const doImport = async (file) => {
    const data = await file.text()
    const r = await api(`/admin/db_import?merge=${merge ? 1 : 0}`, 'POST', data, { headers: { 'Content-Type': 'application/octet-stream' } })
    alert(r.status === 200 ? `导入完成：${r.data.executed} 语句` : (r.data || '导入失败'))
  }
  return (
    <div className="space-y-4">
      <Card className="p-5">
        <h3 className="mb-3 font-semibold">数据库</h3>
        <div className="flex flex-wrap gap-2 text-sm text-muted-foreground">
          <span>当前版本: <Badge>{v.data?.current_db_version || '—'}</Badge></span>
          <span>需初始化: {String(v.data?.need_initialization)}</span>
        </div>
        <div className="mt-3 flex flex-wrap gap-2">
          <Button size="sm" variant="outline" onClick={doBackup}>下载备份 (.db)</Button>
          <label className="file:mr-2 flex items-center gap-2">
            <span className="text-sm">{merge ? '合并导入' : '覆盖导入'}</span>
            <input type="checkbox" checked={merge} onChange={e => setMerge(e.target.checked)} />
            <input type="file" accept=".sql" onChange={e => e.target.files[0] && doImport(e.target.files[0])} />
          </label>
        </div>
      </Card>
    </div>
  )
}
