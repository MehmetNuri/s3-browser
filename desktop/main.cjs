const { app, BrowserWindow, ipcMain, dialog, clipboard, shell, Tray, Menu, nativeImage, nativeTheme, session, safeStorage, Notification } = require('electron')
const { join, isAbsolute } = require('node:path')
const { pathToFileURL } = require('node:url')
const { execFile, spawn } = require('node:child_process')
const { createInterface } = require('node:readline')
const { readFileSync } = require('node:fs')
const { Backend } = require('./backend.cjs')
const { createTrayMenu, activityLabel } = require('./tray-menu.cjs')
const { createTrayActivity } = require('./tray-activity.cjs')
// Host-only backend methods are deliberately absent from this set.
const rendererMethods = new Set(require('../backend/api-methods.json').renderer)
const maxRequestSize = 2 * 1024 * 1024
if (process.argv.includes('--s3browser-tray-helper')) {
  require('./tray-helper.cjs').start()
} else {
let window, backend, tray, legacyTray
let backendReady = false
let trayActivity, trayMenuKey
// Operations announced by the backend, and single transfers such as retries.
const batches = new Map()
const transfers = new Map()
let status = { state: 'idle' }
let outcome = { done: 0, failed: 0 }
let statusTimer, settleTimer
let legacyTrayReady = false
let quitting = false
let closeToTray = true
const S3BROWSER_DEBUG = process.env.S3BROWSER_DEBUG === '1'
let language = 'en'
const htmlPath = join(__dirname, '..', 'frontend', 'dist', 'index.html')
const rendererURL = pathToFileURL(htmlPath).href
app.setAppUserModelId('dev.s3browser.S3Browser')

let virtualDisplay = false
if (process.platform === 'linux') {
  try {
    const machine = readFileSync('/sys/class/dmi/id/product_name', 'utf8')
    virtualDisplay = /vmware|virtualbox/i.test(machine)
  } catch { /* Machine information is unavailable on some systems. */ }
}
if (virtualDisplay || process.env.S3BROWSER_DISABLE_GPU === '1') app.disableHardwareAcceleration()


function trustedSender(event) {
  if (!window || window.isDestroyed() || !event.senderFrame) return false
  if (event.sender !== window.webContents || event.senderFrame !== window.webContents.mainFrame) return false
  try {
    const url = new URL(event.senderFrame.url)
    url.hash = ''; url.search = ''
    return url.href === rendererURL
  } catch { return false }
}
function sendToRenderer(name, data) {
  if (window && !window.isDestroyed()) window.webContents.send('backend:event', name, data)
}
function showAbout() { showWindow(); sendToRenderer('show-about', null) }
function showWindow() {
  if (!window || window.isDestroyed()) return
  if (window.isMinimized()) window.restore()
  window.show()
  window.focus()
}
// Works out one status for the tray and the taskbar from everything in progress.
function currentStatus() {
  const active = [...batches.values()]
  const running = [...transfers.values()]
  // Bytes of the transfers in flight count as fractions of a file.
  const partial = running.reduce((sum, t) => sum + (t.total > 0 ? Math.min(1, t.done / t.total) : 0), 0)
  let parts, fraction
  if (active.length) {
    parts = active.map((b) => ({ kind: b.kind, done: b.done + b.failed + b.cancelled, total: b.total }))
    const total = parts.reduce((sum, p) => sum + p.total, 0)
    const finished = parts.reduce((sum, p) => sum + p.done, 0)
    fraction = total ? Math.min(1, (finished + Math.min(partial, total - finished)) / total) : null
  } else if (running.length) {
    const kinds = [...new Set(running.map((t) => t.kind))]
    parts = kinds.map((kind) => ({ kind, done: 0, total: running.filter((t) => t.kind === kind).length }))
    fraction = partial / running.length
  } else return null
  const kinds = [...new Set(parts.map((p) => p.kind))]
  return { state: 'active', kind: kinds.length === 1 ? kinds[0] : 'mixed', parts, fraction }
}
function updateStatus() {
  statusTimer = undefined
  const next = currentStatus()
  if (next) {
    clearTimeout(settleTimer); settleTimer = undefined
    // A transfer leaves the list just before its batch counts it; do not step back meanwhile.
    next.key = [...batches.keys()].join()
    if (status.state === 'active' && status.key === next.key && next.fraction !== null) next.fraction = Math.max(status.fraction ?? 0, next.fraction)
    status = next
  } else if (status.state === 'active') {
    // Show the outcome for a moment; a failure stays visible longer.
    status = outcome.failed ? { state: 'failed', failed: outcome.failed } : outcome.done ? { state: 'done' } : { state: 'idle' }
    const hold = status.state === 'failed' ? 10000 : 3500
    outcome = { done: 0, failed: 0 }
    notifyOutcome()
    if (status.state !== 'idle') settleTimer = setTimeout(() => { settleTimer = undefined; status = { state: 'idle' }; refreshTray() }, hold)
  } else return
  refreshTray()
}
// Tells the user that work finished while the window was hidden or in the background.
function notifyOutcome() {
  if (status.state === 'idle' || !Notification.isSupported()) return
  if (window && !window.isDestroyed() && window.isVisible() && window.isFocused()) return
  const notification = new Notification({ title: 'S3 Browser', body: activityLabel(language, status), silent: status.state === 'done' })
  notification.on('click', showWindow)
  notification.show()
}
function trackActivity(name, data) {
  if (!data || typeof data.id !== 'number') return
  if (name === 'batch') {
    if (data.active) batches.set(data.id, data)
    else if (batches.delete(data.id)) { outcome.done += data.done; outcome.failed += data.failed }
  } else if (data.state === 'running') transfers.set(data.id, data)
  else if (transfers.delete(data.id) && !batches.size) {
    if (data.state === 'done') outcome.done++
    else if (data.state === 'error') outcome.failed++
  }
  // Progress arrives many times per second; the tray needs far fewer updates.
  statusTimer ??= setTimeout(updateStatus, 400)
}
function refreshTray() {
  const label = activityLabel(language, status)
  if (S3BROWSER_DEBUG && label !== refreshTray.logged) { refreshTray.logged = label; console.info('Tray status:', label || 'idle') }
  if (window && !window.isDestroyed()) window.setProgressBar(status.state === 'active' ? status.fraction ?? 2 : -1)
  if (legacyTrayReady) legacyTray.stdin.write(JSON.stringify({ language, closeToTray, status }) + '\n')
  if (!tray || tray.isDestroyed()) return
  tray.setToolTip(label ? `S3 Browser — ${label}` : 'S3 Browser')
  trayActivity?.update(status)
  // Rebuilding an open menu closes it, so only do that when its content changes.
  const menuLabel = activityLabel(language, status, false)
  const key = JSON.stringify([language, closeToTray, menuLabel, nativeTheme.shouldUseDarkColors])
  if (key === trayMenuKey) return
  trayMenuKey = key
  tray.setContextMenu(createTrayMenu({ language, closeToTray, status: menuLabel, show: showWindow,
    about: showAbout,
    setCloseToTray: async (enabled) => {
      try { await backend.call('SetCloseToTray', [enabled]); closeToTray = enabled; refreshTray() }
      catch (error) { dialog.showErrorBox('Settings could not be saved', error.message); trayMenuKey = undefined; refreshTray() }
    }, quit: () => app.quit()
  }))
}
function trayAvailable() {
  if (legacyTrayReady && legacyTray && !legacyTray.killed) return Promise.resolve(true)
  if (!tray || tray.isDestroyed()) return Promise.resolve(false)
  if (process.platform !== 'linux') return Promise.resolve(true)
  const bounds = tray.getBounds()
  if (bounds.width > 0 && bounds.height > 0) return Promise.resolve(true)
  return new Promise((resolve) => execFile('gdbus', ['call', '--session', '--dest', 'org.freedesktop.DBus', '--object-path', '/org/freedesktop/DBus', '--method', 'org.freedesktop.DBus.NameHasOwner', 'org.kde.StatusNotifierWatcher'], { timeout: 1500 }, (error, output) => resolve(!error && output.includes('true'))))
}
function filters(options) {
  return (options?.filters || []).map(({ name, pattern }) => ({ name, extensions: pattern.split(';').map((value) => value.replace(/^\*\./, '')) }))
}
// Without a real keyring Chromium falls back to a fixed password ("basic_text"),
// which protects nothing; the backend then keeps its documented plaintext format.
function secretStorageAvailable() {
  if (!safeStorage.isEncryptionAvailable()) return false
  return process.platform !== 'linux' || ['gnome_libsecret', 'kwallet', 'kwallet5', 'kwallet6'].includes(safeStorage.getSelectedStorageBackend())
}
async function hostRequest(method, options) {
  switch (method) {
    case 'secretAvailable': return secretStorageAvailable()
    case 'encryptSecret': {
      if (typeof options !== 'string' || !secretStorageAvailable()) throw new Error('Secret storage is unavailable')
      const sealed = safeStorage.encryptString(options)
      // Never hand back a value that this keychain cannot open again.
      if (safeStorage.decryptString(sealed) !== options) throw new Error('Secret storage verification failed')
      return sealed.toString('base64')
    }
    case 'decryptSecret': {
      if (typeof options !== 'string' || !secretStorageAvailable()) throw new Error('Secret storage is unavailable')
      try { return safeStorage.decryptString(Buffer.from(options, 'base64')) }
      catch (error) {
        console.error(`Stored secret could not be decrypted (backend ${process.platform === 'linux' ? safeStorage.getSelectedStorageBackend() : process.platform}): ${error.message}`)
        throw error
      }
    }
    case 'openFile': case 'openFiles': case 'openDirectory': {
      const properties = method === 'openDirectory' ? ['openDirectory', 'createDirectory'] : method === 'openFiles' ? ['openFile', 'multiSelections'] : ['openFile']
      const result = await dialog.showOpenDialog(window, { title: options?.title, filters: filters(options), properties })
      if (method === 'openFiles') return result.canceled ? [] : result.filePaths
      return result.canceled ? '' : result.filePaths[0]
    }
    case 'saveFile': {
      const result = await dialog.showSaveDialog(window, { title: options?.title, defaultPath: options?.defaultFilename, filters: filters(options) })
      return result.canceled ? '' : result.filePath
    }
    case 'clipboard': clipboard.writeText(options); return null
    case 'openConfigDir': {
      const error = await shell.openPath(options)
      if (error) throw new Error(error)
      return null
    }
    default: throw new Error('Unknown desktop operation')
  }
}
// Applies to every web contents, including any created unexpectedly.
app.on('web-contents-created', (_event, contents) => {
  contents.setWindowOpenHandler(() => ({ action: 'deny' }))
  contents.on('will-navigate', (event) => event.preventDefault())
  contents.on('will-redirect', (event) => event.preventDefault())
  contents.on('will-attach-webview', (event) => event.preventDefault())
})
if (!app.requestSingleInstanceLock()) {
  console.info('S3 Browser is already running. Bringing its window to the front.')
  app.quit()
}
else {
  app.on('second-instance', showWindow)
  app.whenReady().then(async () => {
    Menu.setApplicationMenu(null)
    nativeTheme.on('updated', refreshTray)
    const osName = { linux: 'linux', win32: 'win', darwin: 'mac' }[process.platform]
    const executableName = `s3browser-backend${process.platform === 'win32' ? '.exe' : ''}`
    const executable = app.isPackaged ? join(process.resourcesPath, 'backend', executableName) : join(__dirname, '..', 'build', 'backend', `${osName}-${process.arch}`, executableName)
    backend = new Backend(executable, hostRequest, (name, data) => {
      if (name === 'language-changed') { language = data; refreshTray(); return }
      if (name === 'batch' || name === 'transfer') trackActivity(name, data)
      if (['transfer', 'cap', 'show-about', 'backup'].includes(name)) sendToRenderer(name, data)
    }, (error) => {
      // Without the backend every operation would fail; do not keep a dead window open.
      if (quitting || !backendReady) return
      dialog.showErrorBox('S3 Browser stopped', error.message)
      app.quit()
    }, { ...process.env, S3BROWSER_APP_VERSION: app.getVersion(), S3BROWSER_ELECTRON_VERSION: process.versions.electron })
    await backend.ready
    backendReady = true
    closeToTray = (await backend.call('GetSettings')).closeToTray
    session.defaultSession.setPermissionRequestHandler((_contents, _permission, callback) => callback(false))
    session.defaultSession.setPermissionCheckHandler(() => false)
    window = new BrowserWindow({ width: 1280, height: 820, minWidth: 900, minHeight: 560, show: false,
      // Matches the shell background so the window does not flash white in a dark desktop.
      backgroundColor: nativeTheme.shouldUseDarkColors ? '#0c0c0e' : '#f4f4f5',
      title: 'S3 Browser', icon: app.isPackaged ? join(process.resourcesPath, 'tray.png') : join(__dirname, '..', 'packaging', 'icons', 's3browser-64.png'),
      webPreferences: { preload: join(__dirname, 'preload.cjs'), nodeIntegration: false, contextIsolation: true, sandbox: true, webSecurity: true, webviewTag: false }
    })
    ipcMain.handle('backend:call', async (event, method, args) => {
      if (!trustedSender(event) || !rendererMethods.has(method) || !Array.isArray(args) || JSON.stringify(args).length > maxRequestSize) throw new Error('Invalid desktop request')
      return backend.call(method, args)
    })
    // The preload resolves these paths from files the user dropped; the renderer cannot name paths itself.
    ipcMain.handle('backend:upload-dropped', async (event, bucket, prefix, paths) => {
      const valid = trustedSender(event) && typeof bucket === 'string' && typeof prefix === 'string' &&
        Array.isArray(paths) && paths.length > 0 && paths.every((path) => typeof path === 'string' && isAbsolute(path)) &&
        JSON.stringify([bucket, prefix, paths]).length <= maxRequestSize
      if (!valid) throw new Error('Invalid desktop request')
      return backend.call('UploadPaths', [bucket, prefix, paths])
    })
    window.on('close', async (event) => {
      if (quitting) return
      event.preventDefault()
      if (closeToTray && await trayAvailable()) window.hide()
      else app.quit()
    })
    window.once('ready-to-show', showWindow)
    await window.loadFile(htmlPath)
    showWindow()
    const needsLegacyTray = await new Promise((resolve) => {
      if (process.platform !== 'linux' || !process.env.DISPLAY) return resolve(false)
      execFile('gdbus', ['call', '--session', '--dest', 'org.freedesktop.DBus', '--object-path', '/org/freedesktop/DBus', '--method', 'org.freedesktop.DBus.NameHasOwner', 'org.kde.StatusNotifierWatcher'], { timeout: 1500 }, (error, output) => {
        if (!error && output.includes('true')) return resolve(false)
        backend.call('HasLegacyTrayHost').then(resolve).catch(() => resolve(false))
      })
    })
    if (needsLegacyTray) {
      const args = ['--s3browser-tray-helper', '--ozone-platform=x11', '--gtk-version=3']
      if (!app.isPackaged) args.unshift(app.getAppPath())
      legacyTray = spawn(process.execPath, args, { stdio: ['pipe', 'pipe', 'pipe'], env: { ...process.env, GDK_BACKEND: 'x11' } })
      legacyTray.stdin.on('error', () => {})
      legacyTray.on('error', (error) => { legacyTrayReady = false; console.error('Tray unavailable:', error.message) })
      legacyTray.on('exit', () => { legacyTrayReady = false })
      legacyTray.stderr.on('data', (data) => process.stderr.write(data))
      createInterface({ input: legacyTray.stdout }).on('line', async (line) => {
        try {
          const message = JSON.parse(line)
          if (message.action === 'ready') {
            legacyTrayReady = true; refreshTray()
            if (process.env.S3BROWSER_DEBUG === '1') console.info('Legacy GNOME tray helper ready')
          }
          else if (message.action === 'show') showWindow()
          else if (message.action === 'about') showAbout()
          else if (message.action === 'quit') app.quit()
          else if (message.action === 'closeToTray') {
            const enabled = message.value === true
            await backend.call('SetCloseToTray', [enabled]); closeToTray = enabled; refreshTray()
          }
        } catch (error) { console.error('Tray operation failed:', error.message); refreshTray() }
      })
    } else {
      try {
        const trayFile = process.platform === 'darwin' ? 'trayTemplate.png' : process.platform === 'win32' ? 'tray.ico' : 'tray.png'
        const sourceFile = process.platform === 'darwin' ? 'trayTemplate.png' : process.platform === 'win32' ? 's3browser.ico' : 's3browser-64.png'
        const image = nativeImage.createFromPath(app.isPackaged ? join(process.resourcesPath, trayFile) : join(__dirname, '..', 'packaging', 'icons', sourceFile))
        if (image.isEmpty()) throw new Error('Tray icon could not be loaded')
        if (process.platform === 'darwin') image.setTemplateImage(true)
        const trayImage = process.platform === 'linux' ? image.resize({ width: 24, height: 24 }) : image
        tray = new Tray(trayImage)
        trayActivity = createTrayActivity(tray, trayImage, { template: process.platform === 'darwin' })
        tray.on('click', showWindow); refreshTray()
      } catch (error) { console.error('Tray unavailable:', error.message) }
    }
    app.on('activate', showWindow)
  }).catch((error) => { dialog.showErrorBox('S3 Browser could not start', error.message); app.quit() })
}
app.on('before-quit', () => { quitting = true; clearTimeout(statusTimer); clearTimeout(settleTimer); trayActivity?.stop(); backend?.stop(); legacyTray?.stdin.end() })
app.on('window-all-closed', () => app.quit())

}
