<script lang="ts">
  import { onMount } from 'svelte'
  import { fade, slide } from 'svelte/transition'
  import { flip } from 'svelte/animate'
  import { prefersReducedMotion } from 'svelte/motion'
  import * as api from './api'
  import { EventsOn, OnFileDrop, OnFileDropOff, errorText } from './runtime'
  import type { main } from './models'
  import ProfileModal from './ProfileModal.svelte'
  import Capabilities from './Capabilities.svelte'
  import About from './About.svelte'
  import Modal from './Modal.svelte'
  import HeadersModal from './HeadersModal.svelte'
  import BucketSettings from './BucketSettings.svelte'
  import ObjectSettings from './ObjectSettings.svelte'
  import CopyModal from './CopyModal.svelte'
  import Analyzer from './Analyzer.svelte'
  import Backups from './Backups.svelte'
  import Transfers from './Transfers.svelte'
  import Palette, { type Command } from './Palette.svelte'
  import logo from './assets/images/logo.png'
  import { t, i18n, languages, setLang, syncBackend } from './i18n.svelte'

  type Transfer = api.Transfer

  let profiles = $state<main.Profile[]>([])
  let active = $state<main.Profile | null>(null)
  let buckets = $state<main.Bucket[]>([])
  let listWarning = $state('')
  let bucket = $state('')
  let prefix = $state('')
  let items = $state<main.S3Object[]>([])
  let nextToken = $state('')
  let selected = $state<Set<string>>(new Set())
  let filter = $state('')
  let loading = $state(false)
  let tab = $state<'browser' | 'caps' | 'analyze' | 'backups'>('browser')
  let copying = $state<string[] | null>(null)
  let palette = $state<Command[] | null>(null)
  let editingText = $state<string | null>(null)
  let savingText = $state(false)

  // Favorite folders per connection; a convenience kept in the renderer's storage.
  type Favorite = { bucket: string; prefix: string }
  function savedFavorites(): Record<string, Favorite[]> {
    try {
      const saved = JSON.parse(localStorage.getItem('favorites') ?? '{}')
      return saved && typeof saved === 'object' && !Array.isArray(saved) ? saved : {}
    } catch { return {} }
  }
  let favorites = $state<Record<string, Favorite[]>>(savedFavorites())
  let myFavorites = $derived(active ? favorites[active.id] ?? [] : [])
  let isFavorite = $derived(myFavorites.some((f) => f.bucket === bucket && f.prefix === prefix))
  function storeFavorites() { try { localStorage.setItem('favorites', JSON.stringify(favorites)) } catch {} }
  function toggleFavorite() {
    if (!active || !bucket) return
    favorites[active.id] = isFavorite
      ? myFavorites.filter((f) => f.bucket !== bucket || f.prefix !== prefix)
      : [...myFavorites, { bucket, prefix }]
    storeFavorites()
  }
  function openFavorite(f: Favorite) { tab = 'browser'; bucket = f.bucket; navigate(f.prefix) }
  let toast = $state<{ text: string; err: boolean } | null>(null)
  let editing = $state<main.Profile | null | undefined>(undefined) // undefined = closed
  let info = $state<main.ObjectInfo | null>(null)
  let preview = $state('')
  let transfers = $state<Transfer[]>([])
  let queue = $state<api.TransferQueue>({ limit: 4, paused: false, queued: 0, running: 0, failed: 0, done: 0, speed: 0, bandwidthKBps: 0 })
  let showTransfers = $state(false)
  let sortBy = $state<'name' | 'size' | 'modified'>('name')
  let sortAsc = $state(true)
  let previewImage = $state('')
  let search = $state<(api.SearchResult & { query: string }) | null>(null)
  let versions = $state<api.ObjectVersion[] | null>(null)
  let versioning = $state<string | null>(null) // null: unknown or not supported
  let editingHeaders = $state(false)
  let bucketSettings = $state(false)
  let uploadOpen = $state(false)
  let mounts = $state<api.Mount[]>([])
  let mountOpen = $state(false)
  let currentMount = $derived(mounts.find((m) => m.profileId === active?.id && m.bucket === bucket && m.prefix === (prefix || '')) ?? null)
  let objectSettings = $state(false)
  let dragging = $state(false)
  let dialog = $state<{ title: string; text?: string; input?: boolean; options?: { value: string; label: string }[]; optionsLabel?: string; notes?: Record<string, string>; dangerValue?: string; cancel?: boolean; value?: string; danger?: boolean; ok: string; resolve: (v: string | null) => void } | null>(null)

  // The interface follows the OS theme, including changes while running. The
  // toggle stores an override only while it differs from the OS, so switching
  // back resumes following the system instead of pinning a theme forever.
  type Theme = 'dark' | 'light'
  const systemTheme = window.matchMedia('(prefers-color-scheme: dark)')
  function savedOverride(): Theme | null {
    try {
      localStorage.removeItem('theme') // legacy key that pinned a theme permanently
      const saved = localStorage.getItem('themeOverride')
      return saved === 'dark' || saved === 'light' ? saved : null
    } catch { return null }
  }
  let systemDark = $state(systemTheme.matches)
  let themeOverride = $state<Theme | null>(savedOverride())
  let dark = $derived(themeOverride ? themeOverride === 'dark' : systemDark)
  $effect(() => { document.documentElement.dataset.theme = dark ? 'dark' : 'light' })
  function setOverride(theme: Theme | null) {
    themeOverride = theme
    try {
      if (theme) localStorage.setItem('themeOverride', theme)
      else localStorage.removeItem('themeOverride')
    } catch {}
  }
  function toggleTheme() {
    const next: Theme = dark ? 'light' : 'dark'
    setOverride((next === 'dark') === systemDark ? null : next)
  }
  function systemThemeChanged(event: MediaQueryListEvent) {
    systemDark = event.matches
    if (themeOverride && (themeOverride === 'dark') === systemDark) setOverride(null)
  }

  function notify(text: string, err = false) {
    toast = { text, err }
    const t = toast
    setTimeout(() => { if (toast === t) toast = null }, err ? 6000 : 2500)
  }
  async function guard<T>(fn: () => Promise<T>): Promise<T | undefined> {
    try { return await fn() } catch (e) { notify(errorText(e), true) }
  }
  // Successful void calls return null, so use a separate success flag.
  async function act(fn: () => Promise<unknown>): Promise<boolean> {
    try { await fn(); return true } catch (e) { notify(errorText(e), true); return false }
  }
  function ask(opts: Omit<NonNullable<typeof dialog>, 'resolve'>): Promise<string | null> {
    return new Promise((resolve) => (dialog = { ...opts, resolve }))
  }
  function closeDialog(v: string | null) { dialog?.resolve(v); dialog = null }

  function fmtSize(n: number) {
    if (!n) return '0 B'
    const u = ['B', 'KB', 'MB', 'GB', 'TB']
    const i = Math.min(Math.floor(Math.log(n) / Math.log(1024)), u.length - 1)
    return (n / 1024 ** i).toFixed(i ? 1 : 0) + ' ' + u[i]
  }

  // Remix Icon names by extension; the chip colour comes from the icon family.
  const extIcons: Record<string, string> = {
    png: 'image', jpg: 'image', jpeg: 'image', gif: 'image', webp: 'image', svg: 'image', bmp: 'image', ico: 'image', tif: 'image', tiff: 'image', heic: 'image', avif: 'image', psd: 'image', raw: 'image',
    mp4: 'video', mov: 'video', webm: 'video', mkv: 'video', avi: 'video', m4v: 'video', wmv: 'video', flv: 'video', ts: 'file-code',
    mp3: 'music-2', wav: 'music-2', ogg: 'music-2', flac: 'music-2', aac: 'music-2', m4a: 'music-2', wma: 'music-2', opus: 'music-2',
    zip: 'file-zip', gz: 'file-zip', tgz: 'file-zip', tar: 'file-zip', '7z': 'file-zip', rar: 'file-zip', bz2: 'file-zip', xz: 'file-zip', zst: 'file-zip', jar: 'file-zip', war: 'file-zip', apk: 'file-zip',
    pdf: 'file-pdf-2',
    json: 'braces', yaml: 'braces', yml: 'braces', toml: 'braces', xml: 'braces', ini: 'braces', conf: 'braces', cfg: 'braces', env: 'braces', properties: 'braces', pom: 'braces', gradle: 'braces', lock: 'braces',
    js: 'file-code', mjs: 'file-code', cjs: 'file-code', jsx: 'file-code', tsx: 'file-code', html: 'file-code', htm: 'file-code', css: 'file-code', scss: 'file-code', less: 'file-code', vue: 'file-code', svelte: 'file-code',
    go: 'file-code', rs: 'file-code', py: 'file-code', rb: 'file-code', php: 'file-code', java: 'file-code', kt: 'file-code', kts: 'file-code', scala: 'file-code', c: 'file-code', h: 'file-code', cpp: 'file-code', hpp: 'file-code', cs: 'file-code', swift: 'file-code', m: 'file-code', dart: 'file-code', lua: 'file-code', pl: 'file-code', r: 'file-code', sql: 'file-code', graphql: 'file-code', proto: 'file-code',
    sh: 'terminal-box', bash: 'terminal-box', zsh: 'terminal-box', fish: 'terminal-box', ps1: 'terminal-box', bat: 'terminal-box', cmd: 'terminal-box', makefile: 'terminal-box',
    exe: 'install', msi: 'install', dmg: 'install', pkg: 'install', deb: 'install', rpm: 'install', appimage: 'install', snap: 'install', flatpak: 'install', bin: 'cpu', so: 'cpu', dll: 'cpu', dylib: 'cpu', wasm: 'cpu', iso: 'disc', img: 'disc', vmdk: 'disc', qcow2: 'disc',
    txt: 'file-text', log: 'file-text', md: 'markdown', markdown: 'markdown', rst: 'file-text', rtf: 'file-text', tex: 'file-text', nfo: 'file-text',
    csv: 'file-excel-2', tsv: 'file-excel-2', xlsx: 'file-excel-2', xls: 'file-excel-2', ods: 'file-excel-2', numbers: 'file-excel-2',
    docx: 'file-word-2', doc: 'file-word-2', odt: 'file-word-2', pages: 'file-word-2', pptx: 'file-ppt-2', ppt: 'file-ppt-2', odp: 'file-ppt-2', key: 'file-ppt-2',
    md5: 'fingerprint', sha1: 'fingerprint', sha256: 'fingerprint', sha512: 'fingerprint', sig: 'fingerprint', asc: 'fingerprint', gpg: 'fingerprint', pem: 'key-2', crt: 'key-2', cer: 'key-2', p12: 'key-2', pfx: 'key-2', pub: 'key-2',
    ttf: 'font-family', otf: 'font-family', woff: 'font-family', woff2: 'font-family', eot: 'font-family',
    sqlite: 'database-2', db: 'database-2', parquet: 'database-2', avro: 'database-2', orc: 'database-2', bak: 'database-2', dump: 'database-2',
    epub: 'book-2', mobi: 'book-2', azw3: 'book-2', ics: 'calendar-line', vcf: 'contacts-book-2', torrent: 'download-cloud-2',
  }
  // Files named by convention rather than by extension.
  const nameIcons: Record<string, string> = {
    dockerfile: 'ship', containerfile: 'ship', makefile: 'terminal-box', license: 'file-text', readme: 'markdown', changelog: 'markdown', '.gitignore': 'git-branch', '.dockerignore': 'ship', '.env': 'braces', '.editorconfig': 'braces',
  }
  // Colour family of the icon chip in the file list.
  const kinds: Record<string, string> = {
    image: 'image', video: 'video', 'music-2': 'audio', 'file-zip': 'archive', 'file-pdf-2': 'pdf', braces: 'code', 'file-code': 'code', 'terminal-box': 'code', ship: 'code', 'git-branch': 'code',
    'file-text': 'doc', markdown: 'doc', 'file-excel-2': 'sheet', 'file-word-2': 'doc', 'file-ppt-2': 'pdf', fingerprint: 'other', 'key-2': 'archive', install: 'archive', cpu: 'other', disc: 'other',
    'font-family': 'doc', 'database-2': 'sheet', 'book-2': 'doc', 'calendar-line': 'doc', 'contacts-book-2': 'doc', 'download-cloud-2': 'archive',
  }
  function iconName(name: string) {
    const lower = name.toLowerCase()
    // "Dockerfile.jvm" and "README.tr" are still a Dockerfile and a readme.
    const base = lower.split('.')[0] || lower
    if (nameIcons[lower]) return nameIcons[lower]
    if (nameIcons[base] && (lower === base || lower.startsWith(base + '.'))) return nameIcons[base]
    const ext = lower.includes('.') ? lower.slice(lower.lastIndexOf('.') + 1) : ''
    return extIcons[ext] ?? 'file'
  }
  function fileKind(name: string) {
    return kinds[iconName(name)] ?? 'other'
  }
  // "3 hours ago" for recent changes, a short date otherwise; the exact time stays in the tooltip.
  function relativeTime(stamp: string) {
    if (!stamp) return ''
    const date = new Date(stamp.replace(' ', 'T'))
    const seconds = (date.getTime() - Date.now()) / 1000
    if (Number.isNaN(seconds)) return stamp
    const format = new Intl.RelativeTimeFormat(i18n.lang, { numeric: 'auto' })
    const age = Math.abs(seconds)
    if (age < 60) return format.format(0, 'minute')
    if (age < 3600) return format.format(Math.round(seconds / 60), 'minute')
    if (age < 86400) return format.format(Math.round(seconds / 3600), 'hour')
    if (age < 7 * 86400) return format.format(Math.round(seconds / 86400), 'day')
    return date.toLocaleDateString(i18n.lang, { year: 'numeric', month: 'short', day: 'numeric' })
  }

  // Most Remix icons come in a -line variant; a few exist only in one weight.
  const singleWeightIcons = new Set(['font-family'])
  function fileIcon(name: string) {
    const icon = iconName(name)
    return icon.endsWith('-line') || singleWeightIcons.has(icon) ? `ri-${icon}` : `ri-${icon}-line`
  }

  async function loadProfiles() { profiles = (await guard(api.ListProfiles)) ?? [] }

  async function exportProfiles() {
    if (await guard(api.ExportProfiles)) notify(t('profilesExported'))
  }

  async function importProfiles() {
    const count = await guard(api.ImportProfiles)
    if (count) {
      await loadProfiles()
      notify(t('profilesImported', { n: count }))
    }
  }

  // The backend handles calls concurrently, so connections are sent one at a
  // time; otherwise an earlier profile could end up as the active client.
  let connectQueue: Promise<unknown> = Promise.resolve()
  let connectSeq = 0
  async function connect(p: main.Profile) {
    const seq = ++connectSeq
    active = p; buckets = []; bucket = ''; items = []; prefix = ''; listWarning = ''; info = null
    loading = true
    const attempt = connectQueue.then(() => act(() => api.Connect(p.id)))
    connectQueue = attempt
    const connected = await attempt
    if (seq !== connectSeq) return // a newer connection replaced this one
    if (connected) {
      await loadBuckets()
      if (seq !== connectSeq) return
      if (buckets.length) openBucket(buckets[0].name)
    }
    loading = false
  }

  // Keep pinned buckets available when account-wide listing is denied.
  async function loadBuckets() {
    try {
      const res = await api.ListBuckets()
      buckets = res.buckets ?? []
      listWarning = res.warning
    } catch (e) {
      buckets = []
      listWarning = errorText(e)
    }
  }

  async function linkBucket() {
    const name = await ask({ title: t('linkBucket'), text: t('linkBucketText'), input: true, value: '', ok: t('link_') })
    if (!name?.trim()) return
    if (await act(() => api.AddProfileBucket(name.trim()))) {
      await loadBuckets()
      openBucket(name.trim())
      notify(t('bucketLinked'))
    }
  }

  async function unlinkBucket(name: string) {
    if (await ask({ title: t('unlinkTitle'), text: t('unlinkText', { name }), ok: t('remove') }) === null) return
    if (await act(() => api.RemoveProfileBucket(name))) {
      await loadBuckets()
      if (bucket === name) { bucket = ''; items = [] }
    }
  }

  async function removeProfile(p: main.Profile) {
    if (await ask({ title: t('deleteProfileTitle'), text: t('deleteProfileText', { name: p.name }), danger: true, ok: t('delete') }) === null) return
    if (!(await act(() => api.DeleteProfile(p.id)))) return
    if (active?.id === p.id) { active = null; buckets = []; bucket = ''; items = []; info = null }
    delete favorites[p.id]; storeFavorites()
    await loadProfiles()
  }

  async function newBucket() {
    const name = await ask({ title: t('newBucket'), input: true, value: '', ok: t('create') })
    if (!name?.trim()) return
    if (await act(() => api.CreateBucket(name.trim()))) {
      await loadBuckets()
      notify(t('bucketCreated'))
    }
  }

  async function removeBucket(name: string) {
    const r = await ask({ title: t('deleteBucketTitle'), text: t('deleteBucketText', { name }), input: true, value: '', danger: true, ok: t('delete') })
    if (r?.trim() !== name) { if (r !== null) notify(t('nameMismatch'), true); return }
    if (await act(() => api.DeleteBucket(name, true))) {
      buckets = buckets.filter((b) => b.name !== name)
      if (bucket === name) { bucket = ''; items = [] }
      notify(t('bucketDeleted'))
    }
  }

  function openBucket(name: string) { bucket = name; navigate('') }

  async function navigate(p: string) {
    prefix = p; selected = new Set(); filter = ''; info = null; search = null
    await refresh()
  }

  async function runSearch() {
    const b = bucket, p = prefix, query = filter.trim()
    if (!b || !query) return
    loading = true
    const res = await guard(() => api.SearchObjects(b, p, query))
    if (b === bucket && p === prefix) {
      if (res) { search = { ...res, items: res.items ?? [], query }; selected = new Set(); filter = '' }
      loading = false
    }
  }

  async function refresh(silent = false) {
    if (!bucket) return
    if (search) { // results are a snapshot; only an explicit refresh repeats the search
      if (!silent) { filter = search.query; await runSearch() }
      return
    }
    const b = bucket, p = prefix
    if (!silent) loading = true
    try {
      const res = await api.ListObjects(b, p, '')
      if (b !== bucket || p !== prefix) return // navigated away meanwhile
      items = res.items ?? []
      nextToken = res.nextToken ?? ''
      const keys = new Set(items.map((i) => i.key))
      if ([...selected].some((k) => !keys.has(k))) selected = new Set([...selected].filter((k) => keys.has(k)))
      if (info && !keys.has(info.key)) info = null // object was removed elsewhere
    } catch (e) {
      if (!silent) { notify(errorText(e), true); items = []; nextToken = '' }
    } finally {
      if (!silent) loading = false
    }
  }

  async function loadMore() {
    const b = bucket, p = prefix
    loading = true
    const res = await guard(() => api.ListObjects(b, p, nextToken))
    if (b !== bucket || p !== prefix) return // navigated away meanwhile
    if (res) { items = [...items, ...(res.items ?? [])]; nextToken = res.nextToken ?? '' }
    loading = false
  }

  let crumbs = $derived.by(() => {
    const parts = prefix.split('/').filter(Boolean)
    return parts.map((name, i) => ({ name, path: parts.slice(0, i + 1).join('/') + '/' }))
  })

  let view = $derived.by(() => {
    const f = filter.toLowerCase()
    const source = search?.items ?? items
    const list = f ? source.filter((i) => i.name.toLowerCase().includes(f)) : [...source]
    const dir = sortAsc ? 1 : -1
    return list.sort((a, b) => {
      if (a.isFolder !== b.isFolder) return a.isFolder ? -1 : 1
      const va = a[sortBy], vb = b[sortBy]
      return (va < vb ? -1 : va > vb ? 1 : 0) * dir
    })
  })
  let shown = $derived(search?.items ?? items)
  let totalSize = $derived(shown.reduce((s, i) => s + (i.size || 0), 0))
  let allChecked = $derived(view.length > 0 && view.every((i) => selected.has(i.key)))

  function toggle(key: string) {
    const s = new Set(selected)
    s.has(key) ? s.delete(key) : s.add(key)
    selected = s
  }
  function toggleAll() { selected = allChecked ? new Set() : new Set(view.map((i) => i.key)) }
  function setSort(k: typeof sortBy) { if (sortBy === k) sortAsc = !sortAsc; else { sortBy = k; sortAsc = true } }

  async function openItem(i: main.S3Object) {
    if (i.isFolder) return navigate(i.key)
    showInfo(i.key)
  }

  const imageExtensions = /\.(png|jpe?g|gif|webp|bmp)$/i
  let infoSeq = 0
  async function showInfo(key: string) {
    const seq = ++infoSeq
    const b = bucket
    preview = ''; previewImage = ''; versions = null; editingText = null
    const r = await guard(() => api.HeadObject(b, key))
    if (!r || seq !== infoSeq) return // another object was opened meanwhile
    info = r
    if (r.size <= 4 * 1024 * 1024 && (imageExtensions.test(key) || /^image\/(png|jpeg|gif|webp|bmp)/.test(r.contentType || ''))) {
      // A failed preview is not worth an error toast; the details stay usable.
      const image = await api.PreviewImage(b, key).catch(() => '')
      if (seq === infoSeq) previewImage = image
    } else if (r.size < 2_000_000 && /^(text\/|application\/(json|xml|javascript|x-yaml|yaml|toml))/.test(r.contentType || '') ) {
      const text = (await guard(() => api.PreviewText(b, key))) ?? ''
      if (seq === infoSeq) preview = text
    }
  }

  async function saveText() {
    if (!info || editingText === null) return
    const key = info.key, etag = info.etag, text = editingText
    savingText = true
    const saved = await act(() => api.SaveText(bucket, key, text, etag))
    savingText = false
    if (!saved) return
    notify(t('textSaved'))
    refresh(true)
    showInfo(key)
  }

  function openPalette() {
    const action = (id: string, label: string, icon: string, run: () => void): Command => ({ id: 'a:' + id, label, icon, hint: t('hintAction'), run })
    const browsing = !!bucket && tab === 'browser'
    palette = [
      ...(browsing ? [
        action('upload', t('uploadFile'), 'ri-upload-2-line', () => upload(false)),
        action('uploadFolder', t('uploadFolder'), 'ri-folder-upload-line', () => upload(true)),
        action('sync', t('syncFolder'), 'ri-loop-right-line', sync),
        action('bucketSettings', t('bucketSettings'), 'ri-settings-3-line', () => (bucketSettings = true)),
        action('mount', t('mountBucket'), 'ri-hard-drive-2-line', mountCurrent),
        action('folder', t('newFolder'), 'ri-folder-add-line', newFolder),
        action('refresh', t('refresh'), 'ri-refresh-line', () => refresh()),
        action('favorite', isFavorite ? t('removeFavorite') : t('addFavorite'), 'ri-star-line', toggleFavorite),
      ] : []),
      ...(bucket ? [action('analyze', t('analyzer'), 'ri-pie-chart-2-line', () => (tab = 'analyze'))] : []),
      ...(active ? [action('backups', t('backups'), 'ri-history-line', () => (tab = 'backups'))] : []),
      ...(active ? [action('caps', t('capabilities'), 'ri-shield-check-line', () => (tab = 'caps'))] : []),
      action('connection', t('newConnection'), 'ri-add-line', () => (editing = null)),
      action('transfers', t('transfers'), 'ri-arrow-up-down-line', () => (showTransfers = !showTransfers)),
      action('theme', dark ? t('toLight') : t('toDark'), dark ? 'ri-sun-line' : 'ri-moon-line', toggleTheme),
      action('about', t('about'), 'ri-information-line', () => (showAbout = true)),
      ...myFavorites.map((f): Command => ({ id: 'f:' + f.bucket + '/' + f.prefix, label: f.bucket + '/' + f.prefix, icon: 'ri-star-line', hint: t('hintFavorite'), run: () => openFavorite(f) })),
      ...buckets.map((b): Command => ({ id: 'b:' + b.name, label: b.name, icon: 'ri-archive-drawer-line', hint: t('hintBucket'), run: () => { tab = 'browser'; openBucket(b.name) } })),
      ...profiles.map((p): Command => ({ id: 'p:' + p.id, label: p.name, icon: 'ri-plug-line', hint: t('hintConnection'), run: () => connect(p) })),
      ...(browsing ? view.slice(0, 300).map((i): Command => ({ id: 'i:' + i.key, label: i.name + (i.isFolder ? '/' : ''), icon: i.isFolder ? 'ri-folder-3-line' : fileIcon(i.name), hint: t('hintItem'), run: () => openItem(i) })) : []),
    ]
  }

  async function loadVersions() {
    if (!info) return
    const key = info.key
    const b = bucket
    const [list, status] = await Promise.all([guard(() => api.ListVersions(b, key)), api.BucketVersioning(b).catch(() => null)])
    if (list && info?.key === key) { versions = list; versioning = status }
  }

  async function toggleVersioning() {
    const b = bucket, enable = versioning !== 'Enabled'
    const confirmed = await ask({
      title: enable ? t('enableVersioning') : t('suspendVersioning'), ok: enable ? t('enableVersioning') : t('suspendVersioning'),
      text: t(enable ? 'enableVersioningText' : 'suspendVersioningText', { bucket: b }),
    })
    if (confirmed === null || !(await act(() => api.SetBucketVersioning(b, enable)))) return
    await loadVersions()
  }

  async function restoreVersion(v: api.ObjectVersion) {
    if (!info) return
    const key = info.key
    if (await ask({ title: t('restore'), text: t('restoreText', { date: v.modified }), ok: t('restore') }) === null) return
    if (!(await act(() => api.RestoreVersion(bucket, key, v.versionId)))) return
    notify(t('versionRestored'))
    refresh(true)
    await showInfo(key)
    await loadVersions()
  }

  async function deleteVersion(v: api.ObjectVersion) {
    if (!info) return
    const key = info.key
    if (await ask({ title: t('deleteVersionTitle'), text: t('deleteVersionText', { date: v.modified }), danger: true, ok: t('delete') }) === null) return
    if (!(await act(() => api.DeleteVersion(bucket, key, v.versionId)))) return
    notify(t('versionDeleted'))
    await loadVersions()
    refresh(true)
  }

  async function reveal(key: string) {
    tab = 'browser'
    await navigate(key.slice(0, key.lastIndexOf('/') + 1))
    showInfo(key)
  }

  async function sync() {
    const b = bucket, p = prefix
    const mode = await ask({
      title: t('syncFolder'), text: t('syncModeText'), optionsLabel: t('syncMode'), value: 'add', ok: t('chooseFolder'),
      options: [
        { value: 'add', label: t('modeAdd') }, { value: 'mirror', label: t('modeMirror') },
        { value: 'download', label: t('modeDownload') }, { value: 'downloadMirror', label: t('modeDownloadMirror') },
      ],
      notes: { add: t('modeAddHint'), mirror: t('modeMirrorHint'), download: t('modeDownloadHint'), downloadMirror: t('modeDownloadMirrorHint') },
      dangerValue: 'mirror,downloadMirror',
    })
    if (mode === null) return
    const down = mode.startsWith('download'), mirror = mode.endsWith('irror')
    const res = await guard(() => down ? api.SyncToFolder(b, p, mirror) : api.SyncFolder(b, p, mirror))
    if (res && (res.uploaded || res.downloaded || res.skipped || res.deleted)) {
      const n = down ? res.downloaded : res.uploaded
      notify(down
        ? (mirror ? t('syncDownDoneMirror', { n, s: res.skipped, d: res.deleted }) : t('syncDownDone', { n, s: res.skipped }))
        : (mirror ? t('syncDoneMirror', { u: n, s: res.skipped, d: res.deleted }) : t('syncDone', { u: n, s: res.skipped })))
    }
    refresh(true)
  }

  async function folderSize(i: main.S3Object) {
    const b = bucket
    notify(t('calculating'))
    const stats = await guard(() => api.FolderStats(b, i.key))
    if (!stats) return
    toast = null
    const params = { n: stats.objects.toLocaleString(), size: fmtSize(stats.size) }
    await ask({ title: i.name + '/', text: t(stats.truncated ? 'folderSizeTruncated' : 'folderSizeText', params), cancel: false, ok: t('close') })
  }

  async function newFolder() {
    const name = await ask({ title: t('newFolder'), input: true, value: '', ok: t('create') })
    const folder = name?.trim().replace(/^\/+|\/+$/g, '').trim()
    if (!folder) return
    if (await act(() => api.CreateFolder(bucket, prefix + folder + '/'))) refresh()
  }

  async function upload(folder = false) {
    const n = await guard(() => folder ? api.PickFolderAndUpload(bucket, prefix) : api.PickAndUpload(bucket, prefix))
    if (n) notify(t('nUploaded', { n }))
    refresh(true)
  }

  async function download(keys: string[]) {
    const n = await guard(() => api.Download(bucket, prefix, keys))
    if (n) notify(t('nDownloaded', { n }))
  }

  async function remove(keys: string[]) {
    const folders = keys.filter((k) => k.endsWith('/')).length
    const text = folders ? t('deleteItemsFoldersText', { n: keys.length, f: folders }) : t('deleteItemsText', { n: keys.length })
    if (await ask({ title: t('delete'), text, danger: true, ok: t('delete') }) === null) return
    const ok = await act(() => api.DeleteKeys(bucket, keys))
    if (ok) {
      notify(t('deleted')); selected = new Set(); info = null
      if (search) search = { ...search, items: search.items.filter((i) => !keys.includes(i.key)) }
    }
    refresh(true) // partial deletes may have happened even on error
  }

  async function rename(i: main.S3Object) {
    const name = await ask({ title: t('renameTitle'), text: t('renameText'), input: true, value: i.key, ok: t('save') })
    const target = name?.trim()
    if (!target || target === i.key) return
    if (await act(() => api.RenameObject(bucket, i.key, target))) refresh()
  }

  async function copyLink(key: string) {
    const b = bucket
    const seconds = await ask({
      title: t('copyPresigned'), text: t('linkExpiryText'), optionsLabel: t('linkExpiry'), value: '3600', ok: t('copyLink'),
      options: [{ value: '900', label: t('expiry15m') }, { value: '3600', label: t('expiry1h') }, { value: '86400', label: t('expiry1d') }, { value: '604800', label: t('expiry7d') }],
    })
    if (seconds === null) return
    const url = await guard(() => api.Presign(b, key, Number(seconds)))
    if (url && await act(() => api.CopyToClipboard(url))) notify(t('presignCopied'))
  }

  let activeTransfers = $derived(queue.running + queue.queued)

  const storageClassOptions = ['STANDARD', 'STANDARD_IA', 'ONEZONE_IA', 'INTELLIGENT_TIERING', 'GLACIER_IR', 'GLACIER', 'DEEP_ARCHIVE'].map((value) => ({ value, label: value }))
  async function changeStorageClass(keys: string[]) {
    const objects = keys.filter((key) => !key.endsWith('/'))
    if (!objects.length) return notify(t('selectObjectsOnly'), true)
    const b = bucket
    const cls = await ask({ title: t('changeStorageClass'), ok: t('apply'), text: t('storageClassText', { n: objects.length }), options: storageClassOptions, value: 'STANDARD' })
    if (cls === null) return
    const n = await guard(() => api.SetStorageClass(b, objects, cls))
    if (n !== undefined && n !== null) { notify(t('storageClassChanged', { n })); selected = new Set(); refresh(true) }
  }

  async function loadMounts() {
    try { mounts = (await api.ListMounts()) ?? [] } catch { mounts = [] }
  }
  async function mountCurrent() {
    const b = bucket, p = prefix
    const mode = await ask({
      title: t('mountBucket'), text: t('mountText'), optionsLabel: t('mountMode'), value: 'rw', ok: t('mount'),
      options: [{ value: 'rw', label: t('mountReadWrite') }, { value: 'ro', label: t('mountReadOnly') }],
      notes: { rw: t('mountReadWriteHint'), ro: t('mountReadOnlyHint') },
    })
    if (mode === null) return
    const m = await guard(() => api.MountBucket(b, p, mode === 'ro'))
    if (m) { notify(t('mounted', { path: m.path })); await loadMounts() }
  }
  async function unmount(id: string) {
    if (await act(() => api.UnmountBucket(id))) await loadMounts()
  }

  async function openExternally(key: string) {
    const b = bucket
    if (await act(() => api.EditExternally(b, key))) notify(t('openedExternally'))
  }

  // Transfer events arrive many times per second for thousands of files; they
  // are applied in batches so the list is rendered a few times a second at most.
  const transferIndex = new Map<number, number>()
  let transferBuffer: Transfer[] = []
  let transferFlush: ReturnType<typeof setTimeout> | undefined
  function applyTransferEvents() {
    transferFlush = undefined
    const events = transferBuffer
    transferBuffer = []
    let added = false
    for (const ev of events) {
      const i = transferIndex.get(ev.id)
      if (i !== undefined && transfers[i]?.id === ev.id) transfers[i] = ev
      else { transferIndex.set(ev.id, transfers.length); transfers.push(ev); added = true }
    }
    if (added) showTransfers = true
  }
  function queueTransferEvent(ev: Transfer) {
    transferBuffer.push(ev)
    transferFlush ??= setTimeout(applyTransferEvents, 250)
  }
  async function loadTransfers() {
    try {
      const [list, state] = await Promise.all([api.GetTransfers(), api.GetTransferQueue()])
      transferBuffer = []
      transfers = list ?? []
      transferIndex.clear()
      transfers.forEach((tr, i) => transferIndex.set(tr.id, i))
      queue = state
    } catch (e) { notify(errorText(e), true) }
  }

  function keyboardShortcut(event: KeyboardEvent) {
    if (dialog || editing !== undefined || showAbout || editingHeaders || bucketSettings || objectSettings || copying || palette) return
    if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') { event.preventDefault(); openPalette(); return }
    const target = event.target as HTMLElement | null
    if (target?.closest('input, textarea, select, [contenteditable="true"]')) return
    if (event.key === 'Escape') { langOpen = false; uploadOpen = false; selected = new Set(); return }
    if (!bucket || tab !== 'browser') return
    if (event.key === 'F5' || ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'r')) {
      event.preventDefault(); refresh(); return
    }
    if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'a') {
      event.preventDefault(); selected = new Set(view.map((item) => item.key)); return
    }
    if (event.key === 'Delete' && selected.size) { event.preventDefault(); remove([...selected]) }
  }

  let views = $derived([
    { id: 'browser' as const, label: t('browser'), icon: 'ri-folder-open-line', disabled: false },
    { id: 'analyze' as const, label: t('analyzer'), icon: 'ri-pie-chart-2-line', disabled: !bucket },
    { id: 'backups' as const, label: t('backups'), icon: 'ri-history-line', disabled: !active },
    { id: 'caps' as const, label: t('capabilities'), icon: 'ri-shield-check-line', disabled: !active },
  ])
  let showAbout = $state(false)
  let langOpen = $state(false)

  onMount(() => {
    syncBackend()
    loadProfiles()
    const offAbout = EventsOn('show-about', () => (showAbout = true))
    const closeMenus = () => { langOpen = false; uploadOpen = false; mountOpen = false }
    const onKey = keyboardShortcut
    window.addEventListener('click', closeMenus)
    window.addEventListener('keydown', onKey)
    systemTheme.addEventListener('change', systemThemeChanged)
    // dragenter/dragleave also fire for child elements, so count the depth.
    let dragDepth = 0
    const hasFiles = (event: DragEvent) => event.dataTransfer?.types.includes('Files') ?? false
    const onDragEnter = (event: DragEvent) => { if (hasFiles(event)) { dragDepth++; dragging = true } }
    const onDragLeave = (event: DragEvent) => { if (hasFiles(event) && --dragDepth <= 0) { dragDepth = 0; dragging = false } }
    const onDragEnd = () => { dragDepth = 0; dragging = false }
    window.addEventListener('dragenter', onDragEnter)
    window.addEventListener('dragleave', onDragLeave)
    window.addEventListener('drop', onDragEnd)
    const offT = EventsOn('transfer', queueTransferEvent)
    const offE = EventsOn('edit', (ev: api.EditEvent) => {
      if (ev.state === 'uploaded') { notify(t('editUploaded', { name: ev.key.split('/').pop() ?? ev.key })); refresh(true) }
      else notify(t('editFailed', { name: ev.key.split('/').pop() ?? ev.key, error: ev.error }), true)
    })
    const offM = EventsOn('mount', (ev: api.MountEvent) => {
      loadMounts()
      if (ev.state === 'unmounted') notify(t('unmounted'))
    })
    loadMounts()
    const offQ = EventsOn('queue', (ev: api.TransferQueue) => {
      queue = ev
      // Finished transfers removed on the backend disappear from the list too.
      const known = ev.queued + ev.running + ev.failed + ev.done
      if (known < transfers.length) loadTransfers()
    })
    loadTransfers()
    OnFileDrop(async (files) => {
      if (!bucket || tab !== 'browser') return notify(t('openBucketFirst'), true)
      const n = await guard(() => api.UploadDropped(bucket, prefix, files))
      if (n) notify(t('nUploaded', { n }))
      refresh(true)
    })
    // Periodic background refresh while the window is visible and idle.
    const timer = setInterval(() => {
      if (document.visibilityState === 'visible' && tab === 'browser' && !dialog && !loading) refresh(true)
    }, 15000)
    const onFocus = () => { if (tab === 'browser' && !dialog) refresh(true) }
    window.addEventListener('focus', onFocus)
    return () => { offT(); offQ(); offE(); offM(); offAbout(); OnFileDropOff(); clearInterval(timer); systemTheme.removeEventListener('change', systemThemeChanged); window.removeEventListener('dragenter', onDragEnter); window.removeEventListener('dragleave', onDragLeave); window.removeEventListener('drop', onDragEnd); window.removeEventListener('focus', onFocus); window.removeEventListener('click', closeMenus); window.removeEventListener('keydown', onKey) }
  })
