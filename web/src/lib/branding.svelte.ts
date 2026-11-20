// Branding manager for 9router-go white-labeling.
// Dynamically updates document.title, favicon, apple-touch-icon, meta tags (OpenGraph, Twitter card, description), and accent color CSS variables in the DOM.

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

declare global {
  interface Window {
    __INITIAL_BRANDING__?: Partial<BrandConfig>
  }
}

const ACCENT_COLOR_KEY = 'app_accent_color'

function initStoredAccent(): string {
  if (typeof localStorage === 'undefined') return ''
  return localStorage.getItem(ACCENT_COLOR_KEY) || ''
}

function initInitialBrand(): BrandConfig {
  const initial = (typeof window !== 'undefined' && window.__INITIAL_BRANDING__) || {}
  const storedAccent = initStoredAccent()
  return {
    ...initial,
    accentColor: initial.accentColor || storedAccent,
  }
}

let activeBrand = $state<BrandConfig>(initInitialBrand())

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

function setOrCreateMeta(attr: 'name' | 'property', attrValue: string, content: string | undefined): void {
  if (typeof document === 'undefined' || content === undefined) return
  let el = document.querySelector<HTMLMetaElement>(`meta[${attr}="${attrValue}"]`)
  if (!el) {
    el = document.createElement('meta')
    el.setAttribute(attr, attrValue)
    document.head.appendChild(el)
  }
  el.content = content
}

export function applyDOMBranding(): void {
  if (typeof document === 'undefined') return

  const brandName = activeBrand.appName?.trim() || '9router-go'
  const subtitle = activeBrand.appSubtitle?.trim()
  const pageDescription = subtitle || 'High-Throughput AI Gateway - Unified LLM Proxy & Router'
  const fullTitle = subtitle
    ? `${brandName} - ${subtitle}`
    : brandName !== '9router-go'
      ? `${brandName} - High-Throughput AI Gateway`
      : '9router-go - High-Throughput AI Gateway'

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

  // 2. Mobile App title & Description
  setOrCreateMeta('name', 'apple-mobile-web-app-title', brandName)
  setOrCreateMeta('name', 'description', pageDescription)

  // 3. OpenGraph tags
  setOrCreateMeta('property', 'og:type', 'website')
  setOrCreateMeta('property', 'og:site_name', brandName)
  setOrCreateMeta('property', 'og:title', fullTitle)
  setOrCreateMeta('property', 'og:description', pageDescription)

  // 4. Social Cover Image (loginBgUrl or appLogoUrl)
  const coverImage = activeBrand.loginBgUrl?.trim() || activeBrand.appLogoUrl?.trim()
  if (coverImage) {
    const fullCoverUrl =
      typeof window !== 'undefined' && coverImage.startsWith('/')
        ? `${window.location.origin}${coverImage}`
        : coverImage
    setOrCreateMeta('property', 'og:image', fullCoverUrl)
    setOrCreateMeta('name', 'twitter:image', fullCoverUrl)
    setOrCreateMeta('name', 'twitter:card', 'summary_large_image')
  } else {
    setOrCreateMeta('name', 'twitter:card', 'summary')
  }

  // 5. Twitter Card tags
  setOrCreateMeta('name', 'twitter:title', fullTitle)
  setOrCreateMeta('name', 'twitter:description', pageDescription)

  // 6. Update Document Title if it is default or empty
  if (!document.title || document.title.includes('9router-go')) {
    document.title = fullTitle
  }

  // 7. Dynamic Accent Color CSS Variables & Theme Color
  const color = activeBrand.accentColor?.trim()
  if (color && /^#[0-9a-fA-F]{3,8}$/.test(color)) {
    setOrCreateMeta('name', 'theme-color', color)
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
    setOrCreateMeta('name', 'theme-color', '#0b0e13')
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
