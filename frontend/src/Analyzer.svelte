<script lang="ts">
  import { errorText } from './runtime'
  import { AbortStaleUploads, AnalyzeBucket, ListIncompleteUploads, type AnalysisEntry, type BucketAnalysis, type IncompleteUpload } from './api'
  import { t, type Key } from './i18n.svelte'

  let { bucket, prefix, onreveal }: { bucket: string; prefix: string; onreveal: (key: string) => void } = $props()

  // svelte-ignore state_referenced_locally
  let scope = $state(prefix)
  let result = $state<BucketAnalysis | null>(null)
  let analyzed = $state('')
  let running = $state(false)
  let error = $state('')
  let seq = 0
  // null: the server cannot list them
  let uploads = $state<IncompleteUpload[] | null>([])
  let confirming = $state(false)
  let notice = $state('')
  let stale = $derived((uploads ?? []).filter((u) => u.stale))

  async function loadUploads() {
    confirming = false
    try { uploads = (await ListIncompleteUploads(bucket)) ?? [] } catch { uploads = null }
  }

  async function abortStale() {
    if (!confirming) { confirming = true; return }
    confirming = false; error = ''
    try { notice = t('nAborted', { n: await AbortStaleUploads(bucket) }) } catch (e) { error = errorText(e) }
    await loadUploads()
  }

  let crumbs = $derived.by(() => {
    const parts = scope.split('/').filter(Boolean)
    return parts.map((name, i) => ({ name, path: parts.slice(0, i + 1).join('/') + '/' }))
  })

  async function run(target = scope) {
    const mine = ++seq
    scope = target; running = true; error = ''; notice = ''
    loadUploads()
    try {
      const res = await AnalyzeBucket(bucket, target)
      if (mine === seq) { result = res; analyzed = target }
    } catch (e) {
      if (mine === seq) error = errorText(e)
    }
    if (mine === seq) running = false
  }

  function fmtSize(n: number) {
    if (!n) return '0 B'
    const u = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
    const i = Math.min(Math.floor(Math.log(n) / Math.log(1024)), u.length - 1)
    return (n / 1024 ** i).toFixed(i ? 1 : 0) + ' ' + u[i]
  }
  const count = (n: number) => n.toLocaleString()
  const share = (size: number) => result?.size ? Math.round((size / result.size) * 1000) / 10 : 0

  const ageLabels: Record<string, Key> = { '30d': 'age30d', '90d': 'age90d', '1y': 'age1y', older: 'ageOlder' }
  const folderName = (name: string) => name || t('filesHere')
  const typeName = (name: string) => name === '*' ? t('other') : name ? '.' + name : t('noExtension')
  const ageName = (name: string) => t(ageLabels[name] ?? 'other')
</script>

