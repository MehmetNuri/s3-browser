const { app, Tray, nativeImage, nativeTheme } = require('electron')
const { join } = require('node:path')
const { createInterface } = require('node:readline')
const { createTrayMenu, activityLabel } = require('./tray-menu.cjs')
const { createTrayActivity } = require('./tray-activity.cjs')

exports.start = () => {
  let tray, activity
  let language = 'en'
  let closeToTray = true
  let status = { state: 'idle' }
  let menuKey
  const send = (action, value) => process.stdout.write(JSON.stringify({ action, value }) + '\n')
  const refresh = (force = false) => {
    if (!tray) return
    const label = activityLabel(language, status)
    tray.setToolTip(label ? `S3 Browser — ${label}` : 'S3 Browser')
    activity.update(status)
    // Rebuilding an open menu closes it, so only do that when its content changes.
    const menuLabel = activityLabel(language, status, false)
    const key = JSON.stringify([language, closeToTray, menuLabel, nativeTheme.shouldUseDarkColors])
    if (!force && key === menuKey) return
    menuKey = key
    tray.setContextMenu(createTrayMenu({ language, closeToTray, status: menuLabel,
      show: () => send('show'), about: () => send('about'),
      setCloseToTray: (value) => send('closeToTray', value), quit: () => send('quit')
    }))
  }
  nativeTheme.on('updated', () => refresh(true))
  app.disableHardwareAcceleration()
  app.whenReady().then(() => {
    const file = app.isPackaged ? join(process.resourcesPath, 'tray.png') : join(__dirname, '..', 'packaging', 'icons', 's3browser-64.png')
    const image = nativeImage.createFromPath(file).resize({ width: 24, height: 24 })
    if (image.isEmpty()) throw new Error('Tray icon could not be loaded')
    tray = new Tray(image)
    activity = createTrayActivity(tray, image)
    tray.on('click', () => send('show'))
    refresh()
    send('ready')
  }).catch((error) => { console.error(error.message); app.exit(1) })
  createInterface({ input: process.stdin }).on('line', (line) => {
    try {
      const message = JSON.parse(line)
      language = message.language === 'tr' ? 'tr' : 'en'
      closeToTray = Boolean(message.closeToTray)
      if (message.status && typeof message.status === 'object') status = message.status
      refresh()
    } catch (error) { console.error('Invalid tray message:', error.message) }
  }).on('close', () => app.quit())
}
