import { useState, useRef, useEffect } from 'react'
import DOMPurify from 'dompurify'
import { api } from '../../api/client'
import type { AttachmentInfo } from '../../api/types'

interface EmailRendererProps {
  bodyText: string
  bodyHtml: string
  attachments?: AttachmentInfo[]
}

export function EmailRenderer({ bodyText, bodyHtml, attachments = [] }: EmailRendererProps) {
  const [mode, setMode] = useState<'text' | 'html'>(bodyHtml ? 'html' : 'text')
  const [loadExternalImages, setLoadExternalImages] = useState(false)
  const iframeRef = useRef<HTMLIFrameElement>(null)

  const hasHtml = !!bodyHtml

  // Process HTML content
  useEffect(() => {
    if (mode !== 'html' || !bodyHtml || !iframeRef.current) return

    let html = bodyHtml

    // Replace cid: references with inline attachment URLs
    if (attachments.length > 0) {
      html = html.replace(/cid:([^"'\s>]+)/gi, (_match, cid: string) => {
        const att = attachments.find(
          (a) => a.filename === cid || a.filename === `${cid}`,
        )
        if (att) return api.attachmentInlineUrl(att.id)
        return `cid:${cid}`
      })
    }

    // Sanitize HTML
    const clean = DOMPurify.sanitize(html, {
      ALLOWED_TAGS: [
        'a', 'b', 'blockquote', 'br', 'caption', 'center', 'code', 'col', 'colgroup',
        'dd', 'div', 'dl', 'dt', 'em', 'font', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6',
        'hr', 'i', 'img', 'li', 'ol', 'p', 'pre', 'small', 'span', 'strike', 'strong',
        'sub', 'sup', 'table', 'tbody', 'td', 'tfoot', 'th', 'thead', 'tr', 'u', 'ul',
        'style',
      ],
      ALLOWED_ATTR: [
        'align', 'alt', 'bgcolor', 'border', 'cellpadding', 'cellspacing', 'class',
        'color', 'colspan', 'dir', 'face', 'height', 'href', 'hspace', 'id', 'lang',
        'rowspan', 'size', 'src', 'style', 'target', 'title', 'type', 'valign',
        'vspace', 'width',
      ],
      ADD_ATTR: ['target'],
    })

    // Build complete HTML document for iframe
    const imgPolicy = loadExternalImages ? '' : 'img[src^="http"] { display: none !important; }'
    const doc = `
      <!DOCTYPE html>
      <html>
      <head>
        <meta charset="utf-8">
        <style>
          body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            font-size: 14px;
            line-height: 1.5;
            color: #333;
            margin: 0;
            padding: 16px;
            word-wrap: break-word;
            overflow-wrap: break-word;
          }
          a { color: #2563eb; }
          img { max-width: 100%; height: auto; }
          blockquote { border-left: 3px solid #ddd; margin: 8px 0; padding-left: 12px; color: #666; }
          pre { overflow-x: auto; background: #f5f5f5; padding: 8px; border-radius: 4px; }
          ${imgPolicy}
        </style>
      </head>
      <body>${clean}</body>
      </html>
    `

    const iframe = iframeRef.current
    const iframeDoc = iframe.contentDocument || iframe.contentWindow?.document
    if (iframeDoc) {
      iframeDoc.open()
      iframeDoc.write(doc)
      iframeDoc.close()

      // Auto-resize iframe to content height
      const resizeObserver = new ResizeObserver(() => {
        if (iframeDoc.body) {
          iframe.style.height = `${iframeDoc.body.scrollHeight + 32}px`
        }
      })
      if (iframeDoc.body) {
        resizeObserver.observe(iframeDoc.body)
      }

      // Make links open in new tab
      iframeDoc.addEventListener('click', (e) => {
        const a = (e.target as HTMLElement).closest('a')
        if (a && a.href) {
          e.preventDefault()
          window.open(a.href, '_blank', 'noopener,noreferrer')
        }
      })

      return () => resizeObserver.disconnect()
    }
  }, [mode, bodyHtml, attachments, loadExternalImages])

  return (
    <div className="rounded-lg border border-zinc-200 dark:border-zinc-800">
      {/* Mode toggle */}
      {hasHtml && (
        <div className="flex items-center gap-2 px-3 py-2 border-b border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-900">
          <button
            onClick={() => setMode('text')}
            className={`px-2 py-0.5 rounded text-xs ${
              mode === 'text'
                ? 'bg-zinc-200 dark:bg-zinc-700 text-zinc-900 dark:text-zinc-100'
                : 'text-zinc-500 hover:text-zinc-900 dark:hover:text-zinc-100'
            }`}
          >
            Text
          </button>
          <button
            onClick={() => setMode('html')}
            className={`px-2 py-0.5 rounded text-xs ${
              mode === 'html'
                ? 'bg-zinc-200 dark:bg-zinc-700 text-zinc-900 dark:text-zinc-100'
                : 'text-zinc-500 hover:text-zinc-900 dark:hover:text-zinc-100'
            }`}
          >
            HTML
          </button>
          {mode === 'html' && !loadExternalImages && (
            <button
              onClick={() => setLoadExternalImages(true)}
              className="ml-auto text-xs text-blue-600 dark:text-blue-400 hover:underline"
            >
              Load external images
            </button>
          )}
        </div>
      )}

      {/* Content */}
      {mode === 'text' ? (
        <pre className="p-4 text-sm whitespace-pre-wrap font-mono overflow-x-auto">
          {bodyText || '(no text content)'}
        </pre>
      ) : (
        <iframe
          ref={iframeRef}
          sandbox="allow-same-origin"
          title="Email content"
          className="w-full border-0 min-h-[200px]"
          style={{ height: '400px' }}
        />
      )}
    </div>
  )
}
