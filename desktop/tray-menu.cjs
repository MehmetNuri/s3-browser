const { app, Menu, nativeImage, nativeTheme } = require('electron')
const { join } = require('node:path')

const activityWords = {
  en: { upload: 'Uploading', download: 'Downloading', copy: 'Copying', move: 'Moving', sync: 'Syncing', preparing: 'preparing…', done: 'Transfers finished', failed: (n) => `${n} ${n === 1 ? 'transfer' : 'transfers'} failed` },
  tr: { upload: 'Yükleniyor', download: 'İndiriliyor', copy: 'Kopyalanıyor', move: 'Taşınıyor', sync: 'Eşitleniyor', preparing: 'hazırlanıyor…', done: 'Aktarımlar tamamlandı', failed: (n) => `${n} aktarım başarısız` }
}

// Describes the current transfers, for example "Uploading 12/340, Downloading 1/3 · 42%".
exports.activityLabel = (language, status, withPercent = true) => {
  const words = activityWords[language === 'tr' ? 'tr' : 'en']
  if (!status || status.state === 'idle') return ''
  if (status.state === 'done') return words.done
  if (status.state === 'failed') return words.failed(status.failed)
  const parts = status.parts.map(({ kind, done, total }) => `${words[kind] ?? words.copy} ${total ? `${done}/${total}` : words.preparing}`)
  const percent = withPercent && status.fraction !== null ? ` · ${language === 'tr' ? '%' : ''}${Math.floor(status.fraction * 100)}${language === 'tr' ? '' : '%'}` : ''
  return parts.join(', ') + percent
}

exports.createTrayMenu = ({ language, closeToTray, status, show, about, setCloseToTray, quit }) => {
  const labels = language === 'tr'
    ? { show: 'Uygulamayı aç', about: 'Hakkında', background: 'Arka planda çalış', quit: 'Uygulamadan çık' }
    : { show: 'Open S3 Browser', about: 'About', background: 'Keep running in background', quit: 'Quit S3 Browser' }
  const theme = nativeTheme.shouldUseDarkColors ? 'dark' : 'light'
  const directory = app.isPackaged ? join(process.resourcesPath, 'tray-menu') : join(__dirname, '..', 'packaging', 'icons', 'tray-menu')
  const icon = (name) => nativeImage.createFromPath(join(directory, `${name}-${theme}.png`))
  return Menu.buildFromTemplate([
    { label: status || 'S3 Browser', enabled: false },
    { type: 'separator' },
    { label: labels.show, icon: icon('window'), click: show },
    { label: labels.about, icon: icon('about'), click: about },
    { type: 'separator' },
    { label: labels.background, type: 'checkbox', checked: closeToTray, click: (item) => setCloseToTray(item.checked) },
    { type: 'separator' },
    { label: labels.quit, icon: icon('quit'), click: quit }
  ])
}
