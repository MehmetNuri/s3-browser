const { spawn } = require('node:child_process')
const { createInterface } = require('node:readline')

class Backend {
  constructor(executable, hostRequest, emitEvent, onStop, env = process.env) {
    this.pending = new Map()
    this.nextID = 0
    this.stopped = false
    this.child = spawn(executable, [], { stdio: ['pipe', 'pipe', 'pipe'], windowsHide: true, env })
    this.ready = new Promise((resolve, reject) => {
      const stop = (error) => {
        if (this.stopped) return
        this.stopped = true
        reject(error)
        for (const request of this.pending.values()) request.reject(error)
        this.pending.clear()
        onStop?.(error)
      }
      this.child.once('error', stop)
      this.child.once('exit', () => stop(new Error('The storage backend stopped')))
      createInterface({ input: this.child.stdout }).on('line', async (line) => {
        try {
          if (line.length > 8 * 1024 * 1024) throw new Error('Backend message is too large')
          const message = JSON.parse(line)
          if (message.type === 'ready') resolve()
          else if (message.type === 'response') {
            const request = this.pending.get(message.id)
            if (!request) return
            this.pending.delete(message.id)
            if (message.error) request.reject(new Error(message.error))
            else request.resolve(message.result ?? null)
          } else if (message.type === 'event') emitEvent(message.event, message.data)
          else if (message.type === 'host-request') {
            try {
              const result = await hostRequest(message.method, message.args?.[0])
              this.send({ type: 'host-response', id: message.id, result: result ?? null })
            } catch (error) { this.send({ type: 'host-response', id: message.id, error: error.message }) }
          }
        } catch (error) { console.error('Desktop protocol failed:', error.message); this.child.kill() }
      })
    })
    this.child.stderr.on('data', (data) => process.stderr.write(data))
    this.child.stdin.on('error', () => {})
  }
  send(message) { if (!this.stopped && this.child.stdin.writable) this.child.stdin.write(JSON.stringify(message) + '\n') }
  async call(method, args = []) {
    await this.ready
    // A request sent to a dead process would never be answered.
    if (this.stopped) throw new Error('The storage backend stopped')
    const id = ++this.nextID
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject })
      this.send({ type: 'request', id, method, args })
    })
  }
  stop() { this.child.stdin.end(); setTimeout(() => this.child.kill(), 2000).unref() }
}
module.exports = { Backend }
