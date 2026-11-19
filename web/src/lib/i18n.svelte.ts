// Simple and fast reactive i18n store for 9router-go dashboard.
// Supports English ('en') and Bahasa Indonesia ('id').

export type Lang = 'en' | 'id'

const STORAGE_KEY = '9router-lang'

let currentLang = $state<Lang>(
  typeof window !== 'undefined' && window.localStorage.getItem(STORAGE_KEY) === 'id' ? 'id' : 'en'
)

export const translations: Record<Lang, Record<string, string>> = {
  en: {
    'nav.endpoint': 'Endpoint',
    'nav.providers': 'Providers',
    'nav.combos': 'Combos',
    'nav.quota': 'Quota',
    'nav.usage': 'Usage',
    'nav.console': 'Console Log',
    'nav.proxypools': 'Proxy Pools',
    'nav.apikeys': 'API Keys',
    'nav.settings': 'Settings',
    'nav.plans': 'Plans',
    'nav.persona': 'Persona',
    'common.search': 'Search models, providers, pages…',
    'common.active': 'Active',
    'common.disabled': 'Disabled',
    'common.error': 'Error',
    'common.banned': 'Banned',
    'common.copy': 'Copy',
    'common.test': 'Test',
    'common.pause': 'Pause',
    'common.resume': 'Resume',
    'common.clear': 'Clear',
    'common.refresh': 'Refresh',
    'common.export': 'Export',
    'common.wrap': 'Wrap',
    'common.language': 'Language / Bahasa',
  },
  id: {
    'nav.endpoint': 'Titik Akhir',
    'nav.providers': 'Penyedia',
    'nav.combos': 'Kombo',
    'nav.quota': 'Kuota',
    'nav.usage': 'Penggunaan',
    'nav.console': 'Log Konsol',
    'nav.proxypools': 'Kolam Proksi',
    'nav.apikeys': 'Kunci API',
    'nav.settings': 'Pengaturan',
    'nav.plans': 'Paket',
    'nav.persona': 'Persona',
    'common.search': 'Cari model, penyedia, halaman…',
    'common.active': 'Aktif',
    'common.disabled': 'Nonaktif',
    'common.error': 'Kesalahan',
    'common.banned': 'Diblokir',
    'common.copy': 'Salin',
    'common.test': 'Uji',
    'common.pause': 'Jeda',
    'common.resume': 'Lanjutkan',
    'common.clear': 'Bersihkan',
    'common.refresh': 'Segarkan',
    'common.export': 'Ekspor',
    'common.wrap': 'Lipat Baris',
    'common.language': 'Bahasa / Language',
  },
}

export function getLang(): Lang {
  return currentLang
}

export function setLang(lang: Lang): void {
  currentLang = lang
  if (typeof window !== 'undefined') {
    window.localStorage.setItem(STORAGE_KEY, lang)
  }
}

export function toggleLang(): void {
  setLang(currentLang === 'en' ? 'id' : 'en')
}

export function t(key: string, fallback?: string): string {
  return translations[currentLang]?.[key] ?? fallback ?? key
}

export const i18n = {
  get lang() {
    return currentLang
  },
  set lang(l: Lang) {
    setLang(l)
  },
  t,
  setLang,
  toggleLang,
}
