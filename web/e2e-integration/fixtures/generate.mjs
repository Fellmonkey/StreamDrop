import { closeSync, existsSync, mkdirSync, openSync, writeFileSync, writeSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const dir = path.dirname(fileURLToPath(import.meta.url))
const isoPath = path.join(dir, 'ubuntu-24.04-live-server.iso')
const htmlPath = path.join(dir, 'download-page.html')
const zipPath = path.join(dir, 'notes.zip')
const jsonPath = path.join(dir, 'data.json')

mkdirSync(dir, { recursive: true })

// 48 MiB ISO-like blob: deterministic bytes, supports range reads.
if (!existsSync(isoPath)) {
  const size = 48 * 1024 * 1024
  const chunk = Buffer.alloc(256 * 1024)
  for (let i = 0; i < chunk.length; i++) chunk[i] = (i * 31 + 7) & 0xff
  const fd = openSync(isoPath, 'w')
  let written = 0
  while (written < size) {
    const n = Math.min(chunk.length, size - written)
    writeSync(fd, chunk, 0, n)
    written += n
  }
  closeSync(fd)
}

writeFileSync(
  htmlPath,
  '<!doctype html><html><head><title>Download page</title></head>' +
    '<body><h1>Download</h1><a href="/iso/ubuntu-24.04-live-server.iso">ISO</a></body></html>',
)

if (!existsSync(zipPath)) {
  const b = Buffer.alloc(200 * 1024)
  for (let i = 0; i < b.length; i++) b[i] = (i * 13 + 5) & 0xff
  writeFileSync(zipPath, b)
}

writeFileSync(jsonPath, JSON.stringify({ artifact: true, generatedAt: new Date().toISOString() }))
