<script lang="ts">
  import { slide } from 'svelte/transition'
  import { prefersReducedMotion } from 'svelte/motion'
  import * as api from './api'
  import type { Transfer, TransferQueue } from './api'
  import { errorText } from './runtime'
  import { t, type Key } from './i18n.svelte'

  let {
    transfers, queue, fmtSize, onclose, onerror, onchanged,
  }: {
    transfers: Transfer[]; queue: TransferQueue; fmtSize: (n: number) => string
    onclose: () => void; onerror: (message: string) => void; onchanged: () => void
  } = $props()

  type Filter = 'all' | 'active' | 'failed' | 'done'
  let filter = $state<Filter>('all')
  let pending = $state<Set<number>>(new Set())
  let busy = $state(false)

  const filters: { value: Filter; label: Key }[] = [
    { value: 'all', label: 'queueAll' }, { value: 'active', label: 'queueActive' },
    { value: 'failed', label: 'queueFailed' }, { value: 'done', label: 'queueDone' },
  ]
  const limits = [1, 2, 4, 6, 8, 12, 16]
  // Kilobytes per second; 0 is unlimited.
  const bandwidths = [0, 256, 512, 1024, 2048, 5120, 10240, 20480, 51200]
  const bandwidthLabel = (kbps: number) => kbps === 0 ? t('unlimited') : kbps >= 1024 ? `${kbps / 1024} MB/s` : `${kbps} KB/s`
  const icons: Record<string, string> = {
    upload: 'ri-upload-2-line', download: 'ri-download-2-line', copy: 'ri-file-transfer-line', move: 'ri-file-transfer-line', sync: 'ri-loop-right-line',
  }
  const isActive = (tr: Transfer) => tr.state === 'running' || tr.state === 'queued'
  const isFailed = (tr: Transfer) => tr.state === 'error' || tr.state === 'cancelled'

  // Only this many rows are rendered; a folder of thousands of files would otherwise stall the window.
  const rowLimit = 150
  // Running first, then the queue in order, then the newest finished transfers.
  let rows = $derived.by(() => {
    const list = transfers.filter((tr) =>
      filter === 'all' || (filter === 'active' && isActive(tr)) || (filter === 'failed' && isFailed(tr)) || (filter === 'done' && tr.state === 'done'))
    const rank = (tr: Transfer) => tr.state === 'running' ? 0 : tr.state === 'queued' ? 1 : 2
    return list.sort((a, b) => rank(a) - rank(b) || (rank(a) === 2 ? b.id - a.id : rank(a) === 1 ? a.order - b.order : a.id - b.id))
  })
  let visibleRows = $derived(rows.slice(0, rowLimit))
  let bytesDone = $derived(transfers.reduce((sum, tr) => sum + (tr.state === 'done' ? tr.total : tr.done), 0))
  let bytesTotal = $derived(transfers.reduce((sum, tr) => sum + tr.total, 0))
  let remaining = $derived(queue.speed > 0 ? Math.max(0, bytesTotal - bytesDone) / queue.speed : 0)

  function fmtDuration(seconds: number) {
    if (!isFinite(seconds) || seconds <= 0) return ''
    if (seconds < 60) return t('etaSeconds', { n: Math.ceil(seconds) })
    if (seconds < 3600) return t('etaMinutes', { n: Math.ceil(seconds / 60) })
    return t('etaHours', { h: Math.floor(seconds / 3600), m: Math.ceil((seconds % 3600) / 60) })
  }

  function status(tr: Transfer) {
    switch (tr.state) {
      case 'error': return t('errorPrefix') + tr.error
      case 'cancelled': return t('transferCancelled')
      case 'done': return fmtSize(tr.total)
      case 'queued': return tr.total ? `${t('queued')} · ${fmtSize(tr.total)}` : t('queued')
      default: {
        const progress = `${fmtSize(tr.done)} / ${fmtSize(tr.total)}`
        return tr.speed ? `${progress} · ${fmtSize(tr.speed)}/s` : progress
      }
    }
  }

  async function run(fn: () => Promise<unknown>, refresh = false) {
    try { await fn(); if (refresh) onchanged() } catch (e) { onerror(errorText(e)) }
  }
  async function perItem(id: number, fn: () => Promise<unknown>, refresh = false) {
    if (pending.has(id)) return
    pending = new Set([...pending, id])
    try { await run(fn, refresh) } finally { pending = new Set([...pending].filter((value) => value !== id)) }
  }
  // Queued rows can be dragged onto another queued row (placed before it) or
  // onto the "end of queue" slot.
  let dragging = $state<number | null>(null)
  let dropTarget = $state<number | null>(null) // 0 marks the end of the queue
  function dragStart(event: DragEvent, id: number) {
    dragging = id
    event.dataTransfer?.setData('text/plain', String(id))
    if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move'
  }
  function dragOver(event: DragEvent, id: number) {
    if (dragging === null || dragging === id) return
    event.preventDefault()
    dropTarget = id
  }
  async function drop(event: DragEvent, id: number) {
    event.preventDefault()
    const moved = dragging
    dragging = null; dropTarget = null
    if (moved === null || moved === id) return
    await run(() => api.ReorderTransfer(moved, id))
  }
  function dragEnd() { dragging = null; dropTarget = null }
  let queuedRows = $derived(rows.filter((tr) => tr.state === 'queued').length)

  async function bulk(fn: () => Promise<unknown>, refresh = false) {
    if (busy) return
    busy = true
    try { await run(fn, refresh) } finally { busy = false }
  }
