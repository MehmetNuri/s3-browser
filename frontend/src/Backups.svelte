<script lang="ts">
  import { onMount } from 'svelte'
  import { CreateBackupJob, DeleteBackupJob, ListBackupJobs, RunBackupJob, type BackupJob } from './api'
  import { EventsOn, errorText } from './runtime'
  import type { main } from './models'
  import { t, i18n, type Key } from './i18n.svelte'

  let { profiles, buckets, bucket, prefix }: { profiles: main.Profile[]; buckets: main.Bucket[]; bucket: string; prefix: string } = $props()

  let jobs = $state<BackupJob[]>([])
  let error = $state('')
  let creating = $state(false)
  // svelte-ignore state_referenced_locally
  let draft = $state({ name: '', bucket, prefix, mirror: 'false', interval: '0' })

  const intervals: { value: string; label: Key }[] = [
    { value: '0', label: 'scheduleManual' }, { value: '15', label: 'schedule15m' }, { value: '60', label: 'scheduleHourly' },
    { value: '360', label: 'schedule6h' }, { value: '1440', label: 'scheduleDaily' },
  ]
  const intervalLabel = (minutes: number) => t(intervals.find((i) => Number(i.value) === minutes)?.label ?? 'scheduleManual')
  const profileName = (id: string) => profiles.find((p) => p.id === id)?.name ?? '—'
  const when = (stamp: string) => stamp ? new Date(stamp).toLocaleString(i18n.lang) : t('neverRan')

  async function load() {
    try { jobs = (await ListBackupJobs()) ?? [] } catch (e) { error = errorText(e) }
  }
  onMount(() => {
    load()
    return EventsOn('backup', load) // a job started or finished, possibly on schedule
  })

  async function create(event: SubmitEvent) {
    event.preventDefault()
    creating = true; error = ''
    try {
      const job = await CreateBackupJob(draft.name, draft.bucket, draft.prefix, draft.mirror === 'true', Number(draft.interval))
      if (job.id) { draft.name = ''; await load() } // an empty id means the folder dialog was cancelled
    } catch (e) { error = errorText(e) }
    creating = false
  }

  async function run(job: BackupJob) {
    error = ''
    try { await RunBackupJob(job.id) } catch (e) { error = errorText(e) }
    load()
  }

  async function remove(job: BackupJob) {
    error = ''
    try { await DeleteBackupJob(job.id) } catch (e) { error = errorText(e) }
    load()
  }
</script>

<div class="backups">
  <form class="new" onsubmit={create}>
    <label class="wide">{t('backupName')}<input bind:value={draft.name} placeholder={t('backupNameHint')} disabled={creating} /></label>
    <label>{t('destBucket')}
      <select bind:value={draft.bucket} disabled={creating}>
        {#each buckets as b (b.name)}<option value={b.name}>{b.name}</option>{/each}
      </select>
    </label>
    <label>{t('destPrefix')}<input bind:value={draft.prefix} placeholder="backups/" spellcheck="false" disabled={creating} /></label>
    <label>{t('syncMode')}
      <select bind:value={draft.mirror} disabled={creating}>
        <option value="false">{t('modeAdd')}</option>
        <option value="true">{t('modeMirror')}</option>
      </select>
    </label>
    <label>{t('schedule')}
      <select bind:value={draft.interval} disabled={creating}>
        {#each intervals as i}<option value={i.value}>{t(i.label)}</option>{/each}
      </select>
    </label>
    <button type="submit" class="primary" disabled={creating || !draft.name.trim() || !draft.bucket}><i class="ri-folder-add-line"></i> {t('chooseFolderCreate')}</button>
    <p class="hint wide" class:danger={draft.mirror === 'true'}>{draft.mirror === 'true' ? t('modeMirrorHint') : t('modeAddHint')} {t('scheduleHint')}</p>
  </form>

  {#if error}<div class="err selectable" role="alert">{error}</div>{/if}

  <div class="list">
    {#each jobs as job (job.id)}
      <div class="job">
        <span class="state {job.running ? 'running' : job.lastStatus}" title={job.lastStatus}>
          <i class={job.running ? 'ri-loader-4-line spin' : job.lastStatus === 'error' ? 'ri-error-warning-line' : job.lastStatus === 'ok' ? 'ri-checkbox-circle-line' : 'ri-time-line'}></i>
        </span>
        <div class="meta">
          <b>{job.name}</b>
          <span class="mono path" title={job.dir}>{job.dir} → {job.bucket}/{job.prefix}</span>
          <span class="muted">
            {profileName(job.profileId)} · {job.mirror ? t('modeMirror') : t('modeAdd')} · {intervalLabel(job.intervalMinutes)} · {when(job.lastRun)}{#if job.lastMessage} · {job.lastMessage}{/if}
          </span>
        </div>
        <button onclick={() => run(job)} disabled={job.running}><i class="ri-play-line"></i> {t('runNow')}</button>
        <button class="ghost danger" title={t('delete')} onclick={() => remove(job)} disabled={job.running}><i class="ri-delete-bin-line"></i></button>
      </div>
    {:else}
      <div class="empty muted">{t('noBackups')}</div>
    {/each}
  </div>
</div>

<style>
  .backups { display: flex; flex-direction: column; height: 100%; min-height: 0; }
  .new { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)) auto; gap: 10px 12px; align-items: end; padding: 14px 16px; border-bottom: 1px solid var(--border); }
  .wide { grid-column: 1 / -1; }
  .hint { margin: 0; color: var(--muted); font-size: 12px; line-height: 1.5; }
  .hint.danger { color: var(--danger-text); }
  .err { margin: 12px 16px 0; padding: 8px 12px; border-radius: var(--radius-md); background: var(--danger-soft); color: var(--danger-text); }
  .list { flex: 1; overflow: auto; padding: 12px 16px; display: flex; flex-direction: column; gap: 8px; }
  .job { display: flex; align-items: center; gap: 12px; padding: 10px 12px; border: 1px solid var(--border); border-radius: var(--radius-md); }
  .meta { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; font-size: 12px; }
  .meta b { font-size: 13px; color: var(--text-strong); }
  .path, .meta .muted { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .state { display: grid; place-items: center; width: 30px; height: 30px; border-radius: 9px; background: var(--neutral-soft); color: var(--muted); flex: none; }
  .state.ok { background: var(--success-soft); color: var(--success); }
  .state.error { background: var(--danger-soft); color: var(--danger); }
  .state.running { background: var(--accent-soft); color: var(--accent); }
  .empty { margin: auto; max-width: 420px; text-align: center; line-height: 1.6; }
  .danger { color: var(--danger); }
  .spin { display: inline-block; animation: spin 1s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }
  @media (max-width: 1100px) { .new { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
</style>
