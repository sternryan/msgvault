import { Link } from 'react-router-dom'
import { useStats, useAggregate } from '../api/hooks'
import { formatBytes, formatNumber } from '../lib/utils'
import { Mail, HardDrive, Paperclip, Users, BarChart3 } from 'lucide-react'
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  CartesianGrid,
} from 'recharts'

export function DashboardPage() {
  const { data: statsData, isLoading: statsLoading } = useStats()
  const { data: timeData } = useAggregate({
    groupBy: 'time',
    timeGranularity: 'month',
    sortField: 'name',
    sortDir: 'asc',
    limit: 120,
  })
  const { data: senderData } = useAggregate({
    groupBy: 'senders',
    sortField: 'count',
    sortDir: 'desc',
    limit: 15,
  })
  const { data: domainData } = useAggregate({
    groupBy: 'domains',
    sortField: 'count',
    sortDir: 'desc',
    limit: 15,
  })

  if (statsLoading) {
    return <DashboardSkeleton />
  }

  const stats = statsData?.stats
  const accounts = statsData?.accounts

  return (
    <div className="space-y-8">
      <h1 className="text-2xl font-semibold">Dashboard</h1>

      {/* Stats cards */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <StatCard
          icon={<Mail size={20} />}
          label="Messages"
          value={formatNumber(stats?.messageCount ?? 0)}
        />
        <StatCard
          icon={<HardDrive size={20} />}
          label="Total Size"
          value={formatBytes(stats?.totalSize ?? 0)}
        />
        <StatCard
          icon={<Paperclip size={20} />}
          label="Attachments"
          value={formatNumber(stats?.attachmentCount ?? 0)}
        />
        <StatCard
          icon={<Users size={20} />}
          label="Accounts"
          value={formatNumber(accounts?.length ?? 0)}
        />
      </div>

      {/* Accounts list */}
      {accounts && accounts.length > 0 && (
        <div className="rounded-lg border border-zinc-200 dark:border-zinc-800 p-4">
          <h2 className="text-sm font-medium text-zinc-500 dark:text-zinc-400 mb-3">Accounts</h2>
          <div className="space-y-2">
            {accounts.map((acc) => (
              <div key={acc.id} className="flex items-center gap-2 text-sm">
                <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200">
                  {acc.sourceType}
                </span>
                <span>{acc.identifier}</span>
                {acc.displayName && acc.displayName !== acc.identifier && (
                  <span className="text-zinc-400">({acc.displayName})</span>
                )}
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Message volume chart */}
      {timeData && timeData.length > 0 && (
        <div className="rounded-lg border border-zinc-200 dark:border-zinc-800 p-4">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-sm font-medium text-zinc-500 dark:text-zinc-400">
              Message Volume (Monthly)
            </h2>
            <Link
              to="/aggregate?groupBy=time&timeGranularity=month"
              className="text-xs text-blue-600 dark:text-blue-400 hover:underline flex items-center gap-1"
            >
              <BarChart3 size={12} /> View all
            </Link>
          </div>
          <ResponsiveContainer width="100%" height={250}>
            <BarChart data={timeData}>
              <CartesianGrid strokeDasharray="3 3" className="stroke-zinc-200 dark:stroke-zinc-700" />
              <XAxis
                dataKey="key"
                tick={{ fontSize: 11 }}
                className="text-zinc-500"
                interval="preserveStartEnd"
              />
              <YAxis tick={{ fontSize: 11 }} className="text-zinc-500" />
              <Tooltip
                contentStyle={{
                  backgroundColor: 'var(--color-zinc-50, #fafafa)',
                  border: '1px solid var(--color-zinc-200, #e4e4e7)',
                  borderRadius: '6px',
                  fontSize: '12px',
                }}
                formatter={(value: number) => [formatNumber(value), 'Messages']}
              />
              <Bar dataKey="count" fill="#3b82f6" radius={[2, 2, 0, 0]} />
            </BarChart>
          </ResponsiveContainer>
        </div>
      )}

      {/* Top senders & domains side by side */}
      <div className="grid md:grid-cols-2 gap-4">
        {senderData && senderData.length > 0 && (
          <div className="rounded-lg border border-zinc-200 dark:border-zinc-800 p-4">
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-sm font-medium text-zinc-500 dark:text-zinc-400">Top Senders</h2>
              <Link
                to="/aggregate?groupBy=senders"
                className="text-xs text-blue-600 dark:text-blue-400 hover:underline"
              >
                View all
              </Link>
            </div>
            <ResponsiveContainer width="100%" height={300}>
              <BarChart data={senderData.slice(0, 10)} layout="vertical">
                <CartesianGrid strokeDasharray="3 3" className="stroke-zinc-200 dark:stroke-zinc-700" />
                <XAxis type="number" tick={{ fontSize: 11 }} />
                <YAxis
                  type="category"
                  dataKey="key"
                  tick={{ fontSize: 11 }}
                  width={150}
                  tickFormatter={(v: string) => (v.length > 22 ? v.slice(0, 20) + '...' : v)}
                />
                <Tooltip
                  formatter={(value: number) => [formatNumber(value), 'Messages']}
                  contentStyle={{ fontSize: '12px', borderRadius: '6px' }}
                />
                <Bar dataKey="count" fill="#8b5cf6" radius={[0, 2, 2, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </div>
        )}

        {domainData && domainData.length > 0 && (
          <div className="rounded-lg border border-zinc-200 dark:border-zinc-800 p-4">
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-sm font-medium text-zinc-500 dark:text-zinc-400">Top Domains</h2>
              <Link
                to="/aggregate?groupBy=domains"
                className="text-xs text-blue-600 dark:text-blue-400 hover:underline"
              >
                View all
              </Link>
            </div>
            <ResponsiveContainer width="100%" height={300}>
              <BarChart data={domainData.slice(0, 10)} layout="vertical">
                <CartesianGrid strokeDasharray="3 3" className="stroke-zinc-200 dark:stroke-zinc-700" />
                <XAxis type="number" tick={{ fontSize: 11 }} />
                <YAxis
                  type="category"
                  dataKey="key"
                  tick={{ fontSize: 11 }}
                  width={150}
                  tickFormatter={(v: string) => (v.length > 22 ? v.slice(0, 20) + '...' : v)}
                />
                <Tooltip
                  formatter={(value: number) => [formatNumber(value), 'Messages']}
                  contentStyle={{ fontSize: '12px', borderRadius: '6px' }}
                />
                <Bar dataKey="count" fill="#06b6d4" radius={[0, 2, 2, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </div>
        )}
      </div>
    </div>
  )
}

function StatCard({ icon, label, value }: { icon: React.ReactNode; label: string; value: string }) {
  return (
    <div className="rounded-lg border border-zinc-200 dark:border-zinc-800 p-4">
      <div className="flex items-center gap-2 text-zinc-500 dark:text-zinc-400 mb-1">
        {icon}
        <span className="text-xs font-medium">{label}</span>
      </div>
      <p className="text-2xl font-semibold">{value}</p>
    </div>
  )
}

function DashboardSkeleton() {
  return (
    <div className="space-y-8 animate-pulse">
      <div className="h-8 w-40 bg-zinc-200 dark:bg-zinc-800 rounded" />
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        {[...Array(4)].map((_, i) => (
          <div key={i} className="h-20 bg-zinc-200 dark:bg-zinc-800 rounded-lg" />
        ))}
      </div>
      <div className="h-64 bg-zinc-200 dark:bg-zinc-800 rounded-lg" />
    </div>
  )
}
