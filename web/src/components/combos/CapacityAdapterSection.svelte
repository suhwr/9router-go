<script lang="ts">
  import { ArrowDown, ArrowUp, Brain, Eye, Headphones, Plus, X } from 'lucide-svelte'
  import Button from '../../lib/ui/Button.svelte'
  import Card from '../../lib/ui/Card.svelte'
  import Toggle from '../../lib/ui/Toggle.svelte'
  import type { CapacityAdapterState } from './types'
  import { getModelCaps } from '../../lib/models'

  interface Props {
    capacityAdapter: CapacityAdapterState
    onSaveAdapter: (next: CapacityAdapterState) => void
    onOpenModelPicker: (target: 'vision' | 'audio') => void
  }

  let {
    capacityAdapter,
    onSaveAdapter,
    onOpenModelPicker,
  }: Props = $props()

  let expandedVision = $state(true)
  let expandedAudio = $state(true)

  function toggleAdapter(type: 'vision' | 'audioInput', enabled: boolean) {
    onSaveAdapter({
      ...capacityAdapter,
      [type]: { ...capacityAdapter[type], enabled },
    })
  }

  function toggleAdapterRoundRobin(type: 'vision' | 'audioInput', roundRobin: boolean) {
    onSaveAdapter({
      ...capacityAdapter,
      [type]: { ...capacityAdapter[type], roundRobin },
    })
  }

  function moveAdapterModel(type: 'vision' | 'audioInput', index: number, delta: number) {
    const arr = [...capacityAdapter[type].models]
    const target = index + delta
    if (target < 0 || target >= arr.length) return
    const temp = arr[index]
    arr[index] = arr[target]
    arr[target] = temp
    onSaveAdapter({
      ...capacityAdapter,
      [type]: { ...capacityAdapter[type], models: arr },
    })
  }

  function removeAdapterModel(type: 'vision' | 'audioInput', index: number) {
    const arr = capacityAdapter[type].models.filter((_, i) => i !== index)
    onSaveAdapter({
      ...capacityAdapter,
      [type]: { ...capacityAdapter[type], models: arr },
    })
  }
</script>

