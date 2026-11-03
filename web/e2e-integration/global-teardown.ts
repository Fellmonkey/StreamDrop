import fs from 'node:fs'
import path from 'node:path'
import { loadEnv } from './env.mjs'

export default async function globalTeardown() {
  const env = loadEnv()
  const pidFile = path.join(env.root, '.playwright-pids.txt')
  if (!fs.existsSync(pidFile)) return
  const pids = fs.readFileSync(pidFile, 'utf8').split('\n').filter(Boolean).map(Number)
  for (const pid of pids) {
    try {
      process.kill(pid)
    } catch {
      /* already gone */
    }
  }
  fs.rmSync(pidFile, { force: true })
}
