import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button, Card, Input, Badge } from '../../components/ui'
import api from '../../lib/api'

export default function TelegramPage({ t }) {
  const q = useQuery({ queryKey: ['tg_settings'], queryFn: () => api('/admin/telegram/settings').then(r => r.data) })
  const [status, setStatus] = useState(null)
  const [bound, setBound] = useState(null)
  const [f, setF] = useState(null)
  const s = f || q.data || { allowList: [], globalMailPushList: [] }
  if (q.data && f === null) setF(q.data)
  const up = (k, v) => setF(x => ({ ...x, [k]: v }))
  const save = async () => { const r = await api('/admin/telegram/settings', 'POST', s); if (r.status === 200) { q.refetch(); alert('已保存') } else alert(r.data || '失败') }
  const init = async () => { const r = await api('/admin/telegram/init', 'POST', {}); alert(r.status === 200 ? 'Webhook 已设置' : (r.data || '失败')) }
  const getStatus = async () => { const r = await api('/admin/telegram/status'); setStatus(r.data) }
  const loadBound = async () => setBound([])
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">电报机器人</h2>
        <div className="flex gap-2">
          <Button size="sm" variant="outline" onClick={init}>设置 Webhook</Button>
          <Button size="sm" variant="outline" onClick={getStatus}>状态</Button>
          <Button size="sm" onClick={save}>保存</Button>
        </div>
      </div>
      {!s && <Card className="p-6 text-center text-sm text-muted-foreground">未配置 TELEGRAM_BOT_TOKEN，功能不可用</Card>}
      {s && (
        <>
          <Card className="p-4">
            <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={!!s.enableAllowList} onChange={e => up('enableAllowList', e.target.checked)} /> 启用白名单</label>
            <div className="mt-2"><label className="text-xs text-muted-foreground">允许用户 ID（逗号分隔）</label><Input value={(s.allowList || []).join(',')} onChange={e => up('allowList', e.target.value.split(',').map(x => x.trim()).filter(Boolean))} /></div>
          </Card>
          <Card className="p-4">
            <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={!!s.enableGlobalMailPush} onChange={e => up('enableGlobalMailPush', e.target.checked)} /> 全局邮件推送</label>
            <div className="mt-2"><label className="text-xs text-muted-foreground">推送目标用户 ID（逗号分隔）</label><Input value={(s.globalMailPushList || []).join(',')} onChange={e => up('globalMailPushList', e.target.value.split(',').map(x => x.trim()).filter(Boolean))} /></div>
          </Card>
          <Card className="p-4">
            <label className="text-xs text-muted-foreground">MiniApp URL</label>
            <Input value={s.miniAppUrl || ''} onChange={e => up('miniAppUrl', e.target.value)} placeholder="https://tempmail.333186.xyz" />
          </Card>
          {status && <Card className="p-4 text-xs"><pre className="whitespace-pre-wrap">{JSON.stringify(status, null, 2)}</pre></Card>}
        </>
      )}
    </div>
  )
}
