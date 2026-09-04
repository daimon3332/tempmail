import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import api from '../lib/api'
import { Card, Button, Input, Select, Spinner } from './ui'
import { ChevronFirst, ChevronLast, ChevronLeft, ChevronRight } from 'lucide-react'

// Generic paginated table that fetches /admin/<path> as {results,count}.
export function PagedTable({ path, queryKey, columns, filters, pageSize: initialPageSize = 100, onRow, query }) {
  const [page, setPage] = useState(0)
  const [pageSize, setPageSize] = useState(initialPageSize)
  const [q, setQ] = useState('')
  const [extra, setExtra] = useState({})
  const qk = [queryKey, page, pageSize, q, extra, query]
  const queryString = useMemo(() => JSON.stringify(query || {}), [query])
  useEffect(() => { setPage(0) }, [q, JSON.stringify(extra), queryString, pageSize])
  const qr = useQuery({
    queryKey: qk,
    queryFn: () => {
      const params = new URLSearchParams({ limit: pageSize, offset: page * pageSize })
      if (q) params.set('query', q)
      if (query) for (const [k, v] of Object.entries(query)) if (v !== '' && v != null) params.set(k, v)
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
      <div className="mt-3 flex flex-wrap items-center justify-between gap-3 text-sm">
        <span className="text-muted-foreground">共 {count} 条 · 第 {count ? page + 1 : 0} / {count ? lastPage + 1 : 0} 页</span>
        <div className="flex flex-wrap items-center gap-1">
          <Select aria-label="每页数量" value={pageSize} onChange={e => setPageSize(Number(e.target.value))} className="h-8">
            {[25, 50, 100].map(n => <option key={n} value={n}>{n} 条/页</option>)}
          </Select>
          <Button size="sm" variant="outline" title="首页" aria-label="首页" disabled={page <= 0} onClick={() => setPage(0)}><ChevronFirst className="h-4 w-4" /></Button>
          <Button size="sm" variant="outline" title="上一页" aria-label="上一页" disabled={page <= 0} onClick={() => setPage(p => p - 1)}><ChevronLeft className="h-4 w-4" /></Button>
          <Input aria-label="跳转页码" className="h-8 w-16 text-center" type="number" min="1" max={Math.max(1, lastPage + 1)} value={count ? page + 1 : ''} onChange={e => { const n = Number(e.target.value); if (Number.isFinite(n) && n >= 1) setPage(Math.min(lastPage, n - 1)) }} />
          <Button size="sm" variant="outline" title="下一页" aria-label="下一页" disabled={page >= lastPage} onClick={() => setPage(p => p + 1)}><ChevronRight className="h-4 w-4" /></Button>
          <Button size="sm" variant="outline" title="末页" aria-label="末页" disabled={page >= lastPage} onClick={() => setPage(lastPage)}><ChevronLast className="h-4 w-4" /></Button>
        </div>
      </div>
    </Card>
  )
}

export { Input, Select }
