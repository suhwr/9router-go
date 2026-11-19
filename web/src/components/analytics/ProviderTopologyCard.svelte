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

  // STAGED CARDIAC RHYTHM (Bertahap sesuai jumlah koneksi & beban aktif)
  // Stage 1 (Resting): 0 active, <= 4 providers -> 3.2s (37 BPM)
  // Stage 2 (Nominal): 1-2 active or 5-9 providers -> 2.1s (57 BPM)
  // Stage 3 (Elevated): 3-5 active or 10-15 providers -> 1.45s (83 BPM)
  // Stage 4 (Peak): 6+ active or 16+ providers -> 0.92s (130 BPM)
  let stageLevel = $derived.by(() => {
    if (activeCount >= 5 || displayProviders.length >= 16) return 4
    if (activeCount >= 2 || displayProviders.length >= 9) return 3
    if (activeCount >= 1 || displayProviders.length >= 4) return 2
    return 1
  })

  let heartbeatDuration = $derived.by(() => {
    switch (stageLevel) {
      case 4: return 0.92
      case 3: return 1.45
      case 2: return 2.10
      default: return 3.20
    }
  })

  let stageInfo = $derived.by(() => {
    switch (stageLevel) {
      case 4:
        return { label: 'Peak Overdrive', color: '#f43f5e', bpm: 130, numRings: 4 }
      case 3:
        return { label: 'Elevated Flow', color: '#06b6d4', bpm: 83, numRings: 3 }
      case 2:
        return { label: 'Active Pulse', color: '#10b981', bpm: 57, numRings: 2 }
      default:
        return { label: 'Resting Rhythm', color: '#64748b', bpm: 37, numRings: 1 }
    }
  })

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
   * Guarantees lines snap seamlessly to card edges without floating gaps.
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

  // Layout calculation: deterministic ellipse orbit with dynamic cubic Bézier S-curves & bus routing
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

      // Exact pixel-perfect border points
      const source = getRectBorderPoint(0, 0, ROUTER_W, ROUTER_H, x, y)
      const target = getRectBorderPoint(x, y, PROVIDER_W, PROVIDER_H, 0, 0)

      // Dynamic Non-Monotonous Cubic Bézier Calculation:
      // Combines natural launch/dock face tangents, direct velocity, and index-based harmonic waves.
      const dx = target.x - source.x
      const dy = target.y - source.y
      const dist = Math.hypot(dx, dy) || 1
      const nx = -dy / dist
      const ny = dx / dist

      // Router launch tangent: horizontal from flanks vs vertical from top/bottom
      const isRouterHorizontal = Math.abs(source.x) >= ROUTER_W / 2 - 1.5
      const launchDirX = isRouterHorizontal ? Math.sign(source.x) : 0
      const launchDirY = isRouterHorizontal ? 0 : Math.sign(source.y)

      // Provider dock tangent: horizontal from sides vs vertical from top/bottom
      const relTargetX = target.x - x
      const relTargetY = target.y - y
      const isTargetHorizontal = Math.abs(relTargetX) >= PROVIDER_W / 2 - 1.5
      const dockDirX = isTargetHorizontal ? -Math.sign(relTargetX) : 0
      const dockDirY = isTargetHorizontal ? 0 : -Math.sign(relTargetY)

      const span = Math.min(95, Math.max(30, dist * 0.38))

      // Harmonic wave unique to each index to give varied S-curves and prevent monotonous arcs
      const waveMagnitude = Math.sin(idx * 1.85 + 0.45) * Math.min(24, dist * 0.08)
      const waveX = nx * waveMagnitude
      const waveY = ny * waveMagnitude

      // CP1 launches along router face with forward momentum + wave harmonic
      const cp1x = source.x + (launchDirX * 0.72 + (dx / dist) * 0.28) * span + waveX * 1.15
      const cp1y = source.y + (launchDirY * 0.72 + (dy / dist) * 0.28) * span + waveY * 1.15

      // CP2 docks into provider face with reverse momentum - wave harmonic
      const cp2x = target.x - (dockDirX * 0.72 + (dx / dist) * 0.28) * span - waveX * 0.85
      const cp2y = target.y - (dockDirY * 0.72 + (dy / dist) * 0.28) * span - waveY * 0.85

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
  class="flex flex-col bg-[#06080e] border border-[#161d2d] rounded-2xl shadow-2xl overflow-hidden animate-fade-in"
  style="height: 480px"
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

    <!-- Right Header Actions & Staged Heartbeat Monitor -->
    <div class="flex items-center gap-2 shrink-0">
      <!-- Staged Heartbeat Monitor Pill -->
      <div
        class="hidden sm:flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[11px] font-mono border transition-all duration-300"
        style="background-color: {stageInfo.color}15; color: {stageInfo.color}; border-color: {stageInfo.color}35;"
        title="Dynamic cardiac rhythm staged by active stream load and connected provider count"
      >
        <span class="material-symbols-outlined text-[14px] animate-pulse">ecg_heart</span>
        <span class="font-bold">{stageInfo.bpm} BPM</span>
        <span class="text-[10px] opacity-75 hidden md:inline">· {stageInfo.label}</span>
      </div>

      <!-- Active Streams Count -->
      <div
        class="flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[11px] font-medium {activeCount > 0
          ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/25'
          : 'bg-[#0e1320] text-slate-400 border border-[#1e293b]'}"
      >
        <span
          class="size-1.5 rounded-full {activeCount > 0
            ? 'bg-emerald-400 animate-pulse'
            : 'bg-slate-500'}"
        ></span>
        <span class="font-mono">
          {activeCount > 0
            ? `${activeCount} stream${activeCount > 1 ? 's' : ''}`
            : `${displayProviders.length} nodes`}
        </span>
      </div>

      <!-- Zoom Controls -->
      <div class="flex items-center rounded-lg border border-[#1e293b] bg-[#0c111c] p-0.5">
        <button
          type="button"
          onclick={() => (zoom = Math.min(2.2, zoom * 1.15))}
          class="p-1 text-slate-400 hover:text-white hover:bg-[#1a2333] rounded transition-colors cursor-pointer flex items-center justify-center"
          title="Zoom In"
          aria-label="Zoom In"
        >
          <span class="material-symbols-outlined text-[16px]">add</span>
        </button>
        <button
          type="button"
          onclick={() => (zoom = Math.max(0.2, zoom / 1.15))}
          class="p-1 text-slate-400 hover:text-white hover:bg-[#1a2333] rounded transition-colors cursor-pointer flex items-center justify-center"
          title="Zoom Out"
          aria-label="Zoom Out"
        >
          <span class="material-symbols-outlined text-[16px]">remove</span>
        </button>
        <button
          type="button"
          onclick={fitView}
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
    class="flex-1 w-full min-w-0 relative overflow-hidden select-none cursor-grab active:cursor-grabbing"
    style="background-color: #05070c; background-image: radial-gradient(circle at 50% 50%, rgba(56, 189, 248, 0.05) 0%, transparent 68%), radial-gradient(circle, rgba(255, 255, 255, 0.05) 1px, transparent 1px); background-size: 100% 100%, 26px 26px;"
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
      <!-- SVG Vector Layer for Orbit, Network Fibers, Radar & Streams -->
      <svg class="absolute inset-0 overflow-visible pointer-events-none" style="transform: translate(0, 0);">
        <defs>
          {#each geometry.nodes as node (node.id)}
            <linearGradient id="active-beam-{node.id}" x1="0%" y1="0%" x2="100%" y2="100%">
              <stop offset="0%" stop-color="#38bdf8" stop-opacity="0.95" />
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
            stroke="#1e293b"
            stroke-width="1.2"
            stroke-dasharray="6 8"
            opacity="0.4"
          />
        {/if}

        <!-- 1. Ambient Connected Fiber Lines (S-curves / filleted bus traces) -->
        {#each geometry.nodes as node (node.id)}
          <path
            d={node.curvePath}
            fill="none"
            stroke={hoveredNodeId === node.id ? '#38bdf8' : '#1e293b'}
            stroke-width={hoveredNodeId === node.id ? '2' : '1.2'}
            stroke-dasharray={hoveredNodeId === node.id ? 'none' : '4 5'}
            opacity={hoveredNodeId === node.id ? '0.9' : '0.45'}
            class="transition-all duration-300"
          />
        {/each}

        <!-- 2. Concentric Radar Pulse Rings from Gateway staged by stageLevel -->
        <g class="topology-gateway-radar">
          <!-- Primary Heartbeat Sonar Ring -->
          <circle cx="0" cy="0" r="32" fill="none" stroke={stageInfo.color} opacity="0">
            <animate attributeName="r" values="32;135" dur="{heartbeatDuration}s" repeatCount="indefinite" />
            <animate attributeName="opacity" values="0.75;0" dur="{heartbeatDuration}s" repeatCount="indefinite" />
            <animate attributeName="stroke-width" values="2.4;0.2" dur="{heartbeatDuration}s" repeatCount="indefinite" />
          </circle>

          <!-- Staged Secondary Waves -->
          {#if stageLevel >= 2}
            <circle cx="0" cy="0" r="32" fill="none" stroke={stageInfo.color} opacity="0">
              <animate attributeName="r" values="32;135" dur="{heartbeatDuration}s" begin="{(heartbeatDuration * 0.35).toFixed(2)}s" repeatCount="indefinite" />
              <animate attributeName="opacity" values="0.7;0" dur="{heartbeatDuration}s" begin="{(heartbeatDuration * 0.35).toFixed(2)}s" repeatCount="indefinite" />
              <animate attributeName="stroke-width" values="2.0;0.2" dur="{heartbeatDuration}s" begin="{(heartbeatDuration * 0.35).toFixed(2)}s" repeatCount="indefinite" />
            </circle>
          {/if}
          {#if stageLevel >= 3}
            <circle cx="0" cy="0" r="32" fill="none" stroke="#38bdf8" opacity="0">
              <animate attributeName="r" values="32;135" dur="{heartbeatDuration}s" begin="{(heartbeatDuration * 0.65).toFixed(2)}s" repeatCount="indefinite" />
              <animate attributeName="opacity" values="0.65;0" dur="{heartbeatDuration}s" begin="{(heartbeatDuration * 0.65).toFixed(2)}s" repeatCount="indefinite" />
              <animate attributeName="stroke-width" values="1.8;0.2" dur="{heartbeatDuration}s" begin="{(heartbeatDuration * 0.65).toFixed(2)}s" repeatCount="indefinite" />
            </circle>
          {/if}
          {#if stageLevel >= 4}
            <circle cx="0" cy="0" r="32" fill="none" stroke="#f43f5e" opacity="0">
              <animate attributeName="r" values="32;135" dur="{heartbeatDuration}s" begin="{(heartbeatDuration * 0.82).toFixed(2)}s" repeatCount="indefinite" />
              <animate attributeName="opacity" values="0.75;0" dur="{heartbeatDuration}s" begin="{(heartbeatDuration * 0.82).toFixed(2)}s" repeatCount="indefinite" />
              <animate attributeName="stroke-width" values="2.2;0.2" dur="{heartbeatDuration}s" begin="{(heartbeatDuration * 0.82).toFixed(2)}s" repeatCount="indefinite" />
            </circle>
          {/if}
        </g>

        <!-- 3. Staged Heartbeat Ripple Cascading to Connected Nodes in Sequence -->
        {#each geometry.nodes as node, idx (node.id)}
          <circle r="2.5" fill={stageInfo.color} opacity="0">
            <animateMotion
              dur="{heartbeatDuration}s"
              begin="{((idx / Math.max(1, geometry.nodes.length)) * heartbeatDuration).toFixed(2)}s"
              repeatCount="indefinite"
              path={node.curvePath}
            />
            <animate
              attributeName="opacity"
              values="0;0.7;0.7;0"
              keyTimes="0;0.2;0.8;1"
              dur="{heartbeatDuration}s"
              begin="{((idx / Math.max(1, geometry.nodes.length)) * heartbeatDuration).toFixed(2)}s"
              repeatCount="indefinite"
            />
          </circle>
        {/each}

        <!-- 4. Active, Recent, or Error Streams with Smooth Fade Transitions (NO Pop-in) -->
        {#each geometry.nodes as node (node.id)}
          <g
            class="transition-opacity duration-700 ease-out"
            style="opacity: {node.isActive ? 1 : 0}; pointer-events: none;"
          >
            <!-- Wide neon ambient glow aura -->
            <path
              d={node.curvePath}
              fill="none"
              stroke={node.color || '#3b82f6'}
              stroke-width="8"
              stroke-opacity="0.25"
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
            <!-- Smooth flowing token packet comets with progressive opacity fade -->
            <circle r="4.2" fill="#38bdf8" opacity="0">
              <animateMotion dur="1.2s" repeatCount="indefinite" path={node.curvePath} />
              <animate attributeName="opacity" values="0;0.95;0.95;0" keyTimes="0;0.12;0.88;1" dur="1.2s" repeatCount="indefinite" />
            </circle>
            <circle r="3.6" fill="#34d399" opacity="0">
              <animateMotion dur="1.4s" repeatCount="indefinite" path={node.returnPath} />
              <animate attributeName="opacity" values="0;0.95;0.95;0" keyTimes="0;0.12;0.88;1" dur="1.4s" repeatCount="indefinite" />
            </circle>
          </g>

          {#if !node.isActive && node.isLast}
            <!-- Warm amber recent trace -->
            <path
              d={node.curvePath}
              fill="none"
              stroke="#f59e0b"
              stroke-width="2.2"
              stroke-dasharray="4 4"
              opacity="0.75"
              class="topology-edge-flow transition-opacity duration-500"
            />
          {:else if !node.isActive && node.isError}
            <!-- Red error trace -->
            <path
              d={node.curvePath}
              fill="none"
              stroke="#ef4444"
              stroke-width="2.5"
              stroke-dasharray="4 4"
              opacity="0.85"
              class="transition-opacity duration-500"
            />
          {/if}
        {/each}
      </svg>

      <!-- Center Gateway Node (0, 0) with Cardiac Lub-Dub Systolic Beat -->
      <div
        class="absolute z-10 flex items-center justify-center px-4 py-2.5 rounded-2xl border-2 pointer-events-auto transition-all cursor-pointer gateway-cardiac-pulse {activeCount > 0
          ? 'border-sky-400 bg-[#0d1424] text-white shadow-xl'
          : 'border-[#1e293b] bg-[#0c111e] text-slate-100 shadow-md'}"
        style="left: 0px; top: 0px; width: {ROUTER_W}px; height: {ROUTER_H}px; transform: translate(-50%, -50%); --hb-dur: {heartbeatDuration}s;"
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

      <!-- Provider Nodes stationed deterministically along orbit with exact dimensions w=168, h=42 -->
      {#each geometry.nodes as node (node.id)}
        <div
          role="button"
          tabindex="0"
          onmouseenter={() => (hoveredNodeId = node.id)}
          onmouseleave={() => (hoveredNodeId = null)}
          class="absolute flex items-center gap-2.5 px-3 py-1.5 rounded-xl border transition-all duration-300 bg-[#090d16]/95 backdrop-blur shadow-xs hover:shadow-xl hover:scale-105 pointer-events-auto cursor-pointer"
          style="left: {node.x}px; top: {node.y}px; width: {PROVIDER_W}px; height: {PROVIDER_H}px; transform: translate(-50%, -50%); border-color: {node.isActive
            ? (node.color || '#3b82f6')
            : hoveredNodeId === node.id
              ? '#38bdf8'
              : node.isLast
                ? '#f59e0b'
                : '#172133'}; box-shadow: {node.isActive
            ? `0 0 18px ${node.color}40, 0 2px 8px rgba(0,0,0,0.5)`
            : hoveredNodeId === node.id
              ? '0 0 14px rgba(56,189,248,0.35)'
              : node.isLast
                ? '0 0 10px rgba(245, 158, 11, 0.25)'
                : 'none'};"
        >
          <div
            class="w-7 h-7 rounded-lg flex items-center justify-center shrink-0 overflow-hidden"
            style="background-color: {node.color}22;"
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
            style="color: {node.isActive ? (node.color || '#f1f5f9') : '#cbd5e1'}"
            title={node.name}
          >
            {node.name}
          </span>

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
            <span class="size-1.5 rounded-full bg-slate-600 shrink-0"></span>
          {/if}
        </div>
      {/each}
    </div>

    <!-- Empty State -->
    {#if displayProviders.length === 0}
      <div class="pointer-events-none absolute inset-0 z-10 flex items-center justify-center px-4 text-center text-xs text-slate-500">
        No providers connected or routed in this period.
      </div>
    {/if}

    <!-- Bottom Legend & Helper Info -->
    <div class="absolute bottom-3 left-3 right-3 z-20 flex items-center justify-between pointer-events-none">
      <div class="flex items-center gap-3 px-3 py-1.5 rounded-lg bg-[#0a0e18]/90 border border-[#161d2d] backdrop-blur text-[10px] text-slate-400 shadow-lg pointer-events-auto font-mono">
        <span class="flex items-center gap-1.5">
          <span class="size-2 rounded-full bg-emerald-400 animate-pulse"></span>
          <span>Live Stream</span>
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
      <span class="hidden sm:inline-block text-[10px] text-slate-500 bg-[#0a0e18]/80 px-2.5 py-1 rounded-md backdrop-blur border border-[#161d2d]/60 font-mono">
        Drag to pan · Scroll to zoom · Hover node to highlight
      </span>
    </div>
  </div>
</div>

<style>
  /* Cardiac Lub-Dub Systolic Heartbeat Pulse for Gateway Core */
  @keyframes gateway-cardiac-pulse {
    0% {
      transform: translate(-50%, -50%) scale(1);
      box-shadow: 0 0 10px rgba(56, 189, 248, 0.2);
    }
    14% {
      transform: translate(-50%, -50%) scale(1.05);
      box-shadow: 0 0 24px rgba(56, 189, 248, 0.55), 0 0 45px rgba(56, 189, 248, 0.25);
    }
    24% {
      transform: translate(-50%, -50%) scale(1.01);
      box-shadow: 0 0 14px rgba(56, 189, 248, 0.3);
    }
    38% {
      transform: translate(-50%, -50%) scale(1.07);
      box-shadow: 0 0 30px rgba(56, 189, 248, 0.65), 0 0 60px rgba(56, 189, 248, 0.35);
    }
    56%, 100% {
      transform: translate(-50%, -50%) scale(1);
      box-shadow: 0 0 10px rgba(56, 189, 248, 0.2);
    }
  }

  .gateway-cardiac-pulse {
    animation: gateway-cardiac-pulse var(--hb-dur, 2.1s) cubic-bezier(0.4, 0, 0.2, 1) infinite;
  }
</style>
