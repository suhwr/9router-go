<script lang="ts">
  import { api } from '../../api/client'
  import Badge from '../../lib/ui/Badge.svelte'
  import Button from '../../lib/ui/Button.svelte'
  import Modal from '../../lib/ui/Modal.svelte'
  import ProviderNodeLimitsField from './ProviderNodeLimitsField.svelte'
  import { EMPTY_NODE_LIMITS, nodeLimitsPatchOf, type NodeLimitsDraft } from '../../lib/node-limits'
  import type { ProviderNodeLimitsPatch } from '../../api/client'

  export type CompatibleNodeType = 'openai-compatible' | 'anthropic-compatible' | 'custom-embedding'

  interface Props {
    isOpen: boolean
    /** Initial node type. The user can switch it inside the modal. */
    type?: CompatibleNodeType
    /** Types offered by the type switch; omit to offer all three. */
    allowedTypes?: CompatibleNodeType[]
    isSubmitting?: boolean
    onClose: () => void
    onSubmit: (data: {
      name: string
      prefix: string
      baseUrl: string
      apiType?: 'chat' | 'responses'
      type: string
      urlSuffix: string
      /** Capacity limits to apply once the node exists; all zero means unlimited. */
      limits: ProviderNodeLimitsPatch
    }) => Promise<void> | void
  }

  let {
    isOpen,
    type = 'openai-compatible',
    allowedTypes = ['openai-compatible', 'anthropic-compatible'],
    onClose,
    onSubmit,
  }: Props = $props()

  const PRESETS: Array<{
    label: string
    name: string
    type: CompatibleNodeType
    url: string
  }> = [
    { label: 'OpenAI (Official)', name: 'OpenAI', type: 'openai-compatible', url: 'https://api.openai.com/v1' },
    { label: 'Anthropic (Claude)', name: 'Anthropic', type: 'anthropic-compatible', url: 'https://api.anthropic.com/v1' },
    { label: 'DeepSeek', name: 'DeepSeek', type: 'openai-compatible', url: 'https://api.deepseek.com/v1' },
    { label: 'Ollama (Local)', name: 'Ollama', type: 'openai-compatible', url: 'http://localhost:11434/v1' },
    { label: 'vLLM / LM Studio', name: 'Local vLLM', type: 'openai-compatible', url: 'http://localhost:8000/v1' },
    { label: 'OpenRouter', name: 'OpenRouter', type: 'openai-compatible', url: 'https://openrouter.ai/api/v1' },
  ]

  let selectedType = $state<CompatibleNodeType>('openai-compatible')
  let formName = $state('')
  let formPrefix = $state('')
  let userEditedPrefix = $state(false)
  let formBaseUrl = $state('https://api.openai.com/v1')
  let checkKey = $state('')
  let checkModelId = $state('')
  let validating = $state(false)
  let validationResult = $state<{ valid: boolean; error?: string; method?: string; dimensions?: number } | null>(null)
  let submitting = $state(false)

  // Advanced settings (collapsed by default to prevent UI overwhelm)
  let showAdvanced = $state(false)
  let formUrlSuffix = $state('')
  let formApiType = $state<'chat' | 'responses'>('chat')
  let limitsDraft = $state<NodeLimitsDraft>({ ...EMPTY_NODE_LIMITS })

  function handleNameChange(val: string) {
    formName = val
    if (!userEditedPrefix) {
      const slug = val
        .toLowerCase()
        .trim()
        .replace(/[^a-z0-9]+/g, '-')
        .replace(/^-|-$/g, '')
      formPrefix = slug
    }
  }

  function applyPreset(preset: typeof PRESETS[0]) {
    selectedType = preset.type
    formName = preset.name
    formPrefix = preset.name.toLowerCase().replace(/[^a-z0-9]+/g, '-')
    userEditedPrefix = false
    formBaseUrl = preset.url
    validationResult = null
  }

  function selectType(next: CompatibleNodeType) {
    if (next === selectedType) return
    selectedType = next
    validationResult = null
    if (next === 'anthropic-compatible' && (formBaseUrl === 'https://api.openai.com/v1' || !formBaseUrl)) {
      formBaseUrl = 'https://api.anthropic.com/v1'
    } else if (next === 'openai-compatible' && (formBaseUrl === 'https://api.anthropic.com/v1' || !formBaseUrl)) {
      formBaseUrl = 'https://api.openai.com/v1'
    }
  }

  // Reset form when modal opens
  $effect(() => {
    if (isOpen) {
      selectedType = type
      formName = ''
      formPrefix = ''
      userEditedPrefix = false
      formUrlSuffix = ''
      formApiType = 'chat'
      checkKey = ''
      checkModelId = ''
      validationResult = null
      submitting = false
      showAdvanced = false
      formBaseUrl = type === 'anthropic-compatible' ? 'https://api.anthropic.com/v1' : 'https://api.openai.com/v1'
    }
  })

  async function handleCheck() {
    if (!formBaseUrl.trim()) return
    validating = true
    try {
      validationResult = await api.validateProviderNode({
        baseUrl: formBaseUrl.trim(),
        apiKey: checkKey.trim(),
        type: selectedType,
        modelId: checkModelId.trim() || undefined,
      })
    } catch (err) {
      validationResult = {
        valid: false,
        error: err instanceof Error ? err.message : String(err),
      }
    } finally {
      validating = false
    }
  }

  function handleSubmit(e?: SubmitEvent) {
    e?.preventDefault()
    if (!formName.trim() || !formPrefix.trim() || !formBaseUrl.trim() || submitting) return
    submitting = true
    Promise.resolve(onSubmit({
      name: formName.trim(),
      prefix: formPrefix.trim(),
      baseUrl: formBaseUrl.trim(),
      apiType: selectedType === 'openai-compatible' ? formApiType : undefined,
      type: selectedType,
      urlSuffix: formUrlSuffix.trim(),
      limits: nodeLimitsPatchOf(limitsDraft),
    })).finally(() => {
      submitting = false
    })
  }
