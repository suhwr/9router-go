<script lang="ts">
  // Port of Next.js ProviderLimits/QuotaTable.js for the Go dashboard.
  import {
    formatResetTime,
    formatResetTimeDisplay,
    getRemainingPercentage,
    getStatusColor,
    getStatusEmoji,
    type NormalizedQuota,
  } from './types'

  interface Props {
    quotas?: NormalizedQuota[]
    compact?: boolean
    sortMode?: 'default' | 'remaining-asc' | 'remaining-desc'
    showSortLabel?: boolean
    onHideQuota?: ((quota: NormalizedQuota) => void) | null
  }

  let {
    quotas = [],
    compact = false,
    sortMode = 'default',
    showSortLabel = false,
    onHideQuota = null,
  }: Props = $props()

  const PAGE_SIZE = 10

  let page = $state(1)

  // `remainingPct` is the *rendered* share (0-100, or null when the row says
  // nothing). It is a separate field from the raw `remaining` the provider
  // reported, because those two are not the same number — upstream overwrites
  // one with the other, and here the raw value must survive for the hide
  // action while the percentage drives the bar, the colour and the sort.
  interface RowQuota extends NormalizedQuota {
    index: number
    remainingPct: number | null
  }

  let normalizedQuotas = $derived<RowQuota[]>(
    quotas.map((quota, index) => ({
      ...quota,
      index,
      remainingPct: getRemainingPercentage(quota),
    })),
  )

  // An unknown row sorts as -1, below every real reading (upstream parity), so
  // it never displaces a row whose percentage is actually known.
  let sortedQuotas = $derived.by<RowQuota[]>(() => {
    if (sortMode === 'remaining-asc') {
      return [...normalizedQuotas].sort(
        (a, b) => (a.remainingPct ?? -1) - (b.remainingPct ?? -1) || a.name.localeCompare(b.name),
      )
    }
    if (sortMode === 'remaining-desc') {
      return [...normalizedQuotas].sort(
        (a, b) => (b.remainingPct ?? -1) - (a.remainingPct ?? -1) || a.name.localeCompare(b.name),
      )
    }
    return normalizedQuotas
  })

  let totalPages = $derived(Math.max(1, Math.ceil(sortedQuotas.length / PAGE_SIZE)))

  $effect(() => {
    // Reset to first page when the data or sort changes
    void sortMode
    void quotas
    page = 1
  })

  $effect(() => {
    if (page > totalPages) page = totalPages
  })

  function colorClassesFor(remaining: number | null) {
    // Unknown reads gray, not red: a quota with no reading is not a drained
    // quota, and red would report an alarm that does not exist.
    if (remaining === null || !Number.isFinite(remaining)) {
      return {
        text: 'text-text-muted',
        bg: 'bg-neutral-400 dark:bg-neutral-600',
        bgLight: 'bg-neutral-500/10',
        border: 'border-border/50',
        icon: 'help',
        iconColor: 'text-text-muted',
      }
    }
    if (remaining > 70) {
      return {
        text: 'text-emerald-600 dark:text-emerald-400',
        bg: 'bg-emerald-500',
        bgLight: 'bg-emerald-500/15',
        border: 'border-emerald-500/30',
        icon: 'check_circle',
        iconColor: 'text-emerald-500',
      }
    }
    if (remaining >= 30) {
      return {
        text: 'text-amber-600 dark:text-amber-400',
        bg: 'bg-amber-500',
        bgLight: 'bg-amber-500/15',
        border: 'border-amber-500/30',
        icon: 'warning',
        iconColor: 'text-amber-500',
      }
    }
    return {
      text: 'text-rose-600 dark:text-rose-400',
      bg: 'bg-rose-500',
      bgLight: 'bg-rose-500/15',
      border: 'border-rose-500/30',
      icon: 'error',
      iconColor: 'text-rose-500',
    }
  }

  const cellPad = $derived(compact ? 'py-1 px-1.5' : 'py-2 px-3')
  const nameText = $derived(compact ? 'text-[11px]' : 'text-sm')
  const resetPrimary = $derived(compact ? 'text-[11px]' : 'text-sm')
  const resetSecondary = $derived(compact ? 'text-[10px] leading-tight' : 'text-xs')

  function fmtNum(n?: number): string {
    return (n || 0).toLocaleString()
  }
</script>

