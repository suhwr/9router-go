<script lang="ts">
  import { fmtTokens } from '../../lib/compactNumber'
  import { costIn } from '../../lib/currency.svelte'
  import { type StatsData } from './types'

  let { stats = {} }: { stats?: StatsData } = $props()

  function fmtSuccessRate(rate?: number | null): string {
    if (rate == null) return '—'
    return `${rate.toFixed(1)}%`
  }

  function rateColor(rate?: number | null): string {
    if (rate == null) return 'text-text-muted'
    if (rate >= 95) return 'text-success'
    if (rate >= 80) return 'text-warning'
    return 'text-red-500'
  }

  function fmtSuccessSubtext(s: StatsData): string {
    const total = s.totalRequests ?? 0
    if (total === 0) return 'No requests recorded'
    const errors = s.totalErrors ?? 0
    const successful = total - errors
    return `${successful.toLocaleString()}/${total.toLocaleString()} success`
  }
</script>

<div class="grid min-w-0 grid-cols-2 gap-3 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-6 sm:gap-3.5">
  <!-- Card 1: Total Requests -->
  <div class="relative overflow-hidden rounded-2xl border border-border/70 bg-gradient-to-b from-surface to-surface-2/40 p-3.5 sm:p-4 shadow-[var(--shadow-soft)] hover:border-border hover:shadow-xs transition-all flex min-w-0 flex-col justify-between">
    <div class="flex items-center justify-between gap-1 mb-1">
      <span class="text-[11px] font-semibold text-text-muted tracking-wide">Total Requests</span>
      <span class="material-symbols-outlined text-[15px] text-text-muted/60">swap_horiz</span>
    </div>
    <span class="truncate text-2xl font-bold tracking-tight text-text-main tabular-nums" title={fmtTokens(stats.totalRequests)}>
      {fmtTokens(stats.totalRequests)}
    </span>
    <span class="text-[10px] text-text-muted/70 truncate mt-0.5">All incoming</span>
  </div>

  <!-- Card 2: Success Rate -->
  <div class="relative overflow-hidden rounded-2xl border border-border/70 bg-gradient-to-b from-surface to-surface-2/40 p-3.5 sm:p-4 shadow-[var(--shadow-soft)] hover:border-border hover:shadow-xs transition-all flex min-w-0 flex-col justify-between">
    <div class="flex items-center justify-between gap-1 mb-1">
      <span class="text-[11px] font-semibold text-text-muted tracking-wide">Success Rate</span>
      <span class="material-symbols-outlined text-[15px] text-emerald-500/80">check_circle</span>
    </div>
    <span class="truncate text-2xl font-bold tracking-tight {rateColor(stats.successRate)} tabular-nums" title={fmtSuccessRate(stats.successRate)}>
      {fmtSuccessRate(stats.successRate)}
    </span>
    <span class="text-[10px] text-text-muted/70 truncate mt-0.5">{fmtSuccessSubtext(stats)}</span>
  </div>

  <!-- Card 3: Total Input Tokens -->
  <div class="relative overflow-hidden rounded-2xl border border-border/70 bg-gradient-to-b from-surface to-surface-2/40 p-3.5 sm:p-4 shadow-[var(--shadow-soft)] hover:border-border hover:shadow-xs transition-all flex min-w-0 flex-col justify-between">
    <div class="flex items-center justify-between gap-1 mb-1">
      <span class="text-[11px] font-semibold text-text-muted tracking-wide">Input Tokens</span>
      <span class="material-symbols-outlined text-[15px] text-primary/80">login</span>
    </div>
    <span class="truncate text-2xl font-bold tracking-tight text-primary tabular-nums" title={fmtTokens(stats.totalPromptTokens)}>
      {fmtTokens(stats.totalPromptTokens)}
    </span>
    <span class="text-[10px] text-text-muted/70 truncate mt-0.5">Prompt payload</span>
  </div>

  <!-- Card 4: Cached Tokens -->
  <div class="relative overflow-hidden rounded-2xl border border-border/70 bg-gradient-to-b from-surface to-surface-2/40 p-3.5 sm:p-4 shadow-[var(--shadow-soft)] hover:border-border hover:shadow-xs transition-all flex min-w-0 flex-col justify-between">
    <div class="flex items-center justify-between gap-1 mb-1">
      <span class="text-[11px] font-semibold text-text-muted tracking-wide">Cached Tokens</span>
      <span class="material-symbols-outlined text-[15px] text-sky-500/80">bolt</span>
    </div>
    <span class="truncate text-2xl font-bold tracking-tight text-info tabular-nums" title={fmtTokens(stats.totalCachedTokens)}>
      {fmtTokens(stats.totalCachedTokens)}
    </span>
    <span class="text-[10px] text-text-muted/70 truncate mt-0.5">Cost-saving hits</span>
  </div>

  <!-- Card 5: Output Tokens -->
  <div class="relative overflow-hidden rounded-2xl border border-border/70 bg-gradient-to-b from-surface to-surface-2/40 p-3.5 sm:p-4 shadow-[var(--shadow-soft)] hover:border-border hover:shadow-xs transition-all flex min-w-0 flex-col justify-between">
    <div class="flex items-center justify-between gap-1 mb-1">
      <span class="text-[11px] font-semibold text-text-muted tracking-wide">Output Tokens</span>
      <span class="material-symbols-outlined text-[15px] text-emerald-500/80">logout</span>
    </div>
    <span class="truncate text-2xl font-bold tracking-tight text-success tabular-nums" title={fmtTokens(stats.totalCompletionTokens)}>
      {fmtTokens(stats.totalCompletionTokens)}
    </span>
    <span class="text-[10px] text-text-muted/70 truncate mt-0.5">Generated output</span>
  </div>

  <!-- Card 6: Estimated Cost -->
  <div class="relative overflow-hidden rounded-2xl border border-border/70 bg-gradient-to-b from-surface to-surface-2/40 p-3.5 sm:p-4 shadow-[var(--shadow-soft)] hover:border-border hover:shadow-xs transition-all flex min-w-0 flex-col justify-between">
    <div class="flex items-center justify-between gap-1 mb-1">
      <span class="text-[11px] font-semibold text-text-muted tracking-wide">Est. Cost</span>
      <span class="material-symbols-outlined text-[15px] text-amber-500/80">payments</span>
    </div>
    <span class="truncate text-2xl font-bold tracking-tight text-warning tabular-nums">
      ~{costIn(stats.totalCost ?? 0)}
    </span>
    <span class="text-[10px] text-text-muted/70 truncate mt-0.5">Estimated, not billed</span>
  </div>
</div>
