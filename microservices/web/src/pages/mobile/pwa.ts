type BeforeInstallPromptEvent = Event & {
  prompt: () => Promise<void>
  userChoice: Promise<{ outcome: 'accepted' | 'dismissed' }>
}

let deferredPrompt: BeforeInstallPromptEvent | null = null
let listening = false

export function isStandaloneDisplay(): boolean {
  return window.matchMedia('(display-mode: standalone)').matches
    || (window.navigator as Navigator & { standalone?: boolean }).standalone === true
}

export function isAppleMobile(): boolean {
  return /iphone|ipad|ipod/i.test(navigator.userAgent)
}

export function registerMobileServiceWorker() {
  if (!import.meta.env.PROD || !('serviceWorker' in navigator)) return
  if (document.documentElement.dataset.nativeApp === '1') return
  void navigator.serviceWorker.register('/sw.js', { scope: '/' }).catch(() => undefined)
}

export function listenForInstallPrompt() {
  if (listening) return
  listening = true
  window.addEventListener('beforeinstallprompt', (event) => {
    event.preventDefault()
    deferredPrompt = event as BeforeInstallPromptEvent
    window.dispatchEvent(new Event('gak-pwa-ready'))
  })
  window.addEventListener('appinstalled', () => {
    deferredPrompt = null
    window.dispatchEvent(new Event('gak-pwa-installed'))
  })
}

export function canPromptInstall() {
  return !!deferredPrompt
}

export async function promptInstall(): Promise<'accepted' | 'dismissed' | 'unavailable'> {
  if (!deferredPrompt) return 'unavailable'
  const prompt = deferredPrompt
  deferredPrompt = null
  await prompt.prompt()
  const choice = await prompt.userChoice
  return choice.outcome
}
