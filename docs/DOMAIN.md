# The domain model

## The central split

A **licence** attaches to a **premises** for a period. A **party** attaches to a
premises for a period. These are separate facts with separate lifetimes.

```
Party ──< Occupancy >── Premises ──< Coverage >── Licence
             (period)                  (period)
```

Neither the licence nor the premises is keyed on a person, and no person record
owns a licence. What connects them is a time-bounded relationship, recorded
separately, which means all of the following are expressible without contortion:

- a licence that follows its holder to a new address, keeping its expiry
- two people at one address over the year, with one licence spanning both
- a room in an HMO that needs its own licence while sharing a parent UPRN
- a hotel where one licence covers fifteen units
- a person with a concession that moves with them, and a premises with a
  concession that stays put

This is the migration the real system has been attempting since the 2002 NAO
report and abandoned twice — in 2015 when the Capita/CSC transition programme
was terminated, and again in December 2019 when the £46m technology refresh was
halted after nine months. The stated ambition each time was to restructure the
main databases "based on individual customers rather than households".

The reason it is hard is that it is posed as a choice of primary key. It isn't.
Both facts are true; they just need separate tables.

## The enforcement wall

Having person-level history creates an obvious hazard: it makes a tracking
database. So reads are split by purpose.

**`projection.Service`** answers person-scoped questions. It can return a
party's full address and licence history. Used for transfers, refunds, payment
plan continuity, and concessions that follow a person.

**`projection.Enforcement`** answers premises-scoped questions only. It returns
one `PremisesSnapshot` at a time, and occupants appear as tokens computed as
`HMAC(salt, premises ‖ party)`. The same person at two addresses produces two
unrelated tokens, so enforcement reads cannot be joined into a person.

The separation is enforced three ways:

1. `Service.History` refuses an actor with the enforcement role.
2. `Enforcement.Snapshot` refuses an actor with any other role.
3. `TestEnforcementCannotBeAskedAboutAParty` asserts by reflection that no
   exported method on `*Enforcement` accepts or returns an `ids.PartyID`. Adding
   one fails the build.

Every enforcement read, allowed or refused, is written to the audit log.

## Declarations

Three kinds, in `domain.Declaration`:

| Kind | Meaning | Expires? |
| --- | --- | --- |
| `NO_LICENCE_NEEDED` | Occupier declares none is needed | Never; offered for review at 24 months |
| `LEGALLY_LICENCE_FREE` | Authority's own determination | Never |
| `ACCESS_WITHDRAWN` | Implied right of access withdrawn | Never |

Two rules matter more than the rest:

**An NLN past its review date is `DUE_FOR_REVIEW`, not lapsed.** It still reads
as declared. Passing the review date is not grounds for enforcement, and the
snapshot reports both facts separately so nothing downstream can conflate them.

**A withdrawal of access never expires and cannot carry a review date.** It
blocks visits indefinitely, is revocable only by the party who made it, and the
register refuses to record one with an expiry attached.

## Fees

`domain.Schedule` resolves (class, concession, date) to an amount, bitemporally:
each rate is recorded as in force over a range of *valid* time, as known from a
moment of *transaction* time. A refund computed today for a licence surrendered
two years ago uses the rate in force then, as it was recorded then, and a later
correction does not silently rewrite the historic answer.

Two things are deliberately errors rather than answers:

- an unrecorded rate — `ErrRateNotKnown`
- an unpublished class/concession combination — `ErrConcessionPolicyUnknown`

`Schedule.Coverage()` reports where the data runs out, per class, so the system
can say what it does not know instead of implying completeness.

## Money

Integer pence throughout, never floats. Rounding is half-up. Division
distributes the remainder so that instalments always sum exactly to the fee —
`Money.Split(52)` of £180.00 gives 48 instalments of £3.47 and 4 of £3.46, which
add back to £180.00 exactly. Amounts cross the API as integer pence for the same
reason, and UPRNs cross as strings because twelve digits exceed the range
JavaScript represents exactly.

## Equality monitoring

`internal/equality` records outcomes against optional, self-declared attributes.
`Observation` carries no party, premises or case identifier, so the data cannot
be re-joined to a person or to a decision. Cells below k (default 10) are
withheld from reports, and a rate is published only when both its outcome cell
and its group total clear k — otherwise a suppressed cell could be reconstructed
by subtraction.

The purpose is to make outcome disparity a standing metric rather than a
Freedom of Information response.
