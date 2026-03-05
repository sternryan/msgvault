import { Routes, Route } from 'react-router-dom'
import { Layout } from './components/layout/Layout'
import { DashboardPage } from './pages/DashboardPage'
import { AggregatePage } from './pages/AggregatePage'
import { MessagesPage } from './pages/MessagesPage'
import { MessagePage } from './pages/MessagePage'
import { ThreadPage } from './pages/ThreadPage'
import { SearchPage } from './pages/SearchPage'
import { DeletionsPage } from './pages/DeletionsPage'

export default function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route path="/" element={<DashboardPage />} />
        <Route path="/aggregate" element={<AggregatePage />} />
        <Route path="/messages" element={<MessagesPage />} />
        <Route path="/messages/:id" element={<MessagePage />} />
        <Route path="/thread/:id" element={<ThreadPage />} />
        <Route path="/search" element={<SearchPage />} />
        <Route path="/deletions" element={<DeletionsPage />} />
      </Route>
    </Routes>
  )
}
