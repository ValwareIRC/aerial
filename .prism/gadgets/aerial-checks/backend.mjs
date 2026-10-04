// Runs the project's own checks and reports them in a form you can read at a
// glance. Nothing here is clever: it shells out to exactly the commands in the
// README, and parses `go test -json` so each package and test can be shown
// separately instead of as a wall of output.

import { existsSync } from 'node:fs'
import { join } from 'node:path'

// Go and Node were extracted portably on this machine and are not on PATH
// (QUESTIONS.md item 4), so look in the known places before giving up.
const GO_CANDIDATES = [
  'go',
  'C:\\Users\\Valerie\\tools\\go\\bin\\go.exe',
  'C:\\Program Files\\Go\\bin\\go.exe',
]
const NPM_CANDIDATES = ['npm', 'C:\\Program Files\\nodejs\\npm.cmd']

let tools = { go: null, npm: null }
let results = {}
let running = null

const CHECKS = [
  {
    id: 'gofmt',
    name: 'Formatting',
    group: 'Backend',
    what: 'gofmt -l .',
    why: 'Every Go file is formatted. CI fails on any that is not.',
  },
  {
    id: 'vet',
    name: 'Vet',
    group: 'Backend',
    what: 'go vet ./...',
    why: 'Suspect constructs the compiler allows but nobody means.',
  },
  {
    id: 'test',
    name: 'Tests',
    group: 'Backend',
    what: 'go test ./...',
    why: 'The domain rules, the enforcement wall, the money arithmetic.',
  },
  {
    id: 'race',
    name: 'Race detector',
    group: 'Backend',
    what: 'go test -race ./...',
    why: 'The app takes one lock per operation. This is what proves it.',
  },
  {
    id: 'types',
    name: 'Types',
    group: 'Front end',
    what: 'npm run typecheck',
    why: 'Strict TypeScript, including unchecked index access.',
  },
  {
    id: 'build',
    name: 'Build',
    group: 'Front end',
    what: 'npm run build',
    why: 'The bundle the console actually ships.',
  },
  {
    id: 'licences',
    name: 'Dependency licences',
    group: 'Licence',
    what: 'npm run licences',
    why: 'LICENCE.md restricts a field of endeavour, so no copyleft dependency may enter the tree.',
  },
]

function blank(check) {
  return {
    id: check.id,
    state: 'idle',
    summary: '',
    detail: '',
    output: '',
    ms: 0,
    packages: [],
    failures: [],
  }
}

async function resolveTool(api, candidates, probe) {
  for (const candidate of candidates) {
    if (candidate.includes('\\') && !existsSync(candidate)) continue
    const r = await api.shell(`"${candidate}" ${probe}`, { timeoutMs: 30000 })
    if (r.code === 0) return candidate
  }
  return null
}

function tail(text, lines = 60) {
  const all = String(text || '').replace(/\r/g, '').split('\n')
  return all.slice(-lines).join('\n').trim()
}

export default {
  async start(api) {
    for (const c of CHECKS) results[c.id] = blank(c)
    results = { ...results, ...((await api.state.get('results', null)) || {}) }

    tools.go = await resolveTool(api, GO_CANDIDATES, 'version')
    tools.npm = await resolveTool(api, NPM_CANDIDATES, '--version')
    api.log('go:', tools.go, 'npm:', tools.npm)
    api.emit('tools', tools)
  },

  handlers: {
    async state(_args, api) {
      return {
        checks: CHECKS,
        results,
        tools,
        running,
        root: api.root,
      }
    },

    async run({ id } = {}, api) {
      const wanted = id && id !== 'all' ? CHECKS.filter((c) => c.id === id) : CHECKS
      if (wanted.length === 0) return { error: `no check called ${id}` }
      for (const check of wanted) await runCheck(check, api)
      await api.state.set('results', results)
      return summarise()
    },

    async clear(_args, api) {
      for (const c of CHECKS) results[c.id] = blank(c)
      await api.state.set('results', results)
      api.emit('result', null)
      return summarise()
    },
  },

  tools: [
    {
      name: 'run',
      description:
        'Run the Aerial project checks (formatting, vet, Go tests, race detector, front-end types and build, dependency licence audit) and return a summary of what passed.',
      parameters: {
        type: 'object',
        properties: {
          check: {
            type: 'string',
            description:
              'Which check to run: gofmt, vet, test, race, types, build, licences, or all. Defaults to all.',
          },
        },
      },
      async execute({ check }, api) {
        const wanted = check && check !== 'all' ? CHECKS.filter((c) => c.id === check) : CHECKS
        if (wanted.length === 0) return { content: `No check called ${check}.`, isError: true }
        for (const c of wanted) await runCheck(c, api)
        await api.state.set('results', results)
        const s = summarise()
        const lines = wanted.map((c) => {
          const r = results[c.id]
          return `${r.state.toUpperCase()}  ${c.name} — ${r.summary}${r.detail ? ` (${r.detail})` : ''}`
        })
        return {
          content: `${s.passed} passed, ${s.failed} failed, ${s.skipped} skipped.\n\n${lines.join('\n')}`,
          isError: s.failed > 0,
        }
      },
    },
    {
      name: 'summary',
      description: 'The result of the last Aerial check run, without running anything.',
      parameters: { type: 'object', properties: {} },
      async execute(_args) {
        const s = summarise()
        const lines = CHECKS.filter((c) => results[c.id].state !== 'idle').map(
          (c) => `${results[c.id].state.toUpperCase()}  ${c.name} — ${results[c.id].summary}`,
        )
        if (lines.length === 0) return 'Nothing has been run yet.'
        return `${s.passed} passed, ${s.failed} failed, ${s.skipped} skipped.\n\n${lines.join('\n')}`
      },
    },
  ],
}

