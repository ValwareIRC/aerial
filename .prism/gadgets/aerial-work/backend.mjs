// Reads the ledger, and checks it against the real diff so it cannot quietly
// go out of date.

import { GROUPS, ITEMS } from './ledger.mjs'

const GIT_CANDIDATES = ['git', 'C:\\Program Files\\Git\\cmd\\git.exe']
let git = null

const AREAS = [
  { match: /^backend\/internal\/domain\//, name: 'Domain core', note: 'Premises, licences, declarations, fees' },
  { match: /^backend\/internal\/app\//, name: 'Application layer', note: 'Wiring, locking, operations' },
  { match: /^backend\/internal\/httpapi\//, name: 'HTTP API', note: 'Routes, guards, middleware' },
  { match: /^backend\/internal\/auth\//, name: 'Sign-in', note: 'Passwords, sessions, lockout' },
  { match: /^backend\/internal\/money\//, name: 'Money', note: 'Integer pence arithmetic' },
  { match: /^backend\/internal\/projection\//, name: 'Projections', note: 'The enforcement wall' },
  { match: /^backend\/internal\//, name: 'Other backend packages', note: '' },
  { match: /^backend\/cmd\//, name: 'Server command', note: 'Startup, timeouts, shutdown' },
  { match: /^frontend\/src\/pages\//, name: 'Console pages', note: '' },
  { match: /^frontend\/scripts\//, name: 'Licence audit', note: 'Required by LICENCE.md' },
  { match: /^frontend\/src\//, name: 'Console shell', note: 'Client, layout, styles' },
  { match: /^frontend\//, name: 'Front-end config', note: '' },
  { match: /^\.github\//, name: 'CI', note: '' },
  { match: /^\.prism\//, name: 'Gadgets', note: '' },
  { match: /\.md$/, name: 'Documentation', note: '' },
  { match: /^\.gitignore$/, name: 'Repository', note: 'What git is told to ignore' },
]

function areaOf(path) {
  const hit = AREAS.find((a) => a.match.test(path))
  return hit ? hit : { name: 'Elsewhere', note: '' }
}

async function resolveGit(api) {
  for (const candidate of GIT_CANDIDATES) {
    const r = await api.shell(`"${candidate}" --version`, { timeoutMs: 20000 })
    if (r.code === 0) return candidate
  }
  return null
}

async function readDiff(api) {
  if (!git) return { files: [], error: 'git was not found on this machine.' }
  const g = `"${git}"`
  const cwd = api.root

  const [status, unstaged, staged, head] = await Promise.all([
    api.shell(`${g} status --porcelain -uall`, { cwd, timeoutMs: 60000 }),
    api.shell(`${g} diff --numstat`, { cwd, timeoutMs: 60000 }),
    api.shell(`${g} diff --cached --numstat`, { cwd, timeoutMs: 60000 }),
    api.shell(`${g} log -1 "--format=%h  %s"`, { cwd, timeoutMs: 60000 }),
  ])

  const counts = new Map()
  for (const block of [unstaged.stdout, staged.stdout]) {
    for (const line of block.split('\n')) {
      const m = line.trim().match(/^(\d+|-)\t(\d+|-)\t(.+)$/)
      if (!m) continue
      const path = m[3].replace(/\\/g, '/')
      const prev = counts.get(path) || { added: 0, removed: 0 }
      counts.set(path, {
        added: prev.added + (m[1] === '-' ? 0 : Number(m[1])),
        removed: prev.removed + (m[2] === '-' ? 0 : Number(m[2])),
      })
    }
  }

  const files = []
  for (const line of status.stdout.split('\n')) {
    if (!line.trim()) continue
    const code = line.slice(0, 2).trim()
    const path = line.slice(3).trim().replace(/^"|"$/g, '').replace(/\\/g, '/')
    if (!path || path.endsWith('/')) continue

    let added = 0
    let removed = 0
    const known = counts.get(path)
    if (known) {
      added = known.added
      removed = known.removed
    } else if (code.includes('?')) {
      // Untracked: nothing to diff against, so count the lines.
      try {
        const text = await api.workspace.read(path)
        added = String(text).split('\n').length
      } catch {
        added = 0
      }
    }

    files.push({
      path,
      code,
      state: code.includes('?') ? 'new' : code === 'D' ? 'deleted' : 'changed',
      added,
      removed,
      area: areaOf(path).name,
      note: areaOf(path).note,
    })
  }

  files.sort((a, b) => a.area.localeCompare(b.area) || a.path.localeCompare(b.path))
  return { files, head: head.stdout.trim() }
}

function ledgerWith(diff) {
  const changed = new Set(diff.files.map((f) => f.path))
  const claimed = new Set()

  const items = ITEMS.map((item) => {
    const files = (item.files || []).map((path) => {
      claimed.add(path)
      const hit = diff.files.find((f) => f.path === path)
      return {
        path,
        added: hit ? hit.added : 0,
        removed: hit ? hit.removed : 0,
        // A file the ledger claims but the diff does not have is either
        // committed already or the ledger has gone stale.
        touched: changed.has(path),
      }
    })
    return { ...item, files }
  })

  const unclaimed = diff.files.filter((f) => !claimed.has(f.path))
  return { items, unclaimed }
}

export default {
  async start(api) {
    git = await resolveGit(api)
    api.log('git:', git, 'root:', api.root)
  },

  handlers: {
    async state(_args, api) {
      const diff = await readDiff(api)
      const { items, unclaimed } = ledgerWith(diff)
      const totals = diff.files.reduce(
        (t, f) => ({ added: t.added + f.added, removed: t.removed + f.removed }),
        { added: 0, removed: 0 },
      )
      return {
        groups: GROUPS,
        items,
        files: diff.files,
        unclaimed: unclaimed.map((f) => f.path),
        head: diff.head || '',
        error: diff.error || '',
        totals,
        root: api.root,
      }
    },
  },

  tools: [
    {
      name: 'summary',
      description:
        'The ledger of work in this workspace: each change with its reason, the files it touched and the tests that hold it, plus anything in the diff the ledger does not account for.',
      parameters: {
        type: 'object',
        properties: {
          group: {
            type: 'string',
            description: 'Limit to one group: licence, bugs, hardening, function, plumbing, open.',
          },
        },
      },
      async execute({ group }, api) {
        const diff = await readDiff(api)
        const { items, unclaimed } = ledgerWith(diff)
        const wanted = group ? items.filter((i) => i.group === group) : items
        const lines = wanted.map((i) => {
          const stale = i.files.filter((f) => !f.touched).map((f) => f.path)
          return [
            `${i.watch ? '[watch] ' : ''}${i.blocked ? '[open] ' : ''}${i.title}`,
            `  files: ${i.files.map((f) => f.path).join(', ') || 'none'}`,
            i.tests ? `  tests: ${i.tests.join(', ')}` : null,
            stale.length ? `  not in the current diff: ${stale.join(', ')}` : null,
          ]
            .filter(Boolean)
            .join('\n')
        })
        const extra = unclaimed.length
          ? `\n\nIn the diff but not in the ledger:\n  ${unclaimed.map((f) => f.path).join('\n  ')}`
          : ''
        return `${wanted.length} entries. ${diff.files.length} files changed.\n\n${lines.join('\n\n')}${extra}`
      },
    },
  ],
}
