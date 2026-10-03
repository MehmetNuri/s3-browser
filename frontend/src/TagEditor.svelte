<script lang="ts">
  import { t } from './i18n.svelte'

  // Key/value rows for tags and metadata; the bound object only gets
  // non-empty keys, so a half-typed row does not reach the backend.
  let { values = $bindable(), keyLabel, valueLabel }: { values: Record<string, string>; keyLabel: string; valueLabel: string } = $props()

  type Row = { id: number; key: string; value: string }
  let seq = 0
  // svelte-ignore state_referenced_locally
  let rows = $state<Row[]>(Object.entries(values).map(([key, value]) => ({ id: seq++, key, value })))

  function publish() {
    const next: Record<string, string> = {}
    for (const row of rows) if (row.key.trim()) next[row.key.trim()] = row.value
    values = next
  }
  function add() { rows = [...rows, { id: seq++, key: '', value: '' }] }
  function remove(id: number) { rows = rows.filter((row) => row.id !== id); publish() }
</script>

<div class="editor">
  {#each rows as row (row.id)}
    <div class="row">
      <input bind:value={row.key} oninput={publish} placeholder={keyLabel} spellcheck="false" aria-label={keyLabel} />
      <input bind:value={row.value} oninput={publish} placeholder={valueLabel} spellcheck="false" aria-label={valueLabel} />
      <button type="button" class="ghost sm" title={t('remove')} onclick={() => remove(row.id)}><i class="ri-close-line"></i></button>
    </div>
  {:else}
    <p class="muted">{t('noEntries')}</p>
  {/each}
  <div><button type="button" class="sm" onclick={add}><i class="ri-add-line"></i> {t('addEntry')}</button></div>
</div>

<style>
  .editor { display: flex; flex-direction: column; gap: 6px; }
  .row { display: grid; grid-template-columns: 1fr 1.4fr 28px; gap: 6px; align-items: center; }
  p { margin: 0; font-size: 13px; }
</style>
