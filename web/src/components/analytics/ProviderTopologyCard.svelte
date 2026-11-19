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
    requests?: number
  }

  interface Props {
    providers?: ProviderNodeItem[]
    activeRequests?: ActiveRequestItem[]
    activePulseMap?: Record<string, number>
    pulseProvider?: string
    lastProvider?: string
    errorProvider?: string
    onRefresh?: () => void
  }

  let {
    providers = [],
    activeRequests = [],
    activePulseMap = {},
    pulseProvider = '',
    lastProvider = '',
    errorProvider = '',
    onRefresh
  }: Props = $props()

  // Displayed providers sorted deterministically ONCE so positions NEVER jump or shuffle
  let displayProviders = $derived(
    [...providers].sort((a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: 'base', numeric: true }))
  )

  let activeCount = $derived.by(() => {
    let count = 0
    for (const n of geometry.nodes) {
      if (n.isActive) count++
    }
    return count
  })

  // Smooth traffic flow status (no frantic BPM jumps)
  let stageInfo = $derived.by(() => {
    if (activeCount >= 5) {
      return { label: 'Peak Flow', color: '#06b6d4' }
    }
    if (activeCount >= 1) {
      return { label: 'Live Routing', color: '#10b981' }
    }
    return { label: 'Standby', color: '#64748b' }
  })

  // Container sizing and fitView state
  let containerEl = $state<HTMLDivElement | null>(null)
  let containerWidth = $state(900)
  let containerHeight = $state(520)

  // Zoom & Pan transforms with hardware acceleration
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

  // 1 SINGLE DETERMINISTIC ORBIT WITH 100% PURE CUBIC BÉZIER CURVES
  // - Clean single orbit: symmetrical, spacious, perfectly distributed around center
  // - Pure Cubic Bézier curves with lateral wave offsets (ZERO straight lines, ZERO sharp angles)
  // - 100% stable stationary positions
  let geometry = $derived.by(() => {
    const s = displayProviders.length
    if (s === 0) {
      return { rx: 280, ry: 160, nodes: [] }
    }

    // Dynamic ellipse radius based on node count for spacious layout
    const rx = Math.max(280, (190 * s) / (2 * Math.PI))
    const ry = Math.max(165, 0.54 * rx)

    const nodes = displayProviders.map((p, idx) => {
      // Deterministic symmetrical angle around single orbit
      const angle = -Math.PI / 2 + (2 * Math.PI * idx) / s
      const x = rx * Math.cos(angle)
      const y = ry * Math.sin(angle)

      const pid = p.id.toLowerCase().trim()
      const pname = (p.name || '').toLowerCase().trim()
      const palias = (p.alias || '').toLowerCase().trim()

      const isCustomNode = pid.startsWith('openai-compatible-') || pid.startsWith('anthropic-compatible-')

      const isMatch = (target: string) => {
        if (!target) return false
        const t = target.toLowerCase().trim()

        // 1. Exact ID or Catalog Alias match
        if (t === pid || (palias && t === palias)) return true

        // 2. Exact Display Name match
        if (pname && t === pname) return true

        const isTargetCustom = t.startsWith('openai-compatible-') || t.startsWith('anthropic-compatible-')

        // 3. Strict Boundary Isolation:
        // A custom compatible target can NEVER match official catalog providers (e.g. 'openai' or 'anthropic')
        if (isTargetCustom && !isCustomNode) {
          return false
        }
        // An official target ('openai', 'anthropic') can NEVER match a custom compatible node
        if (!isTargetCustom && isCustomNode && (t === 'openai' || t === 'anthropic')) {
          return false
        }

        // 4. Custom Node matching by suffix/UUID
        if (isCustomNode) {
          if (pid.endsWith(t) && t.length >= 8) return true
        }

        return false
      }

      // Check real-time active in-flight requests
      let isActive = false
      for (const r of activeRequests) {
        if (r?.provider && isMatch(r.provider)) {
          isActive = true
          break
        }
      }

      // Check live pulses (requests completed in last 2.5s)
      if (!isActive) {
        const now = Date.now()
        for (const [pulsingId, expiry] of Object.entries(activePulseMap)) {
          if (now < expiry && isMatch(pulsingId)) {
            isActive = true
            break
          }
        }
      }

      if (!isActive && pulseProvider && isMatch(pulseProvider)) {
        isActive = true
      }

      const isLast = !isActive && Boolean(lastProvider) && isMatch(lastProvider)
      const isError = !isActive && Boolean(errorProvider) && isMatch(errorProvider)

      // Exact pixel-perfect border points
      const source = getRectBorderPoint(0, 0, ROUTER_W, ROUTER_H, x, y)
      const target = getRectBorderPoint(x, y, PROVIDER_W, PROVIDER_H, 0, 0)

      // PURE CUBIC BÉZIER WAVE: Guaranteed non-straight with lateral perpendicular offset
      const dx = target.x - source.x
      const dy = target.y - source.y
      const dist = Math.hypot(dx, dy) || 1
      const nx = -dy / dist
      const ny = dx / dist

      // Lateral bow alternating per node to give an organic cybernetic weave
      const bow = Math.max(24, Math.min(48, dist * 0.12)) * (idx % 2 === 0 ? 1 : -1)

      const cp1x = source.x + (dx / dist) * (dist * 0.35) + nx * bow
      const cp1y = source.y + (dy / dist) * (dist * 0.35) + ny * bow
      const cp2x = target.x - (dx / dist) * (dist * 0.35) - nx * (bow * 0.6)
      const cp2y = target.y - (dy / dist) * (dist * 0.35) - ny * (bow * 0.6)

      const curvePath = `M ${source.x.toFixed(1)} ${source.y.toFixed(1)} C ${cp1x.toFixed(1)} ${cp1y.toFixed(1)}, ${cp2x.toFixed(1)} ${cp2y.toFixed(1)}, ${target.x.toFixed(1)} ${target.y.toFixed(1)}`
      const returnPath = `M ${target.x.toFixed(1)} ${target.y.toFixed(1)} C ${cp2x.toFixed(1)} ${cp2y.toFixed(1)}, ${cp1x.toFixed(1)} ${cp1y.toFixed(1)}, ${source.x.toFixed(1)} ${source.y.toFixed(1)}`

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

  let hasUserInteracted = $state(false)
  let hasAutoFitted = false

  function clampPan(x: number, y: number, currentZoom: number = zoom) {
    const limitX = Math.max(containerWidth * 0.6, geometry.rx * currentZoom * 0.75 + 120)
    const limitY = Math.max(containerHeight * 0.6, geometry.ry * currentZoom * 0.75 + 100)
    return {
      x: Math.max(-limitX, Math.min(limitX, x)),
      y: Math.max(-limitY, Math.min(limitY, y)),
    }
  }

  function fitView(force = false) {
    if (!containerWidth || !containerHeight) return
    if (!force && hasUserInteracted) return

    const requiredWidth = 2 * geometry.rx + 240
    const requiredHeight = 2 * geometry.ry + 140
    const scaleX = (containerWidth - 36) / requiredWidth
    const scaleY = (containerHeight - 36) / requiredHeight
    zoom = Math.min(1.0, Math.max(0.4, Math.min(scaleX, scaleY)))
    panX = 0
    panY = 0
  }

  function handleResetView() {
    hasUserInteracted = false
    fitView(true)
  }

  function handleZoomIn() {
    hasUserInteracted = true
    const nextZoom = Math.min(2.0, zoom * 1.15)
    const clamped = clampPan(panX, panY, nextZoom)
    panX = clamped.x
    panY = clamped.y
    zoom = nextZoom
  }

  function handleZoomOut() {
    hasUserInteracted = true
    const nextZoom = Math.max(0.35, zoom / 1.15)
    const clamped = clampPan(panX, panY, nextZoom)
    panX = clamped.x
    panY = clamped.y
    zoom = nextZoom
  }

  let prevNodeCount = 0
  $effect(() => {
    const count = geometry.nodes.length
    if (count !== prevNodeCount) {
      prevNodeCount = count
      if (!hasUserInteracted) {
        fitView(false)
      }
    }
  })

  function handlePointerDown(e: PointerEvent) {
    if (e.button !== 0) return
    // Never start canvas drag if clicking buttons, links, or interactive node cards
    if (
      (e.target as HTMLElement).closest('button') ||
      (e.target as HTMLElement).closest('[role="button"]')
    ) {
      return
    }
    isDragging = true
    hasUserInteracted = true
    dragStart = { x: e.clientX - panX, y: e.clientY - panY }
    try {
      ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
    } catch {}
  }

  function handlePointerMove(e: PointerEvent) {
    if (!isDragging) return
    const rawPanX = e.clientX - dragStart.x
    const rawPanY = e.clientY - dragStart.y
    const clamped = clampPan(rawPanX, rawPanY, zoom)
    panX = clamped.x
    panY = clamped.y
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
    hasUserInteracted = true

    // Normalize wheel delta to keep zoom velocity smooth across mice & trackpads
    const normalizedDelta = Math.max(-1, Math.min(1, e.deltaY))
    const zoomFactor = normalizedDelta < 0 ? 1.08 : 0.92
    const nextZoom = Math.min(2.0, Math.max(0.35, zoom * zoomFactor))

    // Zoom smoothly anchored towards mouse cursor
    if (containerEl) {
      const rect = containerEl.getBoundingClientRect()
      const mouseX = e.clientX - rect.left - containerWidth / 2
      const mouseY = e.clientY - rect.top - containerHeight / 2

      const ratio = nextZoom / zoom
      const newPanX = mouseX - (mouseX - panX) * ratio
      const newPanY = mouseY - (mouseY - panY) * ratio
      const clamped = clampPan(newPanX, newPanY, nextZoom)
      panX = clamped.x
      panY = clamped.y
    }

    zoom = nextZoom
  }

  onMount(() => {
    if (!containerEl) return
    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        containerWidth = entry.contentRect.width || 900
        containerHeight = entry.contentRect.height || 520
        if (!hasAutoFitted) {
          hasAutoFitted = true
          fitView(true)
        }
      }
    })
    observer.observe(containerEl)
    fitView(true)
    return () => observer.disconnect()
  })
</script>

<div
  class="flex flex-col bg-[#06080e] border border-[#161d2d] rounded-2xl shadow-2xl overflow-hidden animate-fade-in"
  style="height: 520px; contain: layout style paint;"
>
  <!-- Card Header with status, staged cardiac BPM & controls -->
  <div class="px-4 py-3 border-b border-[#161d2d] bg-[#0a0e18] flex items-center justify-between shrink-0">
    <div class="flex items-center gap-2.5 min-w-0">
      <div class="p-1.5 rounded-lg bg-sky-500/10 text-sky-400 flex items-center justify-center">
        <span class="material-symbols-outlined text-[17px]">hub</span>
      </div>
      <div class="min-w-0">
        <h2 class="text-xs font-bold text-slate-100 tracking-wider uppercase font-mono">Routing Topology</h2>
        <p class="text-[11px] text-slate-400 truncate">Gateway network & real-time routing</p>
      </div>
    </div>

    <!-- Right Header Actions & Status -->
    <div class="flex items-center gap-2 shrink-0">
      <!-- Live Status Pill (Smooth transition, zero jerky BPM jumps) -->
      <div
        class="hidden sm:flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[11px] font-mono border transition-all duration-700 ease-out"
        style="background-color: {stageInfo.color}15; color: {stageInfo.color}; border-color: {stageInfo.color}35;"
      >
        <span class="material-symbols-outlined text-[14px] {activeCount > 0 ? 'animate-pulse' : ''}">
          {activeCount > 0 ? 'online_prediction' : 'sync_saved_locally'}
        </span>
        <span class="font-bold">{stageInfo.label}</span>
      </div>

      <!-- Active Streams Count -->
      <div
        class="flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[11px] font-medium transition-all duration-700 ease-out {activeCount > 0
          ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/25'
          : 'bg-[#0e1320] text-slate-400 border border-[#1e293b]'}"
      >
        <span
          class="size-1.5 rounded-full transition-colors duration-700 {activeCount > 0
            ? 'bg-emerald-400 animate-pulse'
            : 'bg-slate-500'}"
        ></span>
        <span class="font-mono">
          {activeCount > 0
            ? `${activeCount} active stream${activeCount > 1 ? 's' : ''}`
            : `${displayProviders.length} nodes`}
        </span>
      </div>

      <!-- Zoom Controls -->
      <div class="flex items-center rounded-lg border border-[#1e293b] bg-[#0c111c] p-0.5">
        <button
          type="button"
          onclick={handleZoomIn}
          class="p-1 text-slate-400 hover:text-white hover:bg-[#1a2333] rounded transition-colors cursor-pointer flex items-center justify-center"
          title="Zoom In"
          aria-label="Zoom In"
        >
          <span class="material-symbols-outlined text-[16px]">add</span>
        </button>
        <button
          type="button"
          onclick={handleZoomOut}
          class="p-1 text-slate-400 hover:text-white hover:bg-[#1a2333] rounded transition-colors cursor-pointer flex items-center justify-center"
          title="Zoom Out"
          aria-label="Zoom Out"
        >
          <span class="material-symbols-outlined text-[16px]">remove</span>
        </button>
        <button
          type="button"
          onclick={handleResetView}
          class="p-1 text-slate-400 hover:text-white hover:bg-[#1a2333] rounded transition-colors cursor-pointer flex items-center justify-center"
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
          class="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-[#1a2333] transition-colors cursor-pointer flex items-center justify-center border border-[#1e293b]"
          title="Refresh topology"
          aria-label="Refresh topology"
        >
          <span class="material-symbols-outlined text-[16px]">refresh</span>
        </button>
      {/if}
    </div>
  </div>

  <!-- Interactive Deep Dark Topology Canvas -->
  <div
    bind:this={containerEl}
    role="region"
    aria-label="Provider topology map"
    class="flex-1 w-full min-w-0 relative overflow-hidden select-none cursor-grab active:cursor-grabbing touch-none"
    style="background-color: #05070c; background-image: radial-gradient(circle at 50% 50%, rgba(56, 189, 248, 0.04) 0%, transparent 68%), radial-gradient(circle, rgba(255, 255, 255, 0.04) 1px, transparent 1px); background-size: 100% 100%, 28px 28px;"
    onpointerdown={handlePointerDown}
    onpointermove={handlePointerMove}
    onpointerup={handlePointerUp}
    onpointercancel={handlePointerUp}
    onwheel={handleWheel}
  >
    <!-- Centered Viewport Layer (GPU hardware accelerated) -->
    <div
      class="absolute inset-0 pointer-events-none"
      style="transform: translate3d({containerWidth / 2 + panX}px, {containerHeight / 2 + panY}px, 0) scale3d({zoom}, {zoom}, 1); transform-origin: 0 0; will-change: transform;"
    >
      <!-- SVG Vector Layer for Orbit, Fibers, Idle Traveling Dots & Active Streams -->
      <svg class="absolute inset-0 overflow-visible pointer-events-none">
        <defs>
          {#each geometry.nodes as node (node.id)}
            {#if node.isActive}
              <linearGradient id="active-beam-{node.id}" x1="0%" y1="0%" x2="100%" y2="100%">
                <stop offset="0%" stop-color="#38bdf8" stop-opacity="0.95" />
                <stop offset="100%" stop-color={node.color || '#22d3ee'} stop-opacity="1" />
              </linearGradient>
            {/if}
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
            stroke="#1e293b"
            stroke-width="1.2"
            stroke-dasharray="6 8"
            opacity="0.35"
          />
        {/if}

        <!-- 1. Ambient Connected Fiber Lines: Pure continuous Cubic Bézier curves -->
        {#each geometry.nodes as node (node.id)}
          <path
            d={node.curvePath}
            fill="none"
            stroke={hoveredNodeId === node.id ? '#38bdf8' : '#1e293b'}
            stroke-width={hoveredNodeId === node.id ? '2' : '1.2'}
            stroke-dasharray={hoveredNodeId === node.id ? 'none' : '4 5'}
            opacity={hoveredNodeId === node.id ? '0.9' : '0.4'}
            class="transition-all duration-300"
          />
        {/each}

        <!-- 2. Concentric Radar Pulse Rings from Gateway (Fixed continuous 4.4s cycle, smooth fade, zero twitch) -->
        <g
          class="pointer-events-none transition-all duration-700 ease-out"
          style="opacity: {activeCount > 0 ? 0.75 : 0.2}; stroke: {stageInfo.color};"
        >
          <circle cx="0" cy="0" r="32" fill="none" opacity="0">
            <animate attributeName="r" values="32;120" dur="4.4s" repeatCount="indefinite" />
            <animate attributeName="opacity" values="0.75;0" dur="4.4s" repeatCount="indefinite" />
            <animate attributeName="stroke-width" values="1.8;0.2" dur="4.4s" repeatCount="indefinite" />
          </circle>
          <circle cx="0" cy="0" r="32" fill="none" opacity="0">
            <animate attributeName="r" values="32;120" dur="4.4s" begin="2.2s" repeatCount="indefinite" />
            <animate attributeName="opacity" values="0.75;0" dur="4.4s" begin="2.2s" repeatCount="indefinite" />
            <animate attributeName="stroke-width" values="1.8;0.2" dur="4.4s" begin="2.2s" repeatCount="indefinite" />
          </circle>
        </g>

        <!-- 3. Dynamic Streams with Smooth Fade In / Fade Out Transitions -->
        {#each geometry.nodes as node (node.id)}
          <!-- Active Stream: Neon Glow Aura & Laser Beams (Smooth Fade In & Decay Fade Out) -->
          <g
            class="pointer-events-none transition-opacity duration-700 ease-out"
            style="opacity: {node.isActive ? 1 : 0};"
          >
            <!-- Wide neon ambient glow aura -->
            <path
              d={node.curvePath}
              fill="none"
              stroke={node.color || '#3b82f6'}
              stroke-width="6"
              stroke-opacity="0.32"
              stroke-linecap="round"
            />
            <!-- Laser forward energy beam -->
            <path
              d={node.curvePath}
              fill="none"
              stroke="url(#active-beam-{node.id})"
              stroke-width="2.4"
              stroke-linecap="round"
              class="topology-edge-flow"
            />
            <!-- Emerald return data stream -->
            <path
              d={node.returnPath}
              fill="none"
              stroke="#10b981"
              stroke-width="1.6"
              stroke-linecap="round"
              class="topology-edge-flow-reverse"
            />

            <!-- High-energy token packet dots flowing along the active curve (aliran titik lewat) -->
            {#if node.isActive}
              <!-- Primary prompt packet dot (Gateway -> Provider) -->
              <circle r="4.2" fill="#38bdf8" opacity="0">
                <animateMotion dur="1.1s" repeatCount="indefinite" path={node.curvePath} />
                <animate attributeName="opacity" values="0;0.95;0.95;0" keyTimes="0;0.12;0.88;1" dur="1.1s" repeatCount="indefinite" />
              </circle>
              <!-- Secondary trailing token packet dot (Gateway -> Provider) -->
              <circle r="3.2" fill="#7dd3fc" opacity="0">
                <animateMotion dur="1.1s" begin="0.55s" repeatCount="indefinite" path={node.curvePath} />
                <animate attributeName="opacity" values="0;0.85;0.85;0" keyTimes="0;0.12;0.88;1" dur="1.1s" begin="0.55s" repeatCount="indefinite" />
              </circle>
              <!-- Completion response packet dot (Provider -> Gateway) -->
              <circle r="3.6" fill="#34d399" opacity="0">
                <animateMotion dur="1.3s" repeatCount="indefinite" path={node.returnPath} />
                <animate attributeName="opacity" values="0;0.95;0.95;0" keyTimes="0;0.12;0.88;1" dur="1.3s" repeatCount="indefinite" />
              </circle>
            {/if}
          </g>

          <!-- Warm Amber Recent Trace (Smooth Fade In / Fade Out) -->
          <g
            class="pointer-events-none transition-opacity duration-700 ease-out"
            style="opacity: {!node.isActive && node.isLast ? 0.75 : 0};"
          >
            <path
              d={node.curvePath}
              fill="none"
              stroke="#f59e0b"
              stroke-width="1.8"
              stroke-dasharray="4 4"
              class="topology-edge-flow"
            />
          </g>

          <!-- Red Error Trace (Smooth Fade In / Fade Out) -->
          <g
            class="pointer-events-none transition-opacity duration-700 ease-out"
            style="opacity: {!node.isActive && node.isError ? 0.85 : 0};"
          >
            <path
              d={node.curvePath}
              fill="none"
              stroke="#ef4444"
              stroke-width="2"
              stroke-dasharray="4 4"
            />
          </g>
        {/each}
      </svg>

      <!-- Center Gateway Node (0, 0) - Rock-solid stationary, zero scale twitch -->
      <div
        class="absolute z-10 flex items-center justify-center px-4 py-2.5 rounded-2xl border-2 pointer-events-auto cursor-pointer transition-all duration-700 ease-out gateway-ambient-glow {activeCount > 0
          ? 'border-sky-400 bg-[#0d1424] text-white shadow-[0_0_22px_rgba(56,189,248,0.4)]'
          : 'border-[#1e293b] bg-[#0c111e] text-slate-100 shadow-[0_0_10px_rgba(56,189,248,0.12)]'}"
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
        <span class="text-xs font-bold font-mono tracking-tight {activeCount > 0 ? 'text-sky-400' : 'text-slate-100'} truncate">
          {branding.config.appName || 'Gateway'}
        </span>
        {#if activeCount > 0}
          <span class="ml-2 px-1.5 py-0.2 rounded-full bg-sky-500 text-white text-[10px] font-bold shrink-0">
            {activeCount}
          </span>
        {/if}
      </div>

      <!-- Provider Nodes stationed deterministically along orbit (w=168, h=42) -->
      {#each geometry.nodes as node (node.id)}
        <div
          role="button"
          tabindex="0"
          onmouseenter={() => (hoveredNodeId = node.id)}
          onmouseleave={() => (hoveredNodeId = null)}
          class="absolute flex items-center gap-2.5 px-3 py-1.5 rounded-xl border transition-all duration-700 ease-out bg-[#0c101b] shadow-sm hover:scale-105 pointer-events-auto cursor-pointer"
          style="left: {node.x}px; top: {node.y}px; width: {PROVIDER_W}px; height: {PROVIDER_H}px; transform: translate(-50%, -50%); background-color: {node.isActive
            ? '#0e182b'
            : node.isLast
              ? '#14141e'
              : '#0c101b'}; border-color: {node.isActive
            ? (node.color || '#3b82f6')
            : hoveredNodeId === node.id
              ? '#38bdf8'
              : node.isLast
                ? '#f59e0b'
                : '#1e293b'}; box-shadow: {node.isActive
            ? `0 0 16px ${node.color}45, 0 2px 6px rgba(0,0,0,0.5)`
            : hoveredNodeId === node.id
              ? '0 0 12px rgba(56,189,248,0.35)'
              : node.isLast
                ? '0 0 8px rgba(245, 158, 11, 0.25)'
                : 'none'};"
        >
          <div
            class="w-7 h-7 rounded-lg flex items-center justify-center shrink-0 overflow-hidden"
            style="background-color: {node.color}20;"
          >
            {#if !imageErrors[node.id]}
              <ProviderArtwork id={node.id} class="w-5 h-5 rounded-sm object-contain" />
            {:else}
              <span class="text-xs font-bold" style="color: {node.color};">
                {node.textIcon}
              </span>
            {/if}
          </div>
          <div class="min-w-0 flex-1">
            <span
              class="text-xs font-semibold truncate block"
              style="color: {node.isActive ? (node.color || '#f1f5f9') : '#cbd5e1'}"
              title={node.name}
            >
              {node.name}
            </span>
          </div>

          <!-- Active / Status Badge -->
          {#if node.isActive}
            <div class="flex items-center gap-1 shrink-0">
              <span class="text-[9px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded-full bg-emerald-500/20 text-emerald-400 border border-emerald-500/35">
                Live
              </span>
            </div>
          {:else if node.isLast}
            <span class="text-[9px] font-medium px-1.5 py-0.5 rounded-full bg-amber-500/15 text-amber-400 border border-amber-500/30 shrink-0">
              Recent
            </span>
          {:else}
            <span class="size-1.5 rounded-full bg-slate-600/50 shrink-0"></span>
          {/if}
        </div>
      {/each}
    </div>

    <!-- Empty State -->
    {#if displayProviders.length === 0}
      <div class="pointer-events-none absolute inset-0 z-10 flex items-center justify-center px-4 text-center text-xs text-slate-500">
        No providers active or routed in the last 3 days.
      </div>
    {/if}

    <!-- Bottom Clean Legend & Info (Tanpa teks orbit alay) -->
    <div class="absolute bottom-3 left-3 right-3 z-20 flex items-center justify-between pointer-events-none">
      <div class="flex items-center gap-3 px-3 py-1.5 rounded-lg bg-[#0a0e18]/90 border border-[#161d2d] text-[10px] text-slate-400 shadow-lg pointer-events-auto font-mono">
        <span class="flex items-center gap-1.5">
          <span class="size-2 rounded-full bg-emerald-400"></span>
          <span>Live Routing</span>
        </span>
        <span class="flex items-center gap-1.5">
          <span class="size-2 rounded-full bg-amber-400"></span>
          <span>Recent</span>
        </span>
        <span class="flex items-center gap-1.5">
          <span class="size-2 rounded-full bg-slate-600"></span>
          <span>Connected</span>
        </span>
      </div>
      <span class="hidden sm:inline-block text-[10px] text-slate-500 bg-[#0a0e18]/80 px-2.5 py-1 rounded-md border border-[#161d2d]/60 font-mono">
        Drag to pan · Scroll to zoom
      </span>
    </div>
  </div>
</div>

<style>
  /* Calm, continuous ambient glow - fixed duration, zero scale transform, zero layout twitch */
  @keyframes gateway-ambient-glow {
    0%, 100% {
      box-shadow: 0 0 10px rgba(56, 189, 248, 0.12), inset 0 0 6px rgba(56, 189, 248, 0.05);
    }
    50% {
      box-shadow: 0 0 18px rgba(56, 189, 248, 0.22), inset 0 0 10px rgba(56, 189, 248, 0.1);
    }
  }

  .gateway-ambient-glow {
    animation: gateway-ambient-glow 4.8s ease-in-out infinite;
  }
</style>
