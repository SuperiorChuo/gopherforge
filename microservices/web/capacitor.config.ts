import type { CapacitorConfig } from '@capacitor/cli'

// Devices must set CAP_SERVER_URL to a reachable deployment before packaging.
const serverUrl = process.env.CAP_SERVER_URL?.trim() || 'http://localhost:8000'

const config: CapacitorConfig = {
  appId: 'kit.goadmin.console',
  appName: 'GAK 控制台',
  webDir: 'dist',
  server: {
    url: `${serverUrl.replace(/\/$/, '')}/m`,
    cleartext: serverUrl.startsWith('http://'),
    androidScheme: 'https',
    iosScheme: 'https',
    allowNavigation: [
      new URL(serverUrl).hostname,
      'localhost',
      '127.0.0.1',
    ],
  },
  ios: {
    contentInset: 'automatic',
    preferredContentMode: 'mobile',
    backgroundColor: '#0a0b14',
  },
  android: {
    allowMixedContent: true,
    backgroundColor: '#0a0b14',
  },
}

export default config