function summarise() {
  const counts = { passed: 0, failed: 0, skipped: 0, idle: 0 }
  for (const c of CHECKS) {
    const state = results[c.id].state
    if (state === 'pass') counts.passed++
    else if (state === 'fail') counts.failed++
    else if (state === 'skip') counts.skipped++
    else counts.idle++
  }
  return counts
}

async function runCheck(check, api) {
  const started = Date.now()
  results[check.id] = { ...blank(check), state: 'running' }
  running = check.id
  api.emit('result', { id: check.id, result: results[check.id], running })

  let out
  try {
    out = await execute(check, api)
  } catch (e) {
    out = { state: 'fail', summary: 'Could not run', detail: String(e && e.message ? e.message : e) }
  }

  results[check.id] = {
    ...blank(check),
    ...out,
    id: check.id,
    ms: Date.now() - started,
  }
  running = null
  api.emit('result', { id: check.id, result: results[check.id], running })
  return results[check.id]
}

async function execute(check, api) {
  const backend = join(api.root, 'backend')
  const frontend = join(api.root, 'frontend')

  if (check.group !== 'Backend' && !tools.npm) {
    return { state: 'skip', summary: 'npm not found', detail: 'Install Node, or add it to PATH.' }
  }
  if (check.group === 'Backend' && !tools.go) {
    return {
      state: 'skip',
      summary: 'Go not found',
      detail: 'Looked on PATH and in C:\\Users\\Valerie\\tools\\go.',
    }
  }

  const go = `"${tools.go}"`
  const npm = `"${tools.npm}"`

  switch (check.id) {
    case 'gofmt': {
      const gofmt = tools.go.endsWith('go.exe')
        ? `"${tools.go.replace(/go\.exe$/, 'gofmt.exe')}"`
        : 'gofmt'
      const r = await api.shell(`${gofmt} -l .`, { cwd: backend, timeoutMs: 120000 })
      const files = r.stdout.split('\n').map((s) => s.trim()).filter(Boolean)
      if (files.length === 0) return { state: 'pass', summary: 'Every file is formatted' }
      return {
        state: 'fail',
        summary: `${files.length} ${files.length === 1 ? 'file needs' : 'files need'} formatting`,
        failures: files,
        output: files.join('\n'),
      }
    }

    case 'vet': {
      const r = await api.shell(`${go} vet ./...`, { cwd: backend, timeoutMs: 300000 })
      const output = tail(`${r.stdout}\n${r.stderr}`)
      if (r.code === 0) return { state: 'pass', summary: 'Nothing suspect', output }
      return { state: 'fail', summary: 'Vet has something to say', output }
    }

    case 'test':
    case 'race': {
      const flags = check.id === 'race' ? '-race ' : ''
      const r = await api.shell(`${go} test ${flags}-count=1 -json ./...`, {
        cwd: backend,
        timeoutMs: 900000,
      })
      const raw = `${r.stdout}\n${r.stderr}`

      if (/C compiler .* not found|gcc.*not found|requires cgo/i.test(raw)) {
        return {
          state: 'skip',
          summary: 'Needs a C compiler',
          detail:
            'The race detector needs cgo on Windows. CI runs it on Linux instead.',
          output: tail(raw, 12),
        }
      }

      const parsed = parseGoTest(r.stdout)
      const summary = `${parsed.tests} ${parsed.tests === 1 ? 'test' : 'tests'} in ${parsed.packages.filter((p) => p.tests > 0).length} packages`

      if (parsed.failures.length === 0 && r.code === 0) {
        // No output on success: the package list below says everything, and
        // raw -json is unreadable.
        return { state: 'pass', summary, packages: parsed.packages, output: '' }
      }
      return {
        state: 'fail',
        summary:
          parsed.failures.length > 0
            ? `${parsed.failures.length} failing`
            : 'The run did not finish cleanly',
        packages: parsed.packages,
        failures: parsed.failures.map((f) => `${f.package} · ${f.test}`),
        output: parsed.failures.map((f) => `${f.package} · ${f.test}\n${f.output}`).join('\n\n') || tail(raw),
      }
    }

    case 'types': {
      const r = await api.shell(`${npm} run typecheck`, { cwd: frontend, timeoutMs: 600000 })
      const raw = `${r.stdout}\n${r.stderr}`
      const errors = raw
        .split('\n')
        .map((l) => l.trim())
        .filter((l) => /error TS\d+/.test(l))
      if (r.code === 0 && errors.length === 0) return { state: 'pass', summary: 'No type errors' }
      return {
        state: 'fail',
        summary: `${errors.length || 'Some'} type ${errors.length === 1 ? 'error' : 'errors'}`,
        failures: errors,
        output: tail(raw),
      }
    }

    case 'build': {
      const r = await api.shell(`${npm} run build`, { cwd: frontend, timeoutMs: 900000 })
      const raw = `${r.stdout}\n${r.stderr}`
      if (r.code !== 0) return { state: 'fail', summary: 'The build failed', output: tail(raw) }
      const sizes = raw
        .replace(/\u001b\[[0-9;]*m/g, '')
        .split('\n')
        .map((l) => l.trim())
        .filter((l) => /^dist\//.test(l))
      const js = sizes.find((l) => l.includes('.js')) || ''
      const kb = (js.match(/([\d.]+)\s*kB/) || [])[1]
      return {
        state: 'pass',
        summary: kb ? `Bundle ${kb} kB` : 'Bundle built',
        output: sizes.join('\n') || tail(raw, 20),
      }
    }

    case 'licences': {
      const r = await api.shell(`${npm} run licences`, { cwd: frontend, timeoutMs: 300000 })
      const raw = `${r.stdout}\n${r.stderr}`.replace(/\u001b\[[0-9;]*m/g, '')
      const audited = (raw.match(/Audited (\d+)/) || [])[1]
      const breakdown = raw
        .split('\n')
        .map((l) => l.trimEnd())
        .filter((l) => /^\s+\d+\s+\S/.test(l))
      if (r.code === 0) {
        return {
          state: 'pass',
          summary: audited ? `${audited} packages, all compatible` : 'All compatible',
          detail: 'No copyleft, share-alike or non-commercial terms.',
          output: breakdown.join('\n') || tail(raw),
        }
      }
      return {
        state: 'fail',
        summary: 'A dependency licence is a problem',
        detail: 'This would make the restriction in LICENCE.md unenforceable as written.',
        output: tail(raw, 30),
      }
    }
  }
  return { state: 'skip', summary: 'Not implemented' }
}

// go test -json emits one JSON object per line. Collapse it into per-package
// counts and the output of anything that failed.
function parseGoTest(stdout) {
  const packages = new Map()
  const failures = []
  const output = new Map()
  let tests = 0

  for (const line of String(stdout || '').split('\n')) {
    const trimmed = line.trim()
    if (!trimmed.startsWith('{')) continue
    let ev
    try {
      ev = JSON.parse(trimmed)
    } catch {
      continue
    }
    const pkg = (ev.Package || '').split('/').slice(-2).join('/')
    if (!pkg) continue
    if (!packages.has(pkg)) packages.set(pkg, { name: pkg, tests: 0, failed: 0, state: 'pass', ms: 0 })
    const entry = packages.get(pkg)

    if (ev.Test) {
      const key = `${pkg}::${ev.Test}`
      if (ev.Action === 'output') {
        output.set(key, (output.get(key) || '') + ev.Output)
      } else if (ev.Action === 'pass') {
        if (!ev.Test.includes('/')) {
          entry.tests++
          tests++
        }
      } else if (ev.Action === 'fail') {
        entry.failed++
        entry.state = 'fail'
        if (!ev.Test.includes('/')) {
          entry.tests++
          tests++
        }
        failures.push({ package: pkg, test: ev.Test, output: tail(output.get(key) || '', 14) })
      }
      continue
    }

    if (ev.Action === 'fail') entry.state = 'fail'
    if (ev.Action === 'skip' && entry.tests === 0) entry.state = 'none'
    if (typeof ev.Elapsed === 'number' && ev.Action !== 'output') {
      entry.ms = Math.round(ev.Elapsed * 1000)
    }
  }

  return {
    tests,
    failures,
    packages: [...packages.values()].sort((a, b) => a.name.localeCompare(b.name)),
  }
}
