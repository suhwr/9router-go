<script lang="ts">
  import Badge from '../../lib/ui/Badge.svelte'
  import Button from '../../lib/ui/Button.svelte'
  import Modal from '../../lib/ui/Modal.svelte'
  import { api, type ProviderNode, type ProviderNodeLimits, type ProviderNodeLimitsPatch } from '../../api/client'
  import { nodeFormSeed } from './nodeFormSeed'
  import ProviderNodeLimitsField from './ProviderNodeLimitsField.svelte'
  import { nodeLimitsDraftOf, nodeLimitsPatchOf, type NodeLimitsDraft } from '../../lib/node-limits'

  interface Props {
    isOpen: boolean
    node?: ProviderNode | null
    isAnthropic?: boolean
    onClose: () => void
    onSave: (data: {
      name: string
      prefix: string
      apiType?: string
      baseUrl: string
      urlSuffix: string
      limits: ProviderNodeLimitsPatch
    }) => Promise<void> | void
    onLimitsSaved?: (limits: ProviderNodeLimits) => void
  }

  let { isOpen, node = null, isAnthropic = false, onClose, onSave, onLimitsSaved }: Props = $props()

  let formName = $state('')
  let formPrefix = $state('')
  let formUrlSuffix = $state('')
  let formApiType = $state<'chat' | 'responses'>('chat')
  let formBaseUrl = $state('')
  let checkKey = $state('')
  let checkModelId = $state('')
  let validating = $state(false)
  let validation = $state<'success' | 'failed' | null>(null)
  let saving = $state(false)
  let showAdvanced = $state(false)
  let limitsDraft = $state<NodeLimitsDraft>(nodeLimitsDraftOf(null))
  let limitsError = $state('')
  let limitsAfterReset = $state<ProviderNodeLimits | null>(null)

  let seededNodeId: string | null = null
  let wasOpen = false

  $effect(() => {
    const open = isOpen
    const id = node?.id ?? null
    const shouldSeed = open && !!node && (!wasOpen || id !== seededNodeId)
    wasOpen = open
    if (!shouldSeed || !node) return

    seededNodeId = id
    const seed = nodeFormSeed(node, isAnthropic)
    formName = seed.name
    formPrefix = seed.prefix
    formUrlSuffix = seed.urlSuffix
    formApiType = seed.apiType
    formBaseUrl = seed.baseUrl
    checkKey = ''
    checkModelId = ''
    validation = null
    saving = false
    showAdvanced = false
    limitsDraft = nodeLimitsDraftOf(node.limits ?? null)
    limitsError = ''
    limitsAfterReset = null
  })

  let saveDisabled = $derived(
    saving || !formName.trim() || !formPrefix.trim() || !formBaseUrl.trim()
  )

  async function handleValidate() {
    if (!formBaseUrl.trim()) return
    validating = true
    try {
      const res = await api.validateProviderNode({
        baseUrl: formBaseUrl.trim(),
        apiKey: checkKey.trim(),
        type: isAnthropic ? 'anthropic-compatible' : 'openai-compatible',
        modelId: checkModelId.trim() || undefined,
      })
      validation = res.valid ? 'success' : 'failed'
    } catch {
      validation = 'failed'
    } finally {
      validating = false
    }
  }

  async function handleSubmit() {
    if (saveDisabled) return
    saving = true
    limitsError = ''
    try {
      const payload: {
        name: string
        prefix: string
        apiType?: string
        baseUrl: string
        urlSuffix: string
        limits: ProviderNodeLimitsPatch
      } = {
        name: formName.trim(),
        prefix: formPrefix.trim(),
        baseUrl: formBaseUrl.trim(),
        urlSuffix: formUrlSuffix.trim(),
        limits: nodeLimitsPatchOf(limitsDraft),
      }
      if (!isAnthropic) payload.apiType = formApiType
      await onSave(payload)
    } catch (err) {
      limitsError = err instanceof Error ? err.message : String(err)
    } finally {
      saving = false
    }
  }
</script>

