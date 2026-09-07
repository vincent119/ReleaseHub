import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router'

import { App } from '@/app/App'
import { AppProviders } from '@/app/providers/AppProviders'
import '@/shared/styles/global.css'

const rootElement = document.getElementById('root')

if (!rootElement) {
  throw new Error('ReleaseHub root element was not found')
}

createRoot(rootElement).render(
  <StrictMode>
    <AppProviders>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </AppProviders>
  </StrictMode>,
)
