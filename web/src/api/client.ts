import type { ApiResponse } from './types'

const BASE_URL = '/api/v1'

class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

async function fetchApi<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, init)
  const json = (await res.json()) as ApiResponse<T>

  if (!res.ok || json.error) {
    throw new ApiError(res.status, json.error ?? `HTTP ${res.status}`)
  }

  return json.data
}

function buildQuery(params: object): string {
  const sp = new URLSearchParams()
  for (const [k, v] of Object.entries(params as Record<string, unknown>)) {
    if (v === undefined || v === null || v === '') continue
    if (Array.isArray(v)) {
      for (const item of v) {
        sp.append(k, String(item))
      }
    } else {
      sp.set(k, String(v))
    }
  }
  const qs = sp.toString()
  return qs ? `?${qs}` : ''
}

export const api = {
  getStats: () => fetchApi<{ stats: import('./types').TotalStats; accounts: import('./types').AccountInfo[] }>('/stats'),

  listAccounts: () => fetchApi<import('./types').AccountInfo[]>('/accounts'),

  aggregate: (params: import('./types').AggregateParams) =>
    fetchApi<import('./types').AggregateRow[]>(`/aggregate${buildQuery(params)}`),

  subAggregate: (params: import('./types').AggregateParams & Omit<import('./types').MessageFilterParams, 'sortField' | 'sortDir'>) =>
    fetchApi<import('./types').AggregateRow[]>(`/sub-aggregate${buildQuery(params)}`),

  listMessages: (params: import('./types').MessageFilterParams) =>
    fetchApi<import('./types').MessageSummary[]>(`/messages${buildQuery(params)}`),

  getMessage: (id: number) =>
    fetchApi<import('./types').MessageDetail>(`/messages/${id}`),

  getThread: (id: number) =>
    fetchApi<import('./types').MessageSummary[]>(`/messages/${id}/thread`),

  search: (params: import('./types').SearchParams) =>
    fetchApi<import('./types').MessageSummary[]>(`/search${buildQuery(params)}`),

  searchCount: (params: { q: string } & Partial<import('./types').MessageFilterParams>) =>
    fetchApi<{ count: number }>(`/search/count${buildQuery(params)}`),

  attachmentDownloadUrl: (id: number) => `${BASE_URL}/attachments/${id}/download`,
  attachmentInlineUrl: (id: number) => `${BASE_URL}/attachments/${id}/inline`,

  stageDeletion: (body: Record<string, unknown>) =>
    fetchApi<{ gmailIds: string[]; messageCount: number }>('/deletions/stage', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }),

  confirmDeletion: (body: { description: string; gmailIds: string[]; filters: Record<string, unknown> }) =>
    fetchApi<import('./types').DeletionManifest>('/deletions/confirm', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }),

  listDeletions: () => fetchApi<import('./types').DeletionManifest[]>('/deletions'),

  cancelDeletion: (id: string) =>
    fetchApi<{ status: string }>(`/deletions/${id}`, { method: 'DELETE' }),
}
