// Builds the server, runs it on a spare port with a fixed salt, and asks it the
// questions in probes.mjs. The instance is disposable: some probes change data
// on purpose (moving somebody to compare their tokens), so it is rebuilt from
// the seed each run.

import { existsSync } from 'node:fs'
import { join } from 'node:path'
import { PROBES, SECTIONS } from './probes.mjs'

const GO_CANDIDATES = [
  'go',
  'C:\\Users\\Valerie\\tools\\go\\bin\\go.exe',
  'C:\\Program Files\\Go\\bin\\go.exe',
]
const PASSWORD = 'guardrail-probe-password'
const SALT = 'guardrails-fixed-salt'

let go = null
let server = null
let port = 0
let results = {}
let status = { state: 'stopped', detail: '' }

function setStatus(api, state, detail = '') {
  status = { state, detail }
  api.emit('status', status)
}

async function resolveGo(api) {
  for (const candidate of GO_CANDIDATES) {
    if (candidate.includes('\\') && !existsSync(candidate)) continue
    const r = await api.shell(`"${candidate}" version`, { timeoutMs: 30000 })
    if (r.code === 0) return candidate
  }
  return null
}

async function buildServer(api) {
  const out = join(api.dataDir, process.platform === 'win32' ? 'aerald-probe.exe' : 'aerald-probe')
  setStatus(api, 'building')
  const r = await api.shell(`"${go}" build -o "${out}" ./cmd/aerald`, {
    cwd: join(api.root, 'backend'),
    timeoutMs: 600000,
  })
  if (r.code !== 0) throw new Error(`build failed: ${(r.stderr || r.stdout).trim().slice(0, 400)}`)
  return out
}

async function waitForHealth(url, attempts = 60) {
  for (let i = 0; i < attempts; i++) {
    try {
      const r = await fetch(`${url}/api/health`)
      if (r.ok) return true
    } catch {
      // not up yet
    }
    await new Promise((r) => setTimeout(r, 250))
  }
  return false
}

async function startServer(api) {
  await stopServer(api)
  const binary = await buildServer(api)
  port = 8600 + Math.floor(Math.random() * 300)
  const url = `http://127.0.0.1:${port}`

  setStatus(api, 'starting', `port ${port}`)
  server = api.spawn(binary, ['-addr', `127.0.0.1:${port}`], {
    cwd: join(api.root, 'backend'),
    env: { AERIAL_SALT: SALT, AERIAL_DEV_PASSWORD: PASSWORD },
    onData: (chunk) => api.log(String(chunk).trim().slice(0, 200)),
    onExit: (code) => {
      api.log('server exited', code)
      server = null
    },
  })

  if (!(await waitForHealth(url))) {
    await stopServer(api)
    throw new Error('the server did not answer its health check')
  }
  setStatus(api, 'running', `port ${port}`)
  return url
}

async function stopServer(api) {
  if (server) {
    try {
      server.kill()
    } catch {
      // already gone
    }
    server = null
  }
  setStatus(api, 'stopped')
}

// A tiny client: keeps one cookie, parses JSON, and never throws for a status.
function clientFor(url) {
  let cookie = ''
  const c = {
    password: PASSWORD,
    async request(method, path, body, opts = {}) {
      const headers = { 'Content-Type': 'application/json', ...(opts.headers || {}) }
      if (cookie && !opts.noCookie) headers.Cookie = cookie
      const res = await fetch(url + path, {
        method,
        headers,
        body: body === undefined ? undefined : JSON.stringify(body),
      })
      const set = res.headers.getSetCookie ? res.headers.getSetCookie() : []
      for (const line of set) {
        if (line.startsWith('aerial_session=')) cookie = line.split(';')[0]
      }
      const text = await res.text()
      let parsed = null
      try {
        parsed = text ? JSON.parse(text) : null
      } catch {
        parsed = text
      }
      return { status: res.status, body: parsed, headers: res.headers }
    },
    get(path, opts) {
      return c.request('GET', path, undefined, opts)
    },
    post(path, body, opts) {
      return c.request('POST', path, body ?? {}, opts)
    },
    async signIn(username) {
      cookie = ''
      const r = await c.request('POST', '/api/session', { username, password: PASSWORD })
      if (r.status !== 200) throw new Error(`could not sign in as ${username}: ${r.status}`)
      return r.body
    },
  }
  return c
}

async function runProbes(api) {
  if (!go) go = await resolveGo(api)
  if (!go) {
    setStatus(api, 'stopped', 'Go was not found, so the server cannot be built.')
    return { error: 'Go was not found.' }
  }

  results = {}
  api.emit('result', null)

  let url
  try {
    url = await startServer(api)
  } catch (e) {
    setStatus(api, 'stopped', String(e.message || e))
    return { error: String(e.message || e) }
  }

  const client = clientFor(url)
  for (const probe of PROBES) {
    results[probe.id] = { id: probe.id, state: 'running' }
    api.emit('result', { id: probe.id, result: results[probe.id] })
    const started = Date.now()
    let out
    try {
      out = await probe.run(clientFor(url))
    } catch (e) {
      out = { ok: false, found: String(e.message || e) }
    }
    results[probe.id] = {
      id: probe.id,
      state: out.ok ? 'pass' : 'fail',
      found: out.found || '',
      ms: Date.now() - started,
    }
    api.emit('result', { id: probe.id, result: results[probe.id] })
  }

  // The instance has been poked about in; do not leave it running.
  await stopServer(api)
  await api.state.set('results', results)
  return counts()
}

function counts() {
  let passed = 0
  let failed = 0
  for (const probe of PROBES) {
    const r = results[probe.id]
    if (!r) continue
    if (r.state === 'pass') passed++
    else if (r.state === 'fail') failed++
  }
  return { passed, failed, total: PROBES.length }
}

export default {
  async start(api) {
    results = (await api.state.get('results', {})) || {}
    go = await resolveGo(api)
    api.log('go:', go)
  },

  async stop(api) {
    await stopServer(api)
  },

  handlers: {
    async state(_args) {
      return {
        sections: SECTIONS,
        probes: PROBES.map(({ id, section, title, why }) => ({ id, section, title, why })),
        results,
        status,
        counts: counts(),
      }
    },
    async run(_args, api) {
      return runProbes(api)
    },
    async stopServer(_args, api) {
      await stopServer(api)
      return { ok: true }
    },
  },

  tools: [
    {
      name: 'check',
      description:
        'Build and run the Aerial server on a spare port, then probe the guarantees it is supposed to make: the enforcement wall, occupant tokens that do not join up, refusals in place of missing data, no-store on personal data, k-anonymity in reports, and the sign-in lockout. Returns what passed and what did not.',
      parameters: { type: 'object', properties: {} },
      async execute(_args, api) {
        const out = await runProbes(api)
        if (out.error) return { content: out.error, isError: true }
        const lines = PROBES.map((p) => {
          const r = results[p.id] || {}
          return `${r.state === 'pass' ? 'ok  ' : 'FAIL'}  ${p.title} — ${r.found || ''}`
        })
        return {
          content: `${out.passed}/${out.total} guardrails hold.\n\n${lines.join('\n')}`,
          isError: out.failed > 0,
        }
      },
    },
  ],
}
