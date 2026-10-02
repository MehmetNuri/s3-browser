<script lang="ts">
  import { onDestroy } from 'svelte'
  import { RunCapabilities, CopyToClipboard } from './api'
  import { EventsOn, errorText } from './runtime'
  import type { main } from './models'
  import { t, type Key } from './i18n.svelte'

  let { buckets, bucket, profileName }: { buckets: main.Bucket[]; bucket: string; profileName: string } = $props()

  // Follows the open bucket until the user picks a target, including "service only".
  let chosen = $state<string | null>(null)
  let target = $derived(chosen ?? bucket)
  let includeBucket = $state(false)
  let running = $state(false)
  let results = $state<main.CapResult[]>([])
  let error = $state('')

  const off = EventsOn('cap', (r: main.CapResult) => { if (running) results.push(r) })
  onDestroy(off)

  const label = (status: string) => t(`st_${status}` as Key)

  let groups = $derived.by(() => {
    const m = new Map<string, main.CapResult[]>()
    for (const r of results) {
      if (!m.has(r.category)) m.set(r.category, [])
      m.get(r.category)!.push(r)
    }
    return [...m.entries()]
  })
  let counts = $derived(results.reduce((a: Record<string, number>, r) => ((a[r.status] = (a[r.status] ?? 0) + 1), a), {}))

  async function run() {
    running = true; results = []; error = ''
    try { results = await RunCapabilities({ bucket: target, includeBucket } as main.CapOptions) }
    catch (e) { error = errorText(e) }
    running = false
  }

  function copyReport() {
    const lines = [`# ${t('reportTitle')} — ${profileName} / ${target}`, '']
    for (const [cat, rs] of groups) {
      lines.push(`## ${cat}`)
      for (const r of rs) lines.push(`- [${label(r.status)}] ${r.name}${r.detail ? ' — ' + r.detail : ''}`)
      lines.push('')
    }
    CopyToClipboard(lines.join('\n')).catch((e) => (error = errorText(e)))
  }
</script>

<div class="cap">
  <div class="bar">
    <label>{t('testBucket')}
      <select bind:value={() => target, (value) => (chosen = value)} disabled={running}>
        <option value="">{t('serviceOnly')}</option>
        {#each buckets as b}<option value={b.name}>{b.name}</option>{/each}
      </select>
    </label>
    <label class="check"><input type="checkbox" bind:checked={includeBucket} disabled={running} /> {t('tempBucket')}</label>
    <span style="flex:1"></span>
    {#if results.length && !running}<button onclick={copyReport}><i class="ri-file-copy-line"></i> {t('copyReport')}</button>{/if}
    <button class="primary" onclick={run} disabled={running}>{#if running}<i class="ri-loader-4-line spin"></i> {t('running')}{:else}<i class="ri-play-line"></i> {t('runTest')}{/if}</button>
  </div>

  <p class="note muted">{t('capNote', { dir: '.s3browser-captest/' })}</p>

  {#if error}<div class="err selectable">{error}</div>{/if}

  {#if results.length}
    <div class="summary">
      {#each ['ok', 'unsupported', 'denied', 'error', 'skipped'] as s}
        {#if counts[s]}<span class="pill {s}">{label(s)}: {counts[s]}</span>{/if}
      {/each}
    </div>
  {/if}

  <div class="list">
    {#each groups as [cat, rs]}
      <h4>{cat}</h4>
      <table>
        <tbody>
          {#each rs as r (r.id)}
            <tr>
              <td class="st"><span class="pill {r.status}">{label(r.status)}</span></td>
              <td class="name mono">{r.name}</td>
              <td class="detail muted selectable">{r.detail}</td>
              <td class="ms muted">{r.status !== 'skipped' ? r.ms + ' ms' : ''}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/each}
    {#if !results.length && !running}
      <div class="empty muted">{t('capEmpty')}</div>
    {/if}
  </div>
</div>

<style>
  .cap { display: flex; flex-direction: column; height: 100%; min-height: 0; }
  .bar { display: flex; align-items: end; gap: 14px; padding: 12px 16px; border-bottom: 1px solid var(--border); }
  .bar label:first-child { width: 260px; }
  .note { margin: 0; padding: 8px 16px; font-size: 12px; }
  .err { margin: 8px 16px; padding: 8px 10px; border-radius: var(--radius-md); background: var(--danger-soft); color: var(--danger-text); }
  .summary { display: flex; gap: 8px; padding: 4px 16px 8px; }
  .list { flex: 1; overflow: auto; padding: 0 16px 16px; }
  h4 { margin: 16px 0 8px; font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: .06em; color: var(--muted); }
  table { width: 100%; table-layout: fixed; border: 1px solid var(--border); border-radius: var(--radius-md); border-spacing: 0; border-collapse: separate; overflow: hidden; }
  td { padding: 7px 10px; border-top: 1px solid var(--border); vertical-align: top; }
  tr:first-child td { border-top: none; }
  .st { width: 130px; }
  .name { width: 34%; overflow: hidden; text-overflow: ellipsis; }
  .detail { word-break: break-word; }
  .ms { width: 70px; text-align: right; }
  .pill { display: inline-block; padding: 2px 8px; border-radius: 99px; font-size: 11px; font-weight: 600; }
  .pill.ok { background: var(--success-soft); color: var(--success); }
  .pill.unsupported { background: var(--warn-soft); color: var(--warn); }
  .pill.denied { background: var(--info-soft); color: var(--info); }
  .pill.error { background: var(--danger-soft); color: var(--danger-text); }
  .pill.skipped { background: var(--neutral-soft); color: var(--muted); }
  .empty { padding: 40px; text-align: center; }
  .spin { display: inline-block; animation: spin 1s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }
</style>
