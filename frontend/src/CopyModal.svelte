<script lang="ts">
  import { errorText } from './runtime'
  import { onMount } from 'svelte'
  import { CopyToProfile, ListProfileBuckets } from './api'
  import type { main } from './models'
  import { t } from './i18n.svelte'
  import Modal from './Modal.svelte'

  let { profiles, activeId, bucket, prefix, keys, onclose, ondone }: {
    profiles: main.Profile[]
    activeId: string
    bucket: string
    prefix: string
    keys: string[]
    onclose: () => void
    ondone: (count: number, moved: boolean) => void
  } = $props()

  // Another connection is the usual destination; fall back to the current one.
  // svelte-ignore state_referenced_locally
  let profileId = $state(profiles.find((p) => p.id !== activeId)?.id ?? activeId)
  let buckets = $state<string[]>([])
  let dstBucket = $state('')
  let dstPrefix = $state('')
  let move = $state(false)
  let error = $state('')
  let busy = $state(false)
  let loadSeq = 0

  async function loadBuckets() {
    const seq = ++loadSeq
    buckets = []; dstBucket = ''; error = ''
    try {
      const list = await ListProfileBuckets(profileId)
      if (seq !== loadSeq) return
      buckets = (list.buckets ?? []).map((b) => b.name)
      // Listing can be denied while individual buckets are still writable.
      if (!buckets.length && list.warning) error = list.warning
    } catch (e) {
      if (seq === loadSeq) error = errorText(e)
    }
  }
  onMount(() => { loadBuckets() })

  async function copy(event: SubmitEvent) {
    event.preventDefault()
    busy = true; error = ''
    // The key list is reactive state; a proxy cannot cross the IPC boundary.
    try { ondone(await CopyToProfile(bucket, prefix, $state.snapshot(keys), profileId, dstBucket, dstPrefix, move), move) }
    catch (e) { error = errorText(e) }
    busy = false
  }
</script>

<Modal label={t('copyToTitle')} {onclose}>
  <form class="modal" onsubmit={copy}>
    <header>
      <h3>{t('copyToTitle')}</h3>
      <button type="button" class="ghost" title={t('close')} onclick={onclose}><i class="ri-close-line"></i></button>
    </header>
    <p class="hint">{t('copyHint', { n: keys.length })}</p>
    <label>{t('destProfile')}
      <select bind:value={profileId} onchange={loadBuckets} disabled={busy}>
        {#each profiles as p (p.id)}<option value={p.id}>{p.name}{p.id === activeId ? ' ' + t('thisConnection') : ''}</option>{/each}
      </select>
    </label>
    <label>{t('destBucket')}
      <!-- svelte-ignore a11y_autofocus -->
      <input autofocus bind:value={dstBucket} list="copy-buckets" spellcheck="false" autocomplete="off" disabled={busy} />
      <datalist id="copy-buckets">{#each buckets as name}<option value={name}></option>{/each}</datalist>
    </label>
    <label>{t('destPrefix')}
      <input bind:value={dstPrefix} placeholder="backups/2026/" spellcheck="false" disabled={busy} />
    </label>
    <label class="check"><input type="checkbox" bind:checked={move} disabled={busy} /> {t('moveOption')}</label>
    {#if error}<div class="msg selectable" role="alert">{error}</div>{/if}
    <footer>
      <button type="button" onclick={onclose}>{t('cancel')}</button>
      <button type="submit" class={move ? 'danger' : 'primary'} disabled={busy || !dstBucket.trim()}>
        {#if busy}<i class="ri-loader-4-line spin"></i>{:else}<i class="ri-file-transfer-line"></i>{/if} {move ? t('startMove') : t('startCopy')}
      </button>
    </footer>
  </form>
</Modal>

<style>
  .modal { width: min(480px, 94vw); display: flex; flex-direction: column; gap: 12px; background: var(--panel); border: 1px solid var(--border); border-radius: var(--radius-lg); padding: 20px 22px; box-shadow: var(--shadow); }
  header { display: flex; justify-content: space-between; align-items: center; }
  h3 { margin: 0; font-size: 15px; font-weight: 650; color: var(--text-strong); }
  .hint { margin: -4px 0 2px; color: var(--muted); font-size: 12px; line-height: 1.5; }
  .msg { padding: 8px 10px; border-radius: var(--radius-md); background: var(--danger-soft); color: var(--danger-text); word-break: break-word; }
  footer { display: flex; justify-content: flex-end; gap: 8px; margin-top: 4px; }
  .spin { display: inline-block; animation: spin 1s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }
</style>
