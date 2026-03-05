import { Link } from 'react-router-dom'
import { ChevronRight } from 'lucide-react'

export interface Crumb {
  label: string
  to?: string
}

export function Breadcrumbs({ items }: { items: Crumb[] }) {
  if (items.length === 0) return null

  return (
    <nav className="flex items-center gap-1 text-sm text-zinc-500 dark:text-zinc-400 mb-4">
      <Link to="/" className="hover:text-zinc-900 dark:hover:text-zinc-100">
        Home
      </Link>
      {items.map((item, i) => (
        <span key={i} className="flex items-center gap-1">
          <ChevronRight size={14} />
          {item.to ? (
            <Link to={item.to} className="hover:text-zinc-900 dark:hover:text-zinc-100">
              {item.label}
            </Link>
          ) : (
            <span className="text-zinc-900 dark:text-zinc-100">{item.label}</span>
          )}
        </span>
      ))}
    </nav>
  )
}
