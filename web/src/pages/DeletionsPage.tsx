import { useDeletions, useCancelDeletion } from '../api/hooks'
import { Breadcrumbs } from '../components/layout/Breadcrumbs'
import { formatNumber } from '../lib/utils'
import { Trash2, X, AlertTriangle, CheckCircle, Clock, Loader2 } from 'lucide-react'

const STATUS_CONFIG: Record<string, { icon: React.ReactNode; color: string; label: string }> = {
  pending: { icon: <Clock size={14} />, color: 'text-amber-600 bg-amber-50 dark:bg-amber-900/30 dark:text-amber-400', label: 'Pending' },
  in_progress: { icon: <Loader2 size={14} className="animate-spin" />, color: 'text-blue-600 bg-blue-50 dark:bg-blue-900/30 dark:text-blue-400', label: 'In Progress' },
  completed: { icon: <CheckCircle size={14} />, color: 'text-green-600 bg-green-50 dark:bg-green-900/30 dark:text-green-400', label: 'Completed' },
  failed: { icon: <AlertTriangle size={14} />, color: 'text-red-600 bg-red-50 dark:bg-red-900/30 dark:text-red-400', label: 'Failed' },
}

export function DeletionsPage() {
  const { data: deletions, isLoading } = useDeletions()
  const cancelMutation = useCancelDeletion()

  return (
    <div className="space-y-4">
      <Breadcrumbs items={[{ label: 'Deletions' }]} />

      <div className="flex items-center gap-2">
        <h1 className="text-xl font-semibold">Deletion Batches</h1>
        {deletions && deletions.length > 0 && (
          <span className="text-sm text-zinc-400">({deletions.length})</span>
        )}
      </div>

      <div className="rounded-lg border border-zinc-200 dark:border-zinc-800 p-4 bg-amber-50 dark:bg-amber-900/10">
        <p className="text-sm text-amber-800 dark:text-amber-300">
          Deletion batches are staged here. Run <code className="px-1 py-0.5 rounded bg-amber-100 dark:bg-amber-900/50">msgvault delete-staged</code> from the CLI to execute them.
        </p>
      </div>

      {isLoading ? (
        <div className="animate-pulse space-y-2">
          {[...Array(3)].map((_, i) => (
            <div key={i} className="h-16 bg-zinc-100 dark:bg-zinc-800 rounded" />
          ))}
        </div>
      ) : !deletions || deletions.length === 0 ? (
        <div className="py-12 text-center text-zinc-400">
          <Trash2 size={32} className="mx-auto mb-2 opacity-30" />
          <p>No deletion batches staged</p>
          <p className="text-xs mt-1">Select messages in the aggregate or message views to stage deletions</p>
        </div>
      ) : (
        <div className="rounded-lg border border-zinc-200 dark:border-zinc-800 divide-y divide-zinc-100 dark:divide-zinc-800/50">
          {deletions.map((del) => {
            const statusCfg = STATUS_CONFIG[del.status] ?? STATUS_CONFIG['pending']!
            return (
              <div key={del.id} className="px-4 py-3 flex items-center gap-4">
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium truncate">{del.description || del.id}</span>
                    <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs ${statusCfg.color}`}>
                      {statusCfg.icon} {statusCfg.label}
                    </span>
                  </div>
                  <div className="text-xs text-zinc-400 mt-0.5">
                    {formatNumber(del.gmail_ids?.length ?? 0)} messages &middot; Created{' '}
                    {new Date(del.created_at).toLocaleDateString()} by {del.created_by}
                  </div>
                </div>
                {(del.status === 'pending' || del.status === 'in_progress') && (
                  <button
                    onClick={() => cancelMutation.mutate(del.id)}
                    disabled={cancelMutation.isPending}
                    className="p-1.5 rounded text-zinc-400 hover:text-red-600 hover:bg-red-50 dark:hover:bg-red-900/30 transition-colors"
                    title="Cancel"
                  >
                    <X size={16} />
                  </button>
                )}
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
