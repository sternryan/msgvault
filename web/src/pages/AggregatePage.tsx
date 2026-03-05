import { useSearchParams, useNavigate } from 'react-router-dom'
import { useAggregate, useSubAggregate, useAccounts } from '../api/hooks'
import { Breadcrumbs, type Crumb } from '../components/layout/Breadcrumbs'
import { AggregateTable } from '../components/aggregate/AggregateTable'
import type { ViewType, SortField, SortDirection, TimeGranularity, MessageFilterParams } from '../api/types'
import { VIEW_TYPE_LABELS } from '../lib/utils'

const VIEW_TYPES: ViewType[] = ['senders', 'senderNames', 'recipients', 'recipientNames', 'domains', 'labels', 'time']
const TIME_GRANULARITIES: TimeGranularity[] = ['year', 'month', 'day']

export function AggregatePage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const navigate = useNavigate()

  const groupBy = (searchParams.get('groupBy') as ViewType) || 'senders'
  const sortField = (searchParams.get('sortField') as SortField) || 'count'
  const sortDir = (searchParams.get('sortDir') as SortDirection) || 'desc'
  const timeGranularity = (searchParams.get('timeGranularity') as TimeGranularity) || 'month'
  const sourceId = searchParams.get('sourceId') ? Number(searchParams.get('sourceId')) : undefined
  const attachmentsOnly = searchParams.get('attachmentsOnly') === 'true'
  const limit = Number(searchParams.get('limit')) || 1000

  // Sub-aggregate context (drill-down from a parent view)
  const filterKey = searchParams.get('filterKey')
  const filterView = searchParams.get('filterView') as ViewType | null

  const { data: accounts } = useAccounts()

  // Determine if we're in sub-aggregate mode
  const isSubAggregate = filterKey !== null && filterView !== null
  const filterParams: Omit<MessageFilterParams, 'sortField' | 'sortDir'> = {}
  if (isSubAggregate && filterView && filterKey) {
    switch (filterView) {
      case 'senders': filterParams.sender = filterKey; break
      case 'senderNames': filterParams.senderName = filterKey; break
      case 'recipients': filterParams.recipient = filterKey; break
      case 'recipientNames': filterParams.recipientName = filterKey; break
      case 'domains': filterParams.domain = filterKey; break
      case 'labels': filterParams.label = filterKey; break
      case 'time':
        filterParams.timePeriod = filterKey
        filterParams.timeGranularity = timeGranularity
        break
    }
  }

  const aggSortField = (['count', 'size', 'attachmentSize', 'name'] as const).includes(sortField as 'count' | 'size' | 'attachmentSize' | 'name')
    ? sortField as 'count' | 'size' | 'attachmentSize' | 'name'
    : 'count'

  const aggregateQuery = useAggregate({
    groupBy,
    sortField: aggSortField,
    sortDir,
    timeGranularity: groupBy === 'time' ? timeGranularity : undefined,
    sourceId,
    attachmentsOnly,
    limit,
  })

  const subAggregateQuery = useSubAggregate({
    groupBy,
    sortField: aggSortField,
    sortDir,
    timeGranularity: groupBy === 'time' ? timeGranularity : undefined,
    sourceId,
    attachmentsOnly,
    limit,
    ...filterParams,
  })

  const query = isSubAggregate ? subAggregateQuery : aggregateQuery
  const rows = query.data ?? []

  const setParam = (key: string, value: string | undefined) => {
    const next = new URLSearchParams(searchParams)
    if (value === undefined) {
      next.delete(key)
    } else {
      next.set(key, value)
    }
    setSearchParams(next, { replace: true })
  }

  const handleRowClick = (key: string) => {
    if (isSubAggregate) {
      // Second level drill-down: go to message list
      const params = new URLSearchParams()
      if (filterView && filterKey) {
        // Carry parent filter
        switch (filterView) {
          case 'senders': params.set('sender', filterKey); break
          case 'senderNames': params.set('senderName', filterKey); break
          case 'recipients': params.set('recipient', filterKey); break
          case 'recipientNames': params.set('recipientName', filterKey); break
          case 'domains': params.set('domain', filterKey); break
          case 'labels': params.set('label', filterKey); break
          case 'time':
            params.set('timePeriod', filterKey)
            params.set('timeGranularity', timeGranularity)
            break
        }
      }
      // Add current drill-down
      switch (groupBy) {
        case 'senders': params.set('sender', key); break
        case 'senderNames': params.set('senderName', key); break
        case 'recipients': params.set('recipient', key); break
        case 'recipientNames': params.set('recipientName', key); break
        case 'domains': params.set('domain', key); break
        case 'labels': params.set('label', key); break
        case 'time':
          params.set('timePeriod', key)
          params.set('timeGranularity', timeGranularity)
          break
      }
      navigate(`/messages?${params.toString()}`)
    } else {
      // First level: drill down into sub-aggregate
      const next = new URLSearchParams(searchParams)
      next.set('filterKey', key)
      next.set('filterView', groupBy)
      // Switch to a different view for the sub-aggregate
      const nextGroupBy = groupBy === 'senders' ? 'recipients' : 'senders'
      next.set('groupBy', nextGroupBy)
      setSearchParams(next)
    }
  }

  const handleSort = (field: SortField) => {
    if (field === sortField) {
      setParam('sortDir', sortDir === 'desc' ? 'asc' : 'desc')
    } else {
      setParam('sortField', field)
      setParam('sortDir', 'desc')
    }
  }

  // Build breadcrumbs
  const crumbs: Crumb[] = [{ label: 'Aggregate', to: '/aggregate' }]
  if (isSubAggregate && filterView) {
    crumbs.push({
      label: VIEW_TYPE_LABELS[filterView] ?? filterView,
      to: `/aggregate?groupBy=${filterView}`,
    })
    crumbs.push({ label: filterKey ?? '' })
  }

  return (
    <div className="space-y-4">
      <Breadcrumbs items={crumbs} />

      {/* View type tabs */}
      <div className="flex flex-wrap items-center gap-2">
        {VIEW_TYPES.map((vt) => (
          <button
            key={vt}
            onClick={() => {
              const next = new URLSearchParams()
              next.set('groupBy', vt)
              if (sourceId) next.set('sourceId', String(sourceId))
              if (attachmentsOnly) next.set('attachmentsOnly', 'true')
              if (vt === 'time') next.set('timeGranularity', timeGranularity)
              if (isSubAggregate && filterKey && filterView) {
                next.set('filterKey', filterKey)
                next.set('filterView', filterView)
              }
              setSearchParams(next)
            }}
            className={`px-3 py-1.5 rounded-md text-sm transition-colors ${
              vt === groupBy
                ? 'bg-zinc-900 text-white dark:bg-zinc-100 dark:text-zinc-900'
                : 'bg-zinc-100 dark:bg-zinc-800 text-zinc-600 dark:text-zinc-400 hover:bg-zinc-200 dark:hover:bg-zinc-700'
            }`}
          >
            {VIEW_TYPE_LABELS[vt]}
          </button>
        ))}
      </div>

      {/* Filter bar */}
      <div className="flex items-center gap-3 text-sm">
        {groupBy === 'time' && (
          <div className="flex items-center gap-1">
            <span className="text-zinc-500">Granularity:</span>
            {TIME_GRANULARITIES.map((g) => (
              <button
                key={g}
                onClick={() => setParam('timeGranularity', g)}
                className={`px-2 py-0.5 rounded text-xs ${
                  g === timeGranularity
                    ? 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200'
                    : 'text-zinc-500 hover:text-zinc-900 dark:hover:text-zinc-100'
                }`}
              >
                {g.charAt(0).toUpperCase() + g.slice(1)}
              </button>
            ))}
          </div>
        )}

        {accounts && accounts.length > 1 && (
          <select
            value={sourceId ?? ''}
            onChange={(e) => setParam('sourceId', e.target.value || undefined)}
            className="px-2 py-1 rounded border border-zinc-200 dark:border-zinc-700 bg-white dark:bg-zinc-900 text-sm"
          >
            <option value="">All accounts</option>
            {accounts.map((acc) => (
              <option key={acc.id} value={acc.id}>
                {acc.identifier}
              </option>
            ))}
          </select>
        )}

        <label className="flex items-center gap-1 cursor-pointer">
          <input
            type="checkbox"
            checked={attachmentsOnly}
            onChange={(e) => setParam('attachmentsOnly', e.target.checked ? 'true' : undefined)}
            className="rounded"
          />
          <span className="text-zinc-500">Attachments only</span>
        </label>
      </div>

      {/* Table */}
      <AggregateTable
        rows={rows}
        isLoading={query.isLoading}
        sortField={sortField}
        sortDir={sortDir}
        onSort={handleSort}
        onRowClick={handleRowClick}
        viewType={groupBy}
      />
    </div>
  )
}
