import { spawn } from 'node:child_process'
import fs from 'node:fs'
import net from 'node:net'
import path from 'node:path'
import { loadEnv } from './env.mjs'

const PID_FILE = '.playwright-pids.txt'

function canConnect(port: number) {
  return new Promise<boolean>((resolve) => {
    const s = net.connect(port, '127.0.0.1')
    s.once('connect', () => {
      s.destroy()
      resolve(true)
    })
    s.once('error', () => resolve(false))
  })
}

async function waitPort(port: number, ms: number) {
  const t0 = Date.now()
  while (Date.now() - t0 < ms) {
    if (await canConnect(port)) return
    await new Promise((r) => setTimeout(r, 500))
  }
  throw new Error(`port ${port} did not come up`)
}

export default async function globalSetup() {
  const env = loadEnv()
  const pids: number[] = []

  // 1. fixture files
  await import('./fixtures/generate.mjs')

  // 2. local source file server (detached — survives Playwright's job cleanup)
  if (!(await canConnect(env.fixturePort))) {
    const child = spawn(process.execPath, [path.join(env.root, 'web', 'e2e-integration', 'fixture-server.mjs')], {
      env: { ...process.env, FIXTURE_PORT: String(env.fixturePort) },
      stdio: 'ignore',
      detached: true,
    })
    child.unref()
    pids.push(child.pid!)
    await waitPort(env.fixturePort, 15_000)
  }

  // 3. StreamDrop app (reuse an already running instance if present)
  const appPort = Number(new URL(env.base).port || 8085)
  if (!(await canConnect(appPort))) {
    const exe = path.join(env.root, 'streamdrop.exe')
    if (!fs.existsSync(exe)) {
      throw new Error('streamdrop.exe not found — build it first (make build-windows)')
    }
    const child = spawn(exe, ['--no-tray', '--no-browser'], {
      cwd: env.root,
      env: { ...process.env, STREAMDROP_CONSOLE_LOG: '1' },
      stdio: 'ignore',
      detached: true,
    })
    child.unref()
    pids.push(child.pid!)
    await waitPort(appPort, 40_000)
  }

  if (pids.length) fs.writeFileSync(path.join(env.root, PID_FILE), pids.join('\n'))
  console.log(`[setup] app=${env.base} fixtures=127.0.0.1:${env.fixturePort} cloud=${env.ok ? 'enabled' : 'DISABLED (no .env creds)'}`)
}
