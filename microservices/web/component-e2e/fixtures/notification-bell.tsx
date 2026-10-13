import { createRoot } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import '@/i18n/init'
import NotificationBell from '@/components/common/NotificationBell'

createRoot(document.getElementById('root')!).render(
  <MemoryRouter><NotificationBell /></MemoryRouter>,
)
