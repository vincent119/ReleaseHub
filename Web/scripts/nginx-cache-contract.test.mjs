import assert from 'node:assert/strict'
import { execFileSync, spawnSync } from 'node:child_process'
import { existsSync, readFileSync, readdirSync } from 'node:fs'
import { setTimeout as delay } from 'node:timers/promises'
import { fileURLToPath, URL } from 'node:url'
import path from 'node:path'
import test from 'node:test'

const webRoot = fileURLToPath(new URL('..', import.meta.url))
const nginxConfig = path.join(webRoot, 'nginx/nginx.conf')
const dist = path.join(webRoot, 'dist')
const image =
  process.env.RELEASEHUB_NGINX_TEST_IMAGE ??
  'nginxinc/nginx-unprivileged:1.29-alpine'

test('Nginx 設定區分 HTML、hashed asset 與 SPA route', () => {
  const config = readFileSync(nginxConfig, 'utf8')
  assert.match(
    config,
    /location = \/index\.html\s*\{[^}]*Cache-Control "no-store"/s,
  )
  assert.match(
    config,
    /location \^~ \/assets\/\s*\{[^}]*Cache-Control "public, max-age=31536000, immutable";[^}]*try_files \$uri =404;/s,
  )
  assert.match(
    config,
    /location \/\s*\{[^}]*try_files \$uri \$uri\/ \/index\.html;/s,
  )
})

const dockerAvailable =
  spawnSync('docker', ['info', '--format', '{{.ServerVersion}}'], {
    stdio: 'ignore',
  }).status === 0 &&
  spawnSync('docker', ['image', 'inspect', image], { stdio: 'ignore' })
    .status === 0

test(
  'Nginx 實際回應包含正確 cache header 且遺失 asset 為 404',
  { skip: !dockerAvailable || !existsSync(dist) },
  async () => {
    const container = execFileSync(
      'docker',
      [
        'run',
        '-d',
        '-p',
        '127.0.0.1::8080',
        '--add-host',
        'releasehub-api:127.0.0.1',
        '-v',
        `${nginxConfig}:/etc/nginx/nginx.conf:ro`,
        '-v',
        `${dist}:/usr/share/nginx/html:ro`,
        image,
      ],
      { encoding: 'utf8' },
    ).trim()

    try {
      const mapping = execFileSync('docker', ['port', container, '8080/tcp'], {
        encoding: 'utf8',
      }).trim()
      const port = mapping.match(/:(\d+)$/)?.[1]
      assert.ok(port, `無法判讀容器埠號：${mapping}`)
      const base = `http://127.0.0.1:${port}`
      let index
      for (let attempt = 0; attempt < 20; attempt += 1) {
        try {
          index = await globalThis.fetch(`${base}/index.html`)
          break
        } catch {
          await delay(100)
        }
      }
      assert.ok(index, 'Nginx 未啟動')
      assert.equal(index.status, 200)
      assert.equal(index.headers.get('cache-control'), 'no-store')

      const asset = readdirSync(path.join(dist, 'assets')).find((name) =>
        name.endsWith('.js'),
      )
      assert.ok(asset, 'build 未產生 JavaScript asset')
      const existing = await globalThis.fetch(`${base}/assets/${asset}`)
      assert.equal(existing.status, 200)
      assert.equal(
        existing.headers.get('cache-control'),
        'public, max-age=31536000, immutable',
      )

      const missing = await globalThis.fetch(`${base}/assets/missing-hash.js`)
      assert.equal(missing.status, 404)
      assert.doesNotMatch(await missing.text(), /<!doctype html>/i)

      const route = await globalThis.fetch(`${base}/requests/example`)
      assert.equal(route.status, 200)
      assert.equal(route.headers.get('cache-control'), 'no-store')
    } catch (error) {
      const logs = spawnSync('docker', ['logs', container], {
        encoding: 'utf8',
      })
      throw new Error(`${error}\n${logs.stdout ?? ''}\n${logs.stderr ?? ''}`, {
        cause: error,
      })
    } finally {
      spawnSync('docker', ['rm', '-f', container], { stdio: 'ignore' })
    }
  },
)
