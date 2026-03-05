import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from './client'
import type { AggregateParams, MessageFilterParams, SearchParams } from './types'

// --- Stats ---

export function useStats() {
  return useQuery({
    queryKey: ['stats'],
    queryFn: () => api.getStats(),
  })
}

// --- Accounts ---

export function useAccounts() {
  return useQuery({
    queryKey: ['accounts'],
    queryFn: () => api.listAccounts(),
  })
}

// --- Aggregation ---

export function useAggregate(params: AggregateParams) {
  return useQuery({
    queryKey: ['aggregate', params],
    queryFn: () => api.aggregate(params),
    enabled: !!params.groupBy,
  })
}

export function useSubAggregate(params: AggregateParams & Omit<MessageFilterParams, 'sortField' | 'sortDir'>) {
  return useQuery({
    queryKey: ['subAggregate', params],
    queryFn: () => api.subAggregate(params),
    enabled: !!params.groupBy,
  })
}

// --- Messages ---

export function useMessages(params: MessageFilterParams) {
  return useQuery({
    queryKey: ['messages', params],
    queryFn: () => api.listMessages(params),
  })
}

export function useMessage(id: number | undefined) {
  return useQuery({
    queryKey: ['message', id],
    queryFn: () => api.getMessage(id!),
    enabled: id !== undefined,
  })
}

export function useThread(id: number | undefined) {
  return useQuery({
    queryKey: ['thread', id],
    queryFn: () => api.getThread(id!),
    enabled: id !== undefined,
  })
}

// --- Search ---

export function useSearch(params: SearchParams) {
  return useQuery({
    queryKey: ['search', params],
    queryFn: () => api.search(params),
    enabled: !!params.q,
  })
}

export function useSearchCount(params: { q: string } & Partial<MessageFilterParams>) {
  return useQuery({
    queryKey: ['searchCount', params],
    queryFn: () => api.searchCount(params),
    enabled: !!params.q,
  })
}

// --- Deletions ---

export function useDeletions() {
  return useQuery({
    queryKey: ['deletions'],
    queryFn: () => api.listDeletions(),
  })
}

export function useStageDeletion() {
  return useMutation({
    mutationFn: (body: Record<string, unknown>) => api.stageDeletion(body),
  })
}

export function useConfirmDeletion() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: { description: string; gmailIds: string[]; filters: Record<string, unknown> }) =>
      api.confirmDeletion(body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['deletions'] })
    },
  })
}

export function useCancelDeletion() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.cancelDeletion(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['deletions'] })
    },
  })
}
