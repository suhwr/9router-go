<!--
  PeriodSelect — the window picker shared by the Usage sections.

  The Overview already had a dropdown with a custom-window input; Compression
  Analytics had a four-button strip of uppercase labels (issue #200). Both
  sections render this control now, so they read as one page and neither
  depends on how wide the header happens to be.

  The option list is a prop because the endpoints disagree on what a window may
  be: /api/usage/stats accepts custom <n>d / <n>h windows, while
  /api/analytics/compression resolves only 24h/7d/30d/all and answers anything
  else with its 24h default — so compression passes showCustom={false} rather
  than offering a control that would silently lie.

  The panel used to be centred on the trigger with `-translate-x-1/2`, which
  pushed it past both viewport edges once the header wrapped on a phone. It is
  measured against the trigger and clamped to the viewport by
  lib/ui/menuPosition instead (issue #224).
-->
<script lang="ts">
  import { maxPanelWidth, placePanel, placementStyle } from '../../lib/ui/menuPosition'
  import { portal } from '../../lib/ui/portal'
  import {
    PERIODS,
    HOUR_PERIODS,
    DAY_PERIODS,
    normalizeCustomPeriod,
    periodLabel,
    type Period,
    type PeriodPreset,
  } from './types'

  interface Props {
    value: Period
    options?: { value: PeriodPreset; label: string }[]
    showCustom?: boolean
    busy?: boolean
    onChange?: (period: Period) => void
  }

  let {
    value,
    options = PERIODS,
    showCustom = true,
    busy = false,
    onChange = () => {},
  }: Props = $props()

  let open = $state(false)
  let customInput = $state('')
  let customError = $state('')
  let trigger: HTMLButtonElement | null = $state(null)
  let panel: HTMLDivElement | null = $state(null)
  let panelStyle = $state('')

  /** The panel's preferred width, capped against the viewport at placement. */
  const MIN_WIDTH = '16rem'

  const selectedLabel = $derived(periodLabel(value))
  const isPreset = $derived(options.some((o) => o.value === value))

  function select(next: Period) {
    onChange(next)
    open = false
    customError = ''
  }

  function applyCustom() {
    const normalized = normalizeCustomPeriod(customInput)
    if (!normalized) {
      customError = 'Enter a number of days or hours, like 14d or 12h.'
      return
    }
    select(normalized)
  }

  function place(): void {
    if (!trigger || !panel) return
    panelStyle = `${placementStyle(
      placePanel({
        align: 'right',
        rect: trigger.getBoundingClientRect(),
        panelWidth: panel.offsetWidth,
      }),
    )};max-width:${maxPanelWidth()};min-width:min(${MIN_WIDTH}, ${maxPanelWidth()})`
  }

  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      open = false
      trigger?.focus()
      return
    }
    // A menu left open behind the next control makes the following focusable
    // element unreachable, so Tab closes it instead.
    if (event.key === 'Tab') open = false
  }

  $effect(() => {
    if (!open) return
    function handleDocClick(e: MouseEvent): void {
      const target = e.target as HTMLElement | null
      if (!trigger?.contains(target) && !panel?.contains(target)) open = false
    }
    function reposition(): void {
      place()
    }
    document.addEventListener('click', handleDocClick)
    document.addEventListener('keydown', onKeydown)
    window.addEventListener('resize', reposition)
    window.addEventListener('scroll', reposition, true)
    return () => {
      document.removeEventListener('click', handleDocClick)
      document.removeEventListener('keydown', onKeydown)
      window.removeEventListener('resize', reposition)
      window.removeEventListener('scroll', reposition, true)
    }
  })

  $effect(() => {
    if (open && panel) place()
  })
</script>

