import { spawnSync } from 'node:child_process'
import { mkdirSync } from 'node:fs'
import { join } from 'node:path'
const platform = process.env.BACKEND_PLATFORM || process.platform
const arch = process.env.BACKEND_ARCH || process.arch
const goOS = { linux: 'linux', win32: 'windows', darwin: 'darwin' }[platform]
const goArch = { x64: 'amd64', arm64: 'arm64' }[arch]
if (!goOS || !goArch) throw new Error(`Unsupported backend target: ${platform}/${arch}`)
const directory = join(process.cwd(), 'build', 'backend', `${{linux:'linux',win32:'win',darwin:'mac'}[platform]}-${arch}`)
mkdirSync(directory, { recursive: true })
const file = join(directory, `s3browser-backend${platform === 'win32' ? '.exe' : ''}`)
const result = spawnSync('go', ['build', '-trimpath', '-ldflags=-s -w', '-o', file, '.'], {
  cwd: 'backend', stdio: 'inherit', env: { ...process.env, GOOS: goOS, GOARCH: goArch, CGO_ENABLED: '0' }
})
process.exit(result.status ?? 1)