<!-- One measure per list, so every bar uses the same hue and each row carries its own label and value. -->
{#snippet bars(title: string, entries: AnalysisEntry[], label: (name: string) => string, open?: (name: string) => void)}
  {@const max = Math.max(1, ...entries.map((e) => e.size))}
  <section class="card">
    <h4>{title}</h4>
    {#each entries as e (e.name)}
      {@const hint = `${label(e.name)} — ${fmtSize(e.size)} · ${share(e.size)}% · ${t('nObjectsShort', { n: count(e.objects) })}`}
      <div class="row" title={hint}>
        {#if open && e.name}
          <button class="link" onclick={() => open(e.name)}><i class="ri-folder-3-fill"></i>{label(e.name)}</button>
        {:else}
          <span class="label">{label(e.name)}</span>
        {/if}
        <span class="track">{#if e.size}<span class="bar" style="width:{(e.size / max) * 100}%"></span>{/if}</span>
        <span class="value">{fmtSize(e.size)}</span>
        <span class="muted count">{count(e.objects)}</span>
      </div>
    {:else}
      <p class="muted">—</p>
    {/each}
  </section>
{/snippet}

<div class="analyzer">
  <div class="top">
    <div class="crumbs">
      <button class="ghost" onclick={() => run('')} disabled={running}><i class="ri-archive-drawer-line"></i> {bucket}</button>
      {#each crumbs as c}<span class="muted">/</span><button class="ghost" onclick={() => run(c.path)} disabled={running}>{c.name}</button>{/each}
    </div>
    <span style="flex:1"></span>
    <button class="primary" onclick={() => run()} disabled={running}>
      {#if running}<i class="ri-loader-4-line spin"></i> {t('analyzing')}{:else}<i class="ri-pie-chart-2-line"></i> {t('analyze')}{/if}
    </button>
  </div>

  <div class="body">
    {#if error}<div class="err selectable" role="alert">{error}</div>{/if}
    {#if !result}
      <div class="empty muted">{running ? t('analyzing') : t('analyzeHint')}</div>
    {:else}
      {#if result.truncated}<div class="warn">{t('scanTruncated', { n: count(result.objects) })}</div>{/if}
      <div class="tiles">
        <div class="tile"><span class="muted">{t('totalSize')}</span><b>{fmtSize(result.size)}</b></div>
        <div class="tile"><span class="muted">{t('objectsCount')}</span><b>{count(result.objects)}</b></div>
        <div class="tile"><span class="muted">{t('wastedSpace')}</span><b>{fmtSize(result.wasted)}</b><span class="muted small">{share(result.wasted)}%</span></div>
        <div class="tile"><span class="muted">{t('averageSize')}</span><b>{fmtSize(result.objects ? result.size / result.objects : 0)}</b></div>
      </div>

      <div class="grid">
        {@render bars(t('byFolder'), result.folders, folderName, (name) => run(analyzed + name))}
        {@render bars(t('byType'), result.types, typeName)}
        {@render bars(t('byAge'), result.ages, ageName)}
        {@render bars(t('byClass'), result.classes, (name) => name)}
      </div>

      <section class="card">
        <h4>{t('largestObjects')}</h4>
        <table>
          <tbody>
            {#each result.largest as o (o.key)}
              <tr>
                <td class="key"><button class="link mono" title={t('showInBrowser')} onclick={() => onreveal(o.key)}>{o.key}</button></td>
                <td class="muted">{o.modified}</td>
                <td class="value">{fmtSize(o.size)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </section>

      <section class="card">
        <h4>{t('duplicates')} <span class="muted hint">{t('duplicatesHint')}</span></h4>
        {#each result.duplicates as d (d.keys[0])}
          <div class="dup">
            <div class="duphead">
              <span>{t('copiesN', { n: count(d.copies), size: fmtSize(d.size) })}</span>
              <b>{t('wastedN', { size: fmtSize(d.wasted) })}</b>
            </div>
            {#each d.keys as key}
              <button class="link mono" title={t('showInBrowser')} onclick={() => onreveal(key)}>{key}</button>
            {/each}
            {#if d.copies > d.keys.length}<span class="muted">+{count(d.copies - d.keys.length)}</span>{/if}
          </div>
        {:else}
          <p class="muted">{t('noDuplicates')}</p>
        {/each}
      </section>

      <section class="card">
        <h4>{t('incompleteUploads')} <span class="muted hint">{t('incompleteHint')}</span></h4>
        {#if notice}<p class="notice">{notice}</p>{/if}
        {#if uploads === null}
          <p class="muted">{t('incompleteUnknown')}</p>
        {:else}
          <table>
            <tbody>
              {#each uploads as u (u.uploadId)}
                <tr>
                  <td class="key mono">{u.key}</td>
                  <td class="muted">{u.initiated}{#if !u.stale} · {t('recentUpload')}{/if}</td>
                  <td class="value">{u.size >= 0 ? fmtSize(u.size) : '—'}</td>
                </tr>
              {/each}
            </tbody>
          </table>
          {#if !uploads.length}<p class="muted">{t('noIncomplete')}</p>{/if}
          {#if stale.length}
            <button class="danger cleanup" onclick={abortStale}>
              <i class="ri-delete-bin-line"></i> {confirming ? t('abortConfirm', { n: stale.length }) : t('abortStale', { n: stale.length })}
            </button>
          {/if}
        {/if}
      </section>
    {/if}
  </div>
</div>

<style>
  .analyzer { display: flex; flex-direction: column; height: 100%; min-height: 0; }
  .top { display: flex; align-items: center; gap: 8px; padding: 8px 10px; border-bottom: 1px solid var(--border); }
  .crumbs { display: flex; align-items: center; min-width: 0; overflow: hidden; }
  .crumbs button { padding: 3px 7px; font-weight: 600; color: var(--text-strong); }
  .body { flex: 1; overflow: auto; padding: 16px; display: flex; flex-direction: column; gap: 14px; }
  .empty { margin: auto; max-width: 440px; text-align: center; line-height: 1.6; }
  .err, .warn { padding: 8px 12px; border-radius: var(--radius-md); }
  .err { background: var(--danger-soft); color: var(--danger-text); }
  .warn { background: var(--warn-soft); color: var(--warn); }

  .tiles { display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); gap: 12px; }
  .tile { display: flex; flex-direction: column; gap: 4px; padding: 12px 14px; border: 1px solid var(--border); border-radius: var(--radius-md); font-size: 12px; }
  .tile b { font-size: 20px; font-weight: 650; letter-spacing: -.02em; color: var(--text-strong); }
  .small { font-size: 11px; }

  .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(360px, 1fr)); gap: 14px; }
  .card { border: 1px solid var(--border); border-radius: var(--radius-md); padding: 12px 14px; min-width: 0; }
  h4 { margin: 0 0 8px; font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: .05em; color: var(--muted); }
  .hint { margin-inline-start: 6px; font-weight: 400; text-transform: none; letter-spacing: 0; }
  p { margin: 0; }

  .row { display: grid; grid-template-columns: minmax(0, 150px) minmax(40px, 1fr) 76px 64px; align-items: center; gap: 10px; padding: 4px 0; font-size: 12px; }
  .label, .link { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .link { display: block; min-height: 0; padding: 0; border: none; background: none; box-shadow: none; text-align: start; color: var(--text); max-width: 100%; }
  .link:hover:not(:disabled) { background: none; color: var(--accent); }
  .link i { margin-inline-end: 6px; color: var(--accent); vertical-align: -2px; }
  .track { height: 8px; }
  .bar { display: block; height: 100%; min-width: 2px; border-radius: 0 4px 4px 0; background: var(--accent); }
  .row:hover .bar { background: var(--accent2); }
  .value { text-align: end; color: var(--text-strong); white-space: nowrap; }
  .count { text-align: end; }

  table { width: 100%; border-collapse: collapse; table-layout: fixed; font-size: 12px; }
  td { padding: 4px 0; border-top: 1px solid var(--row-border); white-space: nowrap; }
  tr:first-child td { border-top: none; }
  td.key { width: 60%; overflow: hidden; }
  td.value { width: 90px; }

  .dup { display: flex; flex-direction: column; gap: 3px; padding: 8px 0; border-top: 1px solid var(--row-border); font-size: 12px; }
  .dup:first-of-type { border-top: none; padding-top: 0; }
  .duphead { display: flex; justify-content: space-between; gap: 10px; margin-bottom: 2px; }
  .duphead b { color: var(--text-strong); }
  .cleanup { margin-top: 10px; }
  .notice { margin-bottom: 8px; color: var(--success); }
  .spin { display: inline-block; animation: spin 1s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }
</style>
