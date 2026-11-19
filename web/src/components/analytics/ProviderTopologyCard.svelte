<script lang="ts">
  import { onMount } from 'svelte'
  import { branding } from '../../lib/branding.svelte'
  import type { ActiveRequestItem } from './types'
  import ProviderArtwork from '../providers/ProviderArtwork.svelte'

  interface ProviderNodeItem {
    id: string
    name: string
    color?: string
    type?: string
    alias?: string
  }

  interface Props {
    providers?: ProviderNodeItem[]
    activeRequests?: ActiveRequestItem[]
    pulseProvider?: string
    lastProvider?: string
    errorProvider?: string
    onRefresh?: () => void
  }

  let {
    providers = [],
    activeRequests = [],
    pulseProvider = '',
    lastProvider = '',
    errorProvider = '',
    onRefresh
  }: Props = $props()

  // Displayed providers in deterministic order
  let displayProviders = $derived(providers)

  let activeProviderIds = $derived.by(() => {
    const set = new Set<string>()
    for (const r of activeRequests) {
      const p = r.provider?.toLowerCase().trim()
      if (p) set.add(p)
    }
    return set
  })

  let hasPulse = $derived(Boolean(pulseProvider))
  let activeCount = $derived(
    Math.max(activeRequests.reduce((sum, r) => sum + (r.count || 1), 0), hasPulse ? 1 : 0)
  )

  // Container sizing and fitView state
  let containerEl = $state<HTMLDivElement | null>(null)
  let containerWidth = $state(800)
  let containerHeight = $state(480)

  // Zoom & Pan transforms
  let zoom = $state(0.85)
  let panX = $state(0)
  let panY = $state(0)
  let isDragging = $state(false)
  let dragStart = { x: 0, y: 0 }
  let hoveredNodeId = $state<string | null>(null)

  // Track image load errors to fallback to initials
  let imageErrors = $state<Record<string, boolean>>({})

  // Dimensions of nodes for pixel-perfect line snapping
  const ROUTER_W = 144
  const ROUTER_H = 44
  const PROVIDER_W = 168
  const PROVIDER_H = 42

  /**
   * Calculates the exact border intersection point on a rectangle centered at (cx, cy)
   * with dimensions (w, h) in the direction towards (targetX, targetY).
   * This guarantees lines snap seamlessly to card edges without floating gaps!
   */
  function getRectBorderPoint(cx: number, cy: number, w: number, h: number, targetX: number, targetY: number) {
    const dx = targetX - cx
    const dy = targetY - cy
    if (Math.abs(dx) < 0.001 && Math.abs(dy) < 0.001) return { x: cx, y: cy }
    const hw = w / 2
    const hh = h / 2
    const sx = Math.abs(hw / (dx || 0.00001))
    const sy = Math.abs(hh / (dy || 0.00001))
    const s = Math.min(sx, sy)
    return { x: cx + dx * s, y: cy + dy * s }
  }

  // Layout calculation: deterministic ellipse orbit with pixel-perfect curved paths
  let geometry = $derived.by(() => {
    const s = displayProviders.length
    const rx = Math.max(300, (200 * s) / (2 * Math.PI))
    const ry = Math.max(180, 0.54 * rx)

    const nodes = displayProviders.map((p, idx) => {
      // Deterministic angle based on fixed index in sorted list
      const angle = -Math.PI / 2 + (2 * Math.PI * idx) / s
      const x = rx * Math.cos(angle)
      const y = ry * Math.sin(angle)

      const pid = p.id.toLowerCase().trim()
      const pname = (p.name || '').toLowerCase().trim()
      const palias = (p.alias || '').toLowerCase().trim()
      const isProviderMatch = (target: string) => {
        if (!target) return false
        const t = target.toLowerCase().trim()
        return t === pid || t === pname || (Boolean(palias) && t === palias)
      }

      const isActive =
        activeProviderIds.has(pid) ||
        activeProviderIds.has(pname) ||
        (Boolean(palias) && activeProviderIds.has(palias)) ||
        (hasPulse && isProviderMatch(pulseProvider))
      const isLast = !isActive && Boolean(lastProvider) && isProviderMatch(lastProvider)
      const isError = !isActive && Boolean(errorProvider) && isProviderMatch(errorProvider)

      // Exact pixel-perfect border points (lines snap 100% seamlessly to card borders)
      const source = getRectBorderPoint(0, 0, ROUTER_W, ROUTER_H, x, y)
      const target = getRectBorderPoint(x, y, PROVIDER_W, PROVIDER_H, 0, 0)

      // Smooth organic curve bow: subtle perpendicular arch so paths feel fluid and cybernetic
      const dx = target.x - source.x
      const dy = target.y - source.y
      const dist = Math.hypot(dx, dy) || 1
      const normalX = -dy / dist
      const normalY = dx / dist
      const bowDirection = idx % 2 === 0 ? 1 : -1
      const bowOffset = Math.min(20, Math.max(10, dist * 0.07)) * bowDirection
      const midX = (source.x + target.x) / 2
      const midY = (source.y + target.y) / 2
      const cx = midX + normalX * bowOffset
      const cy = midY + normalY * bowOffset

      const curvePath = `M ${source.x.toFixed(1)} ${source.y.toFixed(1)} Q ${cx.toFixed(1)} ${cy.toFixed(1)} ${target.x.toFixed(1)} ${target.y.toFixed(1)}`
      const returnPath = `M ${target.x.toFixed(1)} ${target.y.toFixed(1)} Q ${cx.toFixed(1)} ${cy.toFixed(1)} ${source.x.toFixed(1)} ${source.y.toFixed(1)}`

      const textIcon = (p.name || p.id || '?').slice(0, 2).toUpperCase()

      return {
        ...p,
        x,
        y,
        isActive,
        isLast,
        isError,
        curvePath,
        returnPath,
        textIcon,
        color: p.color || '#3b82f6'
      }
    })

    return { rx, ry, nodes }
  })

  function fitView() {
    if (!containerWidth || !containerHeight) return
    const requiredWidth = 2 * geometry.rx + 240
    const requiredHeight = 2 * geometry.ry + 140
    const scaleX = (containerWidth - 36) / requiredWidth
    const scaleY = (containerHeight - 36) / requiredHeight
    zoom = Math.min(1.05, Math.max(0.18, Math.min(scaleX, scaleY)))
    panX = 0
    panY = 0
  }

  let prevNodeCount = 0
  $effect(() => {
    const count = geometry.nodes.length
    if (count !== prevNodeCount) {
      prevNodeCount = count
      fitView()
    }
  })

  function handlePointerDown(e: PointerEvent) {
    if ((e.target as HTMLElement).closest('button')) return
    isDragging = true
    dragStart = { x: e.clientX - panX, y: e.clientY - panY }
    ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
  }

  function handlePointerMove(e: PointerEvent) {
    if (!isDragging) return
    panX = e.clientX - dragStart.x
    panY = e.clientY - dragStart.y
  }

  function handlePointerUp(e: PointerEvent) {
    if (isDragging) {
      isDragging = false
      try {
        ;(e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId)
      } catch {}
    }
  }

  function handleWheel(e: WheelEvent) {
    e.preventDefault()
    const zoomFactor = e.deltaY < 0 ? 1.08 : 0.92
    zoom = Math.min(2.2, Math.max(0.25, zoom * zoomFactor))
  }

  onMount(() => {
    if (!containerEl) return
    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        containerWidth = entry.contentRect.width || 800
        containerHeight = entry.contentRect.height || 480
        fitView()
      }
    })
    observer.observe(containerEl)
    fitView()
    return () => observer.disconnect()
  })
