// Branding manager for 9router-go white-labeling.
// Dynamically updates document.title, favicon, apple-touch-icon, and accent color CSS variables in the DOM.

export interface BrandConfig {
  appName?: string
  appSubtitle?: string
  appLogoUrl?: string
  faviconUrl?: string
  accentColor?: string
  loginTitle?: string
  loginSubtitle?: string
  loginBgUrl?: string
  loginBgBlur?: boolean
  loginCardStyle?: string
  loginHideDefaultPasswordHint?: boolean
  hideBrandFooter?: boolean
  footerText?: string
}

const ACCENT_COLOR_KEY = 'app_accent_color'

function initStoredAccent(): string {
  if (typeof localStorage === 'undefined') return ''
  return localStorage.getItem(ACCENT_COLOR_KEY) || ''
}

let activeBrand = $state<BrandConfig>({
  accentColor: initStoredAccent(),
})

// Quick brightness shift for hover and tint states
function adjustHexBrightness(hex: string, percent: number): string {
  const clean = hex.replace('#', '')
  if (clean.length !== 3 && clean.length !== 6) return hex
  const full = clean.length === 3 ? clean.split('').map((c) => c + c).join('') : clean
  const num = parseInt(full, 16)
  const amt = Math.round(2.55 * percent)
  const R = Math.min(255, Math.max(0, (num >> 16) + amt))
  const G = Math.min(255, Math.max(0, ((num >> 8) & 0x00ff) + amt))
  const B = Math.min(255, Math.max(0, (num & 0x0000ff) + amt))
  return `#${((1 << 24) + (R << 16) + (G << 8) + B).toString(16).slice(1)}`
}

export function setBranding(config: Partial<BrandConfig>): void {
  activeBrand = { ...activeBrand, ...config }
  if (config.accentColor !== undefined && typeof localStorage !== 'undefined') {
    if (config.accentColor) {
      localStorage.setItem(ACCENT_COLOR_KEY, config.accentColor)
    } else {
      localStorage.removeItem(ACCENT_COLOR_KEY)
    }
  }
  applyDOMBranding()
}

export function getBranding(): BrandConfig {
  return activeBrand
}

export function applyDOMBranding(): void {
  if (typeof document === 'undefined') return

  const brandName = activeBrand.appName?.trim() || '9router-go'

  // 1. Dynamic Favicon & Apple Touch Icon
  const iconUrl = activeBrand.faviconUrl?.trim() || activeBrand.appLogoUrl?.trim() || '/favicon.svg'

  const links = document.querySelectorAll<HTMLLinkElement>('link[rel="icon"], link[rel="shortcut icon"]')
  links.forEach((link) => {
    link.href = iconUrl
    if (iconUrl.endsWith('.ico')) {
      link.type = 'image/x-icon'
    } else if (iconUrl.endsWith('.png')) {
      link.type = 'image/png'
    } else if (iconUrl.endsWith('.svg')) {
      link.type = 'image/svg+xml'
    }
  })

  const appleTouch = document.querySelector<HTMLLinkElement>('link[rel="apple-touch-icon"]')
  if (appleTouch) {
    appleTouch.href = iconUrl
  }

  // 2. Mobile App title
  const appTitleMeta = document.querySelector<HTMLMetaElement>('meta[name="apple-mobile-web-app-title"]')
  if (appTitleMeta) {
    appTitleMeta.content = brandName
  }

  // 3. Dynamic Accent Color CSS Variables
  const color = activeBrand.accentColor?.trim()
  if (color && /^#[0-9a-fA-F]{3,8}$/.test(color)) {
    const hoverColor = adjustHexBrightness(color, -15)
    const activeColor = adjustHexBrightness(color, -25)
    const tint50 = adjustHexBrightness(color, 85)
    const tint100 = adjustHexBrightness(color, 75)

    document.documentElement.style.setProperty('--app-primary', color)
    document.documentElement.style.setProperty('--app-primary-hover', hoverColor)
    document.documentElement.style.setProperty('--color-primary', color)
    document.documentElement.style.setProperty('--color-primary-hover', hoverColor)
    document.documentElement.style.setProperty('--color-brand-500', color)
    document.documentElement.style.setProperty('--color-brand-600', hoverColor)
    document.documentElement.style.setProperty('--color-brand-700', activeColor)
    document.documentElement.style.setProperty('--color-brand-50', tint50)
    document.documentElement.style.setProperty('--color-brand-100', tint100)
    document.documentElement.style.setProperty('--shadow-warm', `0 2px 12px -2px ${color}40`)
  } else {
    document.documentElement.style.removeProperty('--app-primary')
    document.documentElement.style.removeProperty('--app-primary-hover')
    document.documentElement.style.removeProperty('--color-primary')
    document.documentElement.style.removeProperty('--color-primary-hover')
    document.documentElement.style.removeProperty('--color-brand-500')
    document.documentElement.style.removeProperty('--color-brand-600')
    document.documentElement.style.removeProperty('--color-brand-700')
    document.documentElement.style.removeProperty('--color-brand-50')
    document.documentElement.style.removeProperty('--color-brand-100')
    document.documentElement.style.removeProperty('--shadow-warm')
  }
}

// Immediately apply stored accent on module initialization in browser
if (typeof document !== 'undefined') {
  applyDOMBranding()
}

export function formatPageTitle(pageTitle?: string): string {
  const brandName = activeBrand.appName?.trim() || '9router-go'
  if (!pageTitle) return brandName
  return `${pageTitle} · ${brandName}`
}

export const branding = {
  get config() {
    return activeBrand
  },
  setBranding,
  getBranding,
  formatPageTitle,
  applyDOMBranding,
}
