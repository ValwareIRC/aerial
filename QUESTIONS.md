# Open questions

Everything below was decided by guess while you were out. All of it is cheap to
change; none of it is load-bearing on the architecture.

## Naming and layout

1. **Project name "Aerial"** — provisional. Neutral enough for another body to
   adopt, on-theme enough to be no accident.
2. **Go module path** `github.com/aerial-project/aerial` — placeholder. Change
   before the first push; it is a find-and-replace across imports.
3. **Repo lives at** `C:\Users\Valerie\aerial`.
4. **Toolchains** — `winget` stalled on an invisible UAC prompt, so Go 1.27.0
   and Node 24.20.0 were extracted portably to `C:\Users\Valerie\tools`. They
   are not on your PATH. Add them, or install properly with admin rights.

## Fee data

5. **Only verified rates are recorded.** Colour is loaded for 2024, 2025 and
   2026; monochrome and ARC only for 2026. Quoting an unrecorded combination
   returns `ErrRateNotKnown` and HTTP 422 rather than a plausible number. If you
   have the historic monochrome and ARC figures, add them to
   `internal/domain/fee_uk.go`.
6. **ARC plus a concession is unpriced on purpose.** Whether the sight
   impairment concession stacks on the already-reduced £7.50 ARC rate is
   unconfirmed, so that combination errors. Needs a policy answer, not a guess.
7. **Rate publication timestamps are approximated** as two months before each
   rate takes effect. Real deployments must record the actual moment a rate was
   entered — that timestamp is what makes historic decisions replayable.
8. **Fee changes are assumed to fall on 1 April.**

## Policy guesses

9. **NLN review cadence is 24 months** — inferred from your "every couple of
   years". The constant is `domain.NLNReviewMonths`.
10. **Refunds pay complete months remaining**, pro rata on the fee actually
    paid, with the part month not refunded. The rule matches the published
    "complete months" description; the exact rounding convention is unconfirmed.
11. **A withdrawal of access is revocable only by the party who made it.** That
    is a policy choice I made, not a legal requirement. It is the strict reading.
12. **A hotel licence covers 15 units** (`UnitsCovered` on the parent premises).
    Believed right, unverified.
13. **Default licence term is 12 months.**
14. **Payment plan shapes**: weekly is 52 instalments, fortnightly 26, both
    requiring a 12-month term; quarterly requires a term divisible by three.
15. **Equality monitoring defaults to k=10** and takes free-form attribute
    names. The attribute set is not fixed anywhere.

## Not built

16. **No persistence.** Everything is in memory and dies with the process. The
    registers are interfaces-in-waiting; a Postgres schema is the obvious next
    piece, and the bitemporal store is already shaped for it.
17. **No user administration.** Sign-in itself is real: PBKDF2-SHA256 at
    210,000 iterations, server-held sessions in an HttpOnly cookie, the role
    taken from the user record and never from the request, and a fifteen-minute
    lockout after five failed attempts. What is missing is everything around
    it — there is no way to add, suspend or re-password an account except in
    code (`App.SeedUsers`), no password change, and no second factor. The
    accounts are seeded with a development password, so the demo must not be
    exposed to a network.
18. **Not modelled**: prosecution and case management, correspondence and letter
    generation, field visit scheduling, AddressBase/UPRN import, a refunds
    ledger, notifications, concession evidence records (the error exists,
    `ErrConcessionNotEvidenced`, but nothing raises it yet).
19. **No Dockerfile, no deployment story.** CI exists
    (`.github/workflows/ci.yml`: formatting, vet, tests under the race
    detector, the dependency licence audit, and the front-end build) but it has
    never run, because there is no remote yet.

## Licence

20. **The £1,000,000 is expressed as 5,555 × the standard colour licence fee**,
    so it self-indexes rather than being eroded by inflation. Swap for a flat
    number if you would rather it stayed literal.
21. **`CONTACT.md` has no real contact route yet.** Section 8 of the licence
    points at it, so it must be filled in before publishing or the commercial
    licence offer is unacceptable in practice.
22. **The licence has not been reviewed by a solicitor.** The anti-avoidance
    clause and the bystander carve-out are the two that most need it.
