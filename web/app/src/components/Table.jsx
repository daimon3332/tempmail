import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import api from '../lib/api'
import { Card, Button, Input, Select, Spinner } from './ui'

// Generic paginated table that fetches /admin/<path> as {results,count}.
export function PagedTable({ path, queryKey, columns, filters, pageSize = 20, onRow, query }) {
  const [page, setPage] = useState(0)
  const [q, setQ] = useState('')
  const [extra, setExtra] = useState({})
  const qk = [queryKey, page, q, extra]
  const qr = useQuery({
    queryKey: qk,
    queryFn: () => {
      const params = new URLSearchParams({ limit: pageSize, offset: page * pageSize })
      if (q) params.set('query', q)
      if (query) params.set('query', q)
      for (const [k, v] of Object.entries(extra)) if (v !== '' && v != null && v !== undefined) params.set(k, v)
      return api(`${path}?${params.toString()}`).then(r => r.data)
    },
  })
  const rows = qr.data?.results || []
  const count = qr.data?.count || 0
  const lastPage = Math.max(0, Math.floor((count - 1) / pageSize))
  return (
    <Card className="p-4">
      <div className="mb-3 flex flex-wrap items-center gap-2">
        {filters && filters({ setExtra, extra, q, setQ })}
        {(q || Object.keys(extra).length) && (
          <Button size="sm" variant="ghost" onClick={() => { setQ(''); setExtra({}); setPage(0) }}>✕ 清除</Button>
        )}
      </div>
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead className="bg-muted text-left text-xs uppercase text-muted-foreground">
            <tr>{columns.map(c => <th key={c.key} className="px-3 py-2">{c.title}</th>)}</tr>
          </thead>
          <tbody>
            {qr.isLoading && <tr><td colSpan={columns.length} className="px-3 py-6 text-center"><Spinner /></td></tr>}
            {!qr.isLoading && rows.length === 0 && <tr><td colSpan={columns.length} className="px-3 py-6 text-center text-muted-foreground">暂无数据</td></tr>}
            {!qr.isLoading && rows.map((row, i) => (
              <tr key={row.id ?? i} onClick={onRow ? () => onRow(row) : undefined} className={`border-t border-border ${onRow ? 'cursor-pointer hover:bg-accent' : ''}`}>
                {columns.map(c => <td key={c.key} className="px-3 py-2">{c.render ? c.render(row) : row[c.key]}</td>)}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div className="mt-3 flex items-center justify-between text-sm">
        <span className="text-muted-foreground">{count} 条</span>
        <div className="flex gap-1">
          <Button size="sm" variant="outline" disabled={page <= 0} onClick={() => setPage(p => p - 1)}>上一页</Button>
          <Button size="sm" variant="outline" disabled={page >= lastPage} onClick={() => setPage(p => p + 1)}>下一页</Button>
        </div>
      </div>
    </Card>
  )
}

export { Input, Select }
