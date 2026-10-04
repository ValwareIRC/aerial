# Aerial

Licence administration for a statutory levy: premises, parties, entitlements,
declarations, fees and payment plans.

Free for anyone to use, **except** the BBC, Capita, and any other administrator
of the UK television licence fee, who need a commercial licence at £1,000,000 a
year. See [LICENCE.md](LICENCE.md).

## Status

The domain core, the projection split, the HTTP API and a console front end are
built and tested. Sign-in is real — password hashing, server-held sessions,
role from the user record, lockout after repeated failures — but there is no
persistence, so everything dies with the process. See
[QUESTIONS.md](QUESTIONS.md) for everything that is deliberately unfinished or
guessed.

## Running it

Toolchains used in development: Go 1.27, Node 24.

```sh
cd backend
AERIAL_SALT=dev-salt go run ./cmd/aerald        # :8080
go run ./cmd/aerald -version                    # version and licence terms
```

```sh
cd frontend
npm install
npm run dev                                      # :5173, proxies /api to :8080
```

`aerald` stops on SIGINT or SIGTERM, letting requests in flight finish first.

Checks:

```sh
cd backend && go vet ./... && go test ./...
cd backend && go test -race ./...      # needs a C toolchain; CI runs it
cd frontend && npm run licences        # dependency licences vs LICENCE.md
cd frontend && npm run build           # types, then the production bundle
```

[CI](.github/workflows/ci.yml) runs all of the above on every push.

## What is here

| Package | Purpose |
| --- | --- |
| `internal/money` | Integer pence, half-up rounding, remainder-preserving division |
| `internal/timeline` | Civil dates, half-open ranges, bitemporal store |
| `internal/ids` | UPRN parsing; premises are keyed on UPRN, not text addresses |
| `internal/domain` | Premises, occupancy, licences, declarations, fee schedule |
| `internal/projection` | The service/enforcement split |
| `internal/payment` | Instalment schedules |
| `internal/equality` | k-anonymous outcome reporting |
| `internal/audit` | Append-only record of reads and refusals |
| `internal/httpapi` | JSON API |
| `frontend` | React console |

## Design in one paragraph

A licence attaches to a **premises**; a person attaches to a premises only for a
**period**. Those are separate facts with separate lifetimes, so a licence can
follow its holder to a new address without the database being keyed on people.
Cross-address person history exists, and is available to the service projection
for transfers, refunds and concessions — but the enforcement projection cannot
reach it, and shows occupants only as per-premises tokens that do not join up.
See [docs/DOMAIN.md](docs/DOMAIN.md) and [docs/DECISIONS.md](docs/DECISIONS.md).

## Dependencies

The backend has no third-party dependencies at all. The front end uses React,
Vite and TypeScript, and every installed package is MIT, ISC, Apache-2.0,
BSD-3-Clause or CC-BY-4.0. No GPL, LGPL, AGPL or SSPL anywhere, which the
licence in [LICENCE.md](LICENCE.md) requires — copyleft terms cannot be combined
with its field-of-use restriction.

That is checked rather than asserted. `npm run licences` audits every installed
package and fails the build on anything copyleft, share-alike or
non-commercial, and on any licence it does not recognise. Run it whenever the
lockfile changes; CI runs it too.
