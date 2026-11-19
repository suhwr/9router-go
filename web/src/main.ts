// The Material Symbols font is declared in `src/index.css` and shipped from
// `src/assets`, not imported here. The npm package's stylesheet points at its
// own 3.9 MB variable font with every axis live; the vendored copy is the
// same font instanced down to the axes the UI pins (349 KB) and still carries
// every glyph, so no icon name can render as tofu.
import { mount } from 'svelte'
import App from './App.svelte'
import './index.css'
import { registerServiceWorker } from './lib/pwa'

registerServiceWorker()

// Protect icon font ligatures from browser translation (e.g. Google Translate).
// When browser translates text, it must not translate the icon name inside
// .material-symbols-outlined (e.g. "refresh" -> "Segarkan"), which breaks icon rendering.
function protectIcons(node: Node) {
  if (node instanceof HTMLElement) {
    if (node.classList.contains('material-symbols-outlined')) {
      node.setAttribute('translate', 'no')
      node.classList.add('notranslate')
    }
    const icons = node.querySelectorAll('.material-symbols-outlined')
    for (let i = 0; i < icons.length; i++) {
      icons[i].setAttribute('translate', 'no')
      icons[i].classList.add('notranslate')
    }
  }
}
if (typeof document !== 'undefined') {
  protectIcons(document.documentElement)
  const iconObserver = new MutationObserver((mutations) => {
    for (const m of mutations) {
      for (let i = 0; i < m.addedNodes.length; i++) {
        protectIcons(m.addedNodes[i])
      }
    }
  })
  iconObserver.observe(document.documentElement, { childList: true, subtree: true })
}

const app = mount(App, {
  target: document.getElementById('app')!,
})
export default app
