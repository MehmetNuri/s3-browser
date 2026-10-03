<script lang="ts">
  import { onMount } from 'svelte'
  import * as api from './api'
  import type { BucketSettings, Distribution, PublicAccessBlock } from './api'
  import { errorText } from './runtime'
  import { t, type Key } from './i18n.svelte'
  import Modal from './Modal.svelte'
  import TagEditor from './TagEditor.svelte'
  import AclEditor from './AclEditor.svelte'

  let { bucket, provider = '', onclose, onnotify }: { bucket: string; provider?: string; onclose: () => void; onnotify: (message: string, error?: boolean) => void } = $props()

  type Tab = 'general' | 'hosting' | 'policy' | 'acl' | 'cors' | 'lifecycle' | 'tags' | 'cloudfront'
  const tabs = $derived.by((): { value: Tab; label: Key }[] => [
    { value: 'general', label: 'settingsGeneral' }, { value: 'hosting', label: 'settingsHosting' }, { value: 'policy', label: 'settingsPolicy' },
    { value: 'acl', label: 'settingsAcl' }, { value: 'cors', label: 'settingsCors' }, { value: 'lifecycle', label: 'settingsLifecycle' }, { value: 'tags', label: 'tags' },
    ...(provider === 'aws' ? [{ value: 'cloudfront' as Tab, label: 'settingsCloudFront' as Key }] : []),
  ])
  let distributions = $state<Distribution[] | null>(null)
  let invalidation = $state('/*')
  async function loadDistributions() {
    error = ''
    try { distributions = await api.ListDistributions(bucket) } catch (e) { error = errorText(e) }
  }
  async function invalidate(id: string) {
    const paths = invalidation.split(/\s+/).filter(Boolean)
    await save(() => api.InvalidatePaths(id, paths).then((ref) => onnotify(t('invalidationCreated', { id: ref }))))
  }
  let tab = $state<Tab>('general')
  let settings = $state<BucketSettings | null>(null)
  let error = $state('')
  let busy = $state(false)
  // Editable copies; the loaded settings stay untouched until a save succeeds.
  let policy = $state('')
  let cors = $state('')
  let lifecycle = $state('')
  let tags = $state<Record<string, string>>({})
  let encryption = $state('')
  let kmsKey = $state('')
  let websiteIndex = $state('')
  let websiteError = $state('')
  let loggingBucket = $state('')
  let loggingPrefix = $state('')
  let lockMode = $state('')
  let lockDays = $state('30')
  let lockUnit = $state<'days' | 'years'>('days')
  let lockAcknowledged = $state(false)
  let block = $state<PublicAccessBlock>({ blockPublicAcls: false, ignorePublicAcls: false, blockPublicPolicy: false, restrictPublicBuckets: false })

  const corsTemplate = JSON.stringify([{ AllowedOrigins: ['https://example.com'], AllowedMethods: ['GET', 'HEAD'], AllowedHeaders: ['*'], ExposeHeaders: ['ETag'], MaxAgeSeconds: 3600 }], null, 2)
  const lifecycleTemplate = JSON.stringify([
    { ID: 'expire-temp', Status: 'Enabled', Filter: { Prefix: 'tmp/' }, Expiration: { Days: 30 } },
    { ID: 'abort-incomplete-uploads', Status: 'Enabled', Filter: { Prefix: '' }, AbortIncompleteMultipartUpload: { DaysAfterInitiation: 7 } },
  ], null, 2)
  const policyTemplate = () => JSON.stringify({
    Version: '2012-10-17',
    Statement: [{ Sid: 'PublicRead', Effect: 'Allow', Principal: '*', Action: ['s3:GetObject'], Resource: [`arn:aws:s3:::${bucket}/*`] }],
  }, null, 2)

  async function load() {
    error = ''
    try {
      settings = await api.GetBucketSettings(bucket)
      policy = settings.policy; cors = settings.cors; lifecycle = settings.lifecycle
      tags = { ...settings.tags }; encryption = settings.encryption; kmsKey = settings.kmsKey
      if (settings.publicAccessBlock) block = { ...settings.publicAccessBlock }
      websiteIndex = settings.websiteIndex; websiteError = settings.websiteError
      if (settings.objectLock) {
        lockMode = settings.objectLock.mode
        if (settings.objectLock.years) { lockUnit = 'years'; lockDays = String(settings.objectLock.years) }
        else if (settings.objectLock.days) { lockUnit = 'days'; lockDays = String(settings.objectLock.days) }
      }
      loggingBucket = settings.loggingBucket; loggingPrefix = settings.loggingPrefix
    } catch (e) { error = errorText(e) }
  }
  onMount(load)

  async function save(fn: () => Promise<unknown>) {
    busy = true; error = ''
    try { await fn(); onnotify(t('settingsSaved')); await load() } catch (e) { error = errorText(e) }
    busy = false
  }
  async function toggleVersioning() {
    await save(() => api.SetBucketVersioning(bucket, settings?.versioning !== 'Enabled'))
  }
  const unsupported = (name: string) => settings?.unsupported?.[name]
