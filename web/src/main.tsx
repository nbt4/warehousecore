import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import './cores-theme.css'
import './i18n' // Initialize i18n
import App from './App'
import { appBasePath, appPath } from './lib/app-paths'
import { initSuiteI18n, pairSuiteTranslations } from './lib/cores-design'
import commonDe from './lib/cores-locales/de.json'
import commonEn from './lib/cores-locales/en.json'
import warehouseDe from './locales/de.json'
import warehouseEn from './locales/en.json'

const commonTranslations = pairSuiteTranslations(commonDe, commonEn)
const warehouseTranslations = pairSuiteTranslations(warehouseDe, warehouseEn)
initSuiteI18n({
  de: { ...commonTranslations.de, ...warehouseTranslations.de },
  en: { ...commonTranslations.en, ...warehouseTranslations.en },
})

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
    void navigator.serviceWorker.register(appPath('/sw.js?v=3'), { scope: `${appBasePath || ''}/`, updateViaCache: 'none' }).catch((error: unknown) => {
      console.error('WarehouseCore service worker registration failed:', error)
    })
  })
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
