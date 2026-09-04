import { useQuery } from '@tanstack/react-query'
import { PagedTable, Input } from '../../components/Table'
import { Button } from '../../components/ui'
import api from '../../lib/api'

export default function OperationLogPage() {
  const q = useQuery({ queryKey: ['oplog'], queryFn: () => api('/admin/operation_log?limit=1&offset=0').then(r => r.data), enabled: false })
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">操作日志</h2>
        <Button size="sm" variant="destructive" onClick={() => { if (confirm('清空日志？')) { api('/admin/operation_log', 'DELETE'); q.refetch() } }}>清空</Button>
      </div>
      <PagedTable path="/admin/operation_log" queryKey="oplog"
        filters={({ setExtra, extra }) => <Input className="max-w-xs" placeholder="按操作/目标过滤" value={extra.target || ''} onChange={e => setExtra(x => ({ ...x, target: e.target.value }))} />}
        columns={[
          { key: 'id', title: 'ID' },
          { key: 'time', title: '时间' },
          { key: 'actor', title: '操作者' },
          { key: 'action', title: '操作' },
          { key: 'target', title: '目标', render: r => <span className="break-all text-xs">{r.target}</span> },
          { key: 'result', title: '结果' },
        ]}
      />
    </div>
  )
}