</script>

<Modal {isOpen} title="Add Compatible Provider" {onClose}>
  <div class="flex flex-col gap-4">
    <!-- 1. Protocol Selector: Big, clear visual cards -->
    <div class="space-y-1.5">
      <label class="text-xs font-semibold text-text-muted uppercase tracking-wider block">
        API Protocol Format
      </label>
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5">
        <button
          type="button"
          onclick={() => selectType('openai-compatible')}
          class="flex items-center gap-3 p-3 rounded-xl border text-left transition-all cursor-pointer {selectedType === 'openai-compatible'
            ? 'border-emerald-500 bg-emerald-500/10 shadow-xs'
            : 'border-border bg-surface hover:bg-surface-2'}"
        >
          <div class="size-8 rounded-lg bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 flex items-center justify-center shrink-0">
            <span class="material-symbols-outlined text-[18px]">neurology</span>
          </div>
          <div class="min-w-0 flex-1">
            <div class="flex items-center justify-between">
              <span class="text-xs font-bold text-text-main">OpenAI Compatible</span>
              {#if selectedType === 'openai-compatible'}
                <span class="material-symbols-outlined text-[16px] text-emerald-500">check_circle</span>
              {/if}
            </div>
            <p class="text-[11px] text-text-muted truncate">OpenAI, DeepSeek, vLLM, Ollama, Groq</p>
          </div>
        </button>

        <button
          type="button"
          onclick={() => selectType('anthropic-compatible')}
          class="flex items-center gap-3 p-3 rounded-xl border text-left transition-all cursor-pointer {selectedType === 'anthropic-compatible'
            ? 'border-purple-500 bg-purple-500/10 shadow-xs'
            : 'border-border bg-surface hover:bg-surface-2'}"
        >
          <div class="size-8 rounded-lg bg-purple-500/15 text-purple-600 dark:text-purple-400 flex items-center justify-center shrink-0">
            <span class="material-symbols-outlined text-[18px]">psychology</span>
          </div>
          <div class="min-w-0 flex-1">
            <div class="flex items-center justify-between">
              <span class="text-xs font-bold text-text-main">Anthropic Compatible</span>
              {#if selectedType === 'anthropic-compatible'}
                <span class="material-symbols-outlined text-[16px] text-purple-500">check_circle</span>
              {/if}
            </div>
            <p class="text-[11px] text-text-muted truncate">Claude, Anthropic Messages API</p>
          </div>
        </button>
      </div>
    </div>

    <!-- 2. Quick Presets -->
    <div class="space-y-1.5">
      <span class="text-[11px] font-medium text-text-muted">Quick Presets:</span>
      <div class="flex flex-wrap gap-1.5">
        {#each PRESETS as preset}
          <button
            type="button"
            onclick={() => applyPreset(preset)}
            class="px-2.5 py-1 rounded-lg text-[11px] font-medium border border-border/80 bg-surface hover:bg-surface-2 text-text-main transition-colors cursor-pointer"
          >
            {preset.label}
          </button>
        {/each}
      </div>
    </div>

    <!-- 3. Primary Fields: Name & Base URL -->
    <div class="space-y-3 pt-1">
      <div>
        <label for="compat-node-name" class="block text-xs font-semibold text-text-main mb-1">
          Provider Name <span class="text-red-500">*</span>
        </label>
        <input
          id="compat-node-name"
          type="text"
          value={formName}
          oninput={(e) => handleNameChange((e.target as HTMLInputElement).value)}
          placeholder="e.g. OpenAI (Official), DeepSeek, Local vLLM"
          class="w-full px-3 py-2 text-xs rounded-lg border border-border bg-surface text-text-main placeholder:text-text-muted focus:outline-none focus:border-brand-500 transition-colors"
          required
        />
        <p class="text-[10px] text-text-muted mt-0.5">A clear name for this provider endpoint in your dashboard.</p>
      </div>

      <div>
        <label for="compat-node-url" class="block text-xs font-semibold text-text-main mb-1">
          Base URL <span class="text-red-500">*</span>
        </label>
        <input
          id="compat-node-url"
          type="text"
          bind:value={formBaseUrl}
          placeholder="https://api.openai.com/v1"
          class="w-full px-3 py-2 text-xs rounded-lg border border-border bg-surface font-mono text-text-main placeholder:text-text-muted focus:outline-none focus:border-brand-500 transition-colors"
          required
        />
        <p class="text-[10px] text-text-muted mt-0.5">Endpoint URL ending in <code>/v1</code> (e.g. <code>https://api.openai.com/v1</code> or <code>http://localhost:11434/v1</code>).</p>
      </div>
    </div>

    <!-- 4. Test & Verification (Optional) -->
    <div class="p-3 rounded-xl border border-border/70 bg-surface-2/40 space-y-2.5">
      <div class="flex items-center justify-between">
        <span class="text-xs font-semibold text-text-main flex items-center gap-1.5">
          <span class="material-symbols-outlined text-[15px] text-brand-500">verified</span>
          <span>Test Connection (Optional)</span>
        </span>
        <span class="text-[10px] text-text-muted">Verify before saving</span>
      </div>

      <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
        <div>
          <label for="compat-check-key" class="block text-[10px] font-medium text-text-muted mb-0.5">API Key</label>
          <input
            id="compat-check-key"
            type="password"
            bind:value={checkKey}
            placeholder="sk-..."
            class="w-full px-2.5 py-1.5 text-xs rounded-lg border border-border bg-surface font-mono text-text-main placeholder:text-text-muted focus:outline-none focus:border-brand-500"
          />
        </div>
        <div>
          <label for="compat-check-model" class="block text-[10px] font-medium text-text-muted mb-0.5">Model ID</label>
          <input
            id="compat-check-model"
            type="text"
            bind:value={checkModelId}
            placeholder={selectedType === 'anthropic-compatible' ? 'claude-3-5-sonnet-20241022' : 'gpt-4o'}
            class="w-full px-2.5 py-1.5 text-xs rounded-lg border border-border bg-surface text-text-main placeholder:text-text-muted focus:outline-none focus:border-brand-500"
          />
        </div>
      </div>

      <div class="flex items-center gap-2">
        <Button
          size="sm"
          variant="secondary"
          onclick={handleCheck}
          disabled={!formBaseUrl.trim() || validating}
        >
          {validating ? 'Testing...' : 'Test Connection'}
        </Button>

        {#if validationResult}
          {#if validationResult.valid}
            <span class="inline-flex items-center gap-1 text-xs font-semibold text-emerald-600 dark:text-emerald-400">
              <span class="material-symbols-outlined text-[16px]">check_circle</span>
              <span>Connection Verified!</span>
            </span>
          {:else}
            <span class="inline-flex items-center gap-1 text-xs text-red-500 max-w-[200px] truncate" title={validationResult.error}>
              <span class="material-symbols-outlined text-[16px]">error</span>
              <span class="truncate">{validationResult.error || 'Connection Failed'}</span>
            </span>
          {/if}
        {/if}
      </div>
    </div>

    <!-- 5. Advanced Settings (Collapsible) -->
    <div class="border-t border-border/70 pt-2">
      <button
        type="button"
        onclick={() => (showAdvanced = !showAdvanced)}
        class="flex items-center justify-between w-full text-xs font-semibold text-text-muted hover:text-text-main py-1 cursor-pointer"
      >
        <span class="flex items-center gap-1.5">
          <span class="material-symbols-outlined text-[16px]">settings</span>
          <span>Advanced Settings (Model Prefix, API Type, Limits)</span>
        </span>
        <span class="material-symbols-outlined text-[16px] transition-transform duration-200 {showAdvanced ? 'rotate-180' : ''}">
          expand_more
        </span>
      </button>

      {#if showAdvanced}
        <div class="pt-3 space-y-3">
          <div>
            <label for="compat-node-prefix" class="block text-xs font-semibold text-text-main mb-1">
              Model Routing Prefix
            </label>
            <input
              id="compat-node-prefix"
              type="text"
              bind:value={formPrefix}
              oninput={() => (userEditedPrefix = true)}
              placeholder="e.g. openai, deepseek"
              class="w-full px-3 py-2 text-xs rounded-lg border border-border bg-surface font-mono text-text-main focus:outline-none focus:border-brand-500"
            />
            <p class="text-[10px] text-text-muted mt-0.5">
              Used in model strings (e.g. <code>{formPrefix || 'provider'}/model-id</code>). Auto-filled from name.
            </p>
          </div>

          <div>
            <label for="compat-node-suffix" class="block text-xs font-semibold text-text-main mb-1">
              Custom Provider ID Suffix (Optional)
            </label>
            <input
              id="compat-node-suffix"
              type="text"
              bind:value={formUrlSuffix}
              placeholder="e.g. prod, backup"
              class="w-full px-3 py-2 text-xs rounded-lg border border-border bg-surface font-mono text-text-main focus:outline-none focus:border-brand-500"
            />
          </div>

          {#if selectedType === 'openai-compatible'}
            <div>
              <label for="compat-api-type" class="block text-xs font-semibold text-text-main mb-1">API Type</label>
              <select
                id="compat-api-type"
                bind:value={formApiType}
                class="w-full px-3 py-2 text-xs rounded-lg border border-border bg-surface text-text-main focus:outline-none focus:border-brand-500 cursor-pointer"
              >
                <option value="chat">Chat Completions (/v1/chat/completions)</option>
                <option value="responses">Responses API (/v1/responses)</option>
              </select>
            </div>
          {/if}

          <div class="pt-1">
            <ProviderNodeLimitsField bind:draft={limitsDraft} />
          </div>
        </div>
      {/if}
    </div>

    <!-- Modal Footer Actions -->
    <div class="flex items-center justify-end gap-2 pt-2 border-t border-border/70">
      <Button onclick={onClose} variant="ghost">
        Cancel
      </Button>
      <Button
        type="button"
        onclick={handleSubmit}
        disabled={!formName.trim() || !formPrefix.trim() || !formBaseUrl.trim() || submitting}
      >
        {submitting ? 'Creating Endpoint...' : 'Create Endpoint'}
      </Button>
    </div>
  </div>
</Modal>
