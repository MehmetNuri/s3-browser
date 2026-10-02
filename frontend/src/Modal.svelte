<script lang="ts">
  import { onMount, type Snippet } from 'svelte'
  import { fade } from 'svelte/transition'
  import { prefersReducedMotion } from 'svelte/motion'

  let { label, onclose, children }: { label: string; onclose: () => void; children: Snippet } = $props()
  let element: HTMLDialogElement

  // Removing a dialog from the document does not restore focus by itself.
  onMount(() => {
    const previous = document.activeElement
    element.showModal()
    return () => { if (previous instanceof HTMLElement && previous.isConnected) previous.focus() }
  })

  function dismiss(event: MouseEvent) {
    if (event.target !== element) return
    const bounds = element.getBoundingClientRect()
    if (event.clientX < bounds.left || event.clientX > bounds.right || event.clientY < bounds.top || event.clientY > bounds.bottom) onclose()
  }
</script>

<dialog bind:this={element} aria-label={label} onclick={dismiss}
  oncancel={(event) => { event.preventDefault(); onclose() }}
  in:fade={{ duration: prefersReducedMotion.current ? 0 : 140 }}>
  {@render children()}
</dialog>

<style>
  dialog { padding: 0; border: 0; max-width: 96vw; max-height: 94vh; color: var(--text); background: transparent; overflow: visible; }
  /* No backdrop blur: with software rendering it tripled the frame time of every dialog. */
  dialog::backdrop { background: var(--mask); }
</style>
