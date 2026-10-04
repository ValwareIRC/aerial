// What was done, and why, in the order it matters rather than the order it
// happened. The agent maintains this file; the file lists are checked against
// the live diff, so an entry that no longer touches anything shows up as stale.
//
// watch: true  marks a change in behaviour that deserves a second opinion.

export const GROUPS = [
  {
    id: 'licence',
    name: 'The licence',
    note: 'LICENCE.md has requirements that software loses quietly. These make them hold.',
  },
  { id: 'bugs', name: 'Bugs fixed', note: 'Things that were wrong, not merely unpolished.' },
  { id: 'hardening', name: 'Hardening', note: 'A staff console holding personal data.' },
  { id: 'function', name: 'Function and polish', note: 'The console, as used.' },
  { id: 'plumbing', name: 'Docs and tooling', note: 'Keeping the written record true.' },
  { id: 'open', name: 'Still open', note: 'Not done, or not verifiable here. Read these.' },
]

export const ITEMS = [
  // --- the licence --------------------------------------------------------
  {
    id: 'licence-notice',
    group: 'licence',
    title: 'The licence notice travels with the software',
    why:
      'Section 4 requires notices to be retained, and the closing note asks that the terms be described accurately. A built bundle and a running binary carry no README, so the notice is now in the code: aerald -version prints it, GET /api/licence serves it without a session, and the console footer shows it on every page.',
    files: [
      'backend/internal/app/app.go',
      'backend/internal/httpapi/api.go',
      'backend/cmd/aerald/main.go',
      'frontend/src/Shell.tsx',
    ],
    tests: ['TestTheLicenceNoticeIsPublicAndAccurate'],
  },
  {
    id: 'licence-audit',
    group: 'licence',
    title: 'Dependency licences are audited, not asserted',
    why:
      'DECISIONS.md said to re-run the audit whenever the lockfile changes, but nothing did it. npm run licences reads every installed package and fails on copyleft, share-alike or non-commercial terms, and on anything it does not recognise. It has no dependencies of its own, for the same reason.',
    files: ['frontend/scripts/check-licences.mjs', 'frontend/package.json', 'README.md'],
    evidence: 'Audits 73 packages: 64 MIT, 5 ISC, 2 Apache-2.0, 1 CC-BY-4.0, 1 BSD-3-Clause.',
  },
  {
    id: 'licence-declared',
    group: 'licence',
    title: 'package.json declares the licence',
    why:
      'It declared none, so npm tooling labels the project UNLICENSED and invites somebody to “fix” it to MIT. It now points at LICENCE.md.',
    files: ['frontend/package.json'],
  },

  // --- bugs ---------------------------------------------------------------
  {
    id: 'gitignore',
    group: 'bugs',
    title: 'The first commit was missing the server’s main package',
    watch: true,
    why:
      'The .gitignore written in the first session had a bare line “aerald” to exclude the compiled binary. A bare name also matches a directory, so it matched backend/cmd/aerald/ and silently excluded the whole main package: the repository as committed cannot build the server. The patterns are now anchored, and the file shows up as untracked. It needs a commit — nothing I can fix in the working tree alone.',
    files: ['.gitignore', 'backend/cmd/aerald/main.go'],
    evidence:
      'Found by the staleness check in this panel: the ledger claimed main.go, the diff had never heard of it. 69 files were tracked where there should have been 70.',
  },
  {
    id: 'instalments',
    group: 'bugs',
    title: 'An instalment due today read as missed',
    why:
      'The switch tested “due on or before today” before its own default, so the DUE state was unreachable and today’s collection showed as MISSED. The running payment total was also never halted on a shortfall, so because odd pence go on the earliest instalments, a later cheaper one could show as PAID while an earlier one was outstanding.',
    files: ['backend/internal/domain/finance.go', 'backend/internal/app/app_test.go'],
    tests: [
      'TestInstalmentDueTodayIsDueNotMissed',
      'TestLaterInstalmentIsNotPaidWhileAnEarlierOneIsOutstanding',
    ],
  },
  {
    id: 'history',
    group: 'bugs',
    title: 'Party history missed licences bought mid-tenancy',
    why:
      'Service.History looked only at the first day of each occupancy, through a pointless one-element loop. A licence bought part way through a tenancy, or renewed during it, was invisible to the projection that transfers, refunds and concession continuity depend on.',
    files: ['backend/internal/projection/service.go', 'backend/internal/projection/projection_test.go'],
    tests: [
      'TestHistoryFindsALicenceBoughtPartWayThroughAnOccupancy',
      'TestHistoryIgnoresSomebodyElsesLicenceAtTheSameAddress',
      'TestEveryLicenceHeldAtAnAddressAppearsInTheHistory',
    ],
  },
  {
    id: 'refunds',
    group: 'bugs',
    title: 'Refunds were worked out twice and disagreed',
    why:
      'The console estimated refunds in TypeScript with the term hard-coded to twelve months, and the licence page showed the ordinary reason’s figure whichever reason was chosen — so “issued in error” quoted a pro-rata figure before paying back the whole fee. The server now prices every reason from the same function the cancellation calls, and the client does no money arithmetic at all.',
    files: [
      'backend/internal/domain/licence.go',
      'backend/internal/domain/records.go',
      'backend/internal/app/queries.go',
      'frontend/src/pages/Licences.tsx',
      'frontend/src/pages/People.tsx',
    ],
    tests: [
      'TestRefundsArePricedForEveryReason',
      'TestRefundQuotedBeforehandIsTheRefundPaid',
      'TestLicenceDetailPricesEveryCancellationReason',
    ],
  },
  {
    id: 'money',
    group: 'bugs',
    title: 'Money arithmetic could wrap silently',
    watch: true,
    why:
      'Proportion multiplied before dividing in int64, so a large enough amount wrapped — and a refund that wraps is a payment in the wrong direction. The product is now exact, via 128-bit arithmetic with a big.Int fallback. Worth a second opinion: Add and MulInt now panic rather than wrap, which is a loud failure where there used to be a quiet wrong number.',
    files: ['backend/internal/money/money.go', 'backend/internal/money/money_test.go'],
    tests: [
      'TestProportionOfALargeAmountDoesNotWrap',
      'TestProportionStillRoundsHalfUpAtScale',
      'TestMultiplicationRefusesToWrap',
      'TestAdditionRefusesToWrap',
    ],
  },
  {
    id: 'locking',
    group: 'bugs',
    title: 'Reads took the lock; writes did not',
    why:
      'The registers have no synchronisation of their own and every request is its own goroutine, so this was a live data race that shows up as a corrupted map under load rather than as a failing test. Every exported App method now takes the lock, with unexported ...Locked forms inside, and CreateAddress holds it across check-then-insert.',
    files: [
      'backend/internal/app/app.go',
      'backend/internal/app/operations.go',
      'backend/internal/app/queries.go',
      'backend/internal/app/people.go',
      'backend/internal/app/addresses.go',
    ],
    tests: ['TestConcurrentUseIsSafe'],
    evidence: 'Only meaningful under -race, which needs a C compiler. CI runs it; this machine cannot.',
  },
  {
    id: 'boundary',
    group: 'bugs',
    title: 'The HTTP layer reached past the app into the registers',
    watch: true,
    why:
      'Handlers called app.Licences.Transfer and app.Records.Notes directly — outside the lock and outside the audit trail. A transfer could land on an address that did not exist, and wrote neither a letter nor an audit entry. Worth a second opinion: the cancel and transfer responses now return a licence summary rather than the raw domain object.',
    files: ['backend/internal/httpapi/api.go', 'backend/internal/app/operations.go'],
    tests: [
      'TestTransferChecksTheTargetAddress',
      'TestALicenceCannotBeTransferredToAnAddressThatDoesNotExist',
    ],
  },
  {
    id: 'visit-role',
    group: 'bugs',
    title: 'Half of the enforcement wall was unguarded',
    why:
      'Snapshot refused any non-enforcement actor; VisitPermitted, on the same projection, refused nobody — contradicting what DOMAIN.md claims. It now refuses and logs the refusal.',
    files: ['backend/internal/projection/enforcement.go', 'backend/internal/projection/projection_test.go'],
    tests: ['TestTheVisitCheckIsAlsoEnforcementOnly'],
  },
  {
    id: 'validation',
    group: 'bugs',
    title: 'Records could be attached to things that did not exist',
    why:
      'Declarations and visits could be filed against unknown addresses, licences issued to unknown people, a cancelled licence renewed, and a child address parented to nothing.',
    files: ['backend/internal/app/app.go', 'backend/internal/app/operations.go', 'backend/internal/app/addresses.go'],
    tests: [
      'TestADeclarationNeedsAnAddressThatExists',
      'TestAVisitNeedsAnAddressThatExists',
      'TestALicenceCannotBeIssuedToSomebodyWhoIsNotOnRecord',
      'TestACancelledLicenceCannotBeRenewed',
    ],
  },

  // --- hardening ----------------------------------------------------------
  {
    id: 'middleware',
    group: 'hardening',
    title: 'Nothing is cached, sniffed, framed or lost to a panic',
    why:
      'Almost every response is somebody’s personal data. Added no-store, nosniff, DENY, no-referrer, COOP, CORP and a CSP, an access log, and a recovery that answers rather than dropping the connection.',
    files: ['backend/internal/httpapi/middleware.go', 'backend/internal/httpapi/hardening_test.go'],
    tests: ['TestPersonalDataIsNeverCached'],
  },
  {
    id: 'bodies',
    group: 'hardening',
    title: 'Request bodies are capped and read once',
    why:
      'There was no size limit, and four handlers decoded JSON by hand. One helper now does it, with a 64 KB cap, and a second JSON object smuggled after the first is refused.',
    files: ['backend/internal/httpapi/middleware.go', 'backend/internal/httpapi/operations.go', 'backend/internal/httpapi/hardening_test.go'],
    tests: ['TestAnOversizedBodyIsRefused', 'TestTrailingRubbishAfterTheJSONIsRefused'],
  },
  {
    id: 'signin',
    group: 'hardening',
    title: 'Sign-in throttled, sessions tidied, credentials out of the bundle',
    why:
      'Five failures lock an account for fifteen minutes, applied identically to unknown usernames so the lockout does not disclose which accounts exist. Expired sessions are swept instead of accumulating. The development usernames and password now render only in a development build.',
    files: [
      'backend/internal/auth/auth.go',
      'backend/internal/auth/auth_test.go',
      'frontend/src/pages/SignIn.tsx',
      'frontend/tsconfig.json',
    ],
    tests: [
      'TestRepeatedFailuresLockTheAccount',
      'TestLockoutDoesNotDiscloseWhetherAnAccountExists',
      'TestOneGoodSignInClearsTheFailureCount',
      'TestExpiredSessionsAreSweptAway',
      'TestRevokeAllForEndsEverySessionOfOneUser',
      'TestRepeatedBadPasswordsAreThrottled',
    ],
    evidence: 'Checked the production bundle: no occurrence of the development password.',
  },
  {
    id: 'lifecycle',
    group: 'hardening',
    title: 'The server has timeouts and stops properly',
    why:
      'Read, write and idle timeouts, so a client that opens a connection and says nothing cannot hold a slot; and a drain on SIGINT or SIGTERM so requests in flight finish.',
    files: ['backend/cmd/aerald/main.go'],
    evidence: 'aerald -version prints the version and the licence terms.',
  },

  // --- function and polish ------------------------------------------------
  {
    id: 'audit-api',
    group: 'function',
    title: 'The access log answers newest first, and narrows',
    why:
      'It used to hand the client the whole log to reverse in JavaScript. It now takes limit, role and refused, answers newest first, and reports honest totals so a filtered view says what it is hiding.',
    files: [
      'backend/internal/httpapi/api.go',
      'frontend/src/pages/AccessLog.tsx',
      'backend/internal/httpapi/hardening_test.go',
    ],
    tests: ['TestTheAuditLogAnswersNewestFirstAndNarrows'],
  },
  {
    id: 'ordering',
    group: 'function',
    title: 'Addresses sort the way people read them',
    why: 'They were sorted as text: 12, 31, 4, 58, 7. Runs of digits now compare as numbers.',
    files: ['backend/internal/app/queries.go'],
    tests: ['TestAddressesSortTheWayPeopleReadThem'],
  },
  {
    id: 'overview',
    group: 'function',
    title: 'The landing page asks one question',
    why:
      'It downloaded the whole register and re-derived “outstanding” in the client. The server now sends the lists behind its own counts, so the rule lives in one place.',
    files: ['backend/internal/app/queries.go', 'frontend/src/pages/Overview.tsx'],
    tests: ['TestOverviewCarriesTheListsBehindItsCounts'],
  },
  {
    id: 'fee-preview',
    group: 'function',
    title: 'The fee is shown before the licence is issued',
    why:
      'Issuing was a leap: the price appeared afterwards, and an unrecorded rate appeared as a failure after the fact. The quote is now shown as the form is filled in, with the refusal in plain words. The fees page also defaults to today rather than a date hard-coded when it was written.',
    files: ['frontend/src/pages/Addresses.tsx', 'frontend/src/pages/Fees.tsx'],
  },
  {
    id: 'a11y',
    group: 'function',
    title: 'Keyboard, screen reader, and paper',
    why:
      'The address picker is a real combobox (arrows, Enter, Escape, announced results). The record tabs follow the ARIA pattern and are shareable by URL. Pages have titles, there is a skip link, focus moves to the heading on navigation and to error summaries when they appear, movement honours the reduced-motion setting, and a printed record prints as the record rather than the furniture.',
    files: [
      'frontend/src/ui.tsx',
      'frontend/src/fields.tsx',
      'frontend/src/styles.css',
      'frontend/src/pages/People.tsx',
      'frontend/index.html',
    ],
  },
  {
    id: 'errors',
    group: 'function',
    title: 'A failure becomes a sentence in one place',
    why:
      'Every page repeated the same incantation to turn an unknown throw into something displayable, and none of them handled a dropped connection or a proxy answering with HTML — which surfaced as a JSON parser complaining about “<”. asApiError does it once, and says “could not reach the service” when that is what happened.',
    files: [
      'frontend/src/api.ts',
      'frontend/src/pages/Enforcement.tsx',
      'frontend/src/pages/Equality.tsx',
      'frontend/src/pages/Fees.tsx',
    ],
  },
  {
    id: 'session-end',
    group: 'function',
    title: 'An expired session returns you to sign-in',
    why:
      'A twelve-hour session expires mid-shift. Any unauthorised answer now drops to sign-in and says why, instead of leaving somebody clicking at a page that will never load.',
    files: ['frontend/src/api.ts', 'frontend/src/Shell.tsx'],
  },

  // --- docs and tooling ---------------------------------------------------
  {
    id: 'docs',
    group: 'plumbing',
    title: 'The written record matches the code again',
    why:
      'QUESTIONS.md still said the actor came from X-Actor-Id headers, and the README said there was no authentication, when PBKDF2 sessions have been real for a while. Four new decisions are recorded: one lock at the boundary, money that refuses to wrap, money calculated once, and the licence travelling with the software.',
    files: ['README.md', 'QUESTIONS.md', 'docs/DECISIONS.md'],
  },
  {
    id: 'ci',
    group: 'plumbing',
    title: 'CI, which is where the race detector lives',
    why:
      'Formatting, vet, tests under the race detector, the licence audit and the front-end build, on every push. The race detector needs a C toolchain this workstation does not have, so it belongs here.',
    files: ['.github/workflows/ci.yml'],
  },
  {
    id: 'gadgets',
    group: 'plumbing',
    title: 'Three panels to keep track by',
    why:
      'Checks runs every gate in one place and says plainly which ones cannot run here. Work is the ledger you are reading, and it checks itself against the real diff. Guardrails builds the server and asks it whether the promises in DOMAIN.md still hold over HTTP. The licence audit was also widened to cover any dependencies a gadget declares, since gadgets install into their own folder.',
    files: [
      '.prism/gadgets/aerial-checks/backend.mjs',
      '.prism/gadgets/aerial-checks/index.html',
      '.prism/gadgets/aerial-checks/gadget.json',
      '.prism/gadgets/aerial-work/backend.mjs',
      '.prism/gadgets/aerial-work/index.html',
      '.prism/gadgets/aerial-work/gadget.json',
      '.prism/gadgets/aerial-work/ledger.mjs',
      '.prism/gadgets/aerial-guardrails/backend.mjs',
      '.prism/gadgets/aerial-guardrails/probes.mjs',
      '.prism/gadgets/aerial-guardrails/index.html',
      '.prism/gadgets/aerial-guardrails/gadget.json',
      'frontend/scripts/check-licences.mjs',
    ],
  },

  // --- still open ---------------------------------------------------------
  {
    id: 'open-race',
    group: 'open',
    title: 'The race detector has never actually run',
    why:
      'TestConcurrentUseIsSafe passes, but without -race it proves very little, and -race needs cgo and a C compiler that is not installed here. The locking fix is verified by review and by CI, not by a local run. Installing a C toolchain, or pushing to a remote, would settle it.',
    files: ['backend/internal/app/app_test.go'],
    blocked: true,
  },
  {
    id: 'open-browser',
    group: 'open',
    title: 'The signed-in console was never walked in a browser',
    why:
      'The sign-in screen was checked in Prism’s browser, and every endpoint the pages render was checked over HTTP, but the browser tool could not run scripts, so nobody has clicked through the signed-in pages. The front-end changes are backed by the type checker and the production build only.',
    files: ['frontend/src/pages/People.tsx', 'frontend/src/pages/Licences.tsx'],
    blocked: true,
  },
  {
    id: 'open-contact',
    group: 'open',
    title: 'CONTACT.md still has no contact route',
    why:
      'Section 8 of the licence offers a commercial licence and points at CONTACT.md, so until it is filled in the offer is one nobody can accept. Deliberately left alone: it needs an address you choose, not one invented for you.',
    files: ['CONTACT.md'],
    blocked: true,
  },
]