{#if quotas && quotas.length > 0}
  <div class="space-y-1.5">
    <div class="flex items-center justify-between gap-2 px-0.5">
      <div class="text-[10px] font-medium text-text-muted">
        {sortedQuotas.length} quota{sortedQuotas.length > 1 ? 's' : ''}
      </div>
      {#if showSortLabel}
        <div
          class="rounded border border-border-subtle bg-surface-2 px-1.5 py-0.5 text-[9px] text-text-muted"
        >
          Sorted by remaining
        </div>
      {/if}
    </div>

    <div class="space-y-1">
      {#each sortedQuotas.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE) as quota (
        `${quota.name}-${quota.index}`
      )}
        {@const isUnlimited = quota.unlimited === true}
        {@const isCreditBalance = quota.isCreditBalance === true}
        {@const isUnknown = quota.remainingPct === null}
        {@const colors = isCreditBalance
          ? {
              text: 'text-blue-600 dark:text-blue-400',
              bg: 'bg-blue-500',
              bgLight: 'bg-blue-500/15',
              border: 'border-blue-500/30',
              icon: 'payments',
              iconColor: 'text-blue-500',
            }
          : colorClassesFor((quota.colorPercentage as number | undefined) ?? quota.remainingPct)}
        {@const countdown = formatResetTime(quota.resetAt)}
        {@const resetDisplay = (quota.emptyMessage as string | undefined) || formatResetTimeDisplay(quota.resetAt)}
        {@const recurring = quota.recurring !== false}
        {@const countdownLabel = recurring ? `in ${countdown}` : `expires in ${countdown}`}

        <div
          class="group/row flex flex-col gap-1 rounded-lg border border-border/70 bg-surface/80 px-2.5 py-1.5 transition-all hover:bg-surface-2 hover:border-border shadow-2xs"
        >
          <!-- Line 1: Icon, Name, Usage/Countdown, Percentage Pill, Hide -->
          <div class="flex items-center justify-between gap-1.5 min-w-0">
            <div class="flex items-center gap-1.5 min-w-0 flex-1">
              <span class="material-symbols-outlined notranslate text-[13px] {colors.iconColor} shrink-0" translate="no">{colors.icon}</span>
              <span class="text-xs font-semibold text-text-main truncate max-w-[130px] sm:max-w-[200px]" title={quota.name}>
                {quota.name}
              </span>
              {#if countdown !== '-' || resetDisplay}
                <span class="hidden sm:inline-flex items-center gap-1 text-[10px] text-text-muted font-medium ml-1 truncate">
                  <span class="material-symbols-outlined notranslate text-[12px] opacity-60" translate="no">schedule</span>
                  {countdown !== '-' ? countdownLabel : resetDisplay}
                </span>
              {/if}
            </div>

            <div class="flex items-center gap-1.5 shrink-0">
              <span
                class="rounded-md px-1.5 py-0.5 text-[10px] font-bold tabular-nums border {isUnlimited
                  ? 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border-emerald-500/30'
                  : isCreditBalance
                    ? 'bg-blue-500/15 text-blue-600 dark:text-blue-400 border-blue-500/30'
                    : isUnknown
                      ? 'bg-surface-3 text-text-muted border-border/50'
                      : `${colors.bgLight} ${colors.text} ${colors.border}`}"
              >
                {isUnlimited ? 'Unlimited' : isCreditBalance ? `${(quota.total || 0).toFixed(2)} ${quota.currency || ''}` : isUnknown ? '—' : `${quota.remainingPct}%`}
              </span>
              {#if typeof onHideQuota === 'function'}
                <button
                  type="button"
                  onclick={() => onHideQuota?.(quota)}
                  class="inline-flex h-5 w-5 shrink-0 items-center justify-center rounded text-text-muted/60 opacity-60 hover:opacity-100 transition-opacity hover:bg-surface-3 hover:text-text-main cursor-pointer"
                  title="Hide this quota row"
                  aria-label="Hide quota {quota.name}"
                >
                  <span class="material-symbols-outlined notranslate text-[13px]" translate="no">visibility_off</span>
                </button>
              {/if}
            </div>
          </div>

          <!-- Line 2: Slim Progress Bar -->
          {#if !isUnlimited && !isCreditBalance && !isUnknown}
            <div class="h-1 w-full rounded-full overflow-hidden bg-black/5 dark:bg-white/10">
              <div
                class="h-full transition-all duration-300 rounded-full {colors.bg}"
                style:width="{Math.min(quota.remainingPct ?? 0, 100)}%"
              ></div>
            </div>
          {/if}

          <!-- Line 3: Compact info (mobile countdown + numbers) -->
          <div class="flex items-center justify-between gap-1 text-[10px] text-text-muted font-mono leading-none">
            <span class="truncate">
              {isUnlimited
                ? `${fmtNum(quota.used)} used`
                : isCreditBalance
                  ? `Credit: ${(quota.total || 0).toFixed(2)} ${quota.currency || ''}`
                  : `${fmtNum(quota.used)} / ${(quota.total || 0) > 0 ? fmtNum(quota.total) : '∞'}`}
            </span>
            {#if (countdown !== '-' || resetDisplay)}
              <span class="sm:hidden inline-flex items-center gap-0.5 text-[9px] font-sans font-medium text-text-muted shrink-0">
                <span class="material-symbols-outlined notranslate text-[11px] opacity-60" translate="no">schedule</span>
                {countdown !== '-' ? countdownLabel : resetDisplay}
              </span>
            {/if}
          </div>
        </div>
      {/each}
    </div>

    {#if totalPages > 1}
      <div class="rounded-md border border-border-subtle bg-surface-2 px-2 py-1.5">
        <div class="flex items-center justify-between gap-2 text-[10px] text-text-muted">
          <span>
            Showing {(page - 1) * PAGE_SIZE + 1}-{Math.min(page * PAGE_SIZE, sortedQuotas.length)}
            of {sortedQuotas.length}
          </span>
          <span>
            Page {page} / {totalPages}
          </span>
        </div>
        <div class="mt-1.5 flex items-center justify-end gap-1">
          <button
            type="button"
            onclick={() => (page = Math.max(1, page - 1))}
            disabled={page === 1}
            class="flex h-6 items-center rounded-md border border-border-subtle px-2 text-[10px] text-text-main transition-colors hover:bg-surface-3 disabled:cursor-not-allowed disabled:opacity-40"
          >
            Prev
          </button>
          <button
            type="button"
            onclick={() => (page = Math.min(totalPages, page + 1))}
            disabled={page === totalPages}
            class="flex h-6 items-center rounded-md border border-border-subtle px-2 text-[10px] text-text-main transition-colors hover:bg-surface-3 disabled:cursor-not-allowed disabled:opacity-40"
          >
            Next
          </button>
        </div>
      </div>
    {/if}
  </div>
{/if}
