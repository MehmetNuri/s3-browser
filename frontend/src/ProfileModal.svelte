<script lang="ts">
  import { errorText } from './runtime'
  import { SaveProfile, TestProfile } from './api'
  import type { main } from './models'
  import { t } from './i18n.svelte'
  import Modal from './Modal.svelte'

  let { profile, onclose, onsaved }: {
    profile: main.Profile | null
    onclose: () => void
    onsaved: (p: main.Profile) => void
  } = $props()

  const empty = {
    id: '', name: '', provider: 'supabase', endpoint: '', region: '', accessKey: '',
    secretKey: '', sessionToken: '', pathStyle: true, skipTLS: false, projectRef: '', buckets: [] as string[],
    storageClass: '', encryption: '', kmsKey: '',
  }
  const storageClassChoices = ['STANDARD', 'STANDARD_IA', 'ONEZONE_IA', 'INTELLIGENT_TIERING', 'GLACIER_IR', 'GLACIER', 'DEEP_ARCHIVE']
  // svelte-ignore state_referenced_locally
  let p = $state<main.Profile>({ ...empty, ...(profile ?? {}) })
  let bucketText = $state((p.buckets ?? []).join(', '))
  let msg = $state('')
  let msgOk = $state(false)
  let busy = $state(false)
  let showSecret = $state(false)

  // Presets fill the endpoint from the region; the user can still edit it.
  type Preset = { id: string; label: string; endpoint?: string; region?: string; pathStyle?: boolean; regionHint?: string }
  const presets: Preset[] = [
    { id: 'supabase', label: 'Supabase Storage', pathStyle: true },
    { id: 'aws', label: 'Amazon S3', pathStyle: false, regionHint: 'us-east-1' },
    { id: 'minio', label: 'MinIO', pathStyle: true, regionHint: 'us-east-1' },
    { id: 'r2', label: 'Cloudflare R2', pathStyle: false, region: 'auto' },
    { id: 'backblaze', label: 'Backblaze B2', endpoint: 'https://s3.{region}.backblazeb2.com', pathStyle: true, regionHint: 'us-west-004' },
    { id: 'wasabi', label: 'Wasabi', endpoint: 'https://s3.{region}.wasabisys.com', pathStyle: false, region: 'us-east-1' },
    { id: 'digitalocean', label: 'DigitalOcean Spaces', endpoint: 'https://{region}.digitaloceanspaces.com', pathStyle: false, regionHint: 'nyc3' },
    { id: 'hetzner', label: 'Hetzner Object Storage', endpoint: 'https://{region}.your-objectstorage.com', pathStyle: false, regionHint: 'fsn1' },
    { id: 'scaleway', label: 'Scaleway Object Storage', endpoint: 'https://s3.{region}.scw.cloud', pathStyle: false, regionHint: 'fr-par' },
    { id: 'ovh', label: 'OVHcloud Object Storage', endpoint: 'https://s3.{region}.io.cloud.ovh.net', pathStyle: false, regionHint: 'gra' },
    { id: 'linode', label: 'Akamai (Linode) Object Storage', endpoint: 'https://{region}.linodeobjects.com', pathStyle: false, regionHint: 'us-east-1' },
    { id: 'exoscale', label: 'Exoscale SOS', endpoint: 'https://sos-{region}.exo.io', pathStyle: false, regionHint: 'ch-gva-2' },
    { id: 'storj', label: 'Storj', endpoint: 'https://gateway.storjshare.io', pathStyle: true, region: 'us-1' },
    { id: 'gcs', label: 'Google Cloud Storage (HMAC)', endpoint: 'https://storage.googleapis.com', pathStyle: true, region: 'auto' },
    { id: 'custom', label: t('otherS3') },
  ]
  let providers = $derived(presets.map((preset) => ({ id: preset.id, label: preset.label })))
  let preset = $derived(presets.find((item) => item.id === p.provider))
  let autoEndpoint = $state(false)

  let supaEndpoint = $derived(p.projectRef ? `https://${p.projectRef}.storage.supabase.co/storage/v1/s3` : '')

  function presetEndpoint(): string {
    const template = preset?.endpoint ?? ''
    return template.includes('{region}') ? (p.region ? template.replace('{region}', p.region) : '') : template
  }
  function onProvider() {
    if (preset?.pathStyle !== undefined) p.pathStyle = preset.pathStyle
    if (preset?.region && !p.region) p.region = preset.region
    if (preset?.endpoint && (autoEndpoint || !p.endpoint)) { p.endpoint = presetEndpoint(); autoEndpoint = true }
  }
  function onRegion() {
    if (preset?.endpoint?.includes('{region}') && (autoEndpoint || !p.endpoint)) { p.endpoint = presetEndpoint(); autoEndpoint = true }
  }

  function withBuckets() {
    return { ...p, buckets: bucketText.split(/[\s,]+/).filter(Boolean) }
  }

  async function test() {
    busy = true; msg = ''
    try { msg = await TestProfile(withBuckets()); msgOk = true }
    catch (e) { msg = errorText(e); msgOk = false }
    busy = false
  }

  async function save() {
    busy = true; msg = ''
    try { onsaved(await SaveProfile(withBuckets())) }
    catch (e) { msg = errorText(e); msgOk = false }
    busy = false
  }
</script>

