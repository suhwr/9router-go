<script lang="ts">
  // Usage trend chart for the Overview section.
  // Drawn as plain SVG with full support for both Tokens and Costs modes,
  // grid lines with scale reference, and distributed timeline markers.
  import { Loader2 } from 'lucide-svelte'
  import { api } from '../../api/client'
  import { fmtTokens } from '../../lib/compactNumber'
  import { costIn } from '../../lib/currency.svelte'

  interface Props {
    period?: string
    /**
     * Which metric the chart draws (tokens or costs).
     */
    viewMode?: 'tokens' | 'costs'
    /** Whether the buckets are hourly. */
    hourly?: boolean
  }

  interface ChartBucket {
    label: string
    tokens: number
    cost: number
    byModel: Record<string, number>
  }

  interface Series {
    family: string
    color: string
    total: number
  }

  let {
    period = '7d',
    viewMode = $bindable('tokens'),
    hourly = $bindable(false),
  }: Props = $props()

  let buckets = $state<ChartBucket[]>([])
  let loading = $state(true)
  let failed = $state(false)
  let hovered = $state<number | null>(null)

  const WIDTH = 640
  const HEIGHT = 200
  const MAX_SERIES = 6
  const FAMILY_COLORS = ['#6366f1', '#f59e0b', '#10b981', '#ef4444', '#8b5cf6', '#06b6d4', '#f43f5e', '#84cc16']
  const OTHER_COLOR = '#64748b'
  const CHART_TIMEOUT_MS = 20000

  $effect(() => {
    const wanted = period
    let cancelled = false
    let settled = false
    loading = true
    failed = false
    const timer = setTimeout(() => {
      if (settled || cancelled) return
      settled = true
      failed = true
      loading = false
    }, CHART_TIMEOUT_MS)
    api
      .getUsageChart(wanted)
      .then((data) => {
        if (cancelled || settled) return
        settled = true
        clearTimeout(timer)
        buckets = Array.isArray(data) ? data : []
        hourly = /^\d\d:\d\d$/.test(buckets[0]?.label || '')
        loading = false
      })
      .catch(() => {
        if (cancelled || settled) return
        settled = true
        clearTimeout(timer)
        buckets = []
        failed = true
        loading = false
      })
    return () => {
      cancelled = true
      clearTimeout(timer)
    }
  })

  // Rank families by period total based on active viewMode (costs vs tokens)
  const families = $derived.by((): Series[] => {
    const totals = new Map<string, number>()
    for (const b of buckets) {
      if (viewMode === 'costs') {
        const bTokens = b.tokens || 0
        const bCost = b.cost || 0
        for (const [family, tokens] of Object.entries(b.byModel || {})) {
          const famCost = bTokens > 0 ? ((tokens || 0) / bTokens) * bCost : 0
          totals.set(family, (totals.get(family) || 0) + famCost)
        }
      } else {
        for (const [family, tokens] of Object.entries(b.byModel || {})) {
          totals.set(family, (totals.get(family) || 0) + (tokens || 0))
        }
      }
    }
    const ranked = [...totals.entries()].sort((a, b) => b[1] - a[1])
    const kept = ranked.slice(0, MAX_SERIES)
    const tail = ranked.slice(MAX_SERIES)
    const series: Series[] = kept.map(([family, total], i) => ({
      family,
      total,
      color: FAMILY_COLORS[i % FAMILY_COLORS.length],
    }))
    if (tail.length > 0) {
      series.push({
        family: 'other',
        total: tail.reduce((sum, [, t]) => sum + t, 0),
        color: OTHER_COLOR,
      })
    }
    return series
  })

  const keptFamilies = $derived(families.slice(0, MAX_SERIES).map((s) => s.family))

  // Per-bucket stack based on viewMode
  const stack = $derived(
    buckets.map((b) => {
      const bTokens = b.tokens || 0
      const bCost = b.cost || 0
      const values = families.map((s) => {
        if (s.family === 'other') {
          const otherTokens = Object.entries(b.byModel || {})
            .filter(([family]) => !keptFamilies.includes(family))
            .reduce((sum, [, t]) => sum + (t || 0), 0)
          return viewMode === 'costs'
            ? (bTokens > 0 ? (otherTokens / bTokens) * bCost : 0)
            : otherTokens
        }
        const famTokens = b.byModel?.[s.family] || 0
        return viewMode === 'costs'
          ? (bTokens > 0 ? (famTokens / bTokens) * bCost : 0)
          : famTokens
      })
      const cumulative: number[] = []
      let acc = 0
      for (const v of values) {
        acc += v
        cumulative.push(acc)
      }
      const bucketTotal = viewMode === 'costs' ? bCost : acc
      return { bucket: b, cumulative, total: bucketTotal }
    })
  )

  const maxTotal = $derived(Math.max(viewMode === 'costs' ? 0.0001 : 1, ...stack.map((s) => s.total)))
  const hasData = $derived(stack.some((s) => s.total > 0))
  const step = $derived(buckets.length > 1 ? WIDTH / (buckets.length - 1) : WIDTH)

  function xAt(index: number): number {
    return buckets.length > 1 ? index * step : WIDTH / 2
  }

  function yAt(value: number): number {
    return HEIGHT - 20 - (value / maxTotal) * (HEIGHT - 35)
  }

  let focusedPoint = $state(0)

  function handlePointKey(event: KeyboardEvent) {
    const index = focusedPoint
    let next = index
    if (event.key === 'ArrowRight') next = index + 1
    else if (event.key === 'ArrowLeft') next = index - 1
    else if (event.key === 'Home') next = 0
    else if (event.key === 'End') next = stack.length - 1
    else if (event.key === 'Escape') {
      event.preventDefault()
      hovered = null
      return
    } else return

    event.preventDefault()
    next = Math.max(0, Math.min(stack.length - 1, next))
    focusedPoint = next
    hovered = next
    const el = event.currentTarget as SVGElement
    const svg = el.ownerSVGElement
    svg?.querySelectorAll<SVGRectElement>('rect')?.[next]?.focus()
  }

  let tableOpen = $state(false)

  function areaPath(band: number): string {
    if (stack.length === 0) return ''
    const top = stack.map((s, i) => `${xAt(i)},${yAt(s.cumulative[band] || 0)}`).join(' L ')
    const bottom = stack
      .map((s, i) => {
        const idx = stack.length - 1 - i
        const lower = band === 0 ? 0 : s.cumulative[band - 1] || 0
        return `${xAt(idx)},${yAt(lower)}`
      })
      .join(' L ')
    return `M ${top} L ${bottom} Z`
  }

  // Generate 4-6 evenly spaced X-axis timeline tick indices
  const xTickIndices = $derived.by((): number[] => {
    if (buckets.length <= 1) return [0]
    if (buckets.length <= 6) return buckets.map((_, i) => i)
    const count = 5
    const indices: number[] = []
    for (let i = 0; i < count; i++) {
      indices.push(Math.round((i * (buckets.length - 1)) / (count - 1)))
    }
    return indices
  })

  function formatValue(v: number): string {
    return viewMode === 'costs' ? costIn(v) : fmtTokens(v)
  }
