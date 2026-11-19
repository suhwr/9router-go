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

  // STAGED CARDIAC RHYTHM:
  // Idle (0 active requests): Very calm, tranquil, slow resting breath (5.5s cycle / ~11 BPM).
  // Active traffic: Awakens progressively to match throughput without CPU churn.
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
      default: return 5.50 // 11 BPM  - Calm Resting Standby
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
        return { label: 'Resting Standby', color: '#64748b', bpm: 11, numRings: 1 }
    }
  })

  // Container sizing and fitView state
  let containerEl = $state<HTMLDivElement | null>(null)
  let containerWidth = $state(800)
  let containerHeight = $state(480)

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

  // 2-CONCENTRIC RINGS & BRANCHING BUS TOPOLOGY GEOMETRY
  // Ring 1 (Inner): Frequently used providers (most requests, active, recent).
  // Ring 2 (Outer): Extended providers branching off from their parent sector trunk.
  let geometry = $derived.by(() => {
    const total = displayProviders.length
    if (total === 0) {
      return { rx1: 240, ry1: 140, rx2: 410, ry2: 240, nodes: [], trunks: [] }
    }

    // 1. Score each provider by frequency of use
    const scoredList = displayProviders.map((p) => {
      const pid = p.id.toLowerCase().trim()
      const pname = (p.name || '').toLowerCase().trim()
      const palias = (p.alias || '').toLowerCase().trim()
      const isMatch = (target: string) => {
        if (!target) return false
        const t = target.toLowerCase().trim()
        return t === pid || t === pname || (Boolean(palias) && t === palias)
      }
      const isActive =
        activeProviderIds.has(pid) ||
        activeProviderIds.has(pname) ||
        (Boolean(palias) && activeProviderIds.has(palias)) ||
        (hasPulse && isMatch(pulseProvider))
      const isLast = !isActive && Boolean(lastProvider) && isMatch(lastProvider)
      const isError = !isActive && Boolean(errorProvider) && isMatch(errorProvider)

      let score = p.requests || 0
      if (isActive) score += 10000
      if (isLast) score += 5000
      if (isError) score += 1000

      return {
        ...p,
        pid,
        isActive,
        isLast,
        isError,
        score
      }
    })

    // 2. Separate into Inner Ring (frequently used) and Outer Ring (secondary)
    let innerCount = total
    if (total > 4) {
      innerCount = Math.min(6, Math.max(3, Math.ceil(total * 0.38)))
    }

    // Sort by score descending to pick top inner nodes
    const sortedByUsage = [...scoredList].sort((a, b) => b.score - a.score || a.name.localeCompare(b.name))
    const innerSet = new Set<string>(sortedByUsage.slice(0, innerCount).map((p) => p.id))

    // Sort inner and outer deterministically by name so positions stay rock-solid
    const innerNodesRaw = scoredList.filter((p) => innerSet.has(p.id)).sort((a, b) => a.name.localeCompare(b.name))
    const outerNodesRaw = scoredList.filter((p) => !innerSet.has(p.id)).sort((a, b) => a.name.localeCompare(b.name))

    const numInner = innerNodesRaw.length
    const rx1 = Math.max(240, 180 + numInner * 16)
    const ry1 = Math.max(140, 0.54 * rx1)

    const rx2 = rx1 + 175
    const ry2 = ry1 + 105

    // 3. Position Inner Nodes on Ring 1
    const sectorTrunks: {
      id: string
      sectorIndex: number
      innerNodeId: string
      angle: number
      trunkPath: string
      junction: { x: number; y: number }
      isActive: boolean
    }[] = []

    const innerCalculated = innerNodesRaw.map((p, idx) => {
      const angle = -Math.PI / 2 + (2 * Math.PI * idx) / numInner
      const x = rx1 * Math.cos(angle)
      const y = ry1 * Math.sin(angle)

      const source = getRectBorderPoint(0, 0, ROUTER_W, ROUTER_H, x, y)
      const target = getRectBorderPoint(x, y, PROVIDER_W, PROVIDER_H, 0, 0)

      // Branching Junction Point J placed at 62% of distance along the sector ray
      const jx = source.x * 0.38 + target.x * 0.62
      const jy = source.y * 0.38 + target.y * 0.62

      // Main Trunk Path from Gateway to Junction J
      const dx = jx - source.x
      const dy = jy - source.y
      const dist = Math.hypot(dx, dy) || 1
      const isHorizontal = Math.abs(source.x) >= ROUTER_W / 2 - 1.5
      const lDirX = isHorizontal ? Math.sign(source.x) : 0
      const lDirY = isHorizontal ? 0 : Math.sign(source.y)
      const span = Math.min(60, dist * 0.45)
      const cp1x = source.x + (lDirX * 0.75 + (dx / dist) * 0.25) * span
      const cp1y = source.y + (lDirY * 0.75 + (dy / dist) * 0.25) * span
      const cp2x = jx - (dx / dist) * span * 0.5
      const cp2y = jy - (dy / dist) * span * 0.5

      const trunkPath = `M ${source.x.toFixed(1)} ${source.y.toFixed(1)} C ${cp1x.toFixed(1)} ${cp1y.toFixed(1)}, ${cp2x.toFixed(1)} ${cp2y.toFixed(1)}, ${jx.toFixed(1)} ${jy.toFixed(1)}`

      // Short branch from Junction J into Inner Node
      const branchToInner = `M ${jx.toFixed(1)} ${jy.toFixed(1)} L ${target.x.toFixed(1)} ${target.y.toFixed(1)}`
      const fullCurvePath = `${trunkPath} L ${target.x.toFixed(1)} ${target.y.toFixed(1)}`
      const returnPath = `M ${target.x.toFixed(1)} ${target.y.toFixed(1)} L ${jx.toFixed(1)} ${jy.toFixed(1)} C ${cp2x.toFixed(1)} ${cp2y.toFixed(1)}, ${cp1x.toFixed(1)} ${cp1y.toFixed(1)}, ${source.x.toFixed(1)} ${source.y.toFixed(1)}`

      sectorTrunks.push({
        id: `trunk-${p.id}`,
        sectorIndex: idx,
        innerNodeId: p.id,
        angle,
        trunkPath,
        junction: { x: jx, y: jy },
        isActive: p.isActive
      })

      return {
        ...p,
        ring: 'inner' as const,
        sectorIndex: idx,
        x,
        y,
        curvePath: fullCurvePath,
        branchPath: branchToInner,
        returnPath,
        textIcon: (p.name || p.id || '?').slice(0, 2).toUpperCase(),
        color: p.color || '#38bdf8'
      }
    })

    // 4. Position Outer Nodes on Ring 2 (Branching from sector trunks)
    const numOuter = outerNodesRaw.length
    const outerCalculated = outerNodesRaw.map((p, idx) => {
      // Assign outer node to an inner sector
      const sectorIndex = idx % numInner
      const parentTrunk = sectorTrunks[sectorIndex]
      const siblingsInSector = outerNodesRaw.filter((_, oIdx) => oIdx % numInner === sectorIndex)
      const siblingPos = siblingsInSector.findIndex((s) => s.id === p.id)
      const totalSiblings = siblingsInSector.length

      // Fan-out angle around parent sector angle
      const spread = Math.min(0.38, Math.PI / (numInner * (totalSiblings + 1)))
      const angle = parentTrunk.angle + (siblingPos - (totalSiblings - 1) / 2) * spread

      const x = rx2 * Math.cos(angle)
      const y = ry2 * Math.sin(angle)

      const j = parentTrunk.junction
      const target = getRectBorderPoint(x, y, PROVIDER_W, PROVIDER_H, j.x, j.y)

      // Outer Branch Path from Junction J to Outer Node
      const bdx = target.x - j.x
      const bdy = target.y - j.y
      const bdist = Math.hypot(bdx, bdy) || 1
      const bSpan = Math.min(85, bdist * 0.4)
      const bcp1x = j.x + (bdx / bdist) * bSpan
      const bcp1y = j.y + (bdy / bdist) * bSpan
      const bcp2x = target.x - (bdx / bdist) * bSpan * 0.8
      const bcp2y = target.y - (bdy / bdist) * bSpan * 0.8

      const branchPath = `M ${j.x.toFixed(1)} ${j.y.toFixed(1)} C ${bcp1x.toFixed(1)} ${bcp1y.toFixed(1)}, ${bcp2x.toFixed(1)} ${bcp2y.toFixed(1)}, ${target.x.toFixed(1)} ${target.y.toFixed(1)}`
      const fullCurvePath = `${parentTrunk.trunkPath} C ${bcp1x.toFixed(1)} ${bcp1y.toFixed(1)}, ${bcp2x.toFixed(1)} ${bcp2y.toFixed(1)}, ${target.x.toFixed(1)} ${target.y.toFixed(1)}`
      const returnPath = `M ${target.x.toFixed(1)} ${target.y.toFixed(1)} C ${bcp2x.toFixed(1)} ${bcp2y.toFixed(1)}, ${bcp1x.toFixed(1)} ${bcp1y.toFixed(1)}, ${j.x.toFixed(1)} ${j.y.toFixed(1)}`

      if (p.isActive) {
        parentTrunk.isActive = true
      }

      return {
        ...p,
        ring: 'outer' as const,
        sectorIndex,
        x,
        y,
        curvePath: fullCurvePath,
        branchPath,
        returnPath,
        textIcon: (p.name || p.id || '?').slice(0, 2).toUpperCase(),
        color: p.color || '#94a3b8'
      }
    })

    return {
      rx1,
      ry1,
      rx2,
      ry2,
      nodes: [...innerCalculated, ...outerCalculated],
      trunks: sectorTrunks
    }
  })

  function fitView() {
    if (!containerWidth || !containerHeight) return
    const requiredWidth = 2 * geometry.rx2 + 250
    const requiredHeight = 2 * geometry.ry2 + 160
    const scaleX = (containerWidth - 36) / requiredWidth
    const scaleY = (containerHeight - 36) / requiredHeight
    zoom = Math.min(1.0, Math.max(0.18, Math.min(scaleX, scaleY)))
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
  style="height: 480px; contain: layout style paint;"
>
  <!-- Card Header with status, staged cardiac BPM & controls -->
  <div class="px-4 py-3 border-b border-[#161d2d] bg-[#0a0e18] flex items-center justify-between shrink-0">
    <div class="flex items-center gap-2.5 min-w-0">
      <div class="p-1.5 rounded-lg bg-sky-500/10 text-sky-400 flex items-center justify-center">
        <span class="material-symbols-outlined text-[17px]">hub</span>
      </div>
      <div class="min-w-0">
        <h2 class="text-xs font-bold text-slate-100 tracking-wider uppercase font-mono">Routing Topology</h2>
        <p class="text-[11px] text-slate-400 truncate">Dual-orbit branched gateway bus</p>
      </div>
    </div>

    <!-- Right Header Actions & Staged Heartbeat Monitor -->
    <div class="flex items-center gap-2 shrink-0">
      <!-- Staged Heartbeat Monitor Pill -->
      <div
        class="hidden sm:flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[11px] font-mono border transition-all duration-500"
        style="background-color: {stageInfo.color}15; color: {stageInfo.color}; border-color: {stageInfo.color}35;"
        title="Dynamic cardiac rhythm: serene 11 BPM on idle, speeds up with active stream load"
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
      <!-- SVG Vector Layer for 2 Rings, Trunks, Branches & Active Streams -->
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

        <!-- Ring 1: Inner Orbit (Frequently Used Providers) -->
        <ellipse
          cx="0"
          cy="0"
          rx={geometry.rx1}
          ry={geometry.ry1}
          fill="none"
          stroke="#38bdf8"
          stroke-width="1.1"
          stroke-dasharray="4 6"
          opacity="0.22"
        />

        <!-- Ring 2: Outer Orbit (Extended Branched Providers) -->
        {#if geometry.rx2 > geometry.rx1}
          <ellipse
            cx="0"
            cy="0"
            rx={geometry.rx2}
            ry={geometry.ry2}
            fill="none"
            stroke="#64748b"
            stroke-width="1.1"
            stroke-dasharray="6 8"
            opacity="0.16"
          />
        {/if}

        <!-- 1. Ambient Branched Bus Lines -->
        <!-- Sector Trunks from Gateway to Junction Points J -->
        {#each geometry.trunks as trunk (trunk.id)}
          <path
            d={trunk.trunkPath}
            fill="none"
            stroke={trunk.isActive ? '#38bdf8' : '#1e293b'}
            stroke-width={trunk.isActive ? '1.8' : '1.2'}
            stroke-dasharray={trunk.isActive ? 'none' : '4 5'}
            opacity={trunk.isActive ? '0.85' : '0.4'}
            class="transition-all duration-300"
          />
          <!-- Glowing Junction Node J -->
          <circle
            cx={trunk.junction.x}
            cy={trunk.junction.y}
            r={trunk.isActive ? '3.5' : '2.5'}
            fill={trunk.isActive ? '#38bdf8' : '#1e293b'}
            stroke={trunk.isActive ? '#38bdf8' : '#334155'}
            stroke-width="1"
            class="transition-all duration-300"
          />
        {/each}

        <!-- Branch Lines to Nodes -->
        {#each geometry.nodes as node (node.id)}
          <path
            d={node.branchPath}
            fill="none"
            stroke={hoveredNodeId === node.id ? '#38bdf8' : node.isActive ? (node.color || '#38bdf8') : '#1e293b'}
            stroke-width={hoveredNodeId === node.id || node.isActive ? '1.8' : '1.2'}
            stroke-dasharray={hoveredNodeId === node.id || node.isActive ? 'none' : '4 5'}
            opacity={hoveredNodeId === node.id || node.isActive ? '0.9' : '0.4'}
            class="transition-all duration-300"
          />
        {/each}

        <!-- 2. Concentric Radar Pulse Rings from Gateway (Slow & Serene on Idle) -->
        <g class="topology-gateway-radar">
          <circle cx="0" cy="0" r="32" fill="none" stroke={stageInfo.color} opacity="0">
            <animate attributeName="r" values="32;130" dur="{heartbeatDuration}s" repeatCount="indefinite" />
            <animate attributeName="opacity" values={activeCount > 0 ? "0.75;0" : "0.25;0"} dur="{heartbeatDuration}s" repeatCount="indefinite" />
            <animate attributeName="stroke-width" values="2.2;0.2" dur="{heartbeatDuration}s" repeatCount="indefinite" />
          </circle>

          {#if stageLevel >= 2}
            <circle cx="0" cy="0" r="32" fill="none" stroke={stageInfo.color} opacity="0">
              <animate attributeName="r" values="32;130" dur="{heartbeatDuration}s" begin="{(heartbeatDuration * 0.35).toFixed(2)}s" repeatCount="indefinite" />
              <animate attributeName="opacity" values="0.7;0" dur="{heartbeatDuration}s" begin="{(heartbeatDuration * 0.35).toFixed(2)}s" repeatCount="indefinite" />
              <animate attributeName="stroke-width" values="1.8;0.2" dur="{heartbeatDuration}s" begin="{(heartbeatDuration * 0.35).toFixed(2)}s" repeatCount="indefinite" />
            </circle>
          {/if}
          {#if stageLevel >= 3}
            <circle cx="0" cy="0" r="32" fill="none" stroke="#38bdf8" opacity="0">
              <animate attributeName="r" values="32;130" dur="{heartbeatDuration}s" begin="{(heartbeatDuration * 0.65).toFixed(2)}s" repeatCount="indefinite" />
              <animate attributeName="opacity" values="0.65;0" dur="{heartbeatDuration}s" begin="{(heartbeatDuration * 0.65).toFixed(2)}s" repeatCount="indefinite" />
              <animate attributeName="stroke-width" values="1.6;0.2" dur="{heartbeatDuration}s" begin="{(heartbeatDuration * 0.65).toFixed(2)}s" repeatCount="indefinite" />
            </circle>
          {/if}
        </g>

        <!-- 3. Active Stream Laser Beams & Comets (ONLY rendered when isActive -> ZERO Idle CPU churn!) -->
        {#each geometry.nodes as node (node.id)}
          {#if node.isActive}
            <g class="topology-active-stream">
              <!-- Wide neon ambient glow aura -->
              <path
                d={node.curvePath}
                fill="none"
                stroke={node.color || '#3b82f6'}
                stroke-width="7"
                stroke-opacity="0.28"
                stroke-linecap="round"
              />
              <!-- Laser forward energy beam -->
              <path
                d={node.curvePath}
                fill="none"
                stroke="url(#active-beam-{node.id})"
                stroke-width="2.6"
                stroke-linecap="round"
                class="topology-edge-flow"
              />
              <!-- Emerald return stream -->
              <path
                d={node.returnPath}
                fill="none"
                stroke="#10b981"
                stroke-width="1.6"
                stroke-linecap="round"
                class="topology-edge-flow-reverse"
              />
              <!-- Token packet comets -->
              <circle r="4" fill="#38bdf8" opacity="0">
                <animateMotion dur="1.2s" repeatCount="indefinite" path={node.curvePath} />
                <animate attributeName="opacity" values="0;0.95;0.95;0" keyTimes="0;0.12;0.88;1" dur="1.2s" repeatCount="indefinite" />
              </circle>
              <circle r="3.4" fill="#34d399" opacity="0">
                <animateMotion dur="1.4s" repeatCount="indefinite" path={node.returnPath} />
                <animate attributeName="opacity" values="0;0.95;0.95;0" keyTimes="0;0.12;0.88;1" dur="1.4s" repeatCount="indefinite" />
              </circle>
            </g>
          {:else if node.isLast}
            <!-- Warm amber recent trace -->
            <path
              d={node.curvePath}
              fill="none"
              stroke="#f59e0b"
              stroke-width="1.8"
              stroke-dasharray="4 4"
              opacity="0.7"
              class="topology-edge-flow"
            />
          {:else if node.isError}
            <!-- Red error trace -->
            <path
              d={node.curvePath}
              fill="none"
              stroke="#ef4444"
              stroke-width="2"
              stroke-dasharray="4 4"
              opacity="0.8"
            />
          {/if}
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

      <!-- Provider Nodes (High-performance solid glass without expensive backdrop-filter lag) -->
      {#each geometry.nodes as node (node.id)}
        <div
          role="button"
          tabindex="0"
          onmouseenter={() => (hoveredNodeId = node.id)}
          onmouseleave={() => (hoveredNodeId = null)}
          class="absolute flex items-center gap-2.5 px-3 py-1.5 rounded-xl border transition-all duration-200 bg-[#0c101b] shadow-sm hover:scale-105 pointer-events-auto cursor-pointer"
          style="left: {node.x}px; top: {node.y}px; width: {PROVIDER_W}px; height: {PROVIDER_H}px; transform: translate(-50%, -50%); border-color: {node.isActive
            ? (node.color || '#3b82f6')
            : hoveredNodeId === node.id
              ? '#38bdf8'
              : node.isLast
                ? '#f59e0b'
                : node.ring === 'inner'
                  ? '#1e293b'
                  : '#141c2c'}; box-shadow: {node.isActive
            ? `0 0 16px ${node.color}40, 0 2px 6px rgba(0,0,0,0.5)`
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

          <!-- Active / Ring Badge -->
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
          {:else if node.ring === 'inner'}
            <!-- Frequently Used Indicator Dot -->
            <span class="size-1.5 rounded-full bg-sky-400/80 shrink-0" title="Inner Orbit (Frequently Used)"></span>
          {:else}
            <span class="size-1.5 rounded-full bg-slate-600/50 shrink-0"></span>
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

    <!-- Bottom Legend & Info -->
    <div class="absolute bottom-3 left-3 right-3 z-20 flex items-center justify-between pointer-events-none">
      <div class="flex items-center gap-3 px-3 py-1.5 rounded-lg bg-[#0a0e18]/90 border border-[#161d2d] text-[10px] text-slate-400 shadow-lg pointer-events-auto font-mono">
        <span class="flex items-center gap-1.5">
          <span class="size-2 rounded-full bg-sky-400"></span>
          <span>Inner Orbit (Frequent)</span>
        </span>
        <span class="flex items-center gap-1.5">
          <span class="size-2 rounded-full bg-slate-500"></span>
          <span>Outer Orbit (Branched)</span>
        </span>
        {#if activeCount > 0}
          <span class="flex items-center gap-1.5">
            <span class="size-2 rounded-full bg-emerald-400 animate-pulse"></span>
            <span>Live Stream</span>
          </span>
        {/if}
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
    animation: gateway-cardiac-pulse var(--hb-dur, 5.5s) cubic-bezier(0.4, 0, 0.2, 1) infinite;
  }
</style>
