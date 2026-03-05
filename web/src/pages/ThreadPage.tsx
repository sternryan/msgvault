import { useParams, Link } from 'react-router-dom'
import { useThread, useMessage } from '../api/hooks'
import { Breadcrumbs } from '../components/layout/Breadcrumbs'
import { formatDateTime } from '../lib/utils'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useState } from 'react'
import { EmailRenderer } from '../components/messages/EmailRenderer'
import type { MessageSummary } from '../api/types'

export function ThreadPage() {
  const { id } = useParams<{ id: string }>()
  const messageId = id ? Number(id) : undefined

  const { data: messages, isLoading: threadLoading } = useThread(messageId)
  const { data: rootMsg } = useMessage(messageId)

  if (threadLoading) {
    return (
      <div className="animate-pulse space-y-4">
        {[...Array(3)].map((_, i) => (
          <div key={i} className="h-24 bg-zinc-200 dark:bg-zinc-800 rounded" />
        ))}
      </div>
    )
  }

  const crumbs = [
    { label: 'Messages', to: '/messages' },
    { label: rootMsg?.subject || '(thread)', to: `/messages/${id}` },
    { label: 'Thread' },
  ]

  return (
    <div className="space-y-4 max-w-4xl">
      <Breadcrumbs items={crumbs} />

      <h1 className="text-xl font-semibold">
        {rootMsg?.subject || '(no subject)'}
        <span className="text-sm text-zinc-400 ml-2">{messages?.length ?? 0} messages in thread</span>
      </h1>

      <div className="space-y-2">
        {messages?.map((msg, i) => (
          <ThreadMessage
            key={msg.id}
            message={msg}
            defaultExpanded={i === (messages.length - 1)} // Expand newest
          />
        ))}
      </div>
    </div>
  )
}

function ThreadMessage({ message, defaultExpanded }: { message: MessageSummary; defaultExpanded: boolean }) {
  const [expanded, setExpanded] = useState(defaultExpanded)
  const { data: detail } = useMessage(expanded ? message.id : undefined)

  return (
    <div className="rounded-lg border border-zinc-200 dark:border-zinc-800">
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center gap-3 px-4 py-3 text-left hover:bg-zinc-50 dark:hover:bg-zinc-900 transition-colors"
      >
        {expanded ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
        <span className="text-sm font-medium truncate">
          {message.fromName || message.fromEmail}
        </span>
        <span className="text-xs text-zinc-400 shrink-0">{formatDateTime(message.sentAt)}</span>
        {!expanded && (
          <span className="text-xs text-zinc-400 truncate ml-2">{message.snippet}</span>
        )}
        <Link
          to={`/messages/${message.id}`}
          onClick={(e) => e.stopPropagation()}
          className="ml-auto text-xs text-blue-600 dark:text-blue-400 hover:underline shrink-0"
        >
          Open
        </Link>
      </button>

      {expanded && detail && (
        <div className="px-4 pb-4 border-t border-zinc-100 dark:border-zinc-800/50">
          <EmailRenderer
            bodyText={detail.bodyText}
            bodyHtml={detail.bodyHtml}
            attachments={detail.attachments}
          />
        </div>
      )}
    </div>
  )
}
