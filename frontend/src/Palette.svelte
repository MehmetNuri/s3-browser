<script module lang="ts">
  export type Command = { id: string; label: string; hint: string; icon: string; run: () => void }
</script>

<script lang="ts">
  import { t } from './i18n.svelte'
  import Modal from './Modal.svelte'

  let { commands, onclose }: { commands: Command[]; onclose: () => void } = $props()

  let query = $state('')
  let index = $state(0)
  let list = $state<HTMLElement>()

  // Every word must appear somewhere in the label or its group.
  let matches = $derived.by(() => {
    const words = query.toLowerCase().split(/\s+/).filter(Boolean)
    return commands.filter((c) => words.every((w) => (c.label + ' ' + c.hint).toLowerCase().includes(w))).slice(0, 60)
  })

  function choose(command: Command | undefined) {
    if (!command) return
    onclose()
    command.run()
  }

  function onkeydown(event: KeyboardEvent) {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault()
      if (!matches.length) return
      index = (index + (event.key === 'ArrowDown' ? 1 : -1) + matches.length) % matches.length
      queueMicrotask(() => list?.querySelector('[aria-selected="true"]')?.scrollIntoView({ block: 'nearest' }))
    } else if (event.key === 'Enter') {
      event.preventDefault()
      choose(matches[index])
    }
  }
</script>

<Modal label={t('commandPalette')} {onclose}>
  <div class="palette">
    <div class="search">
      <i class="ri-search-line"></i>
      <!-- svelte-ignore a11y_autofocus -->
      <input autofocus bind:value={query} oninput={() => (index = 0)} {onkeydown} placeholder={t('paletteHint')} spellcheck="false"
        role="combobox" aria-expanded="true" aria-controls="palette-list" aria-activedescendant={matches[index] ? 'palette-' + index : undefined} />
    </div>
    <div class="list" id="palette-list" role="listbox" bind:this={list}>
      {#each matches as command, i (command.id)}
        <button class="ghost" id={'palette-' + i} role="option" aria-selected={i === index} tabindex="-1"
          onclick={() => choose(command)} onmousemove={() => (index = i)}>
          <i class={command.icon}></i><span class="label">{command.label}</span><span class="muted hint">{command.hint}</span>
        </button>
      {:else}
        <p class="muted">{t('noMatches')}</p>
      {/each}
    </div>
  </div>
</Modal>

<style>
  .palette { width: min(560px, 94vw); background: var(--panel); border: 1px solid var(--border); border-radius: var(--radius-lg); box-shadow: var(--shadow); overflow: hidden; }
  .search { display: flex; align-items: center; gap: 10px; padding: 4px 14px; border-bottom: 1px solid var(--border); color: var(--muted); }
  .search input { border: none; background: transparent; box-shadow: none; padding: 10px 0; font-size: 14px; }
  .list { max-height: min(380px, 60vh); overflow: auto; padding: 6px; display: flex; flex-direction: column; }
  .list button { justify-content: flex-start; gap: 10px; padding: 6px 10px; font-weight: 500; }
  .list button[aria-selected='true'] { background: var(--panel2); }
  .list i { color: var(--muted); }
  .label { overflow: hidden; text-overflow: ellipsis; }
  .hint { margin-left: auto; padding-left: 12px; font-size: 11px; font-weight: 400; }
  p { margin: 0; padding: 14px; text-align: center; }
</style>
