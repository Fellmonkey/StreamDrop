import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MotionConfig } from 'framer-motion'
import { Toaster } from 'sonner'
import './i18n'
import './index.css'
import App from './App'
import { TooltipProvider } from '@/components/ui/primitives'
import { EASE_OUT } from '@/lib/motion'
import { applyAccent, applyStyle, applyTheme, readAccent, readStyle, readTheme } from '@/lib/theme'

applyTheme(readTheme())
applyAccent(readAccent())
applyStyle(readStyle())

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
      staleTime: 2000,
    },
  },
})

const theme = readTheme()

createRoot(document.getElementById('root') as HTMLElement).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <TooltipProvider delayDuration={200}>
        <MotionConfig reducedMotion="user" transition={{ duration: 0.2, ease: EASE_OUT }}>
          <App />
          <Toaster
            theme={theme}
            position="bottom-right"
            toastOptions={{
              style: {
                background: 'var(--card)',
                border: '1px solid var(--border)',
                color: 'var(--card-foreground)',
              },
            }}
          />
        </MotionConfig>
      </TooltipProvider>
    </QueryClientProvider>
  </StrictMode>,
)
