<script lang="ts">
  import { Layers, Plus, HelpCircle, ChevronDown, ChevronUp } from 'lucide-svelte'
  import Button from '../../lib/ui/Button.svelte'

  interface Props {
    onCreateClick: () => void
  }

  let { onCreateClick }: Props = $props()
  let showInfo = $state(false)
</script>

<div class="flex flex-col gap-3 rounded-xl border border-border/60 bg-surface-1/50 p-3.5 sm:p-4 backdrop-blur-xs">
  <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
    <div class="flex items-center gap-2.5 min-w-0">
      <div class="size-8 sm:size-9 rounded-lg bg-brand-500/10 text-brand-500 flex items-center justify-center shrink-0">
        <Layers class="w-4 h-4 sm:w-4.5 sm:h-4.5" />
      </div>
      <div class="min-w-0">
        <h2 class="text-sm sm:text-base font-semibold text-text-main flex items-center gap-2">
          Model Combos
          <button
            type="button"
            onclick={() => (showInfo = !showInfo)}
            class="text-text-muted hover:text-brand-500 transition-colors p-0.5 rounded cursor-pointer"
            title="Toggle strategy explanation"
          >
            <HelpCircle class="w-3.5 h-3.5" />
          </button>
        </h2>
        <p class="text-xs text-text-muted truncate">
          Group models under a unified endpoint with automated routing & fallbacks
        </p>
      </div>
    </div>

    <div class="flex items-center gap-2 shrink-0">
      <Button icon="add" size="sm" onclick={onCreateClick} class="w-full sm:w-auto font-medium shadow-xs">
        Create Combo
      </Button>
    </div>
  </div>

  {#if showInfo}
    <div class="mt-2 pt-3 border-t border-border/50 grid grid-cols-1 md:grid-cols-3 gap-2.5 text-xs text-text-muted">
      <div class="p-2.5 rounded-lg bg-surface-2/60 border border-border/40">
        <div class="font-medium text-text-main flex items-center gap-1.5 mb-1">
          <span class="size-1.5 rounded-full bg-emerald-500"></span>
          Fallback
        </div>
        <p class="text-[11px] leading-relaxed">Tries models sequentially in priority order. Fails over immediately to the next model upon rate limit or failure.</p>
      </div>
      <div class="p-2.5 rounded-lg bg-surface-2/60 border border-border/40">
        <div class="font-medium text-text-main flex items-center gap-1.5 mb-1">
          <span class="size-1.5 rounded-full bg-blue-500"></span>
          Round Robin
        </div>
        <p class="text-[11px] leading-relaxed">Rotates traffic evenly across models on each request to spread quota usage and prevent throttling.</p>
      </div>
      <div class="p-2.5 rounded-lg bg-surface-2/60 border border-border/40">
        <div class="font-medium text-text-main flex items-center gap-1.5 mb-1">
          <span class="size-1.5 rounded-full bg-purple-500"></span>
          Fusion & Smart
        </div>
        <p class="text-[11px] leading-relaxed">Fusion queries panel models in parallel with judge synthesis. Smart Router dynamically analyzes intent.</p>
      </div>
    </div>
  {/if}
</div>
