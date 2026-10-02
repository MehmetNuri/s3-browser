<script lang="ts">
  import { errorText } from './runtime'
  import { UpdateObjectHeaders } from './api'
  import type { main } from './models'
  import { t } from './i18n.svelte'
  import Modal from './Modal.svelte'

  let { bucket, info, onclose, onsaved }: {
    bucket: string
    info: main.ObjectInfo
    onclose: () => void
    onsaved: () => void
  } = $props()

  // svelte-ignore state_referenced_locally
  let headers = $state({
    contentType: info.contentType ?? '', cacheControl: info.cacheControl ?? '',
    contentDisposition: info.contentDisposition ?? '', contentEncoding: info.contentEncoding ?? '',
  })
  let error = $state('')
  let busy = $state(false)

  async function save(event: SubmitEvent) {
    event.preventDefault()
    busy = true; error = ''
    // A state proxy cannot cross the IPC boundary; send a plain copy.
    try { await UpdateObjectHeaders(bucket, info.key, $state.snapshot(headers)); onsaved() }
    catch (e) { error = errorText(e) }
    busy = false
  }
</script>

<Modal label={t('editHeaders')} {onclose}>
  <form class="modal" onsubmit={save}>
    <header>
      <h3>{t('editHeaders')}</h3>
      <button type="button" class="ghost" title={t('close')} onclick={onclose}><i class="ri-close-line"></i></button>
    </header>
    <p class="key mono selectable">{info.key}</p>
    <!-- svelte-ignore a11y_autofocus -->
    <label>Content-Type<input autofocus bind:value={headers.contentType} placeholder="application/octet-stream" spellcheck="false" /></label>
    <label>Cache-Control<input bind:value={headers.cacheControl} placeholder="public, max-age=31536000" spellcheck="false" /></label>
    <label>Content-Disposition<input bind:value={headers.contentDisposition} placeholder="attachment; filename=&quot;file.pdf&quot;" spellcheck="false" /></label>
    <label>Content-Encoding<input bind:value={headers.contentEncoding} placeholder="gzip" spellcheck="false" /></label>
    <p class="hint">{t('headersHint')}</p>
    {#if error}<div class="msg" role="alert">{error}</div>{/if}
    <footer>
      <button type="button" onclick={onclose}>{t('cancel')}</button>
      <button type="submit" class="primary" disabled={busy}>{t('save')}</button>
    </footer>
  </form>
</Modal>

<style>
  .modal { width: min(480px, 94vw); display: flex; flex-direction: column; gap: 12px; background: var(--panel); border: 1px solid var(--border); border-radius: var(--radius-lg); padding: 20px 22px; box-shadow: var(--shadow); }
  header { display: flex; justify-content: space-between; align-items: center; }
  h3 { margin: 0; font-size: 15px; font-weight: 650; color: var(--text-strong); }
  .key { margin: -6px 0 2px; color: var(--muted); word-break: break-all; }
  .hint { margin: 0; color: var(--muted); font-size: 12px; line-height: 1.5; }
  .msg { padding: 8px 10px; border-radius: var(--radius-md); background: var(--danger-soft); color: var(--danger-text); user-select: text; word-break: break-word; }
  footer { display: flex; justify-content: flex-end; gap: 8px; margin-top: 4px; }
</style>
