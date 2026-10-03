const { app, Menu, nativeImage, nativeTheme } = require('electron')
const { join } = require('node:path')

const activityWords = {
  en: { upload: 'Uploading', download: 'Downloading', copy: 'Copying', move: 'Moving', sync: 'Syncing', preparing: 'preparing…', done: 'Transfers finished', failed: (n) => `${n} ${n === 1 ? 'transfer' : 'transfers'} failed` },
  tr: { upload: 'Yükleniyor', download: 'İndiriliyor', copy: 'Kopyalanıyor', move: 'Taşınıyor', sync: 'Eşitleniyor', preparing: 'hazırlanıyor…', done: 'Aktarımlar tamamlandı', failed: (n) => `${n} aktarım başarısız` },
  de: { upload: 'Hochladen', download: 'Herunterladen', copy: 'Kopieren', move: 'Verschieben', sync: 'Synchronisieren', preparing: 'wird vorbereitet…', done: 'Übertragungen abgeschlossen', failed: (n) => `${n} ${n === 1 ? 'Übertragung' : 'Übertragungen'} fehlgeschlagen` },
  fr: { upload: 'Envoi', download: 'Téléchargement', copy: 'Copie', move: 'Déplacement', sync: 'Synchronisation', preparing: 'préparation…', done: 'Transferts terminés', failed: (n) => `${n} transfert${n === 1 ? '' : 's'} en échec` },
  es: { upload: 'Subiendo', download: 'Descargando', copy: 'Copiando', move: 'Moviendo', sync: 'Sincronizando', preparing: 'preparando…', done: 'Transferencias finalizadas', failed: (n) => `${n} transferencia${n === 1 ? '' : 's'} con error` },
  pt: { upload: 'Enviando', download: 'Baixando', copy: 'Copiando', move: 'Movendo', sync: 'Sincronizando', preparing: 'preparando…', done: 'Transferências concluídas', failed: (n) => `${n} transferência${n === 1 ? '' : 's'} com falha` },
  ru: { upload: 'Загрузка', download: 'Скачивание', copy: 'Копирование', move: 'Перемещение', sync: 'Синхронизация', preparing: 'подготовка…', done: 'Передачи завершены', failed: (n) => `Передач с ошибкой: ${n}` },
  ar: { upload: 'جارٍ الرفع', download: 'جارٍ التنزيل', copy: 'جارٍ النسخ', move: 'جارٍ النقل', sync: 'جارٍ المزامنة', preparing: 'جارٍ التحضير…', done: 'اكتملت عمليات النقل', failed: (n) => `فشل ${n} من عمليات النقل` },
  zh: { upload: '正在上传', download: '正在下载', copy: '正在复制', move: '正在移动', sync: '正在同步', preparing: '准备中…', done: '传输已完成', failed: (n) => `${n} 个传输失败` }
}
const menuLabels = {
  en: { show: 'Open S3 Browser', about: 'About', background: 'Keep running in background', quit: 'Quit S3 Browser' },
  tr: { show: 'Uygulamayı aç', about: 'Hakkında', background: 'Arka planda çalış', quit: 'Uygulamadan çık' },
  de: { show: 'S3 Browser öffnen', about: 'Über', background: 'Im Hintergrund weiterlaufen', quit: 'S3 Browser beenden' },
  fr: { show: 'Ouvrir S3 Browser', about: 'À propos', background: 'Continuer en arrière-plan', quit: 'Quitter S3 Browser' },
  es: { show: 'Abrir S3 Browser', about: 'Acerca de', background: 'Seguir en segundo plano', quit: 'Salir de S3 Browser' },
  pt: { show: 'Abrir o S3 Browser', about: 'Sobre', background: 'Continuar em segundo plano', quit: 'Sair do S3 Browser' },
  ru: { show: 'Открыть S3 Browser', about: 'О программе', background: 'Работать в фоне', quit: 'Выйти из S3 Browser' },
  ar: { show: 'فتح S3 Browser', about: 'حول', background: 'الاستمرار في الخلفية', quit: 'إنهاء S3 Browser' },
  zh: { show: '打开 S3 Browser', about: '关于', background: '在后台继续运行', quit: '退出 S3 Browser' }
}

// Describes the current transfers, for example "Uploading 12/340, Downloading 1/3 · 42%".
exports.activityLabel = (language, status, withPercent = true) => {
  const words = activityWords[language] ?? activityWords.en
  if (!status || status.state === 'idle') return ''
  if (status.state === 'done') return words.done
  if (status.state === 'failed') return words.failed(status.failed)
  const parts = status.parts.map(({ kind, done, total }) => `${words[kind] ?? words.copy} ${total ? `${done}/${total}` : words.preparing}`)
  const percent = withPercent && status.fraction !== null ? ` · ${language === 'tr' ? '%' : ''}${Math.floor(status.fraction * 100)}${language === 'tr' ? '' : '%'}` : '' // Turkish writes the sign first
  return parts.join(', ') + percent
}

exports.createTrayMenu = ({ language, closeToTray, status, show, about, setCloseToTray, quit }) => {
  const labels = menuLabels[language] ?? menuLabels.en
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
