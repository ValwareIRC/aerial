import { useCallback, useEffect, useState, type FormEvent } from "react";
import { Link, useParams } from "react-router-dom";
import {
  api,
  asApiError,
  type ApiError,
  longDate,
  ops,
  today,
  type EnforcementView,
  type Reference,
  type Visit,
  type WorklistItem,
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
} from "../ui";

export function Worklist() {
  const [term, setTerm] = useState("");
  const [rows, setRows] = useState<WorklistItem[] | null>(null);
  const [error, setError] = useState<ApiError | null>(null);

  const run = (value: string) => {
    api
      .worklist(value)
      .then(setRows)
      .catch((e: unknown) => setError(asApiError(e)));
  };

  useEffect(() => run(""), []);

  if (error) return <ErrorSummary error={error} />;
  if (!rows) return <Loading />;

  const doNotVisit = rows.filter((r) => !r.visitPermitted);
  const toCall = rows.filter((r) => r.visitPermitted && r.status === "No licence");
  const rest = rows.filter((r) => r.visitPermitted && r.status !== "No licence");

  return (
    <>
      <PageHeader title="Visit list" lede="Addresses on your round. Names are not shown here." />

      <form
        className="search-form"
        role="search"
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          run(term);
        }}
      >
        <Field label="Search by address or postcode">
          <input value={term} onChange={(e) => setTerm(e.target.value)} placeholder="e.g. NE8" />
        </Field>
        <button className="button" type="submit">
          Search
        </button>
      </form>

      {doNotVisit.length > 0 && (
        <Section title="Do not call at these addresses" icon="shield" hue="rose">
          <p className="table-note">
            The occupier has withdrawn permission to call. This does not expire.
          </p>
          <VisitTable rows={doNotVisit} />
        </Section>
      )}

      <Section title="No licence recorded" icon="alert" hue="peach">
        {toCall.length === 0 ? <Empty>Nothing outstanding.</Empty> : <VisitTable rows={toCall} />}
      </Section>

      <Section title="Everything else" icon="pin" hue="mint">
        {rest.length === 0 ? <Empty>Nothing else on your list.</Empty> : <VisitTable rows={rest} />}
      </Section>
    </>
  );
}

function VisitTable({ rows }: { rows: WorklistItem[] }) {
  return (
    <table className="data-table">
      <thead>
        <tr>
          <th>Address</th>
          <th>Licence</th>
          <th>Call permitted</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((r) => (
          <tr key={r.id}>
            <td>
              <Link to={`/worklist/${r.id}`}>{r.addressLine}</Link>
            </td>
            <td>
              <Tag
                tone={r.status === "Licensed" ? "good" : r.status === "No licence" ? "stop" : "info"}
              >
                {r.status}
              </Tag>
              {r.dueForReview && <span className="cell-sub">due to be checked</span>}
            </td>
            <td>{r.visitPermitted ? "Yes" : <strong>No</strong>}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

export function VisitRecord() {
  const { id = "" } = useParams();
  const [view, setView] = useState<EnforcementView | null>(null);
  const [visits, setVisits] = useState<Visit[]>([]);
  const [reference, setReference] = useState<Reference | null>(null);
  const [address, setAddress] = useState("");
  const [error, setError] = useState<ApiError | null>(null);
  const [notice, setNotice] = useState("");

  const [outcome, setOutcome] = useState("NO_ANSWER");
  const [note, setNote] = useState("");
  const [on, setOn] = useState(today());

  const load = useCallback(() => {
    api
      .enforcementView(id)
      .then(setView)
      .catch((e: unknown) => setError(asApiError(e)));
    ops.visits(id).then(setVisits).catch(() => setVisits([]));
  }, [id]);

  useEffect(load, [load]);
  useEffect(() => {
    ops.reference().then(setReference).catch(() => setReference(null));
    api
      .worklist("")
      .then((rows) => setAddress(rows.find((r) => r.id === id)?.addressLine ?? id))
      .catch(() => setAddress(id));
  }, [id]);

  if (error && !view) return <ErrorSummary error={error} />;
  if (!view || !reference) return <Loading />;

  const occupants = view.occupantTokens?.length ?? 0;

  return (
    <>
      <PageHeader overline="Address" title={address} />

      {notice && <Banner tone="good" title={notice} />}
      <ErrorSummary error={error} />

      {!view.visitPermitted && (
        <Banner tone="stop" title="Do not call at this address">
          The occupier has withdrawn permission to call here. This does not expire, and a visit
          cannot be recorded.
        </Banner>
      )}

      {view.noLicenceDeclared && (
        <Banner tone="info" title="The occupier has said no licence is needed">
          {view.dueForReview
            ? "This is still valid and is due to be checked. It is not a reason to call."
            : "This is valid. It is not a reason to call."}
        </Banner>
      )}

      <Section title="What we know" icon="overview" hue="sky">
        <SummaryList
          rows={[
            {
              key: "Licence",
              value: view.licensed ? (
                <>
                  <Tag tone="good">Licensed</Tag> until {longDate(view.licenceExpiry)}
                </>
              ) : view.noLicenceDeclared ? (
                <Tag tone="info">Occupier says none is needed</Tag>
              ) : (
                <Tag tone="stop">No licence recorded</Tag>
              ),
            },
            { key: "Call permitted", value: view.visitPermitted ? "Yes" : "No" },
            {
              key: "People recorded here",
              value: occupants === 1 ? "1 person" : `${occupants} people`,
            },
          ]}
        />
        <p className="field-hint">Names and licence numbers are not shown to visiting officers.</p>
      </Section>

      <Section title="Previous calls" icon="clock" hue="lilac">
        {visits.length === 0 ? (
          <Empty>No calls recorded here.</Empty>
        ) : (
          <table className="data-table">
            <thead>
              <tr>
                <th>Date</th>
                <th>What happened</th>
                <th>Officer</th>
                <th>Note</th>
              </tr>
            </thead>
            <tbody>
              {visits.map((v) => (
                <tr key={v.id}>
                  <td>{longDate(v.on)}</td>
                  <td>
                    {reference.visitOutcomes.find((o) => o.value === v.outcome)?.label ?? v.outcome}
                  </td>
                  <td>{v.officer}</td>
                  <td>{v.note}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Section>

      {view.visitPermitted && (
        <Section title="Record what happened" icon="note" hue="mint">
          <form
            className="action-form"
            onSubmit={(e) => {
              e.preventDefault();
              setError(null);
              ops
                .recordVisit({ premises: id, on, outcome, note })
                .then(() => {
                  setNote("");
                  setNotice("Call recorded.");
                  load();
                })
                .catch((err: unknown) =>
                  setError(asApiError(err)),
                );
            }}
          >
            <div className="action-row">
              <Field label="Date">
                <input type="date" value={on} onChange={(e) => setOn(e.target.value)} />
              </Field>
              <Field label="What happened">
                <select value={outcome} onChange={(e) => setOutcome(e.target.value)}>
                  {reference.visitOutcomes.map((o) => (
                    <option key={o.value} value={o.value}>
                      {o.label}
                    </option>
                  ))}
                </select>
              </Field>
            </div>
            <Field label="Note" hint="Optional. Cannot be changed once saved.">
              <textarea value={note} onChange={(e) => setNote(e.target.value)} rows={2} />
            </Field>
            <button className="button">Record call</button>
          </form>
        </Section>
      )}
    </>
  );
}
