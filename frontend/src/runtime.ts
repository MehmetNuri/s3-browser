interface DesktopBridge {
  call(method: string, args?: unknown[]): Promise<unknown>
  onEvent(name: string, callback: (data: unknown) => void): () => void
  uploadDropped(bucket: string, prefix: string, files: File[]): Promise<number>
}
declare global { interface Window { desktop: DesktopBridge } }
// Electron prefixes rejected IPC calls with its own wording; users only need the reason.
function plainError(error: unknown): Error {
  const text = error instanceof Error ? error.message : String(error)
  return new Error(text.replace(/^Error invoking remote method '[^']*': /, '').replace(/^Error: /, ''))
}
const bridge = window.desktop
export const desktop: DesktopBridge = {
  call: (method, args) => bridge.call(method, args).catch((error) => { throw plainError(error) }),
  onEvent: (name, callback) => bridge.onEvent(name, callback),
  uploadDropped: (bucket, prefix, files) => bridge.uploadDropped(bucket, prefix, files).catch((error) => { throw plainError(error) }),
}
/** The message of a failed call, without the "Error:" label. */
export function errorText(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}
export function EventsOn<T>(name: string, callback: (data: T) => void) { return desktop.onEvent(name, (data) => callback(data as T)) }
let removeDrop: (() => void) | undefined
export function OnFileDrop(callback: (files: File[]) => void) {
  OnFileDropOff()
  const dragover = (event: DragEvent) => event.preventDefault()
  const drop = (event: DragEvent) => {
    event.preventDefault()
    const files = Array.from(event.dataTransfer?.files || [])
    if (files.length) callback(files)
  }
  window.addEventListener('dragover', dragover)
  window.addEventListener('drop', drop)
  removeDrop = () => { window.removeEventListener('dragover', dragover); window.removeEventListener('drop', drop) }
}
export function OnFileDropOff() { removeDrop?.(); removeDrop = undefined }
