<script lang="ts">
  import { Download, Play, Plus, Search, Server } from 'lucide-svelte'
  import type { ProviderConnection, ProviderNode } from '../../api/client'
  import Button from '../../lib/ui/Button.svelte'
  import { PROVIDER_CATALOG, isChatProvider } from '../../lib/providers'
  import ProviderCard from './ProviderCard.svelte'
  import { getProviderStats, matchesFilter, matchesSearch, providerAuthTypes } from './types'

  interface Props {
    connections: ProviderConnection[]
    providerNodes: ProviderNode[]
    onSelectProvider: (id: string) => void
    onToggleAll: (id: string, active: boolean) => void
    /** Opens the unified add-custom-provider dialog; the modal owns the type. */
    onAddCustom: (type?: 'openai-compatible' | 'anthropic-compatible' | 'custom-embedding') => void
    /** Opens the import surface. Optional so existing callers keep working. */
    onImport?: () => void
  }

  let {
    connections = [],
    providerNodes = [],
    onSelectProvider,
    onToggleAll,
    onAddCustom,
    onImport,
  }: Props = $props()

  let searchQuery = $state('')
  let statusFilter = $state<'all' | 'connected' | 'error' | 'disabled' | 'not_connected'>('all')
  let showAllApikey = $state(false)

  const APIKEY_INITIAL_VISIBLE = 20

  // 1. Custom Providers
  let customNodes = $derived(
    providerNodes
      .filter((n) => matchesSearch(n.name, searchQuery, n.id) && matchesFilter(getProviderStats(connections, n.id), statusFilter))
      .map((n) => ({ ...n, stats: getProviderStats(connections, n.id) }))
  )

  // 2. OAuth Providers
  let oauthProviders = $derived(
    PROVIDER_CATALOG
      .filter((p) => isChatProvider(p) && !p.hidden && p.category === 'oauth' && matchesSearch(p.name, searchQuery, p.id, p.alias) && matchesFilter(getProviderStats(connections, p.id, providerAuthTypes(p, ['oauth'])), statusFilter))
      .map((p) => ({ ...p, stats: getProviderStats(connections, p.id, providerAuthTypes(p, ['oauth'])) }))
      .sort((a, b) => (b.stats.connected > 0 ? 1 : 0) - (a.stats.connected > 0 ? 1 : 0) || a.name.localeCompare(b.name))
  )

  // 3. Free Tier Providers
  let freeTierProviders = $derived(
    PROVIDER_CATALOG
      .filter((p) => isChatProvider(p) && !p.hidden && (p.category === 'free' || p.category === 'freeTier') && matchesSearch(p.name, searchQuery, p.id, p.alias) && matchesFilter(getProviderStats(connections, p.id), statusFilter, p.noAuth))
      .map((p) => ({ ...p, stats: getProviderStats(connections, p.id) }))
      .sort((a, b) => (b.stats.connected > 0 || b.noAuth ? 1 : 0) - (a.stats.connected > 0 || a.noAuth ? 1 : 0) || a.name.localeCompare(b.name))
  )

  // 4. API Key Providers
  let apikeyProviders = $derived(
    PROVIDER_CATALOG
      .filter((p) => isChatProvider(p) && !p.hidden && (p.category === 'apikey' || p.category === 'webCookie') && matchesSearch(p.name, searchQuery, p.id, p.alias) && matchesFilter(getProviderStats(connections, p.id, providerAuthTypes(p, ['apikey', 'api_key'])), statusFilter))
      .map((p) => ({ ...p, stats: getProviderStats(connections, p.id, providerAuthTypes(p, ['apikey', 'api_key'])) }))
      .sort((a, b) => (b.stats.connected > 0 ? 1 : 0) - (a.stats.connected > 0 ? 1 : 0) || a.name.localeCompare(b.name))
  )

  let visibleApikeyProviders = $derived(
    showAllApikey || searchQuery.trim() !== '' || statusFilter !== 'all'
      ? apikeyProviders
      : apikeyProviders.slice(0, APIKEY_INITIAL_VISIBLE)
  )
</script>

