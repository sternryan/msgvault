import { formatBytes, formatNumber } from '../../lib/utils'
import type { AggregateRow, SortField, SortDirection } from '../../api/types'
import { ArrowDown, ArrowUp, ArrowUpDown } from 'lucide-react'

interface AggregateTableProps {
  rows: AggregateRow[]
  isLoading: boolean
  sortField: SortField
  sortDir: SortDirection
  onSort: (field: SortField) => void
  onRowClick: (key: string) => void
  viewType: string
}

export function AggregateTable({
  rows,
  isLoading,
  sortField,
  sortDir,
  onSort,
  onRowClick,
}: AggregateTableProps) {
  if (isLoading) {
    return (
      <div className="animate-pulse space-y-2">
        {[...Array(10)].map((_, i) => (
          <div key={i} className="h-10 bg-zinc-100 dark:bg-zinc-800 rounded" />
        ))}
      </div>
    )
  }

  if (rows.length === 0) {
    return (
      <div className="py-12 text-center text-zinc-400">
        No data matches the current filters.
      </div>
    )
  }

  const totalUnique = rows[0]?.totalUnique ?? rows.length

  return (
    <div>
      <div className="text-xs text-zinc-400 mb-2">
        {formatNumber(totalUnique)} unique keys &middot; showing {formatNumber(rows.length)}
      </div>
      <div className="rounded-lg border border-zinc-200 dark:border-zinc-800 overflow-hidden">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-900">
              <SortHeader
                label="Key"
                field="name"
                active={sortField}
                dir={sortDir}
                onSort={onSort}
                className="text-left"
              />
              <SortHeader
                label="Count"
                field="count"
                active={sortField}
                dir={sortDir}
                onSort={onSort}
                className="text-right"
              />
              <SortHeader
                label="Size"
                field="size"
                active={sortField}
                dir={sortDir}
                onSort={onSort}
                className="text-right"
              />
              <SortHeader
                label="Attachments"
                field="attachmentSize"
                active={sortField}
                dir={sortDir}
                onSort={onSort}
                className="text-right"
              />
            </tr>
          </thead>
          <tbody>
            {rows.map((row, i) => (
              <tr
                key={row.key + i}
                onClick={() => onRowClick(row.key)}
                className="border-b border-zinc-100 dark:border-zinc-800/50 hover:bg-zinc-50 dark:hover:bg-zinc-900 cursor-pointer transition-colors"
              >
                <td className="px-3 py-2 font-mono text-sm truncate max-w-xs">
                  {row.key || <span className="text-zinc-400 italic">(empty)</span>}
                </td>
                <td className="px-3 py-2 text-right tabular-nums">{formatNumber(row.count)}</td>
                <td className="px-3 py-2 text-right tabular-nums text-zinc-500">
                  {formatBytes(row.totalSize)}
                </td>
                <td className="px-3 py-2 text-right tabular-nums text-zinc-500">
                  {row.attachmentCount > 0 ? (
                    <span>
                      {formatNumber(row.attachmentCount)} ({formatBytes(row.attachmentSize)})
                    </span>
                  ) : (
                    <span className="text-zinc-300 dark:text-zinc-700">&mdash;</span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function SortHeader({
  label,
  field,
  active,
  dir,
  onSort,
  className = '',
}: {
  label: string
  field: SortField
  active: SortField
  dir: SortDirection
  onSort: (f: SortField) => void
  className?: string
}) {
  const isActive = field === active
  return (
    <th
      className={`px-3 py-2 font-medium text-zinc-500 dark:text-zinc-400 cursor-pointer select-none hover:text-zinc-900 dark:hover:text-zinc-100 ${className}`}
      onClick={() => onSort(field)}
    >
      <span className="inline-flex items-center gap-1">
        {label}
        {isActive ? (
          dir === 'asc' ? <ArrowUp size={14} /> : <ArrowDown size={14} />
        ) : (
          <ArrowUpDown size={14} className="opacity-30" />
        )}
      </span>
    </th>
  )
}
