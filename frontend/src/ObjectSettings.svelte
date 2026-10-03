<script lang="ts">
  import { onMount } from 'svelte'
  import * as api from './api'
  import type { ObjectSettings } from './api'
  import { errorText } from './runtime'
  import { t } from './i18n.svelte'
  import Modal from './Modal.svelte'
  import TagEditor from './TagEditor.svelte'
  import AclEditor from './AclEditor.svelte'

  let { bucket, key, onclose, onnotify, onchanged }: {
    bucket: string; key: string; onclose: () => void
    onnotify: (message: string, error?: boolean) => void; onchanged: () => void
  } = $props()

  const classes = ['STANDARD', 'STANDARD_IA', 'ONEZONE_IA', 'INTELLIGENT_TIERING', 'GLACIER_IR', 'GLACIER', 'DEEP_ARCHIVE', 'REDUCED_REDUNDANCY']
  const archived = (cls: string) => cls === 'GLACIER' || cls === 'DEEP_ARCHIVE'
  let settings = $state<ObjectSettings | null>(null)
  let error = $state('')
  let busy = $state(false)
  let storageClass = $state('STANDARD')
  let tags = $state<Record<string, string>>({})
  let metadata = $state<Record<string, string>>({})
  let retentionMode = $state('GOVERNANCE')
  let retentionUntil = $state('')
  let bypass = $state(false)
  let showAcl = $state(false)
  let restoreDays = $state('7')
  let restoreTier = $state('Standard')

  async function load() {
    error = ''
    try {
      settings = await api.GetObjectSettings(bucket, key)
      storageClass = settings.storageClass; tags = { ...settings.tags }; metadata = { ...settings.metadata }
      if (settings.retention.mode) retentionMode = settings.retention.mode
      retentionUntil = settings.retention.until ? settings.retention.until.slice(0, 10) : ''
    } catch (e) { error = errorText(e) }
  }
  onMount(load)

  async function save(fn: () => Promise<unknown>, message = t('settingsSaved')) {
    busy = true; error = ''
    try { await fn(); onnotify(message); onchanged(); await load() } catch (e) { error = errorText(e) }
    busy = false
  }
  // The restore header reads ongoing-request="true" while S3 works, then
  // ongoing-request="false", expiry-date="…" once the copy is available.
  let restoreState = $derived.by(() => {
    const header = settings?.restore ?? ''
    if (!header) return ''
    if (header.includes('ongoing-request="true"')) return t('restoreInProgress')
    const expiry = /expiry-date="([^"]+)"/.exec(header)?.[1]
    return expiry ? t('restoreAvailableUntil', { date: new Date(expiry).toLocaleString() }) : t('restoreAvailable')
  })
</script>

