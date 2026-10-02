// Writes packaging/flathub/go-sources.json: every Go module the backend needs, as
// flatpak-builder file sources laid out as a GOPROXY directory for the offline build.
// The proxy does not serve byte-stable .info files, and pinned versions build without them.
import { spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { readFileSync, writeFileSync } from 'node:fs'
import { basename, join } from 'node:path'
const result = spawnSync('go', ['mod', 'download', '-json', 'all'], {
  cwd: 'backend', encoding: 'utf8', maxBuffer: 64 * 1024 * 1024, stdio: ['ignore', 'pipe', 'inherit']
})
if (result.status !== 0) process.exit(result.status ?? 1)
const modules = JSON.parse(`[${result.stdout.replace(/\}\s*\{/g, '},{')}]`)
const marker = '/cache/download/'
const sources = []
for (const module of modules) {
  for (const file of [module.GoMod, module.Zip]) {
    if (!file) throw new Error(`Incomplete download for ${module.Path}@${module.Version}`)
    // The cache path already carries the proxy's case-escaped module path.
    const relative = file.slice(file.indexOf(marker) + marker.length)
    sources.push({
      type: 'file',
      url: `https://proxy.golang.org/${relative}`,
      sha256: createHash('sha256').update(readFileSync(file)).digest('hex'),
      dest: join('goproxy', relative, '..'),
      'dest-filename': basename(relative)
    })
  }
}
writeFileSync(join('packaging', 'flathub', 'go-sources.json'), `${JSON.stringify(sources, null, 2)}\n`)
console.log(`${modules.length} modules, ${sources.length} files`)
