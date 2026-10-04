# Decisions

Rationale lives here rather than in code comments.

## 1. Go backend, no third-party dependencies

The licence in `LICENCE.md` restricts a field of endeavour, which cannot be done
to a work incorporating GPL, LGPL, AGPL or SSPL code. The cheapest way to
guarantee that permanently is to depend on nothing. The standard library covers
everything the backend needs, including routing (`net/http` method patterns,
Go 1.22+) and testing.

The front end does have dependencies. All 69 are MIT, ISC, Apache-2.0,
BSD-3-Clause or CC-BY-4.0 (`caniuse-lite`, a build-time dataset). Re-run the
audit whenever the lockfile changes.

## 2. Money is integer pence

Binary floating point cannot represent 0.1 exactly. Fee arithmetic decides what
people are charged and refunded, so the type makes the representation impossible
to get wrong, and `MarshalJSON` emits pence as an integer so no client can
reintroduce a float.

Rounding is half-up rather than banker's, because that is what published fee
tables use: half of the £60.50 monochrome fee is £30.25 exactly, which is why
the published sight impairment rate is not a round number.

## 3. Half-open date ranges

`[start, end)`. Licence periods abut — a licence to 1 April and its renewal from
1 April must neither overlap nor leave a gap — and closed intervals make that an
off-by-one waiting to happen. `Range.End == nil` means "no end recorded", which
is a different fact from "ends today".

## 4. Month arithmetic clamps

31 January plus one month is 28 February, not 3 March. Overflowing would make a
monthly instalment schedule started on the 31st skip a payment date, and would
make a licence issued on 29 February expire on 1 March, disagreeing with the
printed licence.

## 5. Bitemporality is in the core, not bolted on

Two time axes: when a fact was true, and when the system was told. Storing only
the current value makes historic decisions unauditable and makes a correction
silently rewrite the past. The store is append-only; corrections append a later
record.

This costs almost nothing to build in at the start and is close to impossible to
retrofit, which is the general shape of why the real system's two migration
attempts were so expensive.

## 6. Gaps are errors, not fallbacks

`ErrRateNotKnown` and `ErrConcessionPolicyUnknown` exist so that missing data
surfaces as a refusal to answer rather than a plausible wrong number. Both map
to HTTP 422, not 500: the request was well formed; the data is absent.

The same instinct produced `Schedule.Coverage()`, which reports where the
recorded rates begin. A system that cannot say where its own data runs out
invites everyone to assume it is complete.

## 7. Reads are split by purpose, and the split is structural

See [DOMAIN.md](DOMAIN.md). The point worth repeating here: role checks are
policy and can be changed by whoever runs the deployment, but the reflection
test asserting that `*Enforcement` has no method taking a `PartyID` is
structural, and cannot be relaxed without an obvious commit.

## 8. Lessons taken from the two abandoned programmes

- **No vendor owns both the legacy core and its replacement.** CSC supplied the
  system it was contracted to replace, and the 2015 programme died in a
  Capita/CSC commercial dispute, not a technical one. Source-available code
  removes this failure mode structurally.
- **No big-bang.** Both attempts were large and all-or-nothing. The party and
  premises model is additive: it can run alongside an address-keyed legacy store
  with dual writes, migrating reads a slice at a time.
- **Core first.** In both programmes the edge work shipped — contact centres,
  website, handheld units — and the data model never did. So the model is what
  exists here first, and the edges are unbuilt.
- **Legacy carrying cost is a line item.** £9m of the withheld £27.8m went on
  keeping superseded systems alive.

## 9. Withdrawal of access is permanent by default

The strict reading of a withdrawn implied right of access is that it stands
until the person who withdrew it says otherwise. The register enforces that: no
expiry may be attached, and only the party who made it may revoke it. This is a
policy choice rather than a legal requirement — see QUESTIONS.md item 11.

## 10. Equality data is aggregate-only by construction

Disparity cannot be measured without recording the attribute, and recording the
attribute against an individual creates exactly the hazard the projection split
exists to avoid. The resolution is that `Observation` has no identifier field at
all, so the data is unjoinable by construction rather than by policy, and
k-anonymity is applied at the report boundary.

## 11. One lock, at the application boundary

The registers are plain maps and slices with no synchronisation of their own,
and `net/http` serves every request in its own goroutine. Reads used to take
`App.mu` while writes did not, which is the shape of a data race that shows up
as a corrupted map under load rather than as a failing test.

So the rule is now uniform: every exported method on `*App` takes the lock, and
the unexported `...Locked` forms are what the package calls once it is already
held. `CreateAddress` holds it across the duplicate check and the insert,
because two agents typing the same new address at the same moment must not both
pass the check.

The second half of the rule is that the HTTP layer never touches a register. It
used to call `app.Licences.Transfer` and `app.Records.Notes` directly, outside
the lock and outside the audit trail. Those paths go through the app now, which
is also where the letter and the audit entry get written — the transfer had
neither.

`TestConcurrentUseIsSafe` exercises the mixed read/write path from eight
goroutines. It is only meaningful under `-race`, which needs a C toolchain, so
CI runs that and a Windows workstation need not.

## 12. Money arithmetic refuses to wrap

`Proportion` multiplied before dividing, in `int64`. A large enough amount
times a large enough numerator wrapped silently, and a refund that wraps is a
payment in the wrong direction. The multiplication is now exact — 128-bit via
`math/bits`, falling back to `math/big` — and `Add` and `MulInt` panic rather
than wrap.

A panic in a fee calculation is a bad outcome. A wrong charge nobody notices is
a worse one, and this is a type that exists to make the representation
impossible to get wrong.

## 13. Money is calculated once, on the server

The console used to estimate a refund in TypeScript before showing the
confirmation, with the term hard-coded as twelve months, and the licence page
showed the figure for one cancellation reason whichever reason was selected. So
the number somebody read out over the phone was not always the number the
system then paid.

The licence detail now carries a refund for every cancellation reason, from
`Licence.RefundDue` — the same function the cancellation itself calls. The
client does no money arithmetic at all. `TestRefundQuotedBeforehandIsTheRefundPaid`
asserts the quote and the payment agree.

## 14. The licence travels with the software

Section 4 requires attribution notices to be retained, and the closing note in
LICENCE.md asks that the terms be described accurately. Both are easy to lose
in a deployment: a built bundle and a running binary carry no README.

So the notice is in the software. `aerald -version` prints it, `GET
/api/licence` serves it without a session, and the console shows it in the
footer on every page, sign-in included. One set of constants in `internal/app`,
so there is nowhere for a second, friendlier wording to appear.

The dependency rule in decision 1 is likewise executable rather than asserted:
`npm run licences` reads every installed package and fails on anything
copyleft, share-alike or non-commercial, and on any licence it does not
recognise. It has no dependencies itself.

It scans every place in the repository npm packages can land, not just
`frontend/node_modules`: a Prism gadget under `.prism/gadgets/` may declare its
own dependencies, and a rule that only checks one directory is a rule with a
door left open.