</script>

<Modal label={t('bucketSettings')} {onclose}>
  <div class="modal">
    <header>
      <h3><i class="ri-settings-3-line"></i> {t('bucketSettings')} <span class="mono muted">{bucket}</span></h3>
      <button type="button" class="ghost" title={t('close')} onclick={onclose}><i class="ri-close-line"></i></button>
    </header>
    <div class="tabs" role="tablist">
      {#each tabs as item}
        <button type="button" role="tab" class="ghost" class:on={tab === item.value} aria-selected={tab === item.value} onclick={() => { tab = item.value; if (tab === 'cloudfront' && !distributions) loadDistributions() }}>{t(item.label)}</button>
      {/each}
    </div>
    {#if !settings && !error}
      <p class="muted">{t('loading')}</p>
    {:else if settings}
      {#if tab === 'general'}
        <dl>
          <dt>{t('region')}</dt><dd class="mono">{settings.region || '—'}</dd>
          <dt>{t('versioning')}</dt>
          <dd>
            {#if unsupported('versioning')}<span class="muted">{unsupported('versioning')}</span>
            {:else}
              {t(settings.versioning === 'Enabled' ? 'versioningEnabled' : settings.versioning === 'Suspended' ? 'versioningSuspended' : 'versioningOff')}
              <button type="button" class="sm" disabled={busy} onclick={toggleVersioning}>{settings.versioning === 'Enabled' ? t('suspendVersioning') : t('enableVersioning')}</button>
            {/if}
          </dd>
        </dl>
        <section>
          <h4>{t('defaultEncryption')}</h4>
          {#if unsupported('encryption')}<p class="muted">{unsupported('encryption')}</p>{/if}
          <div class="row">
            <select bind:value={encryption} disabled={busy}>
              <option value="">{t('encryptionNone')}</option>
              <option value="AES256">SSE-S3 (AES256)</option>
              <option value="aws:kms">SSE-KMS</option>
            </select>
            {#if encryption === 'aws:kms'}<input bind:value={kmsKey} placeholder={t('kmsKeyPlaceholder')} spellcheck="false" />{/if}
            <button type="button" class="primary sm" disabled={busy} onclick={() => save(() => api.SetBucketEncryption(bucket, encryption, kmsKey))}>{t('save')}</button>
          </div>
        </section>
        <section>
          <h4>{t('objectLock')}</h4>
          {#if unsupported('objectLock')}<p class="muted">{unsupported('objectLock')}</p>
          {:else if settings.objectLock}
            <p class="hint">{settings.objectLock.enabled ? t('objectLockOn') : t('objectLockOffHint')}</p>
            <div class="row">
              <select bind:value={lockMode} disabled={busy}>
                <option value="">{t('noDefaultRetention')}</option>
                <option value="GOVERNANCE">GOVERNANCE</option>
                <option value="COMPLIANCE">COMPLIANCE</option>
              </select>
              {#if lockMode}
                <input type="number" min="1" bind:value={lockDays} disabled={busy} aria-label={t('retentionPeriod')} />
                <select bind:value={lockUnit} disabled={busy}><option value="days">{t('days')}</option><option value="years">{t('years')}</option></select>
              {/if}
              <button type="button" class="primary sm" disabled={busy || (!settings.objectLock.enabled && !lockAcknowledged)} onclick={() => save(() => api.SetObjectLock(bucket, lockMode, lockUnit === 'days' && lockMode ? Number(lockDays) : 0, lockUnit === 'years' && lockMode ? Number(lockDays) : 0))}>
                {settings.objectLock.enabled ? t('save') : t('enableObjectLock')}
              </button>
            </div>
            <p class="hint warn">{t('objectLockWarning')}</p>
            {#if !settings.objectLock.enabled}
              <label><input type="checkbox" bind:checked={lockAcknowledged} /> {t('objectLockAcknowledge')}</label>
            {/if}
          {/if}
        </section>
        <section>
          <h4>{t('publicAccess')}</h4>
          {#if unsupported('publicAccessBlock')}<p class="muted">{unsupported('publicAccessBlock')}</p>
          {:else}
            <label><input type="checkbox" bind:checked={block.blockPublicAcls} /> {t('blockPublicAcls')}</label>
            <label><input type="checkbox" bind:checked={block.ignorePublicAcls} /> {t('ignorePublicAcls')}</label>
            <label><input type="checkbox" bind:checked={block.blockPublicPolicy} /> {t('blockPublicPolicy')}</label>
            <label><input type="checkbox" bind:checked={block.restrictPublicBuckets} /> {t('restrictPublicBuckets')}</label>
            <div class="row end"><button type="button" class="primary sm" disabled={busy} onclick={() => save(() => api.SetPublicAccessBlock(bucket, $state.snapshot(block)))}>{t('save')}</button></div>
          {/if}
        </section>
      {:else if tab === 'hosting'}
        <section class="first">
          <h4>{t('websiteHosting')}</h4>
          {#if unsupported('website')}<p class="muted">{unsupported('website')}</p>{/if}
          <p class="hint">{t('websiteHint')}</p>
          <div class="row">
            <input bind:value={websiteIndex} placeholder="index.html" spellcheck="false" aria-label={t('indexDocument')} />
            <input bind:value={websiteError} placeholder="error.html" spellcheck="false" aria-label={t('errorDocument')} />
            <button type="button" class="primary sm" disabled={busy} onclick={() => save(() => api.SetBucketWebsite(bucket, websiteIndex, websiteError))}>{t('save')}</button>
          </div>
          {#if settings.websiteIndex}<p class="hint">{t('websiteOn', { index: settings.websiteIndex })}</p>{:else}<p class="hint">{t('websiteOff')}</p>{/if}
        </section>
        <section>
          <h4>{t('accessLogging')}</h4>
          {#if unsupported('logging')}<p class="muted">{unsupported('logging')}</p>{/if}
          <p class="hint">{t('loggingHint')}</p>
          <div class="row">
            <input bind:value={loggingBucket} placeholder={t('targetBucket')} spellcheck="false" aria-label={t('targetBucket')} />
            <input bind:value={loggingPrefix} placeholder="logs/" spellcheck="false" aria-label={t('targetPrefix')} />
            <button type="button" class="primary sm" disabled={busy} onclick={() => save(() => api.SetBucketLogging(bucket, loggingBucket, loggingPrefix))}>{t('save')}</button>
          </div>
        </section>
        <section>
          <h4>{t('requesterPays')}</h4>
          {#if unsupported('requesterPays')}<p class="muted">{unsupported('requesterPays')}</p>
          {:else}
            <div class="row">
              <span class="hint">{settings.requesterPays ? t('requesterPaysOn') : t('requesterPaysOff')}</span>
              <span style="flex:1"></span>
              <button type="button" class="sm" disabled={busy} onclick={() => save(() => api.SetRequesterPays(bucket, !settings!.requesterPays))}>{settings.requesterPays ? t('disable') : t('enable')}</button>
            </div>
          {/if}
        </section>
      {:else if tab === 'policy'}
        {#if unsupported('policy')}<p class="muted">{unsupported('policy')}</p>{/if}
        <p class="hint">{t('policyHint')}</p>
        <textarea class="mono" bind:value={policy} rows="14" spellcheck="false" placeholder={'{ "Version": "2012-10-17", "Statement": [] }'}></textarea>
        <div class="row end">
          <button type="button" class="sm" disabled={busy} onclick={() => (policy = policyTemplate())}>{t('insertExample')}</button>
          <button type="button" class="sm" disabled={busy || !policy.trim()} onclick={() => (policy = '')}>{t('clear')}</button>
          <button type="button" class="primary sm" disabled={busy} onclick={() => save(() => api.SetBucketPolicy(bucket, policy))}>{t('save')}</button>
        </div>
      {:else if tab === 'acl'}
        <p class="hint">{t('bucketAclHint')}</p>
        <AclEditor load={() => api.GetBucketAcl(bucket)} save={(acl) => api.SetBucketAcl(bucket, acl)} onsaved={() => onnotify(t('settingsSaved'))} />
      {:else if tab === 'cloudfront'}
        <p class="hint">{t('cloudFrontHint')}</p>
        {#if !distributions}
          <p class="muted">{t('loading')}</p>
        {:else}
          {#each distributions as d (d.id)}
            <section>
              <div class="row"><b>{d.domain}</b><span class="muted">{d.id} · {d.status}{d.enabled ? '' : ' · ' + t('disabled')}</span></div>
              {#if d.aliases.length}<p class="hint">{d.aliases.join(', ')}</p>{/if}
              <p class="hint">{t('originLabel')}: {d.origin}{d.comment ? ' · ' + d.comment : ''}</p>
              <div class="row">
                <input bind:value={invalidation} spellcheck="false" aria-label={t('invalidationPaths')} />
                <button type="button" class="sm" disabled={busy} onclick={() => invalidate(d.id)}><i class="ri-refresh-line"></i> {t('invalidate')}</button>
              </div>
            </section>
          {:else}
            <p class="muted">{t('noDistributions')}</p>
          {/each}
          <div class="row end"><button type="button" class="sm" disabled={busy} onclick={loadDistributions}>{t('refresh')}</button></div>
        {/if}
      {:else if tab === 'cors'}
        {#if unsupported('cors')}<p class="muted">{unsupported('cors')}</p>{/if}
        <p class="hint">{t('corsHint')}</p>
        <textarea class="mono" bind:value={cors} rows="14" spellcheck="false" placeholder="[]"></textarea>
        <div class="row end">
          <button type="button" class="sm" disabled={busy} onclick={() => (cors = corsTemplate)}>{t('insertExample')}</button>
          <button type="button" class="sm" disabled={busy || !cors.trim()} onclick={() => (cors = '')}>{t('clear')}</button>
          <button type="button" class="primary sm" disabled={busy} onclick={() => save(() => api.SetBucketCors(bucket, cors))}>{t('save')}</button>
        </div>
      {:else if tab === 'lifecycle'}
        {#if unsupported('lifecycle')}<p class="muted">{unsupported('lifecycle')}</p>{/if}
        <p class="hint">{t('lifecycleHint')}</p>
        <textarea class="mono" bind:value={lifecycle} rows="14" spellcheck="false" placeholder="[]"></textarea>
        <div class="row end">
          <button type="button" class="sm" disabled={busy} onclick={() => (lifecycle = lifecycleTemplate)}>{t('insertExample')}</button>
          <button type="button" class="sm" disabled={busy || !lifecycle.trim()} onclick={() => (lifecycle = '')}>{t('clear')}</button>
          <button type="button" class="primary sm" disabled={busy} onclick={() => save(() => api.SetBucketLifecycle(bucket, lifecycle))}>{t('save')}</button>
        </div>
      {:else}
        {#if unsupported('tags')}<p class="muted">{unsupported('tags')}</p>{/if}
        <TagEditor bind:values={tags} keyLabel={t('tagKey')} valueLabel={t('tagValue')} />
        <div class="row end"><button type="button" class="primary sm" disabled={busy} onclick={() => save(() => api.SetBucketTags(bucket, $state.snapshot(tags)))}>{t('save')}</button></div>
      {/if}
    {/if}
    {#if error}<div class="msg" role="alert">{error}</div>{/if}
  </div>
</Modal>

<style>
  .modal { width: min(640px, 94vw); display: flex; flex-direction: column; gap: 12px; background: var(--panel); border: 1px solid var(--border); border-radius: var(--radius-lg); padding: 20px 22px; box-shadow: var(--shadow); }
  header { display: flex; justify-content: space-between; align-items: center; }
  h3 { margin: 0; font-size: 15px; font-weight: 650; color: var(--text-strong); display: flex; align-items: center; gap: 8px; }
  h4 { margin: 0 0 6px; font-size: 13px; font-weight: 600; color: var(--text-strong); }
  .tabs { display: flex; gap: 2px; border-bottom: 1px solid var(--border); padding-bottom: 6px; }
  .tabs .on { background: var(--panel2); color: var(--text-strong); }
  dl { display: grid; grid-template-columns: max-content 1fr; gap: 6px 14px; margin: 0; font-size: 13px; }
  dt { color: var(--muted); } dd { margin: 0; display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
  section { display: flex; flex-direction: column; gap: 6px; padding-top: 8px; border-top: 1px solid var(--border); }
  section.first { border-top: 0; padding-top: 0; }
  section label { display: flex; flex-direction: row; align-items: center; gap: 8px; font-size: 13px; }
  .row { display: flex; gap: 8px; align-items: center; } .row input { flex: 1; min-width: 140px; } .row select { flex: 1; } .row.end { justify-content: flex-end; }
  textarea { width: 100%; resize: vertical; min-height: 160px; font-size: 12px; line-height: 1.45; }
  .hint { margin: 0; color: var(--muted); font-size: 12px; line-height: 1.5; }
  .hint.warn { color: var(--danger-text); }
  .row input[type=number] { width: 80px; flex: 0; }
  .msg { padding: 8px 10px; border-radius: var(--radius-md); background: var(--danger-soft); color: var(--danger-text); user-select: text; word-break: break-word; }
</style>
