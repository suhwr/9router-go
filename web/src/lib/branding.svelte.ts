// Branding manager for 9router-go white-labeling.
// Dynamically updates document.title, favicon, and apple-touch-icon in the DOM.

export interface BrandConfig {
  appName?: string
  appSubtitle?: string
  appLogoUrl?: string
  faviconUrl?: string
  loginTitle?: string
  loginSubtitle?: string
  loginBgUrl?: string
  loginBgBlur?: boolean
  loginHideDefaultPasswordHint?: boolean
  hideBrandFooter?: boolean
  footerText?: string
}

let activeBrand = $state<BrandConfig>({})

export function setBranding(config: Partial<BrandConfig>): void {
  activeBrand = { ...activeBrand, ...config }
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
