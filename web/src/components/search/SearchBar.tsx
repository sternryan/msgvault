import { useState, useCallback } from 'react'
import { Search } from 'lucide-react'

interface SearchBarProps {
  onSearch: (query: string) => void
  initialValue?: string
  placeholder?: string
}

export function SearchBar({ onSearch, initialValue = '', placeholder = 'Search emails... (/)' }: SearchBarProps) {
  const [value, setValue] = useState(initialValue)

  const handleSubmit = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault()
      if (value.trim()) {
        onSearch(value.trim())
      }
    },
    [value, onSearch],
  )

  return (
    <form onSubmit={handleSubmit} className="relative w-full">
      <Search size={16} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-zinc-400" />
      <input
        data-search-input
        type="text"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        placeholder={placeholder}
        className="w-full pl-8 pr-3 py-1.5 text-sm rounded-md border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-900 placeholder:text-zinc-400 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
      />
    </form>
  )
}