<div class="relative flex w-full items-center gap-1.5 sm:w-auto sm:self-auto">
  <button
    bind:this={trigger}
    type="button"
    disabled={busy}
    aria-haspopup="listbox"
    aria-expanded={open}
    onclick={() => (open = !open)}
    onkeydown={onKeydown}
    class="inline-flex items-center gap-2 rounded-xl border border-border bg-surface px-3 py-1.5 text-xs font-medium text-text-main shadow-sm transition-colors hover:bg-surface-2 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/50 disabled:opacity-50 sm:text-sm"
  >
    <span>{selectedLabel}</span>
    {#if !isPreset}
      <span class="rounded-md bg-surface-3 px-1.5 py-0.5 font-code text-[10px] text-text-muted">{value}</span>
    {/if}
    <span class="material-symbols-outlined text-[16px] text-text-muted" aria-hidden="true">
      {open ? 'expand_less' : 'expand_more'}
    </span>
  </button>

  {#if open}
    <div
      use:portal
      bind:this={panel}
      role="listbox"
      aria-label="Time window"
      tabindex="-1"
      style={panelStyle}
      onkeydown={onKeydown}
      class="fixed z-50 w-72 rounded-xl border border-border bg-surface p-3 shadow-[var(--shadow-elev)] space-y-3"
    >
      <!-- Group 1: Hours (Barisan Jam) -->
      <div>
        <span class="text-[10px] font-bold text-text-muted uppercase tracking-wider block mb-1.5 px-0.5 font-mono">
          Hours (Jam)
        </span>
        <div class="grid grid-cols-5 gap-1.5">
          {#each HOUR_PERIODS as o (o.value)}
            <button
              type="button"
              role="option"
              aria-selected={value === o.value}
              onclick={() => select(o.value)}
              class="flex items-center justify-center py-1.5 px-1 rounded-lg text-xs font-mono font-medium transition-all {value === o.value
                ? 'bg-brand-500 text-white shadow-xs font-bold'
                : 'bg-surface-2/70 text-text-muted hover:text-text-main hover:bg-surface-2'}"
              title={o.label}
            >
              {o.short}
            </button>
          {/each}
        </div>
      </div>

      <!-- Group 2: Days & Ranges (Barisan Hari) -->
      <div>
        <span class="text-[10px] font-bold text-text-muted uppercase tracking-wider block mb-1.5 px-0.5 font-mono">
          Days & Ranges (Hari)
        </span>
        <div class="grid grid-cols-3 gap-1.5">
          {#each DAY_PERIODS as o (o.value)}
            <button
              type="button"
              role="option"
              aria-selected={value === o.value}
              onclick={() => select(o.value)}
              class="flex items-center justify-center py-1.5 px-2 rounded-lg text-xs font-medium transition-all {value === o.value
                ? 'bg-brand-500 text-white shadow-xs font-bold'
                : 'bg-surface-2/70 text-text-muted hover:text-text-main hover:bg-surface-2'}"
              title={o.label}
            >
              {o.short}
            </button>
          {/each}
        </div>
      </div>

      {#if showCustom}
        <div class="border-t border-border-subtle pt-2.5">
          <label for="custom-period" class="text-[10px] font-bold text-text-muted uppercase tracking-wider block mb-1 px-0.5 font-mono">
            Custom Window
          </label>
          <div class="flex items-center gap-1.5">
            <input
              id="custom-period"
              type="text"
              placeholder="e.g. 14d or 12h"
              bind:value={customInput}
              onkeydown={(e) => e.key === 'Enter' && applyCustom()}
              aria-describedby={customError ? 'period-custom-error' : undefined}
              aria-invalid={customError ? 'true' : undefined}
              class="min-w-0 flex-1 rounded-lg border border-border bg-bg px-2.5 py-1.5 font-code text-xs text-text-main outline-none transition-colors placeholder:text-text-subtle focus:border-brand-500"
            />
            <button
              type="button"
              onclick={applyCustom}
              class="shrink-0 rounded-lg bg-brand-500 px-3 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-primary-hover focus:outline-none"
            >
              Set
            </button>
          </div>
          {#if customError}
            <p id="period-custom-error" class="mt-1 text-[10px] text-red-500" role="alert">
              {customError}
            </p>
          {/if}
        </div>
      {/if}
    </div>
  {/if}

  {#if busy}
    <span class="h-2 w-2 animate-ping rounded-full bg-brand-500" aria-hidden="true"></span>
  {/if}
</div>