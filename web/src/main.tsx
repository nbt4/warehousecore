import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import './cores-theme.css'
import './i18n' // Initialize i18n
import App from './App'

document.addEventListener('wheel', (event) => {
  const target = event.target
  if (target instanceof HTMLInputElement && target.type === 'number' && document.activeElement === target) {
    target.blur()
  }
}, { capture: true, passive: true })

const isStandalone = window.matchMedia('(display-mode: standalone)').matches
  || (navigator as Navigator & { standalone?: boolean }).standalone === true
document.documentElement.classList.toggle('app-standalone', isStandalone)

if (import.meta.env.PROD && 'serviceWorker' in navigator) {
  window.addEventListener('load', () => {
    void navigator.serviceWorker.register('/sw.js?v=3', { scope: '/', updateViaCache: 'none' }).catch((error: unknown) => {
      console.error('WarehouseCore service worker registration failed:', error)
    })
  })
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
