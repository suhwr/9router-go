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
    let count = activeRequests.reduce((sum, r) => sum + (r.count || 1), 0)
    const now = Date.now()
    for (const [id, expiry] of Object.entries(activePulseMap)) {
      if (now < expiry) count++
    }
    if (pulseProvider) count++
    return count
  })

  // STAGED CARDIAC RHYTHM:
  // Idle (0 active requests): Calm, serene, gentle breathing rhythm (~4.8s cycle / ~12 BPM).
  // Active traffic: Progressively speeds up to match throughput.
  let stageLevel = $derived.by(() => {
    if (activeCount >= 6) return 4
    if (activeCount >= 3) return 3
    if (activeCount >= 1) return 2
    return 1 // Idle / Resting
  })

  let heartbeatDuration = $derived.by(() => {
    switch (stageLevel) {
      case 4: return 0.95  // 126 BPM - Peak Overdrive
      case 3: return 1.50  // 80 BPM  - Elevated Flow
      case 2: return 2.40  // 50 BPM  - Active Rhythm
      default: return 4.80 // 12 BPM  - Calm Resting Rhythm
    }
  })

  let stageInfo = $derived.by(() => {
    switch (stageLevel) {
      case 4:
        return { label: 'Peak Overdrive', color: '#f43f5e', bpm: 126, numRings: 4 }
      case 3:
        return { label: 'Elevated Flow', color: '#06b6d4', bpm: 80, numRings: 3 }
      case 2:
        return { label: 'Active Rhythm', color: '#10b981', bpm: 50, numRings: 2 }
      default:
        return { label: 'Resting Rhythm', color: '#64748b', bpm: 12, numRings: 1 }
    }
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
      const isMatch = (target: string) => {
        if (!target) return false
        const t = target.toLowerCase().trim()
        if (t === pid || (pname && t === pname) || (palias && t === palias)) return true
        if (pid.includes(t) || t.includes(pid)) return true
        if (pname && (pname.includes(t) || t.includes(pname))) return true
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

  function fitView() {
    if (!containerWidth || !containerHeight) return
    const requiredWidth = 2 * geometry.rx + 240
    const requiredHeight = 2 * geometry.ry + 140
    const scaleX = (containerWidth - 36) / requiredWidth
    const scaleY = (containerHeight - 36) / requiredHeight
    zoom = Math.min(1.0, Math.max(0.35, Math.min(scaleX, scaleY)))
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
    zoom = Math.min(2.2, Math.max(0.3, zoom * zoomFactor))
  }

  onMount(() => {
    if (!containerEl) return
    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        containerWidth = entry.contentRect.width || 900
        containerHeight = entry.contentRect.height || 520
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

    <!-- Right Header Actions & Staged Heartbeat Monitor -->
    <div class="flex items-center gap-2 shrink-0">
      <!-- Staged Heartbeat Monitor Pill -->
      <div
        class="hidden sm:flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[11px] font-mono border transition-all duration-500"
        style="background-color: {stageInfo.color}15; color: {stageInfo.color}; border-color: {stageInfo.color}35;"
        title="Dynamic cardiac rhythm: serene resting rhythm on idle, speeds up with active stream load"
      >
        <span class="material-symbols-outlined text-[14px] {activeCount > 0 ? 'animate-pulse' : ''}">ecg_heart</span>
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
          onclick={() => (zoom = Math.max(0.3, zoom / 1.15))}
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

        <!-- 2. Concentric Radar Pulse Rings from Gateway -->
        <g class="topology-gateway-radar">
          <circle cx="0" cy="0" r="32" fill="none" stroke={stageInfo.color} opacity="0">
            <animate attributeName="r" values="32;125" dur="{heartbeatDuration}s" repeatCount="indefinite" />
            <animate attributeName="opacity" values={activeCount > 0 ? "0.75;0" : "0.25;0"} dur="{heartbeatDuration}s" repeatCount="indefinite" />
            <animate attributeName="stroke-width" values="2.2;0.2" dur="{heartbeatDuration}s" repeatCount="indefinite" />
          </circle>

          {#if stageLevel >= 2}
            <circle cx="0" cy="0" r="32" fill="none" stroke={stageInfo.color} opacity="0">
              <animate attributeName="r" values="32;125" dur="{heartbeatDuration}s" begin="{(heartbeatDuration * 0.35).toFixed(2)}s" repeatCount="indefinite" />
              <animate attributeName="opacity" values="0.7;0" dur="{heartbeatDuration}s" begin="{(heartbeatDuration * 0.35).toFixed(2)}s" repeatCount="indefinite" />
              <animate attributeName="stroke-width" values="1.8;0.2" dur="{heartbeatDuration}s" begin="{(heartbeatDuration * 0.35).toFixed(2)}s" repeatCount="indefinite" />
            </circle>
          {/if}
          {#if stageLevel >= 3}
            <circle cx="0" cy="0" r="32" fill="none" stroke="#38bdf8" opacity="0">
              <animate attributeName="r" values="32;125" dur="{heartbeatDuration}s" begin="{(heartbeatDuration * 0.65).toFixed(2)}s" repeatCount="indefinite" />
              <animate attributeName="opacity" values="0.65;0" dur="{heartbeatDuration}s" begin="{(heartbeatDuration * 0.65).toFixed(2)}s" repeatCount="indefinite" />
              <animate attributeName="stroke-width" values="1.6;0.2" dur="{heartbeatDuration}s" begin="{(heartbeatDuration * 0.65).toFixed(2)}s" repeatCount="indefinite" />
            </circle>
          {/if}
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

      <!-- Center Gateway Node (0, 0) with Staged Lub-Dub Cardiac Pulse -->
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
  /* Lub-Dub Cardiac Pulse: Very gentle and slow on idle, energetic on active */
  @keyframes gateway-cardiac-pulse {
    0% {
      transform: translate(-50%, -50%) scale(1);
      box-shadow: 0 0 10px rgba(56, 189, 248, 0.2);
    }
    14% {
      transform: translate(-50%, -50%) scale(1.035);
      box-shadow: 0 0 20px rgba(56, 189, 248, 0.45);
    }
    24% {
      transform: translate(-50%, -50%) scale(1.01);
      box-shadow: 0 0 12px rgba(56, 189, 248, 0.25);
    }
    38% {
      transform: translate(-50%, -50%) scale(1.05);
      box-shadow: 0 0 24px rgba(56, 189, 248, 0.55);
    }
    56%, 100% {
      transform: translate(-50%, -50%) scale(1);
      box-shadow: 0 0 10px rgba(56, 189, 248, 0.2);
    }
  }

  .gateway-cardiac-pulse {
    animation: gateway-cardiac-pulse var(--hb-dur, 4.8s) cubic-bezier(0.4, 0, 0.2, 1) infinite;
  }
</style>