</script>

<div class="flex min-w-0 flex-col gap-3">
  {#if loading}
    <div class="flex items-center justify-center py-12 text-text-muted text-xs">
      <Loader2 class="size-5 animate-spin mr-2 text-brand-500" />
      Loading chart...
    </div>
  {:else if failed}
    <div class="py-10 text-center text-xs text-text-muted">Usage chart could not be loaded.</div>
  {:else if !hasData}
    <div class="py-10 text-center text-xs text-text-muted">No usage recorded in this period.</div>
  {:else}
    <div class="min-w-0">
      <svg viewBox={`0 0 ${WIDTH} ${HEIGHT}`} class="w-full h-auto overflow-visible" role="img" aria-label="Usage trend chart">
        <!-- Horizontal Grid Lines and Y-Axis Scale Reference -->
        <g class="text-[10px] fill-text-muted/60 stroke-border-subtle/50 font-mono" stroke-dasharray="3 3">
          <!-- Top line 100% -->
          <line x1="0" y1={yAt(maxTotal)} x2={WIDTH} y2={yAt(maxTotal)} />
          <text x="4" y={Math.max(yAt(maxTotal) - 4, 10)} fill="currentColor" stroke="none">{formatValue(maxTotal)}</text>
          
          <!-- Middle line 50% -->
          <line x1="0" y1={yAt(maxTotal / 2)} x2={WIDTH} y2={yAt(maxTotal / 2)} />
          <text x="4" y={yAt(maxTotal / 2) - 4} fill="currentColor" stroke="none">{formatValue(maxTotal / 2)}</text>
          
          <!-- Baseline 0 -->
          <line x1="0" y1={HEIGHT - 20} x2={WIDTH} y2={HEIGHT - 20} stroke-dasharray="none" class="stroke-border/70" />
          <text x="4" y={HEIGHT - 24} fill="currentColor" stroke="none">0</text>
        </g>

        <!-- Stacked Areas -->
        {#each families as s, band (s.family)}
          <path d={areaPath(band)} fill={s.color} fill-opacity="0.45" stroke={s.color} stroke-width="1.5" />
        {/each}

        <!-- Interactive Columns -->
        {#each stack as _point, i}
          <rect
            tabindex={i === focusedPoint ? 0 : -1}
            aria-label="{stack[i].bucket.label}: {formatValue(stack[i].total)}"
            x={xAt(i) - step / 2}
            y="0"
            width={step}
            height={HEIGHT - 20}
            fill="transparent"
            onmouseenter={() => (hovered = i)}
            onmouseleave={() => (hovered = null)}
            onfocus={() => {
              focusedPoint = i
              hovered = i
            }}
            onblur={() => (hovered = null)}
            onkeydown={handlePointKey}
          />
        {/each}

        <!-- Hover Indicator Line -->
        {#if hovered !== null && stack[hovered]}
          <line
            x1={xAt(hovered)}
            x2={xAt(hovered)}
            y1="0"
            y2={HEIGHT - 20}
            stroke="currentColor"
            stroke-width="1"
            stroke-dasharray="3 3"
            opacity="0.5"
          />
        {/if}
      </svg>

      <!-- Hover Tooltip -->
      {#if hovered !== null && stack[hovered]}
        {@const b = stack[hovered].bucket}
        <div
          role="status"
          aria-live="polite"
          class="mt-2.5 rounded-xl border border-border bg-surface-2 p-3 text-xs text-text-main shadow-md"
        >
          <div class="flex items-center justify-between pb-1.5 border-b border-border-subtle/70 mb-2">
            <span class="font-bold text-text-main flex items-center gap-1.5">
              <span class="material-symbols-outlined notranslate text-[14px] text-text-muted" translate="no">calendar_today</span>
              {b.label}
            </span>
            <span class="font-mono font-bold {viewMode === 'costs' ? 'text-amber-500' : 'text-brand-500'}">
              {viewMode === 'costs' ? costIn(b.cost ?? 0) : `${fmtTokens(b.tokens)} tokens`}
            </span>
          </div>
          <div class="grid grid-cols-2 gap-2 text-[11px] mb-2 font-mono">
            <div class="flex justify-between bg-surface/70 px-2 py-1 rounded-md border border-border-subtle/50">
              <span class="text-text-muted">Total tokens:</span>
              <span class="font-bold">{fmtTokens(b.tokens)}</span>
            </div>
            <div class="flex justify-between bg-surface/70 px-2 py-1 rounded-md border border-border-subtle/50">
              <span class="text-text-muted">Est. cost:</span>
              <span class="font-bold text-warning">~{costIn(b.cost ?? 0)}</span>
            </div>
          </div>
          {#if families.length > 0}
            <div class="space-y-1 pt-1 border-t border-border-subtle/50">
              {#each families as s, band (s.family)}
                {@const val = stack[hovered].cumulative[band] - (band > 0 ? stack[hovered].cumulative[band - 1] : 0)}
                {#if val > 0}
                  <div class="flex items-center justify-between gap-3 text-[11px]">
                    <span class="flex items-center gap-1.5 truncate">
                      <span class="w-2.5 h-2.5 rounded-xs shrink-0" style="background: {s.color}"></span>
                      <span class="truncate">{s.family}</span>
                    </span>
                    <span class="font-mono text-text-muted shrink-0">{formatValue(val)}</span>
                  </div>
                {/if}
              {/each}
            </div>
          {/if}
        </div>
      {/if}
    </div>

    <!-- Timeline Labels (Distributed) -->
    <div class="flex justify-between items-center text-[10px] text-text-muted font-mono pt-1 px-1">
      {#each xTickIndices as idx}
        <span>{buckets[idx]?.label || ''}</span>
      {/each}
    </div>

    <!-- Series Legend & Peak -->
    <div class="flex flex-wrap items-center gap-x-4 gap-y-1 text-[11px] text-text-muted pt-2 border-t border-border-subtle">
      {#each families as s (s.family)}
        <span class="flex items-center gap-1.5">
          <span class="w-2.5 h-2.5 rounded-xs" style="background: {s.color}"></span>
          {s.family}
        </span>
      {/each}
      <span class="ml-auto font-mono font-semibold text-text-main">
        Peak: {formatValue(maxTotal)}
      </span>
    </div>

    <!-- Tabular Breakdown -->
    <details class="text-[11px] text-text-muted" bind:open={tableOpen}>
      <summary class="cursor-pointer select-none">View table</summary>
      {#if tableOpen}
        <table class="mt-2 w-full border-collapse text-left" aria-label="Usage trend table">
          <thead>
            <tr class="border-b border-border-subtle">
              <th scope="col" class="py-1 pr-2 font-medium">{hourly ? 'Hour' : 'Day'}</th>
              <th scope="col" class="py-1 pr-2 text-right font-medium">Tokens</th>
              <th scope="col" class="py-1 text-right font-medium">Est. cost</th>
            </tr>
          </thead>
          <tbody>
            {#each stack as point, i (i)}
              <tr class="border-b border-border-subtle/60">
                <th scope="row" class="py-1 pr-2 font-normal text-text-main">{point.bucket.label}</th>
                <td class="py-1 pr-2 text-right font-mono">{fmtTokens(point.bucket.tokens)}</td>
                <td class="py-1 text-right font-mono">~{costIn(point.bucket.cost ?? 0)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}
    </details>
  {/if}
</div>
