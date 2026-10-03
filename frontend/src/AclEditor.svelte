<script lang="ts">
  import { onMount } from 'svelte'
  import type { Acl, AclGrant } from './api'
  import { errorText } from './runtime'
  import { t } from './i18n.svelte'

  // Grant list of a bucket or object with the well-known groups as presets.
  let { load, save, onsaved }: {
    load: () => Promise<Acl>; save: (acl: Acl) => Promise<unknown>; onsaved: () => void
  } = $props()

  const groups = [
    { uri: 'http://acs.amazonaws.com/groups/global/AllUsers', label: 'aclEveryone' },
    { uri: 'http://acs.amazonaws.com/groups/global/AuthenticatedUsers', label: 'aclAuthenticated' },
    { uri: 'http://acs.amazonaws.com/groups/s3/LogDelivery', label: 'aclLogDelivery' },
  ] as const
  const permissions = ['READ', 'WRITE', 'READ_ACP', 'WRITE_ACP', 'FULL_CONTROL']
  let acl = $state<Acl | null>(null)
  let error = $state('')
  let busy = $state(false)
  let granteeKind = $state<string>(groups[0].uri)
  let granteeValue = $state('')
  let permission = $state('READ')

  async function reload() {
    error = ''
    try { acl = await load() } catch (e) { error = errorText(e) }
  }
  onMount(reload)

  function describe(grant: AclGrant) {
    const group = groups.find((g) => g.uri === grant.grantee)
    if (group) return t(group.label)
    if (grant.type === 'AmazonCustomerByEmail') return grant.grantee
    return grant.name ? `${grant.name} (${grant.grantee.slice(0, 12)}…)` : grant.grantee
  }
  function add() {
    if (!acl) return
    const grant: AclGrant = granteeKind === 'user'
      ? { type: 'CanonicalUser', grantee: granteeValue.trim(), name: '', permission }
      : granteeKind === 'email'
        ? { type: 'AmazonCustomerByEmail', grantee: granteeValue.trim(), name: '', permission }
        : { type: 'Group', grantee: granteeKind, name: '', permission }
    if (!grant.grantee) return
    if (acl.grants.some((g) => g.grantee === grant.grantee && g.permission === grant.permission)) return
    acl.grants = [...acl.grants, grant]
    granteeValue = ''
  }
  function remove(index: number) { if (acl) acl.grants = acl.grants.filter((_, i) => i !== index) }
  async function apply() {
    if (!acl) return
    busy = true; error = ''
    try { await save($state.snapshot(acl)); onsaved(); await reload() } catch (e) { error = errorText(e) }
    busy = false
  }
</script>

<div class="acl">
  {#if acl}
    <p class="hint">{t('aclOwner', { owner: acl.ownerName || acl.ownerId.slice(0, 16) })}</p>
    {#each acl.grants as grant, index}
      <div class="grant">
        <span class="who" title={grant.grantee}>{describe(grant)}</span>
        <span class="perm mono">{grant.permission}</span>
        <button type="button" class="ghost sm" title={t('remove')} onclick={() => remove(index)}><i class="ri-close-line"></i></button>
      </div>
    {:else}
      <p class="muted">{t('aclNoGrants')}</p>
    {/each}
    <div class="add">
      <select bind:value={granteeKind} aria-label={t('aclGrantee')}>
        {#each groups as group}<option value={group.uri}>{t(group.label)}</option>{/each}
        <option value="user">{t('aclCanonicalUser')}</option>
        <option value="email">{t('aclEmail')}</option>
      </select>
      {#if granteeKind === 'user' || granteeKind === 'email'}
        <input bind:value={granteeValue} placeholder={granteeKind === 'user' ? t('aclCanonicalId') : 'user@example.com'} spellcheck="false" />
      {/if}
      <select bind:value={permission} aria-label={t('aclPermission')}>
        {#each permissions as p}<option value={p}>{p}</option>{/each}
      </select>
      <button type="button" class="sm" onclick={add}><i class="ri-add-line"></i> {t('addEntry')}</button>
    </div>
    <div class="end"><button type="button" class="primary sm" disabled={busy} onclick={apply}>{t('saveAcl')}</button></div>
  {:else if !error}
    <p class="muted">{t('loading')}</p>
  {/if}
  {#if error}<div class="msg" role="alert">{error}</div>{/if}
</div>

<style>
  .acl { display: flex; flex-direction: column; gap: 6px; }
  .grant { display: grid; grid-template-columns: minmax(0, 1fr) 110px 28px; gap: 8px; align-items: center; font-size: 13px; }
  .who { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .add { display: flex; gap: 6px; align-items: center; flex-wrap: wrap; } .add input { flex: 1; min-width: 160px; }
  .end { display: flex; justify-content: flex-end; }
  .hint, p { margin: 0; color: var(--muted); font-size: 12px; line-height: 1.5; }
  .msg { padding: 8px 10px; border-radius: var(--radius-md); background: var(--danger-soft); color: var(--danger-text); user-select: text; word-break: break-word; }
</style>