<div class="flex flex-col gap-3">
  <div class="flex items-center justify-between gap-2">
    <div>
      <h3 class="text-sm font-semibold text-text-main flex items-center gap-1.5">
        <Eye class="w-4 h-4 text-brand-500" />
        Capacity Adapters (Vision & Audio Fallback)
      </h3>
      <p class="text-xs text-text-muted mt-0.5">
        Auto-switches to the ordered models below if a model lacks vision or audio capabilities
      </p>
    </div>
  </div>

  <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
    <!-- Vision Adapter Card -->
    <Card padding="sm" class="flex flex-col gap-2.5 transition-all {!capacityAdapter.vision.enabled ? 'opacity-60 bg-surface-2/40' : ''}">
      <div class="flex items-center justify-between gap-2 border-b border-border/40 pb-2">
        <div class="flex items-center gap-2 min-w-0">
          <Toggle
            checked={capacityAdapter.vision.enabled}
            onChange={(v) => toggleAdapter('vision', v)}
          />
          <div class="size-7 rounded-lg bg-blue-500/10 text-blue-500 flex items-center justify-center shrink-0">
            <Eye class="w-3.5 h-3.5" />
          </div>
          <div class="min-w-0">
            <div class="flex items-center gap-1.5 font-semibold text-xs text-text-main">
              Vision Adapter
              <span class="text-[10px] font-normal text-text-muted">({capacityAdapter.vision.models.length} models)</span>
            </div>
          </div>
        </div>

        <div class="flex items-center gap-2 shrink-0">
          <label class="flex items-center gap-1.5 text-xs text-text-muted cursor-pointer select-none">
            <Toggle
              size="sm"
              checked={capacityAdapter.vision.roundRobin}
              disabled={!capacityAdapter.vision.enabled}
              onChange={(v) => toggleAdapterRoundRobin('vision', v)}
            />
            <span class="text-[11px]">Round Robin</span>
          </label>
          <Button
            icon="add"
            variant="outline"
            size="sm"
            onclick={() => onOpenModelPicker('vision')}
            disabled={!capacityAdapter.vision.enabled}
            title="Add Vision model"
          >
            Add
          </Button>
        </div>
      </div>

      <!-- Vision Models List with explicit Priority Order & Touch-Friendly Reorder -->
      <div class="flex flex-col gap-1.5 min-w-0">
        {#if capacityAdapter.vision.models.length === 0}
          <div class="py-3 text-center text-xs text-text-muted italic border border-dashed border-border/60 rounded-lg">
            No vision models configured — click "Add" to select
          </div>
        {:else}
          <div class="flex flex-col gap-1 max-h-[220px] overflow-y-auto custom-scrollbar pr-0.5">
            {#each capacityAdapter.vision.models as model, idx}
              {@const vcaps = getModelCaps(model)}
              <div
                class="flex items-center justify-between gap-1.5 rounded-lg border border-border/60 bg-surface-2/60 px-2.5 py-1.5 text-xs font-mono text-text-main transition-colors hover:bg-surface-2"
              >
                <div class="flex items-center gap-1.5 min-w-0 flex-1">
                  <span class="text-[10px] font-semibold text-text-muted w-4 shrink-0 font-sans">#{idx + 1}</span>
                  <span class="truncate max-w-[160px] sm:max-w-[220px]">{model}</span>
                  {#if vcaps.vision}
                    <Eye class="w-3 h-3 text-blue-500 shrink-0" title="Vision" />
                  {/if}
                  {#if vcaps.reasoning}
                    <Brain class="w-3 h-3 text-amber-500 shrink-0" title="Reasoning" />
                  {/if}
                </div>

                <!-- Touch-friendly Reorder controls (Always Visible!) -->
                <div class="flex items-center gap-1 shrink-0">
                  <button
                    type="button"
                    onclick={() => moveAdapterModel('vision', idx, -1)}
                    disabled={idx === 0}
                    class="size-6 flex items-center justify-center rounded border border-border/50 bg-surface transition-colors {idx === 0
                      ? 'text-text-muted/20 cursor-not-allowed'
                      : 'text-text-muted hover:text-brand-500 hover:bg-surface-3 cursor-pointer'}"
                    title="Move up in priority order"
                  >
                    <ArrowUp class="w-3 h-3" />
                  </button>
                  <button
                    type="button"
                    onclick={() => moveAdapterModel('vision', idx, 1)}
                    disabled={idx === capacityAdapter.vision.models.length - 1}
                    class="size-6 flex items-center justify-center rounded border border-border/50 bg-surface transition-colors {idx === capacityAdapter.vision.models.length - 1
                      ? 'text-text-muted/20 cursor-not-allowed'
                      : 'text-text-muted hover:text-brand-500 hover:bg-surface-3 cursor-pointer'}"
                    title="Move down in priority order"
                  >
                    <ArrowDown class="w-3 h-3" />
                  </button>
                  <button
                    type="button"
                    onclick={() => removeAdapterModel('vision', idx)}
                    class="size-6 flex items-center justify-center rounded border border-border/50 bg-surface text-text-muted hover:text-rose-500 hover:bg-rose-500/10 transition-colors cursor-pointer"
                    title="Remove model"
                  >
                    <X class="w-3 h-3" />
                  </button>
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    </Card>

    <!-- Audio Adapter Card -->
    <Card padding="sm" class="flex flex-col gap-2.5 transition-all {!capacityAdapter.audioInput.enabled ? 'opacity-60 bg-surface-2/40' : ''}">
      <div class="flex items-center justify-between gap-2 border-b border-border/40 pb-2">
        <div class="flex items-center gap-2 min-w-0">
          <Toggle
            checked={capacityAdapter.audioInput.enabled}
            onChange={(v) => toggleAdapter('audioInput', v)}
          />
          <div class="size-7 rounded-lg bg-emerald-500/10 text-emerald-500 flex items-center justify-center shrink-0">
            <Headphones class="w-3.5 h-3.5" />
          </div>
          <div class="min-w-0">
            <div class="flex items-center gap-1.5 font-semibold text-xs text-text-main">
              Audio Adapter
              <span class="text-[10px] font-normal text-text-muted">({capacityAdapter.audioInput.models.length} models)</span>
            </div>
          </div>
        </div>

        <div class="flex items-center gap-2 shrink-0">
          <label class="flex items-center gap-1.5 text-xs text-text-muted cursor-pointer select-none">
            <Toggle
              size="sm"
              checked={capacityAdapter.audioInput.roundRobin}
              disabled={!capacityAdapter.audioInput.enabled}
              onChange={(v) => toggleAdapterRoundRobin('audioInput', v)}
            />
            <span class="text-[11px]">Round Robin</span>
          </label>
          <Button
            icon="add"
            variant="outline"
            size="sm"
            onclick={() => onOpenModelPicker('audio')}
            disabled={!capacityAdapter.audioInput.enabled}
            title="Add Audio model"
          >
            Add
          </Button>
        </div>
      </div>

      <!-- Audio Models List with explicit Priority Order & Touch-Friendly Reorder -->
      <div class="flex flex-col gap-1.5 min-w-0">
        {#if capacityAdapter.audioInput.models.length === 0}
          <div class="py-3 text-center text-xs text-text-muted italic border border-dashed border-border/60 rounded-lg">
            No audio models configured — click "Add" to select
          </div>
        {:else}
          <div class="flex flex-col gap-1 max-h-[220px] overflow-y-auto custom-scrollbar pr-0.5">
            {#each capacityAdapter.audioInput.models as model, idx}
              <div
                class="flex items-center justify-between gap-1.5 rounded-lg border border-border/60 bg-surface-2/60 px-2.5 py-1.5 text-xs font-mono text-text-main transition-colors hover:bg-surface-2"
              >
                <div class="flex items-center gap-1.5 min-w-0 flex-1">
                  <span class="text-[10px] font-semibold text-text-muted w-4 shrink-0 font-sans">#{idx + 1}</span>
                  <span class="truncate max-w-[160px] sm:max-w-[220px]">{model}</span>
                </div>

                <!-- Touch-friendly Reorder controls (Always Visible!) -->
                <div class="flex items-center gap-1 shrink-0">
                  <button
                    type="button"
                    onclick={() => moveAdapterModel('audioInput', idx, -1)}
                    disabled={idx === 0}
                    class="size-6 flex items-center justify-center rounded border border-border/50 bg-surface transition-colors {idx === 0
                      ? 'text-text-muted/20 cursor-not-allowed'
                      : 'text-text-muted hover:text-brand-500 hover:bg-surface-3 cursor-pointer'}"
                    title="Move up in priority order"
                  >
                    <ArrowUp class="w-3 h-3" />
                  </button>
                  <button
                    type="button"
                    onclick={() => moveAdapterModel('audioInput', idx, 1)}
                    disabled={idx === capacityAdapter.audioInput.models.length - 1}
                    class="size-6 flex items-center justify-center rounded border border-border/50 bg-surface transition-colors {idx === capacityAdapter.audioInput.models.length - 1
                      ? 'text-text-muted/20 cursor-not-allowed'
                      : 'text-text-muted hover:text-brand-500 hover:bg-surface-3 cursor-pointer'}"
                    title="Move down in priority order"
                  >
                    <ArrowDown class="w-3 h-3" />
                  </button>
                  <button
                    type="button"
                    onclick={() => removeAdapterModel('audioInput', idx)}
                    class="size-6 flex items-center justify-center rounded border border-border/50 bg-surface text-text-muted hover:text-rose-500 hover:bg-rose-500/10 transition-colors cursor-pointer"
                    title="Remove model"
                  >
                    <X class="w-3 h-3" />
                  </button>
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    </Card>
  </div>
</div>
