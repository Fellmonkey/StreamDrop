import http from 'node:http'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const here = path.dirname(fileURLToPath(import.meta.url))
const FIXTURES = path.join(here, 'fixtures')
const port = Number(process.env.FIXTURE_PORT || 8792)

const TYPES = {
  '.iso': 'application/x-iso9660-image',
  '.html': 'text/html; charset=utf-8',
  '.zip': 'application/zip',
  '.json': 'application/json',
}

http
  .createServer((req, res) => {
    if (req.url === '/__ready__') {
      res.writeHead(200)
      return res.end('ok')
    }
    const u = new URL(req.url ?? '/', 'http://localhost')

    if (u.pathname.startsWith('/redirect/')) {
      const target = '/' + u.pathname.slice('/redirect/'.length)
      res.writeHead(302, { Location: target })
      return res.end()
    }

    const rel0 = decodeURIComponent(u.pathname).replace(/^\/+/, '')
    // Strip the namespace segment: /iso/…, /page/…, /assets/… map to fixture
    // files in the fixtures/ directory.
    const ns = rel0.split('/')[0]
    const rel = ['iso', 'page', 'assets'].includes(ns) ? rel0.slice(ns.length + 1) : rel0
    const file = path.join(FIXTURES, rel)
    if (!fs.existsSync(file) || !fs.statSync(file).isFile()) {
      res.writeHead(404)
      return res.end('not found')
    }

    const stat = fs.statSync(file)
    const type = TYPES[path.extname(file).toLowerCase()] || 'application/octet-stream'
    const range = req.headers.range

    if (range) {
      const m = /bytes=(\d*)-(\d*)/.exec(range)
      if (m) {
        const start = m[1] ? parseInt(m[1], 10) : 0
        const end = m[2] ? parseInt(m[2], 10) : stat.size - 1
        if (start >= stat.size || start > end) {
          res.writeHead(416, { 'Content-Range': `bytes */${stat.size}` })
          return res.end()
        }
        res.writeHead(206, {
          'Content-Type': type,
          'Content-Length': end - start + 1,
          'Content-Range': `bytes ${start}-${end}/${stat.size}`,
          'Accept-Ranges': 'bytes',
        })
        return fs.createReadStream(file, { start, end }).pipe(res)
      }
    }

    res.writeHead(200, {
      'Content-Type': type,
      'Content-Length': stat.size,
      'Accept-Ranges': 'bytes',
    })
    fs.createReadStream(file).pipe(res)
  })
  .listen(port, '127.0.0.1', () => console.log(`[fixture-server] http://127.0.0.1:${port}`))