</script>

{#snippet sortIcon(k: string)}
  {#if sortBy === k}<i class={sortAsc ? 'ri-arrow-up-s-fill' : 'ri-arrow-down-s-fill'}></i>{/if}
{/snippet}

<div class="app">

  <aside>
    <div class="brand">
      <img src={logo} alt="" width="24" height="24" /> S3 Browser
    </div>

    <nav class="nav" aria-label={t('views')}>
      {#each views as v (v.id)}
        <button class="ghost navbtn" class:on={tab === v.id} aria-current={tab === v.id ? 'page' : undefined} disabled={v.disabled} onclick={() => (tab = v.id)}>
          <i class={v.icon}></i> {v.label}
        </button>
      {/each}
    </nav>

    <div class="section-title">
      {t('connections')}
      <span>
        <button class="ghost sm" title={t('importProfiles')} onclick={importProfiles}><i class="ri-upload-2-line"></i></button>
        <button class="ghost sm" title={t('exportProfiles')} disabled={!profiles.length} onclick={exportProfiles}><i class="ri-download-2-line"></i></button>
        <button class="ghost sm" title={t('newConnection')} onclick={() => (editing = null)}><i class="ri-add-line"></i></button>
      </span>
    </div>
    <div class="profiles">
      {#each profiles as p (p.id)}
        <div class="prow" class:active={active?.id === p.id}>
          <button class="ghost pbtn" onclick={() => connect(p)}>
            <span class="avatar {p.provider}">{p.name.trim().charAt(0).toUpperCase()}</span>
            <span class="pname">{p.name}</span>
          </button>
          <button class="ghost sm" title={t('edit')} onclick={() => (editing = p)}><i class="ri-edit-line"></i></button>
          <button class="ghost sm" title={t('delete')} onclick={() => removeProfile(p)}><i class="ri-delete-bin-line"></i></button>
        </div>
      {:else}
        <div class="muted pad">{t('noConnections')}<br /><button class="primary" style="margin-top:8px" onclick={() => (editing = null)}>{t('addConnection')}</button></div>
      {/each}
    </div>

    {#if myFavorites.length}
      <div class="section-title">{t('favorites')}</div>
      <div class="favorites">
        {#each myFavorites as f (f.bucket + '/' + f.prefix)}
          <div class="brow" class:active={bucket === f.bucket && prefix === f.prefix && tab === 'browser'}>
            <button class="ghost pbtn" title={f.bucket + '/' + f.prefix} onclick={() => openFavorite(f)}>
              <i class="ri-star-line"></i> <span class="pname">{f.prefix ? f.prefix.split('/').filter(Boolean).pop() : f.bucket}</span>
            </button>
          </div>
        {/each}
      </div>
    {/if}

    {#if active}
      <div class="section-title">
        {t('buckets')}
        <span>
          <button class="ghost sm" title={t('refresh')} onclick={() => connect(active!)}><i class="ri-refresh-line"></i></button>
          <button class="ghost sm" title={t('linkBucketTitle')} onclick={linkBucket}><i class="ri-link"></i></button>
          <button class="ghost sm" title={t('createBucketTitle')} onclick={newBucket}><i class="ri-add-line"></i></button>
        </span>
      </div>
      <div class="buckets">
        {#each buckets as b (b.name)}
          <div class="brow" class:active={bucket === b.name}>
            <button class="ghost pbtn" onclick={() => { tab = 'browser'; openBucket(b.name) }}><i class="ri-archive-drawer-line"></i> <span class="pname">{b.name}</span></button>
            {#if b.pinned}
              <button class="ghost sm del" title={t('unlinkBucketTitle')} onclick={() => unlinkBucket(b.name)}><i class="ri-link-unlink"></i></button>
            {:else}
              <button class="ghost sm del" title={t('deleteBucketTitle')} onclick={() => removeBucket(b.name)}><i class="ri-delete-bin-line"></i></button>
            {/if}
          </div>
        {:else}
          {#if loading}
            <div class="muted pad">{t('loading')}</div>
          {:else if listWarning}
            <div class="pad nolist">
              <p>{t('cannotList')}</p>
              <p class="muted">{t('cannotListHint')}</p>
              <button class="primary" onclick={linkBucket}><i class="ri-link"></i> {t('linkBucket')}</button>
              <details><summary class="muted">{t('errorDetail')}</summary><p class="mono selectable">{listWarning}</p></details>
            </div>
          {:else}
            <div class="muted pad">{t('noBuckets')}</div>
          {/if}
        {/each}
      </div>
    {/if}
    <div class="aside-foot">
      <button class="ghost grow" onclick={() => (showAbout = true)}><i class="ri-information-line"></i> {t('about')}</button>
      <div class="lang">
        <button class="ghost" title={t('language')} aria-haspopup="menu" aria-expanded={langOpen}
          onclick={(e) => { e.stopPropagation(); langOpen = !langOpen }}>
          <i class="ri-translate-2"></i><span class="code">{i18n.lang.toUpperCase()}</span>
        </button>
        {#if langOpen}
          <div class="menu" role="menu">
            {#each languages as l}
              <button class="ghost" role="menuitemradio" aria-checked={i18n.lang === l.id}
                onclick={() => { setLang(l.id); langOpen = false }}>
                <span class="code">{l.id.toUpperCase()}</span>{l.label}
                {#if i18n.lang === l.id}<i class="ri-check-line check"></i>{/if}
              </button>
            {/each}
          </div>
        {/if}
      </div>
      <button class="ghost" title={dark ? t('toLight') : t('toDark')} onclick={toggleTheme}><i class={dark ? 'ri-sun-line' : 'ri-moon-line'}></i></button>
    </div>
  </aside>

  <main>
    <header class="top">
      <button class="command" onclick={openPalette}>
        <i class="ri-search-line"></i><span>{t('paletteHint')}</span><kbd>Ctrl K</kbd>
      </button>
      <span style="flex:1"></span>
      {#if active}<span class="muted conn" title={active.endpoint || 'AWS'}><span class="live"></span>{active.name}</span>{/if}
      <button class="ghost" onclick={() => (showTransfers = !showTransfers)} aria-pressed={showTransfers}>
        <i class="ri-arrow-up-down-line"></i> {t('transfers')} {#if activeTransfers}<span class="badge">{activeTransfers}</span>{/if}
      </button>
    </header>

    {#if !active}
      <div class="welcome">
        <span class="hero"><i class="ri-cloud-line"></i></span>
        <h2>S3 Browser</h2>
        <p class="muted">{t('welcomeText')}</p>
        <button class="primary" onclick={() => (editing = null)}><i class="ri-add-line"></i> {t('addNewConnection')}</button>
      </div>
    {:else if tab === 'caps' || tab === 'backups' || (tab === 'analyze' && bucket)}
      <!-- Capabilities and Analyzer stay mounted below so results survive tab switches -->
    {:else if !bucket}
      <div class="welcome muted"><span class="hero"><i class="ri-archive-drawer-line"></i></span>{t('pickBucket')}</div>
    {:else}
      <div class="toolbar">
        <div class="crumbs">
          <button class="ghost" onclick={() => navigate('')}><i class="ri-archive-drawer-line"></i> {bucket}</button>
          {#each crumbs as c}<span class="muted">/</span><button class="ghost" onclick={() => navigate(c.path)}>{c.name}</button>{/each}
          <button class="ghost star" class:on={isFavorite} aria-pressed={isFavorite} title={isFavorite ? t('removeFavorite') : t('addFavorite')} onclick={toggleFavorite}>
            <i class={isFavorite ? 'ri-star-fill' : 'ri-star-line'}></i>
          </button>
        </div>
        <span style="flex:1"></span>
        <div class="field">
          <i class="ri-filter-3-line"></i>
          <input class="filter" placeholder={t('filter')} bind:value={filter} onkeydown={(e) => { if (e.key === 'Enter') runSearch() }} />
          <button class="ghost sm" onclick={runSearch} disabled={!filter.trim() || loading} title={t('searchBucket')}><i class="ri-search-line"></i></button>
        </div>
        <div class="group">
          <button class="ghost" onclick={() => navigate(crumbs.length > 1 ? crumbs[crumbs.length - 2].path : '')} disabled={!prefix} title={t('parentFolder')}><i class="ri-arrow-up-line"></i></button>
          <button class="ghost" onclick={() => refresh()} title={t('refresh')}><i class="ri-refresh-line"></i></button>
          <button class="ghost" onclick={newFolder} title={t('newFolder')}><i class="ri-folder-add-line"></i></button>
          <button class="ghost" onclick={() => upload(true)} title={t('uploadFolder')}><i class="ri-folder-upload-line"></i></button>
          <button class="ghost" onclick={sync} title={t('syncFolder')}><i class="ri-loop-right-line"></i></button>
          <button class="ghost" onclick={() => (bucketSettings = true)} title={t('bucketSettings')}><i class="ri-settings-3-line"></i></button>
        </div>
        <div class="mountmenu">
            <button class:on={!!currentMount} aria-pressed={!!currentMount} title={currentMount ? t('mountedTitle', { path: currentMount.path }) : t('mountBucket')} aria-haspopup="menu" aria-expanded={mountOpen}
              onclick={(e) => { e.stopPropagation(); if (mounts.length) mountOpen = !mountOpen; else mountCurrent() }}><i class="ri-hard-drive-2-line"></i><span class="lbl">{currentMount ? t('mountedLabel') : t('mount')}</span></button>
            {#if mountOpen}
              <div class="menu" role="menu">
                {#each mounts as m (m.id)}
                  <div class="mrow">
                    <span class="mono" title={m.path}>{m.profile} / {m.bucket}{m.prefix ? '/' + m.prefix : ''}{m.readOnly ? ' · RO' : ''}</span>
                    <button class="ghost sm" title={t('openFolder')} onclick={() => act(() => api.OpenMountFolder(m.id))}><i class="ri-folder-open-line"></i></button>
                    <button class="ghost sm" title={t('unmount')} onclick={() => unmount(m.id)}><i class="ri-eject-line"></i></button>
                  </div>
                {/each}
                {#if !currentMount}
                  <button class="ghost" role="menuitem" onclick={() => { mountOpen = false; mountCurrent() }}><i class="ri-hard-drive-2-line"></i> {t('mountThis')}</button>
                {/if}
              </div>
            {/if}
        </div>
        <div class="upmenu">
          <button class="primary" onclick={() => upload(false)} title={t('uploadFile')}><i class="ri-upload-2-line"></i><span class="lbl">{t('upload')}</span></button>
          <button class="primary caret" title={t('uploadOptions')} aria-haspopup="menu" aria-expanded={uploadOpen} onclick={(e) => { e.stopPropagation(); uploadOpen = !uploadOpen }}><i class="ri-arrow-down-s-line"></i></button>
          {#if uploadOpen}
            <div class="menu" role="menu">
              <button class="ghost" role="menuitem" onclick={() => { uploadOpen = false; upload(false) }}><i class="ri-file-upload-line"></i> {t('uploadFiles')}</button>
              <button class="ghost" role="menuitem" onclick={() => { uploadOpen = false; upload(true) }}><i class="ri-folder-upload-line"></i> {t('uploadFolder')}</button>
              <p class="muted">{t('dropHint')}</p>
            </div>
          {/if}
        </div>
      </div>

      {#if search}
        <div class="selbar found">
          <i class="ri-search-line"></i>
          <span>{t('searchResults', { n: search.items.length, q: search.query })}{#if search.truncated} · {t('searchTruncated')}{/if}</span>
          <button class="ghost" onclick={() => { search = null; refresh() }}><i class="ri-close-line"></i> {t('clearSearch')}</button>
        </div>
      {/if}

      <div class="content">
        <div class="tablewrap">
          <table>
            <thead>
              <tr>
                <th class="cb"><input type="checkbox" checked={allChecked} onchange={toggleAll} /></th>
                <th class="sortable" onclick={() => setSort('name')}>{t('name')} {@render sortIcon('name')}</th>
                <th class="sortable r" onclick={() => setSort('size')}>{t('size')} {@render sortIcon('size')}</th>
                <th class="sortable" onclick={() => setSort('modified')}>{t('modified')} {@render sortIcon('modified')}</th>
                <th class="act"></th>
              </tr>
            </thead>
            <tbody>
              {#each view as i (i.key)}
                <tr class:sel={selected.has(i.key)} class:cur={info?.key === i.key} ondblclick={() => openItem(i)}>
                  <td class="cb"><input type="checkbox" checked={selected.has(i.key)} onchange={() => toggle(i.key)} /></td>
                  <td class="name">
                    <button class="link" onclick={() => openItem(i)}>
                      <span class="chip {i.isFolder ? 'folder' : fileKind(i.name)}"><i class={i.isFolder ? 'ri-folder-3-fill' : fileIcon(i.name)}></i></span>{i.name}
                    </button>
                  </td>
                  <td class="r muted">{i.isFolder ? '—' : fmtSize(i.size)}</td>
                  <td class="muted" title={i.modified}>{relativeTime(i.modified)}</td>
                  <td class="act">
                    <button class="ghost sm" title={t('download')} onclick={() => download([i.key])}><i class="ri-download-2-line"></i></button>
                    {#if i.isFolder}
                      <button class="ghost sm" title={t('folderSize')} onclick={() => folderSize(i)}><i class="ri-pie-chart-2-line"></i></button>
                    {:else}
                      <button class="ghost sm" title={t('copyPresigned')} onclick={() => copyLink(i.key)}><i class="ri-link"></i></button>
                      <button class="ghost sm" title={t('rename')} onclick={() => rename(i)}><i class="ri-edit-line"></i></button>
                    {/if}
                    <button class="ghost sm danger" title={t('delete')} onclick={() => remove([i.key])}><i class="ri-delete-bin-line"></i></button>
                  </td>
                </tr>
              {:else}
                {#if loading}
                  {#each [72, 48, 60, 38, 54, 44] as width}
                    <tr class="skeleton" aria-hidden="true"><td class="cb"></td><td><span class="bone chipbone"></span><span class="bone" style="width:{width}%"></span></td><td><span class="bone" style="width:48px"></span></td><td><span class="bone" style="width:90px"></span></td><td></td></tr>
                  {/each}
                {:else}
                  <tr><td colspan="5" class="empty muted"><span class="hero"><i class="ri-inbox-2-line"></i></span><br />{search ? t('noMatches') : t('emptyFolder')}</td></tr>
                {/if}
              {/each}
            </tbody>
          </table>
          {#if nextToken && !search}<div class="more"><button onclick={loadMore} disabled={loading}>{t('loadMore')}</button></div>{/if}
        </div>

        {#if info}
          <div class="info">
            <div class="ihead">
              <b class="selectable">{info.key.split('/').pop()}</b>
              <button class="ghost sm" title={t('close')} onclick={() => (info = null)}><i class="ri-close-line"></i></button>
            </div>
            <dl class="selectable">
              <dt>Key</dt><dd class="mono">{info.key}</dd>
              <dt>{t('size')}</dt><dd>{fmtSize(info.size)} ({t('bytes', { n: info.size.toLocaleString() })})</dd>
              <dt>Content-Type</dt><dd class="mono">{info.contentType || '—'}</dd>
              <dt>ETag</dt><dd class="mono">{info.etag || '—'}</dd>
              <dt>{t('modified')}</dt><dd>{info.modified}</dd>
              {#if info.storageClass}<dt>Storage class</dt><dd>{info.storageClass}</dd>{/if}
              {#if info.cacheControl}<dt>Cache-Control</dt><dd class="mono">{info.cacheControl}</dd>{/if}
              {#if info.versionId}<dt>Version</dt><dd class="mono">{info.versionId}</dd>{/if}
              {#each Object.entries(info.metadata ?? {}) as [k, v]}<dt>x-amz-meta-{k}</dt><dd class="mono">{v}</dd>{/each}
            </dl>
            <div class="iacts">
              <button onclick={() => download([info!.key])}><i class="ri-download-2-line"></i> {t('download')}</button>
              <button onclick={() => copyLink(info!.key)}><i class="ri-link"></i> {t('link')}</button>
              <button onclick={() => act(() => api.CopyToClipboard(info!.key))}><i class="ri-file-copy-line"></i> {t('copyKey')}</button>
              <button onclick={() => openExternally(info!.key)}><i class="ri-external-link-line"></i> {t('openWith')}</button>
              <button onclick={() => (editingHeaders = true)}><i class="ri-equalizer-line"></i> {t('headers')}</button>
              <button onclick={() => (objectSettings = true)}><i class="ri-settings-3-line"></i> {t('settings')}</button>
              <button onclick={loadVersions}><i class="ri-history-line"></i> {t('versions')}</button>
            </div>
            {#if versions}
              {#if versioning !== null}
                <div class="vstate">
                  <span>{t('versioning')}: <b>{t(versioning === 'Enabled' ? 'versioningEnabled' : versioning === 'Suspended' ? 'versioningSuspended' : 'versioningOff')}</b></span>
                  <button class="sm" onclick={toggleVersioning}>{versioning === 'Enabled' ? t('suspendVersioning') : t('enableVersioning')}</button>
                </div>
              {/if}
              <div class="versions">
                {#each versions as v (v.versionId)}
                  <div class="vrow">
                    <div class="vmeta">
                      <span>{v.modified}{#if v.isLatest} <span class="tag">{t('latest')}</span>{/if}</span>
                      <span class="muted">{v.deleteMarker ? t('deleteMarker') : fmtSize(v.size)}</span>
                    </div>
                    {#if !v.deleteMarker}
                      <button class="ghost sm" title={t('download')} onclick={() => guard(() => api.DownloadVersion(bucket, info!.key, v.versionId))}><i class="ri-download-2-line"></i></button>
                      {#if !v.isLatest}<button class="ghost sm" title={t('restore')} onclick={() => restoreVersion(v)}><i class="ri-arrow-go-back-line"></i></button>{/if}
                    {/if}
                    <button class="ghost sm danger" title={t('deleteVersionTitle')} onclick={() => deleteVersion(v)}><i class="ri-delete-bin-line"></i></button>
                  </div>
                {:else}
                  <p class="muted">{t('noVersions')}</p>
                {/each}
              </div>
            {/if}
            {#if previewImage}<img class="pimg" src={previewImage} alt={info.key.split('/').pop()} />{/if}
            {#if editingText !== null}
              <textarea class="editor mono" bind:value={editingText} spellcheck="false" aria-label={info.key}></textarea>
              <div class="iacts">
                <button class="primary" onclick={saveText} disabled={savingText || editingText === preview}><i class="ri-save-line"></i> {t('save')}</button>
                <button onclick={() => (editingText = null)} disabled={savingText}>{t('cancel')}</button>
              </div>
            {:else if preview}
              <pre class="preview selectable">{preview}</pre>
              <!-- Only a complete, valid text preview may be saved back. -->
              {#if info.size <= 64 * 1024 && !info.contentEncoding && !preview.includes('\uFFFD')}
                <div class="iacts"><button onclick={() => (editingText = preview)}><i class="ri-edit-line"></i> {t('editText')}</button></div>
              {/if}
            {/if}
          </div>
        {/if}
      </div>

      {#if selected.size}
        <div class="floatbar" role="toolbar" aria-label={t('nSelected', { n: selected.size })} transition:fade={{ duration: prefersReducedMotion.current ? 0 : 120 }}>
          <span class="count">{t('nSelected', { n: selected.size })}</span>
          <button class="ghost" onclick={() => download([...selected])}><i class="ri-download-2-line"></i> {t('download')}</button>
          <button class="ghost" onclick={() => (copying = [...selected])}><i class="ri-file-transfer-line"></i> {t('copyTo')}</button>
          <button class="ghost" onclick={() => changeStorageClass([...selected])}><i class="ri-stack-line"></i> {t('storageClass')}</button>
          <button class="ghost del" onclick={() => remove([...selected])}><i class="ri-delete-bin-line"></i> {t('delete')}</button>
          <button class="ghost" title={t('clearSelection')} onclick={() => (selected = new Set())}><i class="ri-close-line"></i></button>
        </div>
      {/if}

      <footer class="status muted">
        {t('nItems', { n: shown.length + (nextToken && !search ? '+' : '') })} · {fmtSize(totalSize)} {loading ? t('loadingSuffix') : ''}
      </footer>
    {/if}

    {#if active}
      {#key active.id}
        <div class="capwrap" class:hidden={tab !== 'caps'}>
          <Capabilities {buckets} {bucket} profileName={active.name} />
        </div>
      {/key}
    {/if}

    {#if active && tab === 'backups'}
      {#key active.id}
        <div class="capwrap"><Backups {profiles} {buckets} {bucket} {prefix} /></div>
      {/key}
    {/if}

    {#if active && bucket}
      {#key active.id + '/' + bucket}
        <div class="capwrap" class:hidden={tab !== 'analyze'}>
          <Analyzer {bucket} {prefix} onreveal={reveal} />
        </div>
      {/key}
    {/if}

    {#if showTransfers}
      <Transfers {transfers} {queue} {fmtSize} onclose={() => (showTransfers = false)} onerror={(message) => notify(message, true)} onchanged={() => refresh(true)} />
    {/if}
  </main>
</div>

{#if editing !== undefined}
  <ProfileModal profile={editing} onclose={() => (editing = undefined)}
    onsaved={async (p) => { editing = undefined; await loadProfiles(); connect(p) }} />
{/if}

{#if bucketSettings && bucket}
  <BucketSettings {bucket} provider={active?.provider} onclose={() => (bucketSettings = false)} onnotify={(message, error) => notify(message, error)} />
{/if}
{#if objectSettings && info}
  <ObjectSettings {bucket} key={info.key} onclose={() => (objectSettings = false)} onnotify={(message, error) => notify(message, error)}
    onchanged={() => { const key = info!.key; refresh(true); showInfo(key) }} />
{/if}
{#if editingHeaders && info}
  <HeadersModal {bucket} {info} onclose={() => (editingHeaders = false)}
    onsaved={() => { const key = info!.key; editingHeaders = false; notify(t('headersSaved')); refresh(true); showInfo(key) }} />
{/if}

{#if copying && active}
  <CopyModal {profiles} activeId={active.id} {bucket} {prefix} keys={copying} onclose={() => (copying = null)}
    ondone={(n, moved) => {
      const keys = copying ?? []
      copying = null; selected = new Set(); info = null
      notify(t(moved ? 'nMoved' : 'nCopied', { n }))
      if (moved && search) search = { ...search, items: search.items.filter((i) => !keys.includes(i.key)) }
      refresh(true)
    }} />
{/if}

{#if palette}<Palette commands={palette} onclose={() => (palette = null)} />{/if}

{#if showAbout}<About onclose={() => (showAbout = false)} />{/if}

{#if dialog}
  <Modal label={dialog.title} onclose={() => closeDialog(null)}>
    <form class="dlg" onsubmit={(e) => { e.preventDefault(); closeDialog(dialog!.input || dialog!.options ? dialog!.value ?? '' : 'ok') }}>
      <h3>{dialog.title}</h3>
      {#if dialog.text}<p>{dialog.text}</p>{/if}
      <!-- svelte-ignore a11y_autofocus -->
      {#if dialog.input}<input bind:value={dialog.value} autofocus spellcheck="false" />{/if}
      {#if dialog.options}
        <label>{dialog.optionsLabel}
          <select bind:value={dialog.value}>
            {#each dialog.options as option}<option value={option.value}>{option.label}</option>{/each}
          </select>
        </label>
        {#if dialog.notes?.[dialog.value ?? '']}<p class:warn={dialog.dangerValue?.split(',').includes(dialog.value ?? '')}>{dialog.notes[dialog.value ?? '']}</p>{/if}
      {/if}
      <div class="dacts">
        {#if dialog.cancel !== false}<button type="button" onclick={() => closeDialog(null)}>{t('cancel')}</button>{/if}
        <button type="submit" class={dialog.danger || (dialog.dangerValue && dialog.value === dialog.dangerValue) ? 'danger' : 'primary'}>{dialog.ok}</button>
      </div>
    </form>
  </Modal>
{/if}

{#if dragging}
  <div class="drop" transition:fade={{ duration: prefersReducedMotion.current ? 0 : 120 }}>
    <div class="dropcard">
      <span class="hero"><i class="ri-upload-cloud-2-line"></i></span>
      <b>{bucket && tab === 'browser' ? t('dropToUpload') : t('openBucketFirst')}</b>
      {#if bucket && tab === 'browser'}<span class="muted mono">{t('dropTarget', { path: bucket + '/' + prefix })}</span>{/if}
    </div>
  </div>
{/if}

{#if toast}<div class="toast selectable" role="status" aria-live="polite" in:fade={{ duration: prefersReducedMotion.current ? 0 : 140 }} class:err={toast.err}>{toast.text}</div>{/if}

<style>
  /* A single row of exactly the viewport height: tall content scrolls inside its panel instead of pushing the sidebar footer off screen. */
  .app { display: grid; grid-template-columns: 248px 1fr; grid-template-rows: minmax(0, 1fr); height: 100vh; overflow: hidden; background: var(--bg); }
  .nav { display: flex; flex-direction: column; gap: 1px; padding: 2px 8px 6px; }
  .navbtn { justify-content: flex-start; gap: 10px; padding: 6px 10px; color: var(--sidebar-label); }
  .navbtn:hover:not(:disabled) { background: var(--sidebar-hover); }
  .navbtn.on { background: var(--sidebar-selected); color: var(--text-strong); font-weight: 600; box-shadow: var(--shadow-xs); }
  .navbtn.on i { color: var(--accent); }
  .avatar { display: grid; place-items: center; width: 20px; height: 20px; flex: none; border-radius: 6px; font-size: 11px; font-weight: 700; --c: #71717a; color: var(--c); background: color-mix(in srgb, var(--c) 16%, transparent); }
  .avatar.supabase { --c: #10b981; } .avatar.aws { --c: #f59e0b; } .avatar.minio { --c: #e11d48; } .avatar.r2 { --c: #f97316; } /* provider brand hues */
  aside { display: flex; flex-direction: column; min-height: 0; overflow: hidden; }
  .brand { padding: 16px 16px 8px; font-weight: 650; font-size: 14px; letter-spacing: -.01em; color: var(--text-strong); display: flex; align-items: center; gap: 9px; }
  .brand img { border-radius: 6px; }
  .section-title { display: flex; justify-content: space-between; align-items: center; padding: 14px 8px 4px 16px; font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: .05em; color: var(--muted); }
  .profiles { max-height: 35%; overflow: auto; flex: none; }
  .buckets { flex: 1; min-height: 0; overflow: auto; padding-bottom: 10px; }
  .favorites { max-height: 22%; overflow: auto; flex: none; }
  .star { padding: 3px 6px; color: var(--muted) !important; }
  .star.on { color: var(--accent) !important; }
  kbd { font: inherit; font-size: 11px; padding: 1px 6px; border: 1px solid var(--border); border-radius: 5px; background: var(--panel2); }
  .editor { min-height: 260px; resize: vertical; white-space: pre; line-height: 1.5; }
  .prow, .brow { display: flex; align-items: center; margin: 1px 8px; padding: 0 2px; border-radius: var(--radius-md); }
  .prow:hover, .brow:hover { background: var(--sidebar-hover); }
  .prow.active, .brow.active { background: var(--sidebar-selected); box-shadow: var(--shadow-xs); }
  .pbtn { flex: 1; min-width: 0; justify-content: flex-start; gap: 8px; color: var(--sidebar-label); background: transparent !important; border: none !important; }
  .prow.active .pbtn, .brow.active .pbtn { color: var(--text-strong); font-weight: 600; }
  .brow.active .pbtn i { color: var(--accent); }
  .pname { overflow: hidden; text-overflow: ellipsis; }
  .prow .sm, .brow .del { opacity: 0; }
  .prow:hover .sm, .prow:focus-within .sm, .prow.active .sm, .brow:hover .del, .brow:focus-within .del { opacity: .75; }
  .sm { padding: 2px 6px; min-height: 24px; font-size: 12px; }
  .pad { padding: 8px 16px; }
  .aside-foot { margin-top: auto; flex: none; padding: 8px; display: flex; align-items: center; gap: 2px; }
  .aside-foot button { color: var(--sidebar-label); }
  .aside-foot .grow { flex: 1; justify-content: flex-start; }
  .aside-foot button:hover:not(:disabled) { background: var(--sidebar-hover); }
  .nolist p { margin: 0 0 6px; line-height: 1.45; font-size: 12px; }
  .nolist button { margin: 4px 0 8px; }
  .nolist details { font-size: 11px; }
  .nolist details p { word-break: break-word; margin-top: 4px; }

  /* The content area is a raised card on the shell background. */
  main { display: flex; flex-direction: column; min-width: 0; min-height: 0; position: relative; margin: 8px 8px 8px 0; background: var(--panel); border: 1px solid var(--border); border-radius: var(--radius-lg); box-shadow: var(--shadow-xs); overflow: hidden; }
  .top { display: flex; align-items: center; gap: 6px; padding: 8px 10px; border-bottom: 1px solid var(--border); }
  .command { flex: 0 1 380px; min-width: 160px; justify-content: flex-start; gap: 8px; color: var(--muted); background: var(--panel2); border-color: transparent; box-shadow: none; font-weight: 400; }
  .command:hover:not(:disabled) { border-color: var(--field-border); color: var(--text); }
  .command span { flex: 1; text-align: start; overflow: hidden; text-overflow: ellipsis; }
  .live { display: inline-block; width: 7px; height: 7px; margin-inline-end: 7px; border-radius: 50%; background: var(--success); box-shadow: 0 0 0 3px var(--success-soft); }
  .field { display: flex; align-items: center; gap: 2px; flex: 0 1 240px; min-width: 120px; padding: 0 4px 0 10px; border: 1px solid var(--field-border); border-radius: var(--radius-md); background: var(--field); color: var(--muted); transition: border-color .12s, box-shadow .12s; }
  .field:focus-within { border-color: var(--accent); box-shadow: 0 0 0 3px var(--accent-soft); }
  .field input { border: none; background: transparent; box-shadow: none; padding: 6px 6px; }
  .group { display: flex; align-items: center; gap: 1px; padding: 2px; border-radius: 10px; background: var(--panel2); }
  .group button { min-height: 28px; padding: 4px 9px; border-radius: 8px; }
  .group button:hover:not(:disabled) { background: var(--raised); box-shadow: var(--shadow-xs); }
  .conn { margin-inline-end: 6px; max-width: 30%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 12px; }
  .lang { position: relative; }
  .upmenu { position: relative; display: flex; }
  .mountmenu { position: relative; display: flex; margin-inline-start: 6px; }
  .mountmenu > button { gap: 6px; }
  .mountmenu .on { color: var(--accent); border-color: var(--accent); }
  .mountmenu .menu { position: absolute; inset-inline-end: 0; top: calc(100% + 6px); min-width: 300px; padding: 4px; background: var(--panel); border: 1px solid var(--border); border-radius: 10px; box-shadow: var(--shadow); z-index: 40; display: flex; flex-direction: column; gap: 2px; }
  .mountmenu .mrow { display: flex; align-items: center; gap: 4px; padding: 4px 6px; font-size: 12px; } .mountmenu .mrow span { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .mountmenu .menu > button { justify-content: flex-start; gap: 10px; padding: 6px 10px; font-weight: 500; border-radius: 7px; }
  .upmenu > .primary:first-child { border-start-end-radius: 0; border-end-end-radius: 0; }
  .upmenu .caret { padding: 0 6px; border-start-start-radius: 0; border-end-start-radius: 0; border-inline-start: 1px solid color-mix(in srgb, #000 20%, transparent); }
  .upmenu .menu { position: absolute; inset-inline-end: 0; top: calc(100% + 6px); min-width: 220px; padding: 4px; background: var(--panel); border: 1px solid var(--border); border-radius: 10px; box-shadow: var(--shadow); z-index: 40; display: flex; flex-direction: column; }
  .upmenu .menu button { justify-content: flex-start; gap: 10px; padding: 6px 10px; font-weight: 500; border-radius: 7px; }
  .upmenu .menu p { margin: 4px 10px 6px; font-size: 11px; line-height: 1.4; }
  .lang .code { font-size: 11px; font-weight: 600; letter-spacing: .04em; }
  .lang .menu { position: absolute; inset-inline-start: 0; bottom: calc(100% + 6px); min-width: 170px; padding: 4px; background: var(--panel); border: 1px solid var(--border); border-radius: 10px; box-shadow: var(--shadow); z-index: 40; display: flex; flex-direction: column; }
  .lang .menu button { justify-content: flex-start; gap: 10px; padding: 6px 10px; font-weight: 500; border-radius: 7px; }
  .lang .menu button[aria-checked='true'] { color: var(--accent-soft-text); background: var(--accent-soft); }
  .lang .menu .code { width: 20px; color: var(--muted); }
  .lang .menu .check { margin-inline-start: auto; color: var(--accent); }
  .badge { background: var(--accent); color: var(--accent-contrast); border-radius: 99px; padding: 0 6px; font-size: 11px; font-weight: 600; }

  .capwrap { flex: 1; min-height: 0; }
  .capwrap.hidden { display: none; }
  .welcome { margin: auto; padding: 24px; text-align: center; display: flex; flex-direction: column; align-items: center; gap: 10px; max-width: 420px; }
  .welcome h2 { margin: 0; font-size: 18px; font-weight: 650; letter-spacing: -.01em; color: var(--text-strong); }
  .welcome p { margin: 0 0 6px; line-height: 1.5; }
  .hero { display: grid; place-items: center; width: 56px; height: 56px; margin-bottom: 4px; border-radius: 16px; background: var(--accent-soft); color: var(--accent); }
  .hero i[class^="ri-"] { font-size: 26px; }

  .toolbar { display: flex; align-items: center; gap: 6px; padding: 8px 10px; border-bottom: 1px solid var(--border); }
  .crumbs { display: flex; align-items: center; gap: 0; min-width: 120px; flex: 0 1 auto; overflow: hidden; }
  .crumbs button { padding: 3px 7px; font-weight: 600; color: var(--text-strong); }
  .filter { width: 100%; min-width: 0; }
  .selbar { display: flex; align-items: center; gap: 8px; margin: 8px 10px 0; padding: 5px 6px 5px 12px; background: var(--accent-soft); color: var(--accent-soft-text); border-radius: 10px; font-weight: 500; }
  .selbar > span { margin-inline-end: auto; }
  .selbar.found { background: var(--info-soft); color: var(--info); }

  .vstate { display: flex; align-items: center; justify-content: space-between; gap: 8px; font-size: 12px; }
  .versions { display: flex; flex-direction: column; border: 1px solid var(--border); border-radius: var(--radius-md); font-size: 12px; }
  .versions p { margin: 0; padding: 10px; line-height: 1.5; }
  .vrow { display: flex; align-items: center; gap: 2px; padding: 5px 4px 5px 10px; border-top: 1px solid var(--row-border); }
  .vrow:first-child { border-top: none; }
  .vmeta { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
  .tag { margin-inline-start: 4px; padding: 1px 6px; border-radius: 99px; background: var(--success-soft); color: var(--success); font-size: 10px; font-weight: 600; }

  @media (max-width: 1100px) { .toolbar .lbl, .conn { display: none; } }
  .content { flex: 1; display: flex; min-height: 0; }
  .tablewrap { flex: 1; overflow: auto; }
  table { width: 100%; border-collapse: collapse; }
  thead th { position: sticky; top: 0; background: var(--panel); font-size: 11px; font-weight: 600; text-align: start; text-transform: uppercase; letter-spacing: .04em; color: var(--muted); padding: 9px 10px; border-bottom: 1px solid var(--border); z-index: 1; }
  .sortable { cursor: pointer; }
  .sortable:hover { color: var(--text); }
  td { padding: 7px 10px; border-bottom: 1px solid var(--row-border); white-space: nowrap; }
  .chip { display: inline-grid; place-items: center; width: 26px; height: 26px; margin-inline-end: 10px; vertical-align: middle; border-radius: 8px; --c: #71717a; color: var(--c); background: color-mix(in srgb, var(--c) 14%, transparent); }
  .chip.folder { --c: var(--accent); } .chip.image { --c: #8b5cf6; } .chip.video { --c: #f43f5e; } .chip.audio { --c: #ec4899; }
  .chip.archive { --c: #d97706; } .chip.pdf { --c: #ef4444; } .chip.code { --c: #0ea5e9; } .chip.doc { --c: #10b981; } .chip.sheet { --c: #22c55e; }
  .bone { display: inline-block; height: 10px; border-radius: 99px; background: var(--panel2); vertical-align: middle; animation: pulse 1.2s ease-in-out infinite; }
  .chipbone { width: 26px; height: 26px; margin-inline-end: 10px; border-radius: 8px; }
  .skeleton:hover { background: none; }
  @keyframes pulse { 50% { opacity: .45; } }
  .floatbar { position: absolute; inset-inline-start: 50%; bottom: 44px; transform: translateX(-50%); z-index: 15; display: flex; align-items: center; gap: 2px; padding: 5px 6px 5px 14px; border-radius: 14px; background: var(--inverse); color: var(--inverse-text); box-shadow: var(--shadow); white-space: nowrap; }
  .floatbar .count { margin-inline-end: 8px; font-weight: 600; }
  .floatbar button { color: inherit; }
  .floatbar button:hover:not(:disabled) { background: color-mix(in srgb, var(--inverse-text) 14%, transparent); }
  .floatbar .del { color: #f87171; }
  tbody tr:hover { background: var(--row-hover); }
  tr.sel { background: var(--selected-bg); }
  tr.cur { background: var(--info-soft); }
  .r { text-align: end; }
  .cb { width: 34px; padding-inline-start: 14px; }
  .name { width: 100%; max-width: 0; overflow: hidden; text-overflow: ellipsis; }
  .link { background: none; border: none; box-shadow: none; min-height: 0; padding: 2px 0; max-width: 100%; overflow: hidden; text-overflow: ellipsis; display: inline-block; text-align: start; color: var(--text-strong); }
  .link:hover { color: var(--accent); background: none !important; }
  i[class^="ri-"] { font-size: 15px; line-height: 1; }
  .act { width: 1%; text-align: end; }
  .act button { opacity: 0; }
  tr:hover .act button, tr:focus-within .act button { opacity: 1; }
  .danger { color: var(--danger); }
  .empty { text-align: center; padding: 56px !important; line-height: 2.2; }
  .empty .hero { display: inline-grid; background: var(--panel2); color: var(--muted); }
  .more { text-align: center; padding: 12px; }
  .status { padding: 6px 14px; border-top: 1px solid var(--border); font-size: 12px; }

  .info { width: 340px; border-inline-start: 1px solid var(--border); overflow: auto; padding: 14px; display: flex; flex-direction: column; gap: 12px; }
  .ihead { display: flex; justify-content: space-between; align-items: center; gap: 8px; word-break: break-all; color: var(--text-strong); }
  dl { margin: 0; display: grid; grid-template-columns: auto 1fr; gap: 6px 12px; font-size: 12px; }
  dt { color: var(--muted); }
  dd { margin: 0; word-break: break-all; }
  .iacts { display: flex; gap: 6px; flex-wrap: wrap; }
  .preview { margin: 0; background: var(--panel2); border: 1px solid var(--border); border-radius: var(--radius-md); padding: 10px; font-size: 11px; white-space: pre-wrap; word-break: break-all; max-height: 50vh; overflow: auto; }

  .pimg { display: block; max-width: 100%; max-height: 320px; margin: 0 auto; border-radius: var(--radius-md); border: 1px solid var(--border); object-fit: contain; background: repeating-conic-gradient(var(--panel2) 0% 25%, transparent 0% 50%) 50% / 16px 16px; }

  /* pointer-events stay off so the drop reaches the window listeners. */
  .drop { position: fixed; inset: 0; z-index: 90; display: grid; place-items: center; background: var(--mask); pointer-events: none; }
  .dropcard { display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 32px 48px; background: var(--panel); border: 2px dashed var(--accent); border-radius: 20px; box-shadow: var(--shadow); color: var(--text-strong); font-size: 15px; }
  .dropcard .muted { font-size: 12px; font-weight: 400; }


  .dlg { width: min(420px, 92vw); background: var(--panel); border: 1px solid var(--border); border-radius: var(--radius-lg); padding: 22px; display: flex; flex-direction: column; gap: 14px; box-shadow: var(--shadow); }
  .dlg h3 { margin: 0; font-size: 15px; font-weight: 650; color: var(--text-strong); }
  .dlg p { margin: 0; color: var(--muted); line-height: 1.5; }
  .dlg p.warn { color: var(--danger-text); }
  .dacts { display: flex; justify-content: flex-end; gap: 8px; margin-top: 4px; }

  .toast { position: fixed; bottom: 20px; inset-inline-start: 50%; transform: translateX(-50%); background: var(--inverse); color: var(--inverse-text); padding: 9px 16px; border-radius: 10px; z-index: 100; max-width: 70vw; box-shadow: var(--shadow); font-weight: 500; }
  .toast.err { background: #b91c1c; color: #ffffff; }
</style>
