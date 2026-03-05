// Types mirroring Go query models

export interface AggregateRow {
  key: string
  count: number
  totalSize: number
  attachmentSize: number
  attachmentCount: number
  totalUnique: number
}

export interface MessageSummary {
  id: number
  sourceMessageId: string
  conversationId: number
  subject: string
  snippet: string
  fromEmail: string
  fromName: string
  sentAt: string
  sizeEstimate: number
  hasAttachments: boolean
  attachmentCount: number
  labels: string[]
  deletedAt?: string
}

export interface MessageDetail {
  id: number
  sourceMessageId: string
  conversationId: number
  subject: string
  snippet: string
  sentAt: string
  receivedAt?: string
  sizeEstimate: number
  hasAttachments: boolean
  from: Address[]
  to: Address[]
  cc: Address[]
  bcc: Address[]
  bodyText: string
  bodyHtml: string
  labels: string[]
  attachments: AttachmentInfo[]
}

export interface Address {
  email: string
  name: string
}

export interface AttachmentInfo {
  id: number
  filename: string
  mimeType: string
  size: number
}

export interface AccountInfo {
  id: number
  sourceType: string
  identifier: string
  displayName: string
}

export interface TotalStats {
  messageCount: number
  totalSize: number
  attachmentCount: number
  attachmentSize: number
  labelCount: number
  accountCount: number
}

export interface StatsResponse {
  stats: TotalStats
  accounts: AccountInfo[]
}

export interface DeletionManifest {
  id: string
  created_at: string
  created_by: string
  description: string
  status: string
  gmail_ids: string[]
  filters: Record<string, unknown>
  summary?: {
    message_count: number
    total_size_bytes: number
    date_range: [string, string]
  }
}

// API response envelope
export interface ApiResponse<T> {
  data: T
  meta?: Record<string, unknown>
  error?: string
}

// View types for aggregate queries
export type ViewType =
  | 'senders'
  | 'senderNames'
  | 'recipients'
  | 'recipientNames'
  | 'domains'
  | 'labels'
  | 'time'

export type SortField = 'count' | 'size' | 'attachmentSize' | 'name'
export type SortDirection = 'asc' | 'desc'
export type TimeGranularity = 'year' | 'month' | 'day'
export type MessageSortField = 'date' | 'size' | 'subject'

export interface AggregateParams {
  groupBy: ViewType
  sortField?: SortField
  sortDir?: SortDirection
  limit?: number
  timeGranularity?: TimeGranularity
  sourceId?: number
  attachmentsOnly?: boolean
  search?: string
  after?: string
  before?: string
}

export interface MessageFilterParams {
  sender?: string
  senderName?: string
  recipient?: string
  recipientName?: string
  domain?: string
  label?: string
  sourceId?: number
  timePeriod?: string
  timeGranularity?: TimeGranularity
  attachmentsOnly?: boolean
  after?: string
  before?: string
  conversationId?: number
  limit?: number
  offset?: number
  sortField?: MessageSortField
  sortDir?: SortDirection
  emptyTarget?: string[]
}

export interface SearchParams {
  q: string
  mode?: 'fast' | 'deep'
  limit?: number
  offset?: number
  // Plus message filter params for contextual search
  sender?: string
  recipient?: string
  domain?: string
  label?: string
}