</script>

<div class="transfers" transition:slide={{ duration: prefersReducedMotion.current ? 0 : 180 }}>
  <div class="thead">
    <b>{t('transfers')}</b>
    <span class="summary muted">
      {#if queue.running || queue.queued}
        {t('queueSummary', { running: queue.running, queued: queue.queued })}
        {#if queue.speed}· {fmtSize(queue.speed)}/s{/if}
        {#if remaining}· {fmtDuration(remaining)}{/if}
      {:else if queue.failed}
        {t('queueFailedSummary', { n: queue.failed })}
      {:else if transfers.length}
        {t('queueIdle')}
      {/if}
    </span>
    <span style="flex:1"></span>
    <label class="limit" title={t('transferLimitTitle')}>
      <i class="ri-stack-line"></i>
      <select value={String(queue.limit)} onchange={(e) => run(() => api.SetTransferLimit(Number((e.currentTarget as HTMLSelectElement).value)))}>
        {#each limits as n}<option value={String(n)}>{n}</option>{/each}
      </select>
    </label>
    <label class="limit" title={t('bandwidthTitle')}>
      <i class="ri-speed-line"></i>
      <select value={String(queue.bandwidthKBps)} onchange={(e) => run(() => api.SetBandwidthLimit(Number((e.currentTarget as HTMLSelectElement).value)))}>
        {#each bandwidths as kbps}<option value={String(kbps)}>{bandwidthLabel(kbps)}</option>{/each}
      </select>
    </label>
    <button class="ghost sm" title={queue.paused ? t('resumeQueue') : t('pauseQueue')} aria-pressed={queue.paused} onclick={() => run(() => api.PauseTransfers(!queue.paused))}>
      <i class={queue.paused ? 'ri-play-line' : 'ri-pause-line'}></i>
    </button>
    <button class="ghost sm" title={t('close')} onclick={onclose}><i class="ri-close-line"></i></button>
  </div>
  <div class="tbar">
    <div class="tabs" role="tablist">
      {#each filters as f}
        <button class="ghost sm" role="tab" aria-selected={filter === f.value} class:on={filter === f.value} onclick={() => (filter = f.value)}>{t(f.label)}</button>
      {/each}
    </div>
    <span style="flex:1"></span>
    {#if queue.paused}<span class="paused">{t('queuePaused')}</span>{/if}
    {#if queue.failed}
      <button class="ghost sm" disabled={busy} onclick={() => bulk(() => api.RetryFailedTransfers(), true)}><i class="ri-restart-line"></i> {t('retryFailed')}</button>
    {/if}
    {#if queue.queued}
      <button class="ghost sm" disabled={busy} onclick={() => bulk(() => api.CancelQueuedTransfers())}><i class="ri-close-circle-line"></i> {t('cancelQueued')}</button>
    {/if}
    {#if queue.done || queue.failed}
      <button class="ghost sm" disabled={busy} onclick={() => bulk(() => api.ClearTransfers())}>{t('clear')}</button>
    {/if}
  </div>
  <div class="tlist">
    {#each visibleRows as tr (tr.id)}
      <div class="trow {tr.state}" class:dragging={dragging === tr.id} class:target={dropTarget === tr.id}
        draggable={tr.state === 'queued'} role={tr.state === 'queued' ? 'listitem' : undefined}
        ondragstart={(e) => tr.state === 'queued' && dragStart(e, tr.id)} ondragover={(e) => tr.state === 'queued' && dragOver(e, tr.id)}
        ondrop={(e) => tr.state === 'queued' && drop(e, tr.id)} ondragend={dragEnd}>
        <i class="tk {tr.kind} {icons[tr.kind] ?? 'ri-download-2-line'}"></i>
        <span class="tname mono" title={tr.name}>{tr.name}</span>
        <div class="bar"><div class="fill {tr.state}" style="width:{tr.total ? Math.min(100, (tr.done / tr.total) * 100) : tr.state === 'done' ? 100 : 0}%"></div></div>
        <span class="tstate muted" title={tr.error}>{status(tr)}</span>
        <span class="tacts">
          {#if tr.state === 'running'}
            <button class="ghost sm" title={t('cancelTransfer')} disabled={pending.has(tr.id)} onclick={() => perItem(tr.id, () => api.CancelTransfer(tr.id))}><i class="ri-stop-circle-line"></i></button>
          {:else if tr.state === 'queued'}
            <button class="ghost sm" title={t('moveToFront')} disabled={pending.has(tr.id)} onclick={() => perItem(tr.id, () => api.PrioritizeTransfer(tr.id))}><i class="ri-skip-up-line"></i></button>
            <button class="ghost sm" title={t('cancelTransfer')} disabled={pending.has(tr.id)} onclick={() => perItem(tr.id, () => api.CancelTransfer(tr.id))}><i class="ri-close-line"></i></button>
          {:else}
            {#if isFailed(tr)}
              <button class="ghost sm" title={t('retryTransfer')} disabled={pending.has(tr.id)} onclick={() => perItem(tr.id, () => api.RetryTransfer(tr.id), true)}><i class="ri-restart-line"></i></button>
            {/if}
            <button class="ghost sm" title={t('removeTransfer')} disabled={pending.has(tr.id)} onclick={() => perItem(tr.id, () => api.RemoveTransfer(tr.id))}><i class="ri-delete-bin-line"></i></button>
          {/if}
        </span>
      </div>
    {:else}
      <div class="muted pad">{t('noTransfers')}</div>
    {/each}
    {#if rows.length > rowLimit}
      <div class="muted pad">{t('moreTransfers', { n: rows.length - rowLimit })}</div>
    {/if}
    {#if dragging !== null && queuedRows > 1}
      <div class="dropend" class:target={dropTarget === 0} role="listitem" ondragover={(e) => dragOver(e, 0)} ondrop={(e) => drop(e, 0)}>{t('dropToEnd')}</div>
    {/if}
  </div>
</div>

<style>
  .transfers { position: absolute; right: 12px; bottom: 40px; width: 620px; max-width: calc(100% - 24px); max-height: 55%; display: flex; flex-direction: column; background: var(--panel); border: 1px solid var(--border); border-radius: var(--radius-lg); box-shadow: var(--shadow); z-index: 20; overflow: hidden; }
  .thead { display: flex; align-items: center; gap: 4px; padding: 8px 8px 8px 14px; border-bottom: 1px solid var(--border); }
  .summary { font-size: 12px; margin-left: 8px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .limit { display: inline-flex; align-items: center; gap: 4px; font-size: 12px; color: var(--muted); }
  .limit select { font-size: 12px; padding: 2px 4px; }
  .tbar { display: flex; align-items: center; gap: 4px; padding: 4px 8px; border-bottom: 1px solid var(--border); font-size: 12px; }
  .tabs { display: flex; gap: 2px; }
  .tabs .on { background: var(--panel2); color: var(--text-strong); }
  .paused { color: var(--danger-text); margin-right: 6px; }
  .tlist { overflow: auto; padding: 4px 0; }
  .trow { display: grid; grid-template-columns: 18px minmax(0, 1fr) 110px 150px 64px; gap: 8px; align-items: center; padding: 5px 12px; font-size: 12px; }
  .trow.queued { opacity: .75; cursor: grab; }
  .trow.dragging { opacity: .4; }
  .trow.target { box-shadow: inset 0 2px 0 var(--accent); }
  .dropend { margin: 4px 12px; padding: 8px; border: 1px dashed var(--border); border-radius: var(--radius-md); text-align: center; font-size: 12px; color: var(--muted); }
  .dropend.target { border-color: var(--accent); color: var(--text); }
  .tk.upload { color: var(--accent); } .tk.download { color: var(--info); } .tk.copy, .tk.move { color: var(--success); } .tk.sync { color: var(--accent); }
  .tname { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .tstate { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; text-align: right; }
  .tacts { display: flex; justify-content: flex-end; gap: 2px; }
  .bar { height: 4px; background: var(--panel2); border-radius: 99px; overflow: hidden; }
  .fill { height: 100%; background: var(--accent); border-radius: 99px; transition: width .15s; }
  .fill.done { background: var(--success); } .fill.error { background: var(--danger); width: 100% !important; }
  .pad { padding: 14px; }
</style>
