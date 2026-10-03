const { contextBridge, ipcRenderer, webUtils } = require('electron')
const allowedEvents = new Set(['transfer', 'queue', 'edit', 'mount', 'cap', 'show-about', 'backup'])
contextBridge.exposeInMainWorld('desktop', {
  call: (method, args = []) => ipcRenderer.invoke('backend:call', method, args),
  onEvent: (name, callback) => {
    if (!allowedEvents.has(name)) throw new Error('Unknown event')
    const listener = (_event, eventName, data) => { if (name === eventName) callback(data) }
    ipcRenderer.on('backend:event', listener)
    return () => ipcRenderer.removeListener('backend:event', listener)
  },
  // Only files handed over by the user (drag and drop) resolve to a path here.
  uploadDropped: async (bucket, prefix, files) => {
    const paths = Array.from(files, (file) => webUtils.getPathForFile(file)).filter(Boolean)
    return paths.length ? ipcRenderer.invoke('backend:upload-dropped', bucket, prefix, paths) : 0
  }
})