</script>

<div
  class="flex flex-col bg-surface border border-border-subtle rounded-[14px] shadow-[var(--shadow-soft)] overflow-hidden"
  style="height: 480px"
>
  <!-- Card Header with status and controls -->
  <div class="px-4 py-3 border-b border-border-subtle bg-surface-2/30 flex items-center justify-between shrink-0">
    <div class="flex items-center gap-2.5 min-w-0">
      <div class="p-1.5 rounded-lg bg-primary/10 text-primary flex items-center justify-center">
        <span class="material-symbols-outlined text-[17px]">hub</span>
      </div>
      <div class="min-w-0">
        <h2 class="text-xs font-bold text-text-main tracking-tight uppercase">Routing Topology</h2>
        <p class="text-[11px] text-text-muted truncate">Gateway network & real-time routing</p>
      </div>
    </div>

    <!-- Right Header Actions -->
    <div class="flex items-center gap-2 shrink-0">
      <div
        class="hidden sm:flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[11px] font-medium {activeCount > 0
          ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/25'
          : 'bg-surface text-text-muted border border-border-subtle'}"
      >
        <span
          class="size-1.5 rounded-full {activeCount > 0
            ? 'bg-emerald-500 animate-pulse'
            : 'bg-text-muted/50'}"
        ></span>
        <span>
          {activeCount > 0
            ? `${activeCount} active stream${activeCount > 1 ? 's' : ''}`
            : `${displayProviders.length} connected`}
        </span>
      </div>

      <div class="flex items-center rounded-lg border border-border bg-surface p-0.5">
        <button
          type="button"
          onclick={() => (zoom = Math.min(2.2, zoom * 1.15))}
          class="p-1 text-text-muted hover:text-text-main hover:bg-surface-2 rounded transition-colors cursor-pointer flex items-center justify-center"
          title="Zoom In"
          aria-label="Zoom In"
        >
          <span class="material-symbols-outlined text-[16px]">add</span>
        </button>
        <button
          type="button"
          onclick={() => (zoom = Math.max(0.2, zoom / 1.15))}
          class="p-1 text-text-muted hover:text-text-main hover:bg-surface-2 rounded transition-colors cursor-pointer flex items-center justify-center"
          title="Zoom Out"
          aria-label="Zoom Out"
        >
          <span class="material-symbols-outlined text-[16px]">remove</span>
        </button>
        <button
          type="button"
          onclick={fitView}
          class="p-1 text-text-muted hover:text-text-main hover:bg-surface-2 rounded transition-colors cursor-pointer flex items-center justify-center"
          title="Reset View"
          aria-label="Reset View"
        >
          <span class="material-symbols-outlined text-[16px]">crop_free</span>
        </button>
      </div>

      {#if onRefresh}
        <button
          type="button"
          onclick={onRefresh}
          class="p-1.5 rounded-lg text-text-muted hover:text-text-main hover:bg-surface-2 transition-colors cursor-pointer flex items-center justify-center"
          title="Refresh topology"
          aria-label="Refresh topology"
        >
          <span class="material-symbols-outlined text-[16px]">refresh</span>
        </button>
      {/if}
    </div>
  </div>

  <!-- Interactive Topology Canvas -->
  <div
    bind:this={containerEl}
    role="region"
    aria-label="Provider topology map"
    class="flex-1 w-full min-w-0 relative overflow-hidden select-none cursor-grab active:cursor-grabbing bg-bg-subtle/20"
    style="background-image: radial-gradient(circle, var(--color-border) 1px, transparent 1px); background-size: 24px 24px;"
    onpointerdown={handlePointerDown}
    onpointermove={handlePointerMove}
    onpointerup={handlePointerUp}
    onpointercancel={handlePointerUp}
    onwheel={handleWheel}
  >
    <!-- Centered Viewport Layer -->
    <div
      class="absolute inset-0 pointer-events-none"
      style="transform: translate({containerWidth / 2 + panX}px, {containerHeight / 2 + panY}px) scale({zoom}); transform-origin: 0 0;"
    >
      <!-- SVG Vector Layer for Orbit, Network Fibers & Active Streams -->
      <svg class="absolute inset-0 overflow-visible pointer-events-none" style="transform: translate(0, 0);">
        <defs>
          {#each geometry.nodes as node (node.id)}
            <linearGradient id="active-beam-{node.id}" x1="0%" y1="0%" x2="100%" y2="100%">
              <stop offset="0%" stop-color="#38bdf8" stop-opacity="0.9" />
              <stop offset="100%" stop-color={node.color || '#22d3ee'} stop-opacity="1" />
            </linearGradient>
          {/each}
        </defs>

        <!-- Subtle architectural dashed orbit ring -->
        {#if displayProviders.length > 0}
          <ellipse
            cx="0"
            cy="0"
            rx={geometry.rx}
            ry={geometry.ry}
            fill="none"
            stroke="var(--color-border)"
            stroke-width="1.2"
            stroke-dasharray="6 8"
            opacity="0.35"
          />
        {/if}

        <!-- 1. Ambient Connected Fiber Lines: Every connected provider is visibly linked to the Gateway -->
        {#each geometry.nodes as node (node.id)}
          <path
            d={node.curvePath}
            fill="none"
            stroke="var(--color-border)"
            stroke-width={hoveredNodeId === node.id ? '2' : '1.2'}
            stroke-dasharray={hoveredNodeId === node.id ? 'none' : '4 5'}
            opacity={hoveredNodeId === node.id ? '0.85' : '0.4'}
            class="transition-all duration-200"
          />
        {/each}

        <!-- 2. Concentric Radar Pulse Rings from Gateway when Active -->
        {#if activeCount > 0}
          <g class="topology-gateway-radar">
            <circle cx="0" cy="0" r="32" fill="none" stroke="var(--color-primary, #3b82f6)" opacity="0">
              <animate attributeName="r" values="32;120" dur="2.2s" repeatCount="indefinite" />
              <animate attributeName="opacity" values="0.75;0" dur="2.2s" repeatCount="indefinite" />
              <animate attributeName="stroke-width" values="2.2;0.4" dur="2.2s" repeatCount="indefinite" />
            </circle>
            <circle cx="0" cy="0" r="32" fill="none" stroke="#38bdf8" opacity="0">
              <animate attributeName="r" values="32;120" dur="2.2s" begin="1.1s" repeatCount="indefinite" />
              <animate attributeName="opacity" values="0.75;0" dur="2.2s" begin="1.1s" repeatCount="indefinite" />
              <animate attributeName="stroke-width" values="2.2;0.4" dur="2.2s" begin="1.1s" repeatCount="indefinite" />
            </circle>
          </g>
        {/if}

        <!-- 3. Active, Recent, or Error Streams -->
        {#each geometry.nodes as node (node.id)}
          {#if node.isActive}
            <g class="topology-active-stream">
              <!-- Wide neon ambient glow aura -->
              <path
                d={node.curvePath}
                fill="none"
                stroke={node.color || '#3b82f6'}
                stroke-width="8"
                stroke-opacity="0.28"
                stroke-linecap="round"
              />
              <!-- Laser forward energy beam -->
              <path
                d={node.curvePath}
                fill="none"
                stroke="url(#active-beam-{node.id})"
                stroke-width="3"
                stroke-linecap="round"
                class="topology-edge-flow"
              />
              <!-- Emerald return stream -->
              <path
                d={node.returnPath}
                fill="none"
                stroke="#10b981"
                stroke-width="1.8"
                stroke-linecap="round"
                class="topology-edge-flow-reverse"
              />
              <!-- Smooth flowing token packet comets -->
              <circle r="4.5" fill="#38bdf8" opacity="0.95">
                <animateMotion dur="1.2s" repeatCount="indefinite" path={node.curvePath} />
              </circle>
              <circle r="3.8" fill="#34d399" opacity="0.95">
                <animateMotion dur="1.4s" repeatCount="indefinite" path={node.returnPath} />
              </circle>
            </g>
          {:else if node.isLast}
            <!-- Warm amber recent trace -->
            <path
              d={node.curvePath}
              fill="none"
              stroke="#f59e0b"
              stroke-width="2.2"
              stroke-dasharray="4 4"
              opacity="0.75"
              class="topology-edge-flow"
            />
          {:else if node.isError}
            <!-- Red error trace -->
            <path
              d={node.curvePath}
              fill="none"
              stroke="#ef4444"
              stroke-width="2.5"
              stroke-dasharray="4 4"
              opacity="0.85"
            />
          {/if}
        {/each}
      </svg>

      <!-- Center Gateway Node (0, 0) with exact dimensions w=144, h=44 -->
      <div
        class="absolute z-10 flex items-center justify-center px-4 py-2.5 rounded-2xl border-2 {activeCount > 0
          ? 'topology-router-core border-primary bg-surface shadow-xl shadow-primary/20'
          : 'border-border bg-surface shadow-sm'} pointer-events-auto transition-all cursor-pointer"
        style="left: 0px; top: 0px; width: {ROUTER_W}px; height: {ROUTER_H}px; transform: translate(-50%, -50%);"
      >
        <img
          src={branding.config.appLogoUrl || branding.config.faviconUrl || '/favicon.svg'}
          alt={branding.config.appName || 'Gateway'}
          class="w-5 h-5 mr-2 object-contain shrink-0"
          loading="lazy"
          decoding="async"
          onerror={(e) => {
            ;(e.currentTarget as HTMLImageElement).src = '/favicon.svg'
          }}
        />
        <span class="text-xs font-bold {activeCount > 0 ? 'text-primary' : 'text-text-main'} truncate">
          {branding.config.appName || 'Gateway'}
        </span>
        {#if activeCount > 0}
          <span class="ml-2 px-1.5 py-0.2 rounded-full bg-primary text-white text-[10px] font-bold shrink-0">
            {activeCount}
          </span>
        {/if}
      </div>

      <!-- Provider Nodes stationed deterministically along orbit with exact dimensions w=168, h=42 -->
      {#each geometry.nodes as node (node.id)}
        <div
          role="button"
          tabindex="0"
          onmouseenter={() => (hoveredNodeId = node.id)}
          onmouseleave={() => (hoveredNodeId = null)}
          class="absolute flex items-center gap-2.5 px-3 py-1.5 rounded-xl border transition-all duration-200 bg-surface/95 backdrop-blur shadow-xs hover:shadow-lg hover:scale-105 pointer-events-auto cursor-pointer"
          style="left: {node.x}px; top: {node.y}px; width: {PROVIDER_W}px; height: {PROVIDER_H}px; transform: translate(-50%, -50%); border-color: {node.isActive
            ? (node.color || '#3b82f6')
            : hoveredNodeId === node.id
              ? 'var(--color-primary, #3b82f6)'
              : node.isLast
                ? '#f59e0b'
                : 'var(--color-border)'}; box-shadow: {node.isActive
            ? `0 0 16px ${node.color}35, 0 2px 6px rgba(0,0,0,0.08)`
            : hoveredNodeId === node.id
              ? '0 0 12px rgba(59,130,246,0.25)'
              : node.isLast
                ? '0 0 8px rgba(245, 158, 11, 0.2)'
                : 'none'};"
        >
          <div
            class="w-7 h-7 rounded-lg flex items-center justify-center shrink-0 overflow-hidden"
            style="background-color: {node.color}18;"
          >
            {#if !imageErrors[node.id]}
              <ProviderArtwork id={node.id} class="w-5 h-5 rounded-sm object-contain" />
            {:else}
              <span class="text-xs font-bold" style="color: {node.color};">
                {node.textIcon}
              </span>
            {/if}
          </div>
          <span
            class="text-xs font-semibold truncate flex-1"
            style="color: {node.isActive ? (node.color || 'var(--color-text)') : 'var(--color-text)'}"
            title={node.name}
          >
            {node.name}
          </span>

          <!-- Active / Status Badge -->
          {#if node.isActive}
            <div class="flex items-center gap-1 shrink-0">
              <span class="text-[9px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded-full bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border border-emerald-500/30">
                Live
              </span>
            </div>
          {:else if node.isLast}
            <span class="text-[9px] font-medium px-1.5 py-0.5 rounded-full bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/25 shrink-0">
              Recent
            </span>
          {:else}
            <span class="size-1.5 rounded-full bg-text-muted/30 shrink-0"></span>
          {/if}
        </div>
      {/each}
    </div>

    <!-- Empty State -->
    {#if displayProviders.length === 0}
      <div class="pointer-events-none absolute inset-0 z-10 flex items-center justify-center px-4 text-center text-xs text-text-muted">
        No providers connected or routed in this period.
      </div>
    {/if}

    <!-- Bottom Legend & Helper Info -->
    <div class="absolute bottom-3 left-3 right-3 z-20 flex items-center justify-between pointer-events-none">
      <div class="flex items-center gap-3 px-2.5 py-1 rounded-lg bg-surface/90 border border-border-subtle/80 backdrop-blur text-[10px] text-text-muted shadow-xs pointer-events-auto">
        <span class="flex items-center gap-1.5">
          <span class="size-2 rounded-full bg-emerald-500"></span>
          <span>Live Routing</span>
        </span>
        <span class="flex items-center gap-1.5">
          <span class="size-2 rounded-full bg-amber-500"></span>
          <span>Recent</span>
        </span>
        <span class="flex items-center gap-1.5">
          <span class="size-2 rounded-full bg-text-muted/40"></span>
          <span>Connected</span>
        </span>
      </div>
      <span class="hidden sm:inline-block text-[10px] text-text-muted/60 bg-surface/60 px-2 py-0.5 rounded backdrop-blur">
        Drag to pan · Scroll to zoom · Hover node to highlight
      </span>
    </div>
  </div>
</div>