{#if node}
  <Modal {isOpen} title={`Edit ${isAnthropic ? 'Anthropic' : 'OpenAI'} Endpoint`} {onClose}>
    <div class="flex flex-col gap-4">
      <!-- 1. Core Fields -->
      <div class="space-y-3">
        <div>
          <label for="edit-node-name" class="block text-xs font-semibold text-text-main mb-1">
            Provider Name <span class="text-red-500">*</span>
          </label>
          <input
            id="edit-node-name"
            type="text"
            bind:value={formName}
            placeholder={`${isAnthropic ? 'Anthropic' : 'OpenAI'} Endpoint`}
            class="w-full px-3 py-2 text-xs rounded-lg border border-border bg-surface text-text-main focus:outline-none focus:border-brand-500"
            required
          />
        </div>

        <div>
          <label for="edit-node-baseurl" class="block text-xs font-semibold text-text-main mb-1">
            Base URL <span class="text-red-500">*</span>
          </label>
          <input
            id="edit-node-baseurl"
            type="text"
            bind:value={formBaseUrl}
            placeholder={isAnthropic ? 'https://api.anthropic.com/v1' : 'https://api.openai.com/v1'}
            class="w-full px-3 py-2 text-xs rounded-lg border border-border bg-surface font-mono text-text-main focus:outline-none focus:border-brand-500"
            required
          />
        </div>
      </div>

      <!-- 2. Test Connection Box -->
      <div class="p-3 rounded-xl border border-border/70 bg-surface-2/40 space-y-2.5">
        <div class="flex items-center justify-between">
          <span class="text-xs font-semibold text-text-main flex items-center gap-1.5">
            <span class="material-symbols-outlined text-[15px] text-brand-500">verified</span>
            <span>Test Connection</span>
          </span>
          <span class="text-[10px] text-text-muted">Test new endpoint settings</span>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
          <div>
            <label for="edit-check-key" class="block text-[10px] font-medium text-text-muted mb-0.5">API Key</label>
            <input
              id="edit-check-key"
              type="password"
              bind:value={checkKey}
              placeholder="sk-..."
              class="w-full px-2.5 py-1.5 text-xs rounded-lg border border-border bg-surface font-mono text-text-main placeholder:text-text-muted focus:outline-none focus:border-brand-500"
            />
          </div>
          <div>
            <label for="edit-check-model" class="block text-[10px] font-medium text-text-muted mb-0.5">Model ID</label>
            <input
              id="edit-check-model"
              type="text"
              bind:value={checkModelId}
              placeholder={isAnthropic ? 'claude-3-5-sonnet-20241022' : 'gpt-4o'}
              class="w-full px-2.5 py-1.5 text-xs rounded-lg border border-border bg-surface text-text-main placeholder:text-text-muted focus:outline-none focus:border-brand-500"
            />
          </div>
        </div>

        <div class="flex items-center gap-2">
          <Button
            size="sm"
            variant="secondary"
            onclick={handleValidate}
            disabled={!formBaseUrl.trim() || validating}
          >
            {validating ? 'Testing...' : 'Test Connection'}
          </Button>

          {#if validation}
            {#if validation === 'success'}
              <span class="inline-flex items-center gap-1 text-xs font-semibold text-emerald-600 dark:text-emerald-400">
                <span class="material-symbols-outlined text-[16px]">check_circle</span>
                <span>Connection Verified!</span>
              </span>
            {:else}
              <span class="inline-flex items-center gap-1 text-xs text-red-500">
                <span class="material-symbols-outlined text-[16px]">error</span>
                <span>Connection Failed</span>
              </span>
            {/if}
          {/if}
        </div>
      </div>

      <!-- 3. Advanced Settings (Collapsible) -->
      <div class="border-t border-border/70 pt-2">
        <button
          type="button"
          onclick={() => (showAdvanced = !showAdvanced)}
          class="flex items-center justify-between w-full text-xs font-semibold text-text-muted hover:text-text-main py-1 cursor-pointer"
        >
          <span class="flex items-center gap-1.5">
            <span class="material-symbols-outlined text-[16px]">settings</span>
            <span>Advanced Settings (Prefix, URL Suffix, Limits)</span>
          </span>
          <span class="material-symbols-outlined text-[16px] transition-transform duration-200 {showAdvanced ? 'rotate-180' : ''}">
            expand_more
          </span>
        </button>

        {#if showAdvanced}
          <div class="pt-3 space-y-3">
            <div>
              <label for="edit-node-prefix" class="block text-xs font-semibold text-text-main mb-1">
                Model Routing Prefix
              </label>
              <input
                id="edit-node-prefix"
                type="text"
                bind:value={formPrefix}
                placeholder={isAnthropic ? 'ac-prod' : 'oc-prod'}
                class="w-full px-3 py-2 text-xs rounded-lg border border-border bg-surface font-mono text-text-main focus:outline-none focus:border-brand-500"
              />
              <p class="text-[10px] text-text-muted mt-0.5">
                Prefix for model IDs (e.g. <code>{formPrefix}/model-name</code>).
              </p>
            </div>

            <div>
              <label for="edit-node-suffix" class="block text-xs font-semibold text-text-main mb-1">
                Provider ID Suffix
              </label>
              <input
                id="edit-node-suffix"
                type="text"
                bind:value={formUrlSuffix}
                placeholder="e.g. prod, backup"
                class="w-full px-3 py-2 text-xs rounded-lg border border-border bg-surface font-mono text-text-main focus:outline-none focus:border-brand-500"
              />
            </div>

            {#if !isAnthropic}
              <div>
                <label for="edit-node-apitype" class="block text-xs font-semibold text-text-main mb-1">API Type</label>
                <select
                  id="edit-node-apitype"
                  bind:value={formApiType}
                  class="w-full px-3 py-2 text-xs rounded-lg border border-border bg-surface text-text-main focus:outline-none focus:border-brand-500 cursor-pointer"
                >
                  <option value="chat">Chat Completions (/v1/chat/completions)</option>
                  <option value="responses">Responses API (/v1/responses)</option>
                </select>
              </div>
            {/if}

            <div class="pt-1">
              <ProviderNodeLimitsField
                bind:draft={limitsDraft}
                usage={limitsAfterReset ?? node.limits ?? null}
                nodeId={node.id}
                onReset={(limits) => {
                  limitsAfterReset = limits
                  onLimitsSaved?.(limits)
                }}
              />
            </div>
          </div>
        {/if}
      </div>

      {#if limitsError}
        <p class="text-xs text-red-500" role="alert">{limitsError}</p>
      {/if}

      <!-- Modal Footer -->
      <div class="flex items-center justify-end gap-2 pt-2 border-t border-border/70">
        <Button onclick={onClose} variant="ghost">Cancel</Button>
        <Button onclick={handleSubmit} disabled={saveDisabled}>
          {saving ? 'Saving...' : 'Save Changes'}
        </Button>
      </div>
    </div>
  </Modal>
{/if}
