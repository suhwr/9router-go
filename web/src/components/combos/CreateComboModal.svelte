<script lang="ts">
  import {
    ArrowDown,
    ArrowUp,
    Brain,
    Eye,
    GripVertical,
    Layers,
    Loader2,
    Plus,
    X
  } from 'lucide-svelte'
  import type { Combo, ComboRulesPreview, SmartRule } from '../../api/client'
  import { api } from '../../api/client'
  import { getModelCaps } from '../../lib/models'
  import Button from '../../lib/ui/Button.svelte'
  import Input from '../../lib/ui/Input.svelte'
  import Modal from '../../lib/ui/Modal.svelte'
  import {
    COMBO_SORT_OPTIONS,
    DEFAULT_SMART_RULE_MAX_MEMBERS,
    DEFAULT_FIRST_TOKEN_GRACE_SECONDS,
    graceMsToSeconds,
    graceSecondsToMs,
    MAX_FIRST_TOKEN_TIMEOUT_SECONDS,
    RESPONSE_TIMEOUT_HELP,
    secondsToTimeoutMs,
    timeoutMsToSeconds,
    parseSmartRules,
    SMART_RULE_MODES,
    smartRuleSummary,
    sortComboMembers
  } from './types'
  import type { ComboSortKey, ModelMetaMap } from './types'

  interface Props {
    isOpen: boolean
    editingCombo: Combo | null
    models: string[]
    isSaving?: boolean
    /** Server-side refusal (e.g. the name collides with a combo, alias or
     * custom model id). Shown inline so a 409 is never a silent no-op. */
    saveError?: string
    onClose: () => void
    /** Stored timeout (ms) of the combo being edited; 0/undefined = disabled. */
    responseTimeoutMs?: number
    /** Stored grace (ms) after a timeout; undefined = default. */
    responseGraceMs?: number
    /** `graceMs` undefined means "use the default" and clears the stored value. */
    onSave: (
      name: string,
      models: string[],
      matchRules: SmartRule[],
      timeoutMs: number,
      graceMs: number | undefined,
      customPrompt: string
    ) => Promise<void> | void
    onOpenModelPicker: () => void
    onUpdateModels: (models: string[]) => void
  }

  let {
    isOpen,
    editingCombo,
    models,
    isSaving = false,
    saveError = '',
    responseTimeoutMs = 0,
    responseGraceMs = undefined,
    onClose,
    onSave,
    onOpenModelPicker,
    onUpdateModels,
  }: Props = $props()

  let modalName = $state(editingCombo?.name || '')
  let modalNameError = $state('')
  // Seeded once per open: the modal is remounted by the {#key} in CombosView.
  let timeoutInput = $state<string | number | null>(String(timeoutMsToSeconds(responseTimeoutMs) || ''))
  let timeoutMs = $derived(secondsToTimeoutMs(timeoutInput))
  let graceInput = $state<string | number | null>(graceMsToSeconds(responseGraceMs))
  let graceMs = $derived(graceSecondsToMs(graceInput))
  let customPrompt = $state(editingCombo?.customPrompt || '')
  let graceError = $derived(
    graceMs === null
      ? `Enter 0–${MAX_FIRST_TOKEN_TIMEOUT_SECONDS} seconds, or leave empty for the default`
      : ''
  )
  let timeoutError = $derived(
    timeoutMs === null
      ? `Enter 0–${MAX_FIRST_TOKEN_TIMEOUT_SECONDS} seconds, or leave empty to disable`
      : ''
  )
  let editingIdx = $state<number | null>(null)
  let editDraft = $state('')
  let draggedIdx = $state<number | null>(null)
  let dragOverIdx = $state<number | null>(null)
  let activeModalTab = $state<'models' | 'rules' | 'advanced'>('models')

  // Smart rules. The modal is remounted per open (the {#key} block in
  // CombosView), so reading the stored list once here is the reset.
  let rules = $state<SmartRule[]>(parseSmartRules(editingCombo?.matchRules))
  let rulePreview = $state<ComboRulesPreview | null>(null)
  let rulePreviewError = $state('')
  // Load matching models: the write half of the preview. The button only
  // exists while a rule has something to say, and the delta it reports is
  // what the operator sees after pressing it.
  let isLoadingModels = $state(false)
  let loadError = $state('')
  let loadDelta = $state<{ added: number; removed: number } | null>(null)
  // A rule with no pattern cannot match anything, so the load button stays
  // hidden until there is something to load. This is the same "appears after
  // input, disappears when empty" rule the operator asked for.
  let rulesHavePattern = $derived(rules.some((r) => r.pattern.trim().length > 0))
  // The membership the rules would produce, straight from the preview. It is
  // what the per-model exclude acts on, so the operator sees the effect of an
  // exclusion on the same list they excluded it from.
  let previewMembers = $derived(rulePreview?.models ?? [])
  // The rule that currently claims a model, so an exclusion lands on the rule
  // that would otherwise re-add it. A model no rule claims is already a member
  // and has nothing to exclude.
  function ruleOwning(member: string): SmartRule | undefined {
    return rules.find(
      (r) => r.effect !== 'exclude' && Array.isArray(r.resolved) && r.resolved.includes(member)
    )
  }
  function excludeMember(member: string) {
    const owner = ruleOwning(member)
    if (!owner) return
    if (!Array.isArray(owner.excluded)) owner.excluded = []
    if (!owner.excluded.includes(member)) owner.excluded = [...owner.excluded, member]
    // The preview is a function of the rules, so it re-runs on its own; the
    // excluded model drops out of the list above without a manual refresh.
    loadDelta = null
  }

  function addRule() {
    rules = [
      ...rules,
      {
        id: `r${rules.length + 1}-${Date.now().toString(36)}`,
        pattern: '',
        mode: 'glob',
        effect: 'include',
        enabled: true,
        maxMembers: DEFAULT_SMART_RULE_MAX_MEMBERS,
        allKinds: false
      }
    ]
  }

  function removeRule(idx: number) {
    rules = rules.filter((_, i) => i !== idx)
  }
  // Load matching models: preview first so the operator sees the delta, then
  // write it. The merge is the server's job — it keeps the models the operator
  // pinned by hand and drops only the ones no rule claims any more — so this
  // function never computes the membership itself.
  async function loadMatchingModels() {
    if (!editingCombo || isLoadingModels) return
    isLoadingModels = true
    loadError = ''
    try {
      const res = await api.loadComboRules(editingCombo.id, rules)
      loadDelta = { added: res.added.length, removed: res.removed.length }
      // The server's answer is the new membership; adopting it keeps the
      // editor's list and the stored row in step without a refetch.
      if (res.changed && Array.isArray(res.models)) {
        onUpdateModels(res.models)
      }
    } catch (e) {
      loadDelta = null
      loadError = e instanceof Error ? e.message : String(e)
    } finally {
      isLoadingModels = false
    }
  }
  // Sort: a one-shot reorder of the stored list. The operator picks an option,
  // the list is rewritten, and anything can be dragged back afterwards — none
  // of these is a standing order the gateway re-applies on every rule run,
  // which is what kept the alphabetical maxMembers cut from fighting the
  // manual ordering.
  let sortKey = $state<ComboSortKey>('manual')
  let sortNote = $state('')
  let modelMeta = $state<ModelMetaMap | null>(null)

  // Draft model testing state
  let isTestingDraft = $state(false)
  let draftTestResults = $state<Record<string, { ok: boolean; latencyMs: number; error?: string }>>({})
  let draftActiveModel = $state<string | null>(null)
  // Four of the options need data the editor does not already hold, so it is
  // fetched lazily on first use rather than on every open.
  async function ensureMeta() {
    if (modelMeta) return
    try {
      const res = await api.getModelMeta()
      modelMeta = res.meta
    } catch {
      // A failed read must not block the other sorts: the options simply
      // behave as "nothing known", which leaves the order unchanged rather
      // than guessing at it.
      modelMeta = {}
    }
  }
  async function applySort(key: ComboSortKey) {
    sortKey = key
    if (key === 'latency' || key === 'context-window' || key === 'capabilities' || key === 'kind') {
      await ensureMeta()
    }
    const sorted = sortComboMembers(models, key, modelMeta ?? undefined)
    if (sorted.length !== models.length) return
    let changed = false
    for (let i = 0; i < sorted.length; i++) {
      if (sorted[i] !== models[i]) {
        changed = true
        break
      }
    }
    if (!changed) {
      sortNote = 'already in this order'
      return
    }
    onUpdateModels(sorted)
    sortNote = key === 'shuffle' ? 'shuffled once' : 'sorted'
  }

  async function handleTestDraftModels() {
    if (models.length === 0 || isTestingDraft) return
    isTestingDraft = true
    draftActiveModel = null
    // Clear previous results for a fresh run
    draftTestResults = {}
    try {
      for (const model of models) {
        draftActiveModel = model
        try {
          const res = await api.testComboModels(editingCombo?.id || 'draft', { models: [model], kind: 'llm' })
          const r = res.results?.find((x) => x.model === model)
          if (r) {
            draftTestResults[model] = { ok: r.ok, latencyMs: r.latencyMs, error: r.error }
          } else {
            draftTestResults[model] = { ok: false, latencyMs: 0, error: 'No result' }
          }
        } catch (e) {
          draftTestResults[model] = { ok: false, latencyMs: 0, error: e instanceof Error ? e.message : String(e) }
        }
      }
    } finally {
      isTestingDraft = false
      draftActiveModel = null
    }
  }

  function rulePreviewFor(id: string) {
    return rulePreview?.rules.find((r) => r.id === id)
  }

  // The preview is a dry run against the live model set, so it needs a saved
  // combo to ask about. While creating there is nothing to preview yet.
  let rulesKey = $derived(JSON.stringify(rules))
  $effect(() => {
    const key = rulesKey
    if (!editingCombo) {
      rulePreview = null
      rulePreviewError = ''
      return
    }
    const timer = setTimeout(() => {
      void runRulePreview(key)
    }, 400)
    return () => clearTimeout(timer)
  })

  async function runRulePreview(key: string) {
    // A newer edit supersedes the run already in flight, so its answer is
    // dropped rather than shown as if it described the current rules.
    if (key !== rulesKey) return
    if (!editingCombo) return
    rulePreviewError = ''
    try {
      rulePreview = await api.previewComboRules(editingCombo.id, rules)
    } catch (e) {
      rulePreview = null
      rulePreviewError = e instanceof Error ? e.message : String(e)
    }
  }

  function handleDragStart(e: DragEvent, idx: number) {
    draggedIdx = idx
    if (e.dataTransfer) {
      e.dataTransfer.effectAllowed = 'move'
      e.dataTransfer.setData('text/plain', String(idx))
    }
  }

  function handleDragOver(e: DragEvent, idx: number) {
    e.preventDefault()
    if (e.dataTransfer) {
      e.dataTransfer.dropEffect = 'move'
    }
    dragOverIdx = idx
  }

  function handleDragLeave(idx: number) {
    if (dragOverIdx === idx) {
      dragOverIdx = null
    }
  }

  function handleDrop(e: DragEvent, targetIdx: number) {
    e.preventDefault()
    if (draggedIdx === null || draggedIdx === targetIdx) {
      draggedIdx = null
      dragOverIdx = null
      return
    }
    const arr = [...models]
    const [moved] = arr.splice(draggedIdx, 1)
    arr.splice(targetIdx, 0, moved)
    onUpdateModels(arr)
    draggedIdx = null
    dragOverIdx = null
  }

  function handleDragEnd() {
    draggedIdx = null
    dragOverIdx = null
  }
  const VALID_NAME_REGEX = /^[a-zA-Z0-9_.-]+$/

  function validateModalName(name: string): boolean {
    if (!name.trim()) {
      modalNameError = 'Name is required'
      return false
    }
    if (!VALID_NAME_REGEX.test(name.trim())) {
      modalNameError = 'Only letters, numbers, -, _ and . allowed'
      return false
    }
    modalNameError = ''
    return true
  }

  function handleSave() {
    if (!validateModalName(modalName)) return
    if (timeoutMs === null || graceMs === null) return
    onSave(modalName.trim(), models, rules, timeoutMs, graceMs, customPrompt.trim())
  }

  function moveModel(idx: number, delta: number) {
    const arr = [...models]
    const target = idx + delta
    if (target < 0 || target >= arr.length) return
    const temp = arr[idx]
    arr[idx] = arr[target]
    arr[target] = temp
    onUpdateModels(arr)
  }

  function removeModel(idx: number) {
    onUpdateModels(models.filter((_, i) => i !== idx))
    if (editingIdx === idx) editingIdx = null
  }

  function startEdit(idx: number, model: string) {
    editingIdx = idx
    editDraft = model
  }

  function commitEdit(idx: number) {
    const trimmed = editDraft.trim()
    if (trimmed && trimmed !== models[idx]) {
      const arr = [...models]
      arr[idx] = trimmed
      onUpdateModels(arr)
    }
    editingIdx = null
  }
