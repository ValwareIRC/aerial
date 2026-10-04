// The promises this project makes, as questions a running server can be asked.
//
// These are not unit tests. They go over HTTP, against a real build, in the
// roles real staff would hold — which is the only way to check that the wall
// described in docs/DOMAIN.md is still standing in the wiring and not merely
// in the domain package.

export const SECTIONS = [
  {
    id: 'wall',
    name: 'The enforcement wall',
    note: 'Person-level history exists. Enforcement must not be able to reach it.',
  },
  {
    id: 'gaps',
    name: 'Gaps are refusals',
    note: 'Missing data must surface as a refusal to answer, never as a plausible number.',
  },
  { id: 'privacy', name: 'Privacy and access', note: 'Who may see what, and what is kept.' },
  { id: 'signin', name: 'Sign-in', note: 'The front door.' },
]

// Each probe gets a client with sign-in helpers and returns { ok, found }.
// found is shown next to the result, so a pass is readable rather than merely
// green.
export const PROBES = [
  // --- the enforcement wall ----------------------------------------------
  {
    id: 'session-required',
    section: 'wall',
    title: 'Nothing is readable without a session',
    why: 'Every route but the health check and the licence notice requires signing in.',
    async run(c) {
      const r = await c.get('/api/premises')
      return { ok: r.status === 401, found: `${r.status} without a cookie` }
    },
  },
  {
    id: 'role-not-asserted',
    section: 'wall',
    title: 'A client cannot assert its own role',
    why: 'The role comes from the user record. Headers claiming otherwise must count for nothing.',
    async run(c) {
      const r = await c.get('/api/audit', {
        headers: { 'X-Actor-Id': 'm.whitlock', 'X-Actor-Role': 'ADMIN' },
      })
      return { ok: r.status === 401, found: `${r.status} for a self-declared administrator` }
    },
  },
  {
    id: 'enforcement-blocked',
    section: 'wall',
    title: 'An officer cannot open a person record',
    why: 'This is the join the projection split exists to prevent.',
    async run(c) {
      await c.signIn('j.okafor')
      const people = await c.get('/api/people')
      const one = await c.get('/api/people/PARTY1')
      const history = await c.get('/api/parties/PARTY1/history')
      const ok = people.status === 403 && one.status === 403 && history.status === 403
      return { ok, found: `people ${people.status}, record ${one.status}, history ${history.status}` }
    },
  },
  {
    id: 'service-blocked',
    section: 'wall',
    title: 'Service staff cannot use the enforcement projection',
    why: 'The split runs both ways, so an enforcement read cannot be made under a service login.',
    async run(c) {
      await c.signIn('a.reyes')
      const r = await c.get('/api/premises/P1/enforcement')
      return { ok: r.status === 403, found: `${r.status} for a customer services login` }
    },
  },
  {
    id: 'worklist-anonymous',
    section: 'wall',
    title: 'The visit list carries no names and no licence numbers',
    why: 'An officer gets addresses and whether they are licensed. Nothing that identifies a person.',
    async run(c) {
      await c.signIn('j.okafor')
      const r = await c.get('/api/worklist')
      const text = JSON.stringify(r.body || '')
      const leaked = ['Kaur', 'Whitlow', 'Vann', 'Adeyemi', 'Morgan', 'Frayne', 'Bello'].filter((n) =>
        text.includes(n),
      )
      const numbers = (text.match(/\d{4} \d{4} \d{2}/g) || []).length
      return {
        ok: r.status === 200 && leaked.length === 0 && numbers === 0,
        found:
          leaked.length || numbers
            ? `leaked ${leaked.join(', ')}${numbers ? ` and ${numbers} licence numbers` : ''}`
            : `${(r.body || []).length} addresses, no names, no numbers`,
      }
    },
  },
  {
    id: 'snapshot-tokens',
    section: 'wall',
    title: 'An address view names nobody',
    why: 'Occupants appear only as per-premises tokens.',
    async run(c) {
      await c.signIn('j.okafor')
      const r = await c.get('/api/premises/P1/enforcement')
      const tokens = (r.body && r.body.occupantTokens) || []
      const text = JSON.stringify(r.body || '')
      const ok =
        r.status === 200 &&
        tokens.length > 0 &&
        !text.includes('PARTY') &&
        !text.includes('Kaur') &&
        tokens.every((t) => typeof t === 'string' && t.length === 16)
      return { ok, found: ok ? `${tokens.length} token: ${tokens[0]}` : `tokens ${JSON.stringify(tokens)}` }
    },
  },
  {
    id: 'tokens-do-not-join',
    section: 'wall',
    title: 'The same person at two addresses gets two unrelated tokens',
    why:
      'This is what stops enforcement reads being joined into a person. Checked by moving somebody and comparing.',
    async run(c) {
      await c.signIn('a.reyes')
      const move = await c.post('/api/people/PARTY1/address', {
        premises: 'P2',
        on: '2026-12-01',
      })
      if (move.status !== 200) return { ok: false, found: `could not move them: ${move.status}` }

      await c.signIn('j.okafor')
      const before = await c.get('/api/premises/P1/enforcement?on=2026-10-05')
      const after = await c.get('/api/premises/P2/enforcement?on=2026-12-02')
      const a = ((before.body || {}).occupantTokens || [])[0]
      const b = ((after.body || {}).occupantTokens || [])[0]
      return {
        ok: Boolean(a && b && a !== b),
        found: a && b ? `${a} at the old address, ${b} at the new one` : `got ${a} and ${b}`,
      }
    },
  },
  {
    id: 'visit-prohibited',
    section: 'wall',
    title: 'A withdrawal of access stops a visit being recorded',
    why: 'It never expires, and the register refuses the visit rather than warning about it.',
    async run(c) {
      await c.signIn('j.okafor')
      const r = await c.post('/api/visits', {
        premises: 'P8',
        on: '2026-10-05',
        outcome: 'NO_ANSWER',
        note: 'guardrail probe',
      })
      return { ok: r.status === 403, found: `${r.status} at the address that withdrew permission` }
    },
  },

  // --- gaps are refusals --------------------------------------------------
  {
    id: 'rate-not-known',
    section: 'gaps',
    title: 'An unrecorded fee is a refusal',
    why: 'Only verified rates are held. A date before them must not produce a plausible number.',
    async run(c) {
      await c.signIn('a.reyes')
      const r = await c.get('/api/fees/quote?class=MONOCHROME&on=2025-04-01')
      return { ok: r.status === 422, found: `${r.status}: ${(r.body || {}).error || ''}` }
    },
  },
  {
    id: 'concession-unknown',
    section: 'gaps',
    title: 'An unpublished discount combination is a refusal',
    why:
      'Whether a concession stacks on the reduced care-home rate is unconfirmed, so it errors rather than guessing.',
    async run(c) {
      await c.signIn('a.reyes')
      const r = await c.get('/api/fees/quote?class=ARC&concession=BLIND_50&on=2026-04-01')
      return { ok: r.status === 422, found: `${r.status}: ${(r.body || {}).error || ''}` }
    },
  },
  {
    id: 'not-licensable',
    section: 'gaps',
    title: 'A hotel room cannot hold its own licence',
    why: 'One licence covers fifteen units, so the room is covered by the hotel and must refuse its own.',
    async run(c) {
      await c.signIn('a.reyes')
      const r = await c.post('/api/licences', {
        holder: 'PARTY4',
        premises: 'P4',
        class: 'COLOUR',
        start: '2026-10-05',
      })
      return { ok: r.status === 409, found: `${r.status}: ${(r.body || {}).error || ''}` }
    },
  },
  {
    id: 'refunds-priced',
    section: 'gaps',
    title: 'Every cancellation reason is priced by the server',
    why:
      'Issued in error returns the whole fee; the ordinary reason returns complete months. Both come from the same function the cancellation uses.',
    async run(c) {
      await c.signIn('a.reyes')
      const r = await c.get('/api/licences/L1?on=2026-10-05')
      const body = r.body || {}
      const refunds = body.refunds || {}
      const fee = ((body.summary || {}).feePaid) || 0
      const whole = refunds.ISSUED_IN_ERROR
      const ordinary = refunds.NO_LONGER_WATCHING
      const ok =
        r.status === 200 &&
        Object.keys(refunds).length === 5 &&
        whole === fee &&
        ordinary > 0 &&
        ordinary < fee
      return {
        ok,
        found: `fee ${pence(fee)}, in error ${pence(whole)}, ordinary ${pence(ordinary)}`,
      }
    },
  },
  {
    id: 'coverage-honest',
    section: 'gaps',
    title: 'The schedule says where its data runs out',
    why: 'A system that cannot say what it does not know invites everyone to assume it is complete.',
    async run(c) {
      await c.signIn('a.reyes')
      const r = await c.get('/api/fees/coverage')
      const rows = r.body || []
      const ok = r.status === 200 && rows.length >= 3 && rows.every((x) => x.earliestKnown)
      return { ok, found: rows.map((x) => `${x.class} from ${x.earliestKnown}`).join(', ') }
    },
  },

  // --- privacy and access -------------------------------------------------
  {
    id: 'no-store',
    section: 'privacy',
    title: 'A person record is never cached',
    why: 'Nearly every answer is somebody’s personal data, so no browser or proxy may keep it.',
    async run(c) {
      await c.signIn('a.reyes')
      const r = await c.get('/api/people/PARTY1')
      const cache = r.headers.get('cache-control') || ''
      const sniff = r.headers.get('x-content-type-options') || ''
      return {
        ok: cache.includes('no-store') && sniff === 'nosniff',
        found: `${cache || 'no Cache-Control'}; ${sniff || 'no nosniff'}`,
      }
    },
  },
  {
    id: 'equality-suppressed',
    section: 'privacy',
    title: 'Small groups are withheld from the outcome report',
    why:
      'Cells below k are not published, and the report says how much it is holding back rather than pretending there is nothing there.',
    async run(c) {
      await c.signIn('m.whitlock')
      const r = await c.get('/api/equality/ageBand')
      const report = (r.body || {}).report || {}
      const cells = report.cells || []
      const ok = r.status === 200 && report.k >= 10 && cells.length === 0 && report.suppressedCount > 0
      return {
        ok,
        found: `k=${report.k}, ${cells.length} cells published, ${report.suppressedCount} observations withheld`,
      }
    },
  },
  {
    id: 'audit-records-refusals',
    section: 'privacy',
    title: 'A refusal is written down',
    why: 'Every enforcement read and every refusal is on the record, which is what makes the split auditable.',
    async run(c) {
      await c.signIn('j.okafor')
      await c.get('/api/people')
      await c.signIn('m.whitlock')
      const r = await c.get('/api/audit?refused=true&limit=50')
      const entries = (r.body || {}).entries || []
      return {
        ok: r.status === 200 && entries.length > 0 && entries.every((e) => e.allowed === false),
        found: `${entries.length} refusals on the record`,
      }
    },
  },
  {
    id: 'audit-admin-only',
    section: 'privacy',
    title: 'The access log is for administrators only',
    why: 'A log of who looked at whom is itself sensitive.',
    async run(c) {
      await c.signIn('a.reyes')
      const r = await c.get('/api/audit')
      return { ok: r.status === 403, found: `${r.status} for customer services` }
    },
  },
  {
    id: 'bank-details-masked',
    section: 'privacy',
    title: 'Bank details come back masked',
    why: 'Only the last four digits are kept at all.',
    async run(c) {
      await c.signIn('a.reyes')
      const r = await c.get('/api/people/PARTY1')
      const text = JSON.stringify(r.body || '')
      const full = /\b\d{8}\b/.test(text.replace(/"uprn":"\d+"/g, ''))
      const masked = text.includes('**-**-')
      return {
        ok: r.status === 200 && masked && !full,
        found: masked ? 'mandate shown as **-**-nn ****nnnn' : 'no masked mandate found',
      }
    },
  },
  {
    id: 'licence-notice',
    section: 'privacy',
    title: 'The running service states its licence',
    why:
      'LICENCE.md requires its notices to be kept and its terms described accurately. The notice needs no session.',
    async run(c) {
      const r = await c.get('/api/licence', { noCookie: true })
      const body = r.body || {}
      const ok =
        r.status === 200 &&
        /Aerial Public Licence/.test(body.licence || '') &&
        /Capita/.test(body.summary || '') &&
        /source-available/.test(body.openSource || '')
      return { ok, found: ok ? body.licence : `${r.status}: ${JSON.stringify(body).slice(0, 80)}` }
    },
  },

  // --- sign-in ------------------------------------------------------------
  {
    id: 'oversized-body',
    section: 'signin',
    title: 'An oversized request is refused',
    why: 'Without a cap, one client can make the server read for as long as it likes.',
    async run(c) {
      await c.signIn('a.reyes')
      const r = await c.post('/api/notes', {
        subject: 'PARTY1',
        text: 'x'.repeat(70 * 1024),
      })
      return { ok: r.status === 413, found: `${r.status} for a 70 kB body` }
    },
  },
  {
    id: 'lockout',
    section: 'signin',
    title: 'Guessing stops after five attempts',
    why:
      'A staff console on a network is a password-guessing target with a very short list of usernames.',
    async run(c) {
      // A username nobody holds, so a real account is not locked by probing.
      let last = 0
      for (let i = 0; i < 6; i++) {
        const r = await c.post(
          '/api/session',
          { username: 'probe.only', password: 'not-the-password' },
          { noCookie: true },
        )
        last = r.status
      }
      return { ok: last === 429, found: `${last} on the sixth attempt` }
    },
  },
  {
    id: 'signin-works',
    section: 'signin',
    title: 'A real sign-in still works, with the role from the record',
    why: 'The lockout must not be so eager that it keeps staff out.',
    async run(c) {
      const r = await c.post(
        '/api/session',
        { username: 'j.okafor', password: c.password },
        { noCookie: true },
      )
      const role = (r.body || {}).role
      return { ok: r.status === 200 && role === 'ENFORCEMENT', found: `signed in as ${role}` }
    },
  },
]

function pence(n) {
  if (typeof n !== 'number') return String(n)
  const neg = n < 0
  const abs = Math.abs(n)
  return `${neg ? '−' : ''}£${Math.floor(abs / 100)}.${String(abs % 100).padStart(2, '0')}`
}
