import { useParams, Link } from 'react-router-dom'
import { useMessage } from '../api/hooks'
import { api } from '../api/client'
import { Breadcrumbs } from '../components/layout/Breadcrumbs'
import { EmailRenderer } from '../components/messages/EmailRenderer'
import { formatBytes, formatDateTime } from '../lib/utils'
import { Download, MessageSquare, Paperclip } from 'lucide-react'

export function MessagePage() {
  const { id } = useParams<{ id: string }>()
  const messageId = id ? Number(id) : undefined
  const { data: msg, isLoading, error } = useMessage(messageId)

  if (isLoading) {
    return (
      <div className="animate-pulse space-y-4">
        <div className="h-6 w-64 bg-zinc-200 dark:bg-zinc-800 rounded" />
        <div className="h-40 bg-zinc-200 dark:bg-zinc-800 rounded" />
        <div className="h-96 bg-zinc-200 dark:bg-zinc-800 rounded" />
      </div>
    )
  }

  if (error || !msg) {
    return (
      <div className="py-12 text-center">
        <p className="text-zinc-400">Message not found</p>
        <Link to="/messages" className="text-blue-600 dark:text-blue-400 text-sm hover:underline mt-2 inline-block">
          Back to messages
        </Link>
      </div>
    )
  }

  const crumbs = [
    { label: 'Messages', to: '/messages' },
    { label: msg.subject || '(no subject)' },
  ]

  return (
    <div className="space-y-6 max-w-4xl">
      <Breadcrumbs items={crumbs} />

      {/* Header */}
      <div className="space-y-3">
        <h1 className="text-xl font-semibold">{msg.subject || '(no subject)'}</h1>

        <div className="text-sm space-y-1">
          <div className="flex gap-2">
            <span className="text-zinc-500 w-12">From:</span>
            <AddressList addresses={msg.from} />
          </div>
          {msg.to.length > 0 && (
            <div className="flex gap-2">
              <span className="text-zinc-500 w-12">To:</span>
              <AddressList addresses={msg.to} />
            </div>
          )}
          {msg.cc.length > 0 && (
            <div className="flex gap-2">
              <span className="text-zinc-500 w-12">Cc:</span>
              <AddressList addresses={msg.cc} />
            </div>
          )}
          {msg.bcc.length > 0 && (
            <div className="flex gap-2">
              <span className="text-zinc-500 w-12">Bcc:</span>
              <AddressList addresses={msg.bcc} />
            </div>
          )}
        </div>

        <div className="flex items-center gap-4 text-xs text-zinc-500">
          <span>{formatDateTime(msg.sentAt)}</span>
          <span>{formatBytes(msg.sizeEstimate)}</span>
          {msg.labels && msg.labels.length > 0 && (
            <div className="flex gap-1">
              {msg.labels.map((l) => (
                <span
                  key={l}
                  className="px-1.5 py-0.5 rounded bg-zinc-100 dark:bg-zinc-800 text-zinc-500"
                >
                  {l}
                </span>
              ))}
            </div>
          )}
          <Link
            to={`/thread/${msg.id}`}
            className="text-blue-600 dark:text-blue-400 hover:underline flex items-center gap-1"
          >
            <MessageSquare size={12} /> View thread
          </Link>
        </div>
      </div>

      {/* Attachments */}
      {msg.attachments && msg.attachments.length > 0 && (
        <div className="rounded-lg border border-zinc-200 dark:border-zinc-800 p-3">
          <h3 className="text-xs font-medium text-zinc-500 mb-2 flex items-center gap-1">
            <Paperclip size={12} /> {msg.attachments.length} attachment{msg.attachments.length > 1 ? 's' : ''}
          </h3>
          <div className="flex flex-wrap gap-2">
            {msg.attachments.map((att) => (
              <a
                key={att.id}
                href={api.attachmentDownloadUrl(att.id)}
                className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm bg-zinc-50 dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-700 hover:bg-zinc-100 dark:hover:bg-zinc-800 transition-colors"
              >
                <Download size={14} />
                <span className="truncate max-w-48">{att.filename}</span>
                <span className="text-zinc-400 text-xs">({formatBytes(att.size)})</span>
              </a>
            ))}
          </div>
        </div>
      )}

      {/* Email body */}
      <EmailRenderer
        bodyText={msg.bodyText}
        bodyHtml={msg.bodyHtml}
        attachments={msg.attachments}
      />
    </div>
  )
}

function AddressList({ addresses }: { addresses: { email: string; name: string }[] }) {
  return (
    <div className="flex flex-wrap gap-1">
      {addresses.map((addr, i) => (
        <Link
          key={i}
          to={`/messages?sender=${encodeURIComponent(addr.email)}`}
          className="hover:underline"
        >
          {addr.name ? (
            <span>
              {addr.name} <span className="text-zinc-400">&lt;{addr.email}&gt;</span>
            </span>
          ) : (
            addr.email
          )}
          {i < addresses.length - 1 && <span className="text-zinc-300">,</span>}
        </Link>
      ))}
    </div>
  )
}
