import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AppProvider } from './context/AppContext'
import App from './App'
import { refreshSession, keepSessionAlive } from './lib/api'
import './index.css'

const qc = new QueryClient({ defaultOptions: { queries: { retry: 1, refetchOnWindowFocus: false } } })

keepSessionAlive()
refreshSession().finally(() => ReactDOM.createRoot(document.getElementById('root')).render(
  <React.StrictMode>
    <BrowserRouter>
      <QueryClientProvider client={qc}>
        <AppProvider><App /></AppProvider>
      </QueryClientProvider>
    </BrowserRouter>
  </React.StrictMode>
))