<div class="flex flex-col gap-5 sm:gap-6 animate-fade-in">
  <!-- Search / Filter Bar -->
  <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-2.5">
    <div class="relative flex-1 max-w-sm">
      <Search class="w-3.5 h-3.5 text-text-muted absolute left-3 top-1/2 -translate-y-1/2" />
      <input
        type="text"
        bind:value={searchQuery}
        placeholder="Search providers..."
        class="w-full pl-8.5 pr-3 py-1.5 text-xs rounded-lg bg-surface border border-border text-text-main placeholder:text-text-muted focus:outline-none focus:border-brand-500 transition-colors"
      />
    </div>

    <select
      bind:value={statusFilter}
      class="h-8 rounded-lg border border-border bg-surface px-2.5 text-xs text-text-main outline-none transition-colors hover:border-brand-500/40 cursor-pointer"
    >
      <option value="all">All Status</option>
      <option value="connected">Connected</option>
      <option value="error">Error</option>
      <option value="disabled">Disabled</option>
      <option value="not_connected">Not Connected</option>
    </select>
  </div>

  <!-- 1. Custom Providers -->
  <div class="flex flex-col gap-3">
    <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
      <h2 class="text-sm sm:text-base font-semibold text-text-main flex items-center gap-1.5">
        <span>Custom Endpoints</span>
        <span class="text-xs font-normal text-text-muted">({customNodes.length})</span>
      </h2>
      <div class="flex items-center gap-2">
        <Button size="sm" onclick={() => onAddCustom('openai-compatible')} class="w-full sm:w-auto">
          <Plus class="w-3.5 h-3.5 mr-1" /> Add Custom
        </Button>
        {#if onImport}
          <Button size="sm" variant="secondary" onclick={onImport} class="w-full sm:w-auto">
            <Download class="w-3.5 h-3.5 mr-1" /> Import
          </Button>
        {/if}
      </div>
    </div>

    {#if customNodes.length === 0}
      <div class="flex flex-col sm:flex-row items-center justify-between gap-3 p-4 border border-dashed border-border rounded-xl text-text-muted text-xs bg-surface/30">
        <div class="flex items-center gap-2.5">
          <Server class="w-4 h-4 text-text-muted shrink-0" />
          <span>No custom endpoints registered yet — click “Add Custom” to connect an OpenAI or Anthropic compatible API</span>
        </div>
        <Button size="sm" variant="secondary" onclick={() => onAddCustom('openai-compatible')}>
          <Plus class="w-3.5 h-3.5 mr-1" /> Add Custom
        </Button>
      </div>
    {:else}
      <div class="grid grid-cols-1 gap-2.5 sm:grid-cols-2 md:grid-cols-3 xl:grid-cols-4">
        {#each customNodes as node (node.id)}
          <ProviderCard
            id={node.id}
            name={node.name}
            apiType={node.apiType}
            stats={node.stats}
            onClick={() => onSelectProvider(node.id)}
            onToggleAll={(active) => onToggleAll(node.id, active)}
          />
        {/each}
      </div>
    {/if}
  </div>

  <!-- 2. OAuth Providers -->
  {#if oauthProviders.length > 0}
    <div class="flex flex-col gap-3">
      <div class="flex items-center justify-between">
        <h2 class="text-sm sm:text-base font-semibold text-text-main flex items-center gap-1.5">
          <span>OAuth Providers</span>
          <span class="text-xs font-normal text-text-muted">({oauthProviders.length})</span>
        </h2>
      </div>
      <div class="grid grid-cols-1 gap-2.5 sm:grid-cols-2 md:grid-cols-3 xl:grid-cols-4">
        {#each oauthProviders as p (p.id)}
          <ProviderCard
            id={p.id}
            name={p.name}
            stats={p.stats}
            onClick={() => onSelectProvider(p.id)}
            onToggleAll={(active) => onToggleAll(p.id, active)}
          />
        {/each}
      </div>
    </div>
  {/if}

  <!-- 3. Free Tier Providers -->
  {#if freeTierProviders.length > 0}
    <div class="flex flex-col gap-3">
      <div class="flex items-center justify-between">
        <h2 class="text-sm sm:text-base font-semibold text-text-main flex items-center gap-1.5">
          <span>Free Tier Providers</span>
          <span class="text-xs font-normal text-text-muted">({freeTierProviders.length})</span>
        </h2>
      </div>
      <div class="grid grid-cols-1 gap-2.5 sm:grid-cols-2 md:grid-cols-3 xl:grid-cols-4">
        {#each freeTierProviders as p (p.id)}
          <ProviderCard
            id={p.id}
            name={p.name}
            stats={p.stats}
            noAuth={p.noAuth}
            onClick={() => onSelectProvider(p.id)}
            onToggleAll={(active) => onToggleAll(p.id, active)}
          />
        {/each}
      </div>
    </div>
  {/if}

  <!-- 4. API Key Providers -->
  {#if apikeyProviders.length > 0}
    <div class="flex flex-col gap-3">
      <div class="flex items-center justify-between">
        <h2 class="text-sm sm:text-base font-semibold text-text-main flex items-center gap-1.5">
          <span>API Key Providers</span>
          <span class="text-xs font-normal text-text-muted">({apikeyProviders.length})</span>
        </h2>
      </div>
      <div class="grid grid-cols-1 gap-2.5 sm:grid-cols-2 md:grid-cols-3 xl:grid-cols-4">
        {#each visibleApikeyProviders as p (p.id)}
          <ProviderCard
            id={p.id}
            name={p.name}
            stats={p.stats}
            onClick={() => onSelectProvider(p.id)}
            onToggleAll={(active) => onToggleAll(p.id, active)}
          />
        {/each}
      </div>
      {#if !showAllApikey && !searchQuery.trim() && statusFilter === 'all' && apikeyProviders.length > visibleApikeyProviders.length}
        <button
          type="button"
          onclick={() => (showAllApikey = true)}
          class="flex w-full items-center justify-center gap-1.5 rounded-xl border border-dashed border-border py-2 text-xs font-medium text-text-muted hover:text-brand-500 hover:border-brand-500/40 transition-colors cursor-pointer"
        >
          Show all {apikeyProviders.length} providers
        </button>
      {/if}
    </div>
  {/if}
</div>
