import { useSearchParams, Link } from 'react-router-dom'
import { useSearch, useSearchCount } from '../api/hooks'
import { SearchBar } from '../components/search/SearchBar'
import { Breadcrumbs } from '../components/layout/Breadcrumbs'
import { formatBytes, formatDate, formatNumber } from '../lib/utils'
import { Paperclip, ChevronLeft, ChevronRight } from 'lucide-react'
import { useCallback } from 'react'

export function SearchPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const q = searchParams.get('q') || ''
  const mode = (searchParams.get('mode') as 'fast' | 'deep') || 'fast'
  const limit = Number(searchParams.get('limit')) || 100
  const offset = Number(searchParams.get('offset')) || 0

  const { data: results, isLoading } = useSearch({
    q,
    mode,
    limit,
    offset,
  })

  const { data: countData } = useSearchCount({ q })

  const handleSearch = useCallback(
    (query: string) => {
      const next = new URLSearchParams()
      next.set('q', query)
      if (mode !== 'fast') next.set('mode', mode)
      setSearchParams(next)
    },
    [mode, setSearchParams],
  )

  const setParam = (key: string, value: string | undefined) => {
    const next = new URLSearchParams(searchParams)
    if (value === undefined) {
      next.delete(key)
    } else {
      next.set(key, value)
    }
    next.delete('offset') // Reset pagination on param change
    setSearchParams(next, { replace: true })
  }

  return (
    <div className="space-y-4">
      <Breadcrumbs items={[{ label: 'Search' }]} />

      <div className="max-w-xl">
        <SearchBar onSearch={handleSearch} initialValue={q} placeholder="Search emails (e.g., from:alice subject:meeting)" />
      </div>

      {q && (
        <>
          {/* Mode toggle and count */}
          <div className="flex items-center gap-4 text-sm">
            <div className="flex items-center gap-1">
              <button
                onClick={() => setParam('mode', undefined)}
                className={`px-2 py-0.5 rounded text-xs ${
                  mode === 'fast'
                    ? 'bg-zinc-900 text-white dark:bg-zinc-100 dark:text-zinc-900'
                    : 'text-zinc-500 hover:text-zinc-900 dark:hover:text-zinc-100'
                }`}
              >
                Search metadata
              </button>
              <button
                onClick={() => setParam('mode', 'deep')}
                className={`px-2 py-0.5 rounded text-xs ${
                  mode === 'deep'
                    ? 'bg-zinc-900 text-white dark:bg-zinc-100 dark:text-zinc-900'
                    : 'text-zinc-500 hover:text-zinc-900 dark:hover:text-zinc-100'
                }`}
              >
                Search full text
              </button>
            </div>
            {countData && (
              <span className="text-zinc-400">
                {formatNumber(countData.count)} result{countData.count !== 1 ? 's' : ''}
              </span>
            )}
          </div>

          {/* Results */}
          {isLoading ? (
            <div className="animate-pulse space-y-2">
              {[...Array(5)].map((_, i) => (
                <div key={i} className="h-14 bg-zinc-100 dark:bg-zinc-800 rounded" />
              ))}
            </div>
          ) : !results || results.length === 0 ? (
            <div className="py-12 text-center text-zinc-400">
              No results for &ldquo;{q}&rdquo;
              {mode === 'fast' && (
                <div className="mt-2">
                  <button
                    onClick={() => setParam('mode', 'deep')}
                    className="text-blue-600 dark:text-blue-400 hover:underline text-sm"
                  >
                    Try full text search
                  </button>
                </div>
              )}
            </div>
          ) : (
            <div className="rounded-lg border border-zinc-200 dark:border-zinc-800 divide-y divide-zinc-100 dark:divide-zinc-800/50">
              {results.map((msg) => (
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
                    {msg.hasAttachments && <Paperclip size={14} className="text-zinc-400" />}
                    <span className="text-xs text-zinc-400 tabular-nums">
                      {formatBytes(msg.sizeEstimate)}
                    </span>
                  </div>
                </Link>
              ))}
            </div>
          )}

          {/* Pagination */}
          {results && results.length > 0 && (
            <div className="flex items-center justify-between text-sm">
              <span className="text-zinc-500">
                Showing {offset + 1}–{offset + results.length}
                {countData ? ` of ${formatNumber(countData.count)}` : ''}
              </span>
              <div className="flex items-center gap-2">
                <button
                  disabled={offset === 0}
                  onClick={() => {
                    const next = new URLSearchParams(searchParams)
                    next.set('offset', String(Math.max(0, offset - limit)))
                    setSearchParams(next, { replace: true })
                  }}
                  className="p-1 rounded hover:bg-zinc-100 dark:hover:bg-zinc-800 disabled:opacity-30"
                >
                  <ChevronLeft size={16} />
                </button>
                <button
                  disabled={results.length < limit}
                  onClick={() => {
                    const next = new URLSearchParams(searchParams)
                    next.set('offset', String(offset + limit))
                    setSearchParams(next, { replace: true })
                  }}
                  className="p-1 rounded hover:bg-zinc-100 dark:hover:bg-zinc-800 disabled:opacity-30"
                >
                  <ChevronRight size={16} />
                </button>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  )
}
