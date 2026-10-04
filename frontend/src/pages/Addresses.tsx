import { useEffect, useState, type FormEvent } from "react";
import { Link, useParams } from "react-router-dom";
import {
  api,
  asApiError,
  longDate,
  money,
  today,
  type ApiError,
  type Concession,
  type Declaration,
  type LicenceClass,
  type PremisesDetail,
  type PremisesSummary,
  type Quote,
} from "../api";
import {
  Banner,
  Empty,
  ErrorSummary,
  Field,
  Loading,
  PageHeader,
  Section,
  SummaryList,
  Tag,
  statusTone,
  statusWords,
} from "../ui";

export function AddressSearch() {
  const [term, setTerm] = useState("");
  const [submitted, setSubmitted] = useState("");
  const [rows, setRows] = useState<PremisesSummary[] | null>(null);
  const [error, setError] = useState<ApiError | null>(null);
  const [busy, setBusy] = useState(false);

  const run = (value: string) => {
    setBusy(true);
    setError(null);
    api
      .searchPremises(value)
      .then((r) => {
        setRows(r);
        setSubmitted(value);
      })
      .catch((e: unknown) => setError(asApiError(e)))
      .finally(() => setBusy(false));
  };

  useEffect(() => run(""), []);

  const submit = (e: FormEvent) => {
    e.preventDefault();
    run(term);
  };

  return (
    <>
      <PageHeader title="Addresses" lede="Search by address, postcode, surname or licence number." />
      <ErrorSummary error={error} />

      <form className="search-form" onSubmit={submit} role="search">
        <Field label="Search">
          <input
            value={term}
            onChange={(e) => setTerm(e.target.value)}
            placeholder="e.g. CT16 2QP, Rowan Close, Kaur"
            autoFocus
          />
        </Field>
        <button className="button" type="submit" disabled={busy}>
          Search
        </button>
      </form>

      {rows === null ? (
        <Loading />
      ) : rows.length === 0 ? (
        <Empty>No addresses match “{submitted}”.</Empty>
      ) : (
        <>
          <p className="result-count">
            {rows.length} {rows.length === 1 ? "address" : "addresses"}
            {submitted && <> matching “{submitted}”</>}
          </p>
          <table className="data-table">
            <thead>
              <tr>
                <th>Address</th>
                <th>Type</th>
                <th>Occupier</th>
                <th>Licence</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((p) => (
                <tr key={p.id}>
                  <td>
                    <Link to={`/addresses/${p.id}`}>{p.address.line1}</Link>
                    <span className="cell-sub">
                      {[p.address.line2, p.address.town, p.address.postcode]
                        .filter(Boolean)
                        .join(", ")}
                    </span>
                  </td>
                  <td>{p.kindLabel}</td>
                  <td>{p.occupier || "Not recorded"}</td>
                  <td>
                    {p.coveredByParent ? (
                      <Tag>Covered by main licence</Tag>
                    ) : p.licensed ? (
                      <>
                        <Tag tone="good">Licensed</Tag>
                        <span className="cell-sub">{p.licenceNumber}</span>
                      </>
                    ) : p.declared ? (
                      <Tag tone="info">No licence needed</Tag>
                    ) : (
                      <Tag tone="stop">No licence</Tag>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      )}
    </>
  );
}

export function AddressRecord() {
  const { id = "" } = useParams();
  const [detail, setDetail] = useState<PremisesDetail | null>(null);
  const [error, setError] = useState<ApiError | null>(null);
  const [notice, setNotice] = useState("");

  const load = () => {
    api
      .premises(id)
      .then(setDetail)
      .catch((e: unknown) => setError(asApiError(e)));
  };

  useEffect(load, [id]);

  if (error) return <ErrorSummary error={error} />;
  if (!detail) return <Loading />;

  const { summary, premises } = detail;

  return (
    <>
      <PageHeader
        overline="Address record"
        title={premises.address.line1}
        lede={[premises.address.line2, premises.address.town, premises.address.postcode]
          .filter(Boolean)
          .join(", ")}
      />

      {notice && <Banner tone="good" title={notice} />}

      {summary.accessWithdrawn && (
        <Banner tone="stop" title="Do not visit this address">
          The occupier has withdrawn permission to call here. This does not expire.
        </Banner>
      )}

      {summary.coveredByParent && detail.parent && (
        <Banner tone="info" title="Covered by another licence">
          This room is covered by the licence for{" "}
          <Link to={`/addresses/${detail.parent.id}`}>{detail.parent.addressLine}</Link>. It cannot
          hold its own licence.
        </Banner>
      )}

      <Section title="Details" icon="home" hue="peach">
        <SummaryList
          rows={[
            { key: "Type", value: summary.kindLabel },
            {
              key: "Occupier",
              value:
                detail.occupants.length === 0 ? (
                  "Not recorded"
                ) : (
                  detail.occupants.map((o, i) => (
                    <span key={o.id}>
                      {i > 0 && ", "}
                      <Link to={`/people/${o.id}`}>{nameOf(o)}</Link>
                    </span>
                  ))
                ),
            },
            { key: "Property reference", value: summary.uprn },
            {
              key: "Licence",
              value: summary.licensed ? (
                <>
                  <Tag tone="good">Licensed</Tag> {summary.licenceNumber}, expires{" "}
                  {longDate(summary.expiry)}
                </>
              ) : summary.declared ? (
                <>
                  <Tag tone="info">No licence needed</Tag>
                  {summary.dueForReview && " — due to be checked"}
                </>
              ) : (
                <Tag tone="stop">No licence</Tag>
              ),
            },
          ]}
        />
      </Section>

      <Section title="Licences" icon="licence" hue="lilac">
        {detail.licences.length === 0 ? (
          <Empty>No licence has ever been issued for this address.</Empty>
        ) : (
          <table className="data-table">
            <thead>
              <tr>
                <th>Number</th>
                <th>Holder</th>
                <th>Type</th>
                <th>Period</th>
                <th>Paid</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              {detail.licences.map((l) => (
                <tr key={l.id}>
                  <td>
                    <Link to={`/licences/${l.id}`}>{l.number}</Link>
                  </td>
                  <td>{l.holderName}</td>
                  <td>{l.classLabel}</td>
                  <td>
                    {longDate(l.start)}
                    <span className="cell-sub">to {longDate(l.expiry)}</span>
                  </td>
                  <td className="numeric">{money(l.feePaid)}</td>
                  <td>
                    <Tag tone={statusTone(l.status)}>{statusWords(l.status)}</Tag>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Section>

      <Section title="Declarations" icon="note" hue="sky">
        {detail.declarations.length === 0 ? (
          <Empty>None recorded.</Empty>
        ) : (
          <table className="data-table">
            <thead>
              <tr>
                <th>Declaration</th>
                <th>Made</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              {detail.declarations.map((d) => (
                <tr key={d.id}>
                  <td>{declarationWords(d.kind)}</td>
                  <td>{longDate(d.madeOn)}</td>
                  <td>{declarationStatus(d)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Section>

      {!summary.coveredByParent && (
        <Actions detail={detail} onDone={(msg) => { setNotice(msg); load(); }} />
      )}
    </>
  );
}

function Actions({
  detail,
  onDone,
}: {
  detail: PremisesDetail;
  onDone: (message: string) => void;
}) {
  const [error, setError] = useState<ApiError | null>(null);
  const [busy, setBusy] = useState(false);
  const holder = detail.occupants[0]?.id ?? "";
  const [start, setStart] = useState(today());
  const [cls, setCls] = useState<LicenceClass>("COLOUR");
  const [concession, setConcession] = useState<Concession>("NONE");

  // Price it before anybody commits to it. An unrecorded rate or an
  // unpublished discount is a refusal, not a plausible number, so the reason
  // is shown here instead of appearing after the licence is issued.
  const [quote, setQuote] = useState<Quote | null>(null);
  const [quoteProblem, setQuoteProblem] = useState("");
  useEffect(() => {
    let cancelled = false;
    setQuoteProblem("");
    api
      .quote(cls, concession, start)
      .then((q) => {
        if (!cancelled) setQuote(q);
      })
      .catch((e: unknown) => {
        if (cancelled) return;
        setQuote(null);
        setQuoteProblem(asApiError(e).message);
      });
    return () => {
      cancelled = true;
    };
  }, [cls, concession, start]);

  const guard = async (fn: () => Promise<void>) => {
    setBusy(true);
    setError(null);
    try {
      await fn();
    } catch (e) {
      setError(asApiError(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Section title="What do you need to do?" icon="check" hue="mint">
      <ErrorSummary error={error} />

      {!detail.summary.licensed && (
        <form
          className="action-form"
          onSubmit={(e) => {
            e.preventDefault();
            void guard(async () => {
              const r = await api.issueLicence({
                holder,
                premises: detail.summary.id,
                class: cls,
                concession,
                start,
              });
              onDone(`Licence ${r.licence.number} issued.`);
            });
          }}
        >
          <h3>Issue a licence</h3>
          <div className="action-row">
            <Field label="Licence type">
              <select value={cls} onChange={(e) => setCls(e.target.value as typeof cls)}>
                <option value="COLOUR">Standard</option>
                <option value="MONOCHROME">Black and white</option>
                <option value="ARC">Care home resident</option>
              </select>
            </Field>
            <Field label="Discount">
              <select
                value={concession}
                onChange={(e) => setConcession(e.target.value as typeof concession)}
              >
                <option value="NONE">None</option>
                <option value="BLIND_50">Sight impaired — half price</option>
                <option value="OVER_75_PENSION_CREDIT">Over 75 on Pension Credit — free</option>
              </select>
            </Field>
            <Field label="Start date">
              <input type="date" value={start} onChange={(e) => setStart(e.target.value)} />
            </Field>
            <button className="button" disabled={busy || !holder || !quote}>
              Issue licence
            </button>
          </div>

          {quote && (
            <p className="quote-inline">
              Fee: <strong>{money(quote.payable)}</strong>
              {quote.payable !== quote.base && (
                <>
                  {" "}
                  — full price {money(quote.base)}, discount {money(quote.base - quote.payable)}
                </>
              )}
            </p>
          )}
          {quoteProblem && (
            <p className="inline-problem" role="alert">
              {quoteProblem}
            </p>
          )}

          {!holder && <p className="field-hint">Record an occupier before issuing a licence.</p>}
        </form>
      )}

      <div className="action-form">
        <h3>Record a declaration</h3>
        <div className="action-row">
          <button
            className="button button-secondary"
            disabled={busy || !holder}
            onClick={() =>
              void guard(async () => {
                await api.declare(detail.summary.id, "NO_LICENCE_NEEDED", holder, today());
                onDone("Recorded that no licence is needed.");
              })
            }
          >
            No licence needed
          </button>
          <button
            className="button button-secondary"
            disabled={busy || !holder}
            onClick={() =>
              void guard(async () => {
                await api.declare(detail.summary.id, "ACCESS_WITHDRAWN", holder, today());
                onDone("Recorded that permission to call has been withdrawn.");
              })
            }
          >
            Occupier withdraws permission to call
          </button>
        </div>
      </div>
    </Section>
  );
}

function nameOf(p: { title?: string; forename?: string; surname?: string; organisation?: string }) {
  if (p.organisation) return p.organisation;
  return [p.title, p.forename, p.surname].filter(Boolean).join(" ");
}

export function declarationWords(kind: Declaration["kind"]): string {
  switch (kind) {
    case "NO_LICENCE_NEEDED":
      return "Occupier says no licence is needed";
    case "LEGALLY_LICENCE_FREE":
      return "Confirmed no licence is needed";
    case "ACCESS_WITHDRAWN":
      return "Permission to call withdrawn";
    default:
      return kind;
  }
}

function declarationStatus(d: Declaration) {
  if (d.revokedOn) return <Tag>Withdrawn {longDate(d.revokedOn)}</Tag>;
  if (d.kind === "ACCESS_WITHDRAWN") return <Tag tone="stop">In force, does not expire</Tag>;
  if (d.reviewDue && d.reviewDue <= today())
    return <Tag tone="warning">Due to be checked</Tag>;
  if (d.reviewDue) return <Tag tone="good">Valid, check by {longDate(d.reviewDue)}</Tag>;
  return <Tag tone="good">In force</Tag>;
}