<Modal label={profile ? t('editProfile') : t('newConnection')} {onclose}>
  <div class="modal">
    <header>
      <h3>{profile ? t('editProfile') : t('newConnection')}</h3>
      <button class="ghost" title={t('close')} onclick={onclose}><i class="ri-close-line"></i></button>
    </header>

    <div class="grid">
      <!-- svelte-ignore a11y_autofocus -->
      <label>{t('profileName')}<input autofocus bind:value={p.name} placeholder="Supabase prod" /></label>
      <label>{t('provider')}
        <select bind:value={p.provider} onchange={onProvider}>
          {#each providers as pr}<option value={pr.id}>{pr.label}</option>{/each}
        </select>
      </label>

      {#if p.provider === 'supabase'}
        <label>Project ref<input bind:value={p.projectRef} placeholder="abcdefghijklmnop" /></label>
        <label>Region<input bind:value={p.region} placeholder="eu-central-1" /></label>
        <label class="full">{t('supabaseEndpoint')}
          <input bind:value={p.endpoint} placeholder={supaEndpoint || 'https://<ref>.storage.supabase.co/storage/v1/s3'} />
        </label>
        <p class="hint full">{t('supabaseHint')}</p>
      {:else}
        <label class="full">Endpoint {p.provider === 'aws' ? t('optional') : ''}
          <input bind:value={p.endpoint} oninput={() => (autoEndpoint = false)} placeholder={p.provider === 'r2' ? 'https://<account>.r2.cloudflarestorage.com' : p.provider === 'minio' ? 'http://localhost:9000' : preset?.endpoint?.replace('{region}', preset.regionHint ?? 'region') ?? 'https://s3.example.com'} />
        </label>
        <label>Region<input bind:value={p.region} oninput={onRegion} placeholder={preset?.region ?? preset?.regionHint ?? 'us-east-1'} /></label>
        <div class="checks">
          <label class="check"><input type="checkbox" bind:checked={p.pathStyle} /> Path-style</label>
          <label class="check"><input type="checkbox" bind:checked={p.skipTLS} /> {t('skipTLS')}</label>
        </div>
      {/if}

      <label>Access key ID<input bind:value={p.accessKey} autocomplete="off" spellcheck="false" /></label>
      <label>Secret access key
        <div class="row">
          <input type={showSecret ? 'text' : 'password'} bind:value={p.secretKey} autocomplete="off" />
          <button class="ghost" onclick={() => (showSecret = !showSecret)} title={showSecret ? t('hide') : t('show')}><i class={showSecret ? 'ri-eye-off-line' : 'ri-eye-line'}></i></button>
        </div>
      </label>
      <label class="full">{t('sessionToken')}<input bind:value={p.sessionToken} autocomplete="off" /></label>
      <label class="full">{t('bucketsOptional')}
        <input bind:value={bucketText} placeholder="bucket-a, bucket-b" spellcheck="false" />
      </label>
      <p class="hint full">{t('bucketsHint')}</p>
      <label>{t('uploadStorageClass')}
        <select bind:value={p.storageClass}>
          <option value="">{t('providerDefault')}</option>
          {#each storageClassChoices as cls}<option value={cls}>{cls}</option>{/each}
        </select>
      </label>
      <label>{t('uploadEncryption')}
        <select bind:value={p.encryption}>
          <option value="">{t('providerDefault')}</option>
          <option value="AES256">SSE-S3 (AES256)</option>
          <option value="aws:kms">SSE-KMS</option>
        </select>
      </label>
      {#if p.encryption === 'aws:kms'}
        <label class="full">{t('kmsKey')}<input bind:value={p.kmsKey} placeholder={t('kmsKeyPlaceholder')} spellcheck="false" /></label>
      {/if}
      <p class="hint full">{t('uploadDefaultsHint')}</p>
      {#if p.provider === 'aws'}
        <p class="hint full">{t('awsHint')}</p>
      {/if}
    </div>

    {#if msg}<div class="msg" class:ok={msgOk}>{msg}</div>{/if}

    <footer>
      <button onclick={test} disabled={busy}>{t('testConnection')}</button>
      <span style="flex:1"></span>
      <button onclick={onclose}>{t('cancel')}</button>
      <button class="primary" onclick={save} disabled={busy || !p.name.trim()}>{t('save')}</button>
    </footer>
  </div>
</Modal>

<style>
  .modal { width: min(640px, 94vw); max-height: 92vh; overflow: auto; background: var(--panel); border: 1px solid var(--border); border-radius: var(--radius-lg); padding: 20px 22px; box-shadow: var(--shadow); }
  header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; }
  h3 { margin: 0; font-size: 15px; font-weight: 650; color: var(--text-strong); }
  .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
  .full { grid-column: 1 / -1; }
  .checks { display: flex; gap: 14px; align-items: end; padding-bottom: 6px; }
  .row { display: flex; gap: 6px; }
  .hint { margin: -4px 0 0; color: var(--muted); font-size: 12px; line-height: 1.5; }
  .msg { white-space: pre-line; margin-top: 12px; padding: 8px 10px; border-radius: var(--radius-md); background: var(--danger-soft); color: var(--danger-text); user-select: text; word-break: break-word; }
  .msg.ok { background: var(--success-soft); color: var(--success); }
  footer { display: flex; gap: 8px; margin-top: 16px; }
</style>
