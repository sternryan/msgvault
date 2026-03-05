import { useSearchParams, Link } from 'react-router-dom'
import { useMessages } from '../api/hooks'
import { Breadcrumbs, type Crumb } from '../components/layout/Breadcrumbs'
import { formatBytes, formatDate } from '../lib/utils'
import { Paperclip, ChevronLeft, ChevronRight } from 'lucide-react'
import type { MessageSortField, SortDirection, MessageFilterParams } from '../api/types'

export function MessagesPage() {
  const [searchParams, setSearchParams] = useSearchParams()

  const limit = Number(searchParams.get('limit')) || 100
  const offset = Number(searchParams.get('offset')) || 0
  const sortField = (searchParams.get('sortField') as MessageSortField) || 'date'
  const sortDir = (searchParams.get('sortDir') as SortDirection) || 'desc'

  const filterParams: MessageFilterParams = {
    limit,
    offset,
    sortField,
    sortDir,
  }

  // Extract all filter params from URL
  for (const key of ['sender', 'senderName', 'recipient', 'recipientName', 'domain', 'label', 'timePeriod', 'timeGranularity'] as const) {
    const v = searchParams.get(key)
    if (v) (filterParams as Record<string, string>)[key] = v
  }
  if (searchParams.get('sourceId')) filterParams.sourceId = Number(searchParams.get('sourceId'))
  if (searchParams.get('attachmentsOnly') === 'true') filterParams.attachmentsOnly = true
  if (searchParams.get('conversationId')) filterParams.conversationId = Number(searchParams.get('conversationId'))

  const { data: messages, isLoading } = useMessages(filterParams)

  const setParam = (key: string, value: string | undefined) => {
    const next = new URLSearchParams(searchParams)
    if (value === undefined) {
      next.delete(key)
    } else {
      next.set(key, value)
    }
    setSearchParams(next, { replace: true })
  }

  // Build breadcrumbs from filter context
  const crumbs: Crumb[] = []
  const activeFilters: string[] = []
  if (filterParams.sender) activeFilters.push(`Sender: ${filterParams.sender}`)
  if (filterParams.senderName) activeFilters.push(`Sender: ${filterParams.senderName}`)
  if (filterParams.recipient) activeFilters.push(`Recipient: ${filterParams.recipient}`)
  if (filterParams.recipientName) activeFilters.push(`Recipient: ${filterParams.recipientName}`)
  if (filterParams.domain) activeFilters.push(`Domain: ${filterParams.domain}`)
  if (filterParams.label) activeFilters.push(`Label: ${filterParams.label}`)
  if (filterParams.timePeriod) activeFilters.push(`Time: ${filterParams.timePeriod}`)
  if (activeFilters.length > 0) {
    crumbs.push({ label: 'Aggregate', to: '/aggregate' })
    crumbs.push({ label: activeFilters.join(' + ') })
  } else {
    crumbs.push({ label: 'Messages' })
  }

  return (
    <div className="space-y-4">
      <Breadcrumbs items={crumbs} />

      {/* Active filters */}
      {activeFilters.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {Object.entries(filterParams).map(([key, value]) => {
            if (['limit', 'offset', 'sortField', 'sortDir'].includes(key)) return null
            if (!value) return null
            const labelMap: Record<string, string> = {
              sender: 'Sender',
              senderName: 'Sender Name',
              recipient: 'Recipient',
              recipientName: 'Recipient Name',
              domain: 'Domain',
              label: 'Label',
              timePeriod: 'Time',
              sourceId: 'Account',
              attachmentsOnly: 'Attachments',
            }
            const label = labelMap[key]
            if (!label) return null
            return (
              <span
                key={key}
                className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200"
              >
                {label}: {String(value)}
                <button
                  onClick={() => setParam(key, undefined)}
                  className="ml-0.5 hover:text-blue-600 dark:hover:text-blue-300"
                >
                  &times;
                </button>
              </span>
            )
          })}
        </div>
      )}

      {/* Sort controls */}
      <div className="flex items-center gap-3 text-sm">
        <span className="text-zinc-500">Sort by:</span>
        {(['date', 'size', 'subject'] as MessageSortField[]).map((f) => (
          <button
            key={f}
            onClick={() => {
              if (f === sortField) {
                setParam('sortDir', sortDir === 'desc' ? 'asc' : 'desc')
              } else {
                setParam('sortField', f)
                setParam('sortDir', 'desc')
              }
            }}
            className={`px-2 py-0.5 rounded text-xs ${
              f === sortField
                ? 'bg-zinc-900 text-white dark:bg-zinc-100 dark:text-zinc-900'
                : 'text-zinc-500 hover:text-zinc-900 dark:hover:text-zinc-100'
            }`}
          >
            {f.charAt(0).toUpperCase() + f.slice(1)}
            {f === sortField && (sortDir === 'asc' ? ' ↑' : ' ↓')}
          </button>
        ))}
      </div>

      {/* Message list */}
      {isLoading ? (
        <div className="animate-pulse space-y-2">
          {[...Array(10)].map((_, i) => (
            <div key={i} className="h-14 bg-zinc-100 dark:bg-zinc-800 rounded" />
          ))}
        </div>
      ) : !messages || messages.length === 0 ? (
        <div className="py-12 text-center text-zinc-400">No messages match the current filters.</div>
      ) : (
        <div className="rounded-lg border border-zinc-200 dark:border-zinc-800 divide-y divide-zinc-100 dark:divide-zinc-800/50">
          {messages.map((msg) => (
            <Link
              key={msg.id}
              to={`/messages/${msg.id}`}
              className="flex items-center gap-3 px-4 py-3 hover:bg-zinc-50 dark:hover:bg-zinc-900 transition-colors"
            >
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium truncate">
                    {msg.fromName || msg.fromEmail}
                  </span>
                  <span className="text-xs text-zinc-400">{formatDate(msg.sentAt)}</span>
                </div>
                <div className="text-sm truncate">{msg.subject || '(no subject)'}</div>
                {msg.snippet && (
                  <div className="text-xs text-zinc-400 truncate">{msg.snippet}</div>
                )}
              </div>
              <div className="flex items-center gap-2 shrink-0">
                {msg.hasAttachments && (
                  <Paperclip size={14} className="text-zinc-400" />
                )}
                {msg.labels && msg.labels.length > 0 && (
                  <div className="hidden sm:flex gap-1">
                    {msg.labels.slice(0, 3).map((l) => (
                      <span
                        key={l}
                        className="px-1.5 py-0.5 rounded text-[10px] bg-zinc-100 dark:bg-zinc-800 text-zinc-500"
                      >
                        {l}
                      </span>
                    ))}
                  </div>
                )}
                <span className="text-xs text-zinc-400 tabular-nums w-16 text-right">
                  {formatBytes(msg.sizeEstimate)}
                </span>
              </div>
            </Link>
          ))}
        </div>
      )}

      {/* Pagination */}
      {messages && messages.length > 0 && (
        <div className="flex items-center justify-between text-sm">
          <span className="text-zinc-500">
            Showing {offset + 1}–{offset + messages.length}
          </span>
          <div className="flex items-center gap-2">
            <button
              disabled={offset === 0}
              onClick={() => setParam('offset', String(Math.max(0, offset - limit)))}
              className="p-1 rounded hover:bg-zinc-100 dark:hover:bg-zinc-800 disabled:opacity-30"
            >
              <ChevronLeft size={16} />
            </button>
            <button
              disabled={messages.length < limit}
              onClick={() => setParam('offset', String(offset + limit))}
              className="p-1 rounded hover:bg-zinc-100 dark:hover:bg-zinc-800 disabled:opacity-30"
            >
              <ChevronRight size={16} />
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