</script>

<Modal
  {isOpen}
  onClose={onClose}
  title={editingCombo ? 'Edit Combo' : 'Create Combo'}
  size="lg"
>
  <div class="flex flex-col gap-3.5">
    <!-- Name Input -->
    <div>
      <Input
        label="Combo Name"
        bind:value={modalName}
        placeholder="my-combo"
        error={modalNameError}
      />
      <p class="text-[10px] text-text-muted mt-0.5">
        Only letters, numbers, -, _ and . allowed. Client requests to this name route through the model chain.
      </p>
    </div>

    <!-- Segmented Tab Navigation -->
    <div class="flex items-center gap-1 p-1 bg-surface-2 rounded-xl border border-border-subtle shrink-0">
      <button
        type="button"
        onclick={() => (activeModalTab = 'models')}
        class="flex-1 inline-flex items-center justify-center gap-1.5 py-1.5 px-3 rounded-lg text-xs font-semibold transition-all cursor-pointer {activeModalTab === 'models' ? 'bg-surface text-brand-500 shadow-sm border border-border/60' : 'text-text-muted hover:text-text-main'}"
      >
        <Layers class="w-3.5 h-3.5" />
        <span>Models</span>
        <span class="px-1.5 py-0.2 rounded-full text-[10px] font-bold tabular-nums {activeModalTab === 'models' ? 'bg-brand-500/10 text-brand-600 dark:text-brand-400' : 'bg-surface-3 text-text-muted'}">
          {models.length}
        </span>
      </button>

      <button
        type="button"
        onclick={() => (activeModalTab = 'rules')}
        class="flex-1 inline-flex items-center justify-center gap-1.5 py-1.5 px-3 rounded-lg text-xs font-semibold transition-all cursor-pointer {activeModalTab === 'rules' ? 'bg-surface text-brand-500 shadow-sm border border-border/60' : 'text-text-muted hover:text-text-main'}"
      >
        <span class="material-symbols-outlined text-[15px]">auto_awesome</span>
        <span>Smart Rules</span>
        {#if rules.length > 0}
          <span class="px-1.5 py-0.2 rounded-full text-[10px] font-bold tabular-nums bg-purple-500/10 text-purple-600 dark:text-purple-400">
            {rules.length}
          </span>
        {/if}
      </button>

      <button
        type="button"
        onclick={() => (activeModalTab = 'advanced')}
        class="flex-1 inline-flex items-center justify-center gap-1.5 py-1.5 px-3 rounded-lg text-xs font-semibold transition-all cursor-pointer {activeModalTab === 'advanced' ? 'bg-surface text-brand-500 shadow-sm border border-border/60' : 'text-text-muted hover:text-text-main'}"
      >
        <span class="material-symbols-outlined text-[15px]">tune</span>
        <span>Policies & Timeouts</span>
        {#if timeoutMs || customPrompt}
          <span class="w-1.5 h-1.5 rounded-full bg-brand-500"></span>
        {/if}
      </button>
    </div>

    <!-- Tab 1: Models -->
    {#if activeModalTab === 'models'}
      <div class="flex flex-col gap-2.5">
        <div class="flex flex-wrap items-center justify-between gap-y-1.5">
          <div class="flex items-center gap-2">
            <span class="text-xs font-medium text-text-muted">Execution Order (Top = Highest Priority)</span>
          </div>
          <div class="flex flex-wrap items-center gap-1.5">
            <!-- Sort dropdown -->
            <select
              value={sortKey}
              onchange={(e) => applySort(e.currentTarget.value as ComboSortKey)}
              class="rounded border border-border bg-surface px-1.5 py-1 text-[11px] text-text-main focus:outline-none focus:border-brand-500 cursor-pointer shrink-0"
              title="Reorder the members once. You can drag anything back afterwards."
            >
              {#each COMBO_SORT_OPTIONS as opt (opt.value)}
                <option value={opt.value} title={opt.hint}>{opt.label}</option>
              {/each}
            </select>
            {#if sortNote}
              <span class="text-[10px] text-text-subtle shrink-0">{sortNote}</span>
            {/if}
            <button
              type="button"
              onclick={onOpenModelPicker}
              class="inline-flex items-center gap-1 rounded-lg bg-brand-500/10 border border-brand-500/30 px-2 py-1 text-[11px] font-semibold text-brand-500 hover:bg-brand-500/20 transition-colors cursor-pointer shrink-0"
            >
              <Plus class="w-3.5 h-3.5 shrink-0" />
              <span>Add Models</span>
            </button>
            <!-- Test Models -->
            <button
              type="button"
              disabled={models.length === 0 || isTestingDraft}
              onclick={handleTestDraftModels}
              class="inline-flex items-center gap-1 rounded-lg border border-border px-2 py-1 text-[11px] font-medium text-text-muted hover:text-text-main hover:bg-surface-2 transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed shrink-0"
              title={models.length === 0 ? "No models to test" : isTestingDraft ? "Testing draft models..." : "Test draft models (Dead/Alive check)"}
            >
              {#if isTestingDraft}
                <Loader2 class="size-3 text-brand-500 animate-spin shrink-0" />
              {:else}
                <span class="material-symbols-outlined text-[13px] shrink-0">play_circle</span>
              {/if}
              <span>{isTestingDraft ? "Testing..." : "Test Health"}</span>
            </button>
            {#if Object.keys(draftTestResults).length > 0}
              {@const alive = Object.values(draftTestResults).filter((r) => r.ok).length}
              {@const total = Object.keys(draftTestResults).length}
              <span class="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 shrink-0">
                {alive}/{total} Healthy
              </span>
            {/if}
          </div>
        </div>

        {#if models.length === 0}
          <div class="text-center py-8 border border-dashed border-border-subtle rounded-xl bg-surface-2/40 flex flex-col items-center justify-center gap-2">
            <Layers class="w-8 h-8 text-text-muted opacity-40" />
            <p class="text-xs text-text-muted">No models in this combo yet.</p>
            <Button size="sm" variant="outline" onclick={onOpenModelPicker}>
              <Plus class="w-3.5 h-3.5 mr-1" />
              Browse & Add Models
            </Button>
          </div>
        {:else}
          <div class="flex flex-col gap-1.5 max-h-[380px] overflow-y-auto custom-scrollbar pr-1">
            {#each models as model, idx}
              {@const caps = getModelCaps(model)}
              {@const res = draftTestResults[model]}
              {@const isModelActive = isTestingDraft && draftActiveModel === model}
              <div
                class="group flex min-w-0 items-center gap-2 rounded-xl border border-border/60 bg-surface px-2.5 py-1.5 shadow-[var(--shadow-soft)] transition-all hover:border-brand-500/40 {draggedIdx === idx ? 'opacity-40 bg-brand-500/10' : dragOverIdx === idx ? 'border-2 border-brand-500 bg-brand-500/5' : ''}"
                draggable="true"
                ondragstart={(e) => handleDragStart(e, idx)}
                ondragover={(e) => handleDragOver(e, idx)}
                ondragleave={() => handleDragLeave(idx)}
                ondrop={(e) => handleDrop(e, idx)}
                ondragend={handleDragEnd}
              >
                <GripVertical class="w-3.5 h-3.5 text-text-muted/60 cursor-grab active:cursor-grabbing shrink-0 hover:text-text-main" />
                <span class="inline-flex items-center justify-center size-5 rounded-full bg-surface-2 text-[10px] font-bold text-text-muted shrink-0 border border-border/50">
                  {idx + 1}
                </span>

                {#if editingIdx === idx}
                  <input
                    autofocus
                    bind:value={editDraft}
                    onblur={() => commitEdit(idx)}
                    onkeydown={(e) => {
                      if (e.key === 'Enter') commitEdit(idx)
                      if (e.key === 'Escape') editingIdx = null
                    }}
                    class="min-w-0 flex-1 rounded border border-brand-500/40 bg-surface px-2 py-0.5 font-mono text-xs text-text-main outline-none"
                  />
                {:else}
                  <div
                    class="min-w-0 flex-1 cursor-text truncate rounded px-1.5 py-0.5 font-mono text-xs text-text-main hover:bg-surface-2"
                    onclick={() => startEdit(idx, model)}
                    onkeydown={(e) => e.key === 'Enter' && startEdit(idx, model)}
                    role="textbox"
                    tabindex="0"
                    title="Click to rename"
                  >
                    <div class="inline-flex items-center gap-1.5">
                      {#if isModelActive}
                        <Loader2 class="size-3 text-brand-500 animate-spin shrink-0" />
                      {:else if res}
                        <span class="material-symbols-outlined text-[13px] {res.ok ? 'text-emerald-600 dark:text-emerald-400' : 'text-rose-600 dark:text-rose-400'}">
                          {res.ok ? 'check_circle' : 'cancel'}
                        </span>
                      {/if}
                      <span class="font-semibold text-text-main">{model}</span>
                      {#if res?.ok}
                        <span class="text-[10px] text-emerald-600 dark:text-emerald-400 font-sans">{res.latencyMs}ms</span>
                      {/if}
                    </div>
                  </div>
                {/if}

                <!-- Consistent Capability Badges -->
                <div class="flex items-center gap-1 shrink-0">
                  {#if caps.vision}
                    <span
                      class="inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[10px] font-semibold bg-blue-500/10 text-blue-600 dark:text-blue-400 border border-blue-500/20"
                      title="Vision — Supports image and visual inputs"
                    >
                      <Eye class="w-3 h-3 shrink-0" />
                      <span>Vision</span>
                    </span>
                  {/if}
                  {#if caps.reasoning}
                    <span
                      class="inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[10px] font-semibold bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/20"
                      title="Reasoning — Supports step-by-step thinking/reasoning"
                    >
                      <Brain class="w-3 h-3 shrink-0" />
                      <span>Reasoning</span>
                    </span>
                  {/if}
                </div>

                <!-- Reorder buttons -->
                <div class="flex shrink-0 items-center gap-0.5">
                  <button
                    type="button"
                    onclick={() => moveModel(idx, -1)}
                    disabled={idx === 0}
                    class="size-6 flex items-center justify-center rounded border border-border/50 bg-surface-2 transition-colors {idx === 0 ? 'opacity-20 cursor-not-allowed' : 'text-text-muted hover:text-brand-500 hover:bg-surface-3 cursor-pointer'}"
                    title="Move up"
                  >
                    <ArrowUp class="w-3 h-3" />
                  </button>
                  <button
                    type="button"
                    onclick={() => moveModel(idx, 1)}
                    disabled={idx === models.length - 1}
                    class="size-6 flex items-center justify-center rounded border border-border/50 bg-surface-2 transition-colors {idx === models.length - 1 ? 'opacity-20 cursor-not-allowed' : 'text-text-muted hover:text-brand-500 hover:bg-surface-3 cursor-pointer'}"
                    title="Move down"
                  >
                    <ArrowDown class="w-3 h-3" />
                  </button>
                </div>

                <button
                  type="button"
                  onclick={() => removeModel(idx)}
                  class="p-1 hover:bg-red-500/10 rounded-md text-text-muted hover:text-red-500 transition-colors cursor-pointer shrink-0"
                  title="Remove from combo"
                >
                  <X class="w-3.5 h-3.5" />
                </button>
              </div>
            {/each}
          </div>
        {/if}
      </div>

    <!-- Tab 2: Smart Rules -->
    {:else if activeModalTab === 'rules'}
      <div class="flex flex-col gap-3">
        <div class="flex items-center justify-between">
          <div>
            <h4 class="text-xs font-semibold text-text-main">Dynamic Pattern Rules</h4>
            <p class="text-[10px] text-text-muted">
              Models matching these rules dynamically join or get excluded from this combo.
            </p>
          </div>
          <button
            type="button"
            onclick={addRule}
            class="inline-flex items-center gap-1 rounded-lg bg-brand-500/10 border border-brand-500/30 px-2.5 py-1 text-xs font-semibold text-brand-500 hover:bg-brand-500/20 transition-colors cursor-pointer"
          >
            <Plus class="w-3.5 h-3.5" />
            <span>Add Rule</span>
          </button>
        </div>

        {#if rules.length === 0}
          <div class="text-center py-6 border border-dashed border-border-subtle rounded-xl bg-surface-2/40 text-xs text-text-muted italic">
            No dynamic rules configured. This combo only routes through manually selected models.
          </div>
        {:else}
          <div class="flex flex-col gap-2 max-h-[300px] overflow-y-auto custom-scrollbar pr-1">
            {#each rules as rule, idx (rule.id)}
              <div class="rounded-xl border border-border bg-surface-2/60 p-2.5 flex flex-col gap-2">
                <div class="flex items-center gap-2">
                  <input
                    type="checkbox"
                    checked={rule.enabled}
                    onchange={(e) => (rule.enabled = e.currentTarget.checked)}
                    class="size-3.5 shrink-0 cursor-pointer accent-brand-500"
                    title={rule.enabled ? 'Rule is on' : 'Rule is off'}
                  />
                  <input
                    type="text"
                    value={rule.pattern}
                    oninput={(e) => (rule.pattern = e.currentTarget.value)}
                    placeholder="provider/model-pattern* (e.g. google/gemini*)"
                    class="min-w-0 flex-1 rounded-lg border border-border bg-surface px-2.5 py-1 font-mono text-xs text-text-main focus:outline-none focus:border-brand-500"
                  />
                  <button
                    type="button"
                    onclick={() => removeRule(idx)}
                    class="shrink-0 rounded p-1 text-text-muted hover:bg-red-500/10 hover:text-red-500 transition-colors cursor-pointer"
                    title="Remove rule"
                  >
                    <X class="w-3.5 h-3.5" />
                  </button>
                </div>
                <div class="flex flex-wrap items-center gap-2 text-xs">
                  <select
                    value={rule.mode}
                    onchange={(e) => (rule.mode = e.currentTarget.value as SmartRule['mode'])}
                    class="rounded-lg border border-border bg-surface px-2 py-0.5 text-xs text-text-main focus:outline-none focus:border-brand-500 cursor-pointer"
                  >
                    {#each SMART_RULE_MODES as mode (mode.value)}
                      <option value={mode.value}>{mode.label}</option>
                    {/each}
                  </select>
                  <select
                    value={rule.effect}
                    onchange={(e) => (rule.effect = e.currentTarget.value as SmartRule['effect'])}
                    class="rounded-lg border border-border bg-surface px-2 py-0.5 text-xs text-text-main focus:outline-none focus:border-brand-500 cursor-pointer"
                  >
                    <option value="include">Add</option>
                    <option value="exclude">Exclude</option>
                  </select>
                  <label class="flex items-center gap-1 text-[11px] text-text-muted">
                    <span>Max:</span>
                    <input
                      type="number"
                      min="0"
                      value={rule.maxMembers}
                      oninput={(e) => (rule.maxMembers = Math.max(0, Number(e.currentTarget.value) || 0))}
                      class="w-14 rounded-lg border border-border bg-surface px-1.5 py-0.5 text-xs text-text-main focus:outline-none focus:border-brand-500"
                    />
                  </label>
                  <label class="flex items-center gap-1 text-[11px] text-text-muted">
                    <input
                      type="checkbox"
                      checked={rule.allKinds}
                      onchange={(e) => (rule.allKinds = e.currentTarget.checked)}
                      class="size-3 cursor-pointer accent-brand-500"
                    />
                    <span>All modalities</span>
                  </label>
                </div>
                <p class="text-[10px] text-text-muted">
                  {smartRuleSummary(rule, rulePreviewFor(rule.id))}
                </p>
              </div>
            {/each}
          </div>
        {/if}

        {#if rulesHavePattern && editingCombo}
          <div class="flex flex-wrap items-center gap-2 pt-1">
            <button
              type="button"
              onclick={loadMatchingModels}
              disabled={isLoadingModels}
              class="inline-flex items-center gap-1.5 rounded-lg border border-brand-500/40 bg-brand-500/10 px-3 py-1.5 text-xs font-semibold text-brand-500 hover:bg-brand-500/20 transition-colors cursor-pointer disabled:opacity-50"
            >
              {#if isLoadingModels}
                <Loader2 class="size-3.5 animate-spin" />
                <span>Loading…</span>
              {:else}
                <Layers class="w-3.5 h-3.5" />
                <span>Load Matching Models</span>
              {/if}
            </button>
            {#if loadDelta}
              <span class="text-xs text-text-muted">
                {loadDelta.added > 0 || loadDelta.removed > 0
                  ? `+${loadDelta.added} matched${loadDelta.removed > 0 ? `, −${loadDelta.removed} stale` : ''}`
                  : 'Already synchronized'}
              </span>
            {/if}
          </div>
        {/if}

        {#if previewMembers.length > 0}
          <div class="rounded-xl border border-border bg-surface-2/40 p-2.5">
            <div class="flex items-center justify-between mb-1.5">
              <span class="text-xs font-semibold text-text-main">
                {previewMembers.length} model{previewMembers.length === 1 ? '' : 's'} matching rules
              </span>
              <button
                type="button"
                onclick={loadMatchingModels}
                disabled={isLoadingModels}
                class="text-xs font-semibold text-brand-500 hover:underline cursor-pointer disabled:opacity-50"
              >
                Import all into chain
              </button>
            </div>
            <div class="flex flex-wrap gap-1 max-h-28 overflow-y-auto custom-scrollbar">
              {#each previewMembers as member}
                {@const owner = ruleOwning(member)}
                <span class="inline-flex items-center gap-1 rounded-md border border-border bg-surface px-1.5 py-0.5 font-mono text-[10px] text-text-main">
                  <span class="truncate max-w-[200px]">{member}</span>
                  {#if owner}
                    <button
                      type="button"
                      onclick={() => excludeMember(member)}
                      class="shrink-0 rounded p-0.5 text-text-muted hover:text-red-500 cursor-pointer"
                      title="Exclude model"
                    >
                      <X class="w-2.5 h-2.5" />
                    </button>
                  {/if}
                </span>
              {/each}
            </div>
          </div>
        {/if}
      </div>

    <!-- Tab 3: Policies & Advanced Settings -->
    {:else if activeModalTab === 'advanced'}
      <div class="flex flex-col gap-3.5">
        <!-- Response Timeout -->
        <div class="rounded-xl border border-border bg-surface-2/50 p-3.5 flex flex-col gap-3">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <span class="material-symbols-outlined text-[18px] text-brand-500">timer</span>
              <div>
                <label for="combo-response-timeout" class="text-xs font-semibold text-text-main block">Model First-Token Timeout</label>
                <span class="text-[10px] text-text-muted">Fails over to next candidate if model doesn't respond</span>
              </div>
            </div>
            <div class="flex items-center gap-1.5">
              <input
                id="combo-response-timeout"
                type="number"
                min="0"
                max={MAX_FIRST_TOKEN_TIMEOUT_SECONDS}
                step="any"
                placeholder="Off"
                bind:value={timeoutInput}
                class="w-20 rounded-lg border bg-surface px-2.5 py-1 text-xs text-text-main focus:outline-none focus:border-brand-500 text-right {timeoutError ? 'border-red-500' : 'border-border'}"
              />
              <span class="text-xs text-text-muted">sec</span>
            </div>
          </div>
          {#if timeoutError}
            <p class="text-xs text-red-500" role="alert">{timeoutError}</p>
          {/if}

          <!-- Grace Period -->
          <div class="flex items-center justify-between border-t border-border/60 pt-3 {timeoutMs ? '' : 'opacity-40'}">
            <div class="flex items-center gap-2">
              <span class="material-symbols-outlined text-[18px] text-text-muted">hourglass_top</span>
              <div>
                <label for="combo-response-grace" class="text-xs font-semibold text-text-main block">Timeout Cooldown / Grace Period</label>
                <span class="text-[10px] text-text-muted">How long a timed-out model is benched from the combo</span>
              </div>
            </div>
            <div class="flex items-center gap-1.5">
              <input
                id="combo-response-grace"
                type="number"
                min="0"
                max={MAX_FIRST_TOKEN_TIMEOUT_SECONDS}
                step="any"
                placeholder={String(DEFAULT_FIRST_TOKEN_GRACE_SECONDS)}
                disabled={!timeoutMs}
                bind:value={graceInput}
                class="w-20 rounded-lg border bg-surface px-2.5 py-1 text-xs text-text-main focus:outline-none focus:border-brand-500 text-right disabled:cursor-not-allowed {graceError ? 'border-red-500' : 'border-border'}"
              />
              <span class="text-xs text-text-muted">sec</span>
            </div>
          </div>
          {#if graceError}
            <p class="text-xs text-red-500" role="alert">{graceError}</p>
          {/if}
        </div>

        <!-- Custom Prompt -->
        <div class="rounded-xl border border-border bg-surface-2/50 p-3.5 flex flex-col gap-1.5">
          <label class="text-xs font-semibold text-text-main">Custom System Prompt Override</label>
          <textarea
            bind:value={customPrompt}
            placeholder="Leave empty to inherit standard system prompt..."
            rows={4}
            class="w-full rounded-lg border border-border bg-surface px-3 py-2 text-xs font-mono text-text-main focus:outline-none focus:border-brand-500 custom-scrollbar"
          ></textarea>
          <p class="text-[10px] text-text-muted">
            Replaces the global system prompt prefix/suffix for all requests routed through this combo.
          </p>
        </div>
      </div>
    {/if}

    {#if saveError}
      <p class="text-xs text-red-600 dark:text-red-400 break-words" role="alert">{saveError}</p>
    {/if}

    <!-- Footer Actions -->
    <div class="flex items-center justify-between pt-3 border-t border-border-subtle mt-1">
      <span class="text-xs text-text-muted tabular-nums">
        {models.length} model{models.length === 1 ? '' : 's'} in combo
      </span>
      <div class="flex items-center gap-2">
        <Button onclick={onClose} variant="ghost" size="sm">
          Cancel
        </Button>
        <Button
          onclick={handleSave}
          size="sm"
          disabled={!modalName.trim() || !!modalNameError || timeoutMs === null || graceMs === null || isSaving}
          loading={isSaving}
        >
          {isSaving ? 'Saving...' : editingCombo ? 'Save Changes' : 'Create Combo'}
        </Button>
      </div>
    </div>
  </div>
</Modal>