<Modal label={t('objectSettings')} {onclose}>
  <div class="modal">
    <header>
      <h3><i class="ri-settings-3-line"></i> {t('objectSettings')}</h3>
      <button type="button" class="ghost" title={t('close')} onclick={onclose}><i class="ri-close-line"></i></button>
    </header>
    <p class="key mono selectable">{key}</p>
    {#if !settings && !error}
      <p class="muted">{t('loading')}</p>
    {:else if settings}
      <section>
        <h4>{t('storageClass')}</h4>
        <div class="row">
          <select bind:value={storageClass} disabled={busy}>
            {#each classes as cls}<option value={cls}>{cls}</option>{/each}
            {#if !classes.includes(settings.storageClass)}<option value={settings.storageClass}>{settings.storageClass}</option>{/if}
          </select>
          <button type="button" class="primary sm" disabled={busy || storageClass === settings.storageClass} onclick={() => save(() => api.SetStorageClass(bucket, [key], storageClass))}>{t('apply')}</button>
        </div>
        {#if settings.encryption}<p class="hint">{t('encryptedWith', { algorithm: settings.encryption })}</p>{/if}
        {#if archived(settings.storageClass)}
          <div class="restore">
            <p class="hint">{restoreState || t('restoreHint')}</p>
            <div class="row">
              <label>{t('restoreDays')} <input type="number" min="1" max="365" bind:value={restoreDays} disabled={busy} /></label>
              <select bind:value={restoreTier} disabled={busy}>
                <option value="Standard">Standard</option>
                <option value="Bulk">Bulk</option>
                <option value="Expedited">Expedited</option>
              </select>
              <button type="button" class="sm" disabled={busy} onclick={() => save(() => api.RestoreObject(bucket, key, Number(restoreDays), restoreTier), t('restoreRequested'))}><i class="ri-inbox-unarchive-line"></i> {t('restore')}</button>
            </div>
          </div>
        {/if}
      </section>
      <section>
        <h4>{t('publicAccess')}</h4>
        {#if settings.unsupported?.acl}
          <p class="muted">{settings.unsupported.acl}</p>
        {:else}
          <div class="row">
            <span class="hint">{settings.public ? t('objectPublic') : t('objectPrivate')}</span>
            <span style="flex:1"></span>
            <button type="button" class="sm" onclick={() => (showAcl = !showAcl)} aria-expanded={showAcl}>{t('editPermissions')}</button>
            <button type="button" class="sm" class:del={settings.public} disabled={busy} onclick={() => save(() => api.SetObjectPublic(bucket, key, !settings!.public))}>
              {settings.public ? t('makePrivate') : t('makePublic')}
            </button>
          </div>
          {#if showAcl}
            <AclEditor load={() => api.GetObjectAcl(bucket, key)} save={(acl) => api.SetObjectAcl(bucket, key, acl)} onsaved={() => { onnotify(t('settingsSaved')); load() }} />
          {/if}
        {/if}
      </section>
      <section>
        <h4>{t('objectLock')}</h4>
        {#if settings.unsupported?.retention}
          <p class="muted">{settings.unsupported.retention}</p>
        {:else}
          <p class="hint">{settings.retention.mode ? t('retainedUntil', { mode: settings.retention.mode, date: settings.retention.until.slice(0, 10) }) : t('noRetention')}</p>
          <div class="row">
            <select bind:value={retentionMode} disabled={busy}>
              <option value="GOVERNANCE">GOVERNANCE</option>
              <option value="COMPLIANCE">COMPLIANCE</option>
            </select>
            <input type="date" bind:value={retentionUntil} disabled={busy} aria-label={t('retainUntil')} />
            <button type="button" class="primary sm" disabled={busy || !retentionUntil} onclick={() => save(() => api.SetObjectRetention(bucket, key, retentionMode, retentionUntil, bypass))}>{t('apply')}</button>
          </div>
          <label class="check"><input type="checkbox" bind:checked={bypass} /> {t('bypassGovernance')}</label>
          <div class="row">
            <span class="hint">{settings.retention.legalHold ? t('legalHoldOn') : t('legalHoldOff')}</span>
            <span style="flex:1"></span>
            <button type="button" class="sm" disabled={busy} onclick={() => save(() => api.SetObjectLegalHold(bucket, key, !settings!.retention.legalHold))}>{settings.retention.legalHold ? t('liftLegalHold') : t('placeLegalHold')}</button>
          </div>
        {/if}
      </section>
      <section>
        <h4>{t('tags')}</h4>
        {#if settings.unsupported?.tags}<p class="muted">{settings.unsupported.tags}</p>{/if}
        <TagEditor bind:values={tags} keyLabel={t('tagKey')} valueLabel={t('tagValue')} />
        <div class="row end"><button type="button" class="primary sm" disabled={busy} onclick={() => save(() => api.SetObjectTags(bucket, key, $state.snapshot(tags)))}>{t('saveTags')}</button></div>
      </section>
      <section>
        <h4>{t('userMetadata')}</h4>
        <p class="hint">{t('metadataHint')}</p>
        <TagEditor bind:values={metadata} keyLabel={t('metadataName')} valueLabel={t('tagValue')} />
        <div class="row end"><button type="button" class="primary sm" disabled={busy} onclick={() => save(() => api.SetObjectMetadata(bucket, key, $state.snapshot(metadata)))}>{t('saveMetadata')}</button></div>
      </section>
    {/if}
    {#if error}<div class="msg" role="alert">{error}</div>{/if}
  </div>
</Modal>

<style>
  .modal { width: min(560px, 94vw); max-height: 90vh; overflow: auto; display: flex; flex-direction: column; gap: 12px; background: var(--panel); border: 1px solid var(--border); border-radius: var(--radius-lg); padding: 20px 22px; box-shadow: var(--shadow); }
  header { display: flex; justify-content: space-between; align-items: center; }
  h3 { margin: 0; font-size: 15px; font-weight: 650; color: var(--text-strong); display: flex; align-items: center; gap: 8px; }
  h4 { margin: 0 0 6px; font-size: 13px; font-weight: 600; color: var(--text-strong); }
  .key { margin: -6px 0 2px; color: var(--muted); word-break: break-all; }
  section { display: flex; flex-direction: column; gap: 6px; padding-top: 8px; border-top: 1px solid var(--border); }
  .row { display: flex; gap: 8px; align-items: center; } .row.end { justify-content: flex-end; }
  .row label { display: flex; align-items: center; gap: 6px; font-size: 13px; } .row input[type=number] { width: 70px; }
  .restore { display: flex; flex-direction: column; gap: 6px; }
  .hint { margin: 0; color: var(--muted); font-size: 12px; line-height: 1.5; }
  .msg { padding: 8px 10px; border-radius: var(--radius-md); background: var(--danger-soft); color: var(--danger-text); user-select: text; word-break: break-word; }
</style>
