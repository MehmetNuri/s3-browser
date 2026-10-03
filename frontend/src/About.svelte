<script lang="ts">
  import { errorText } from './runtime'
  import { GetAppInfo, OpenConfigDir } from './api'
  import type { main } from './models'
  import { t } from './i18n.svelte'
  import logo from './assets/images/logo.png'
  import { onMount } from 'svelte'
  import Modal from './Modal.svelte'

  let { onclose }: { onclose: () => void } = $props()
  let info = $state<main.AppInfo | null>(null)
  let error = $state('')
  onMount(() => { GetAppInfo().then((i) => (info = i)).catch((e) => (error = errorText(e))) })
</script>

<Modal label={t('about')} {onclose}>
  <div class="about">
    <button class="ghost close" title={t('close')} onclick={onclose}><i class="ri-close-line"></i></button>
    <img src={logo} alt="" width="88" height="88" />
    <h2>{info?.name ?? 'S3 Browser'}</h2>
    <p class="ver">{t('version')} {info?.version ?? ''}</p>
    {#if error}<p role="alert">{error}</p>{/if}
    <p class="desc">{t('aboutDesc')}</p>

    <dl class="selectable">
      <dt>{t('builtWith')}</dt><dd>Go {info?.goVersion} · Electron {info?.electronVersion} · Svelte</dd>
      <dt>{t('platform')}</dt><dd>{info?.platform}</dd>
      <dt>{t('configDir')}</dt><dd class="mono">{info?.configDir}</dd>
      <dt>{t('credentialStorage')}</dt><dd>{info ? t(info.secretStorage === 'keychain' ? 'storageKeychain' : 'storagePlain') : ''}</dd>
    </dl>

    <div class="acts">
      <button onclick={() => OpenConfigDir().catch((e) => (error = errorText(e)))}><i class="ri-folder-settings-line"></i> {t('openConfigDir')}</button>
    </div>
    <p class="copy muted">{info?.copyright}</p>
    <p class="credits muted">{t('credits')}</p>
  </div>
</Modal>

<style>
  .about { position: relative; width: min(440px, 92vw); background: var(--panel); border: 1px solid var(--border); border-radius: var(--radius-lg); box-shadow: var(--shadow); padding: 28px 28px 20px; text-align: center; }
  .close { position: absolute; top: 10px; inset-inline-end: 10px; }
  img { display: block; margin: 0 auto 10px; }
  h2 { margin: 0; font-size: 20px; font-weight: 700; color: var(--text-strong); }
  .ver { margin: 4px 0 12px; color: var(--accent); font-weight: 600; }
  .desc { margin: 0 0 16px; color: var(--muted); line-height: 1.5; }
  dl { display: grid; grid-template-columns: auto 1fr; gap: 6px 12px; text-align: start; margin: 0 0 16px; padding: 12px 14px; background: var(--bg); border: 1px solid var(--border); border-radius: var(--radius-md); font-size: 12px; }
  dt { color: var(--muted); }
  dd { margin: 0; word-break: break-all; }
  .acts { display: flex; justify-content: center; gap: 8px; margin-bottom: 14px; }
  .copy, .credits { margin: 2px 0; font-size: 11px; }
</style>
