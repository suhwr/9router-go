<script lang="ts">
  import Modal from '../../lib/ui/Modal.svelte'
  import { getIconPath } from '../connections/types'
  import { PROVIDER_CATALOG } from '../../lib/providers'

  interface Props {
    open?: boolean
    onClose: () => void
    onSelectProvider?: (id: string) => void
  }

  let { open = false, onClose, onSelectProvider }: Props = $props()

  let searchQuery = $state('')

  const FALLBACK_OAUTH = [
    {
      id: 'antigravity',
      name: 'Google Antigravity',
      desc: 'Google DeepMind AI OAuth proxy session',
      icon: 'rocket_launch',
    },
    {
      id: 'gemini-cli',
      name: 'Gemini CLI',
      desc: 'Google Cloud / Code Assist OAuth authorization',
      icon: 'auto_awesome',
    },
    {
      id: 'codex',
      name: 'OpenAI Codex',
      desc: 'OpenAI Codex CLI PKCE session',
      icon: 'terminal',
    },
    {
      id: 'claude',
      name: 'Claude Code',
      desc: 'Anthropic Claude Code OAuth authorization',
      icon: 'smart_toy',
    },
    {
      id: 'grok-cli',
      name: 'Grok CLI',
      desc: 'xAI Grok Build subscription credits via device code',
      icon: 'auto_awesome',
    },
    {
      id: 'github',
      name: 'GitHub Copilot',
      desc: 'GitHub Copilot subscription authorization',
      icon: 'code',
    },
    {
      id: 'iflow',
      name: 'iFlow AI',
      desc: 'iFlow AI OAuth authorization',
      icon: 'water_drop',
    },
    {
      id: 'qoder',
      name: 'Qoder',
      desc: 'Qoder AI coding assistant session',
      icon: 'psychology',
    },
    {
      id: 'cline',
      name: 'Cline',
      desc: 'Cline extension OAuth login',
      icon: 'smart_toy',
    },
    {
      id: 'clinepass',
      name: 'ClinePass',
      desc: 'ClinePass OAuth & key access',
      icon: 'vpn_key',
    },
    {
      id: 'cursor',
      name: 'Cursor IDE',
      desc: 'Cursor IDE OAuth login',
      icon: 'edit_note',
    },
    {
      id: 'codebuddy-intl',
      name: 'CodeBuddy',
      desc: 'CodeBuddy AI coding assistant',
      icon: 'smart_toy',
    },
    {
      id: 'gitlab',
      name: 'GitLab Duo',
      desc: 'GitLab Duo OAuth integration',
      icon: 'code',
    },
  ]

  let availableProviders = $derived.by(() => {
    // Prefer catalog entries with category === 'oauth' or authModes including 'oauth'
    const catalogOAuth = PROVIDER_CATALOG.filter(
      (p) =>
        !p.hidden &&
        (p.category === 'oauth' || (p.authModes && p.authModes.includes('oauth'))),
    ).map((p) => ({
      id: p.id,
      name: p.name,
      desc: p.notice?.text || `${p.name} OAuth account connection`,
      icon: p.icon || 'vpn_key',
    }))

    const list = catalogOAuth.length > 0 ? catalogOAuth : FALLBACK_OAUTH

    // Make sure primary popular ones are represented
    const seen = new Set(list.map((item) => item.id))
    for (const fb of FALLBACK_OAUTH) {
      if (!seen.has(fb.id)) {
        list.push(fb)
        seen.add(fb.id)
      }
    }

    if (!searchQuery.trim()) return list
    const q = searchQuery.toLowerCase().trim()
    return list.filter(
      (p) =>
        p.id.toLowerCase().includes(q) ||
        p.name.toLowerCase().includes(q) ||
        p.desc.toLowerCase().includes(q),
    )
  })

  function handleSelect(id: string) {
    onClose()
    onSelectProvider?.(id)
  }
</script>

<Modal
  isOpen={open}
  {onClose}
  title="Add OAuth Account"
  icon="vpn_key"
  size="lg"
>
  <div class="space-y-4">
    <p class="text-xs text-text-muted">
      Select an OAuth provider to connect your subscription, CLI session, or developer account.
    </p>

    <!-- Search input -->
    <div class="relative">
      <span class="material-symbols-outlined notranslate absolute left-3 top-2.5 text-[18px] text-text-muted" translate="no">search</span>
      <input
        type="text"
        bind:value={searchQuery}
        placeholder="Search OAuth provider (e.g. Antigravity, Gemini, Codex, Claude)..."
        class="w-full rounded-lg border border-border bg-surface-2 py-2 pl-9 pr-3 text-xs text-text-main placeholder-text-muted/60 outline-none transition-colors focus:border-brand-500"
      />
      {#if searchQuery}
        <button
          type="button"
          onclick={() => (searchQuery = '')}
          class="absolute right-2.5 top-2.5 text-text-muted hover:text-text-main"
          aria-label="Clear search"
        >
          <span class="material-symbols-outlined notranslate text-[16px]" translate="no">close</span>
        </button>
      {/if}
    </div>

    <!-- Provider List Grid -->
    <div class="max-h-[60vh] overflow-y-auto space-y-2 pr-1 custom-scrollbar">
      {#if availableProviders.length === 0}
        <div class="py-8 text-center text-xs text-text-muted">
          No OAuth providers matching "{searchQuery}"
        </div>
      {:else}
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
          {#each availableProviders as prov (prov.id)}
            <button
              type="button"
              onclick={() => handleSelect(prov.id)}
              class="group flex items-start gap-3 rounded-xl border border-border-subtle bg-surface-2/60 p-3 text-left transition-all hover:bg-surface-2 hover:border-brand-500/50 hover:shadow-xs cursor-pointer active:scale-[0.98]"
            >
              <div class="size-9 rounded-lg bg-surface border border-border/60 p-1 flex items-center justify-center shrink-0">
                <img
                  src={getIconPath(prov.id)}
                  alt={prov.name}
                  class="size-full object-contain"
                  onerror={(e) => {
                    ;(e.currentTarget as HTMLImageElement).style.display = 'none'
                  }}
                />
              </div>

              <div class="min-w-0 flex-1">
                <div class="flex items-center justify-between gap-1">
                  <span class="text-xs font-semibold text-text-main group-hover:text-brand-500 transition-colors truncate">
                    {prov.name}
                  </span>
                  <span class="material-symbols-outlined notranslate text-[14px] text-text-muted/60 group-hover:text-brand-500 group-hover:translate-x-0.5 transition-all shrink-0" translate="no">
                    arrow_forward
                  </span>
                </div>
                <p class="text-[10px] text-text-muted line-clamp-2 mt-0.5 leading-snug">
                  {prov.desc}
                </p>
              </div>
            </button>
          {/each}
        </div>
      {/if}
    </div>
  </div>
</Modal>
