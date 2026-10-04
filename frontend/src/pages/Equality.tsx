import { useEffect, useState } from "react";
import { asApiError, ops, type ApiError, type EqualityResponse, type Reference } from "../api";
import { Banner, Empty, ErrorSummary, Field, Loading, PageHeader, Section } from "../ui";

export function Equality() {
  const [outcome, setOutcome] = useState("NO_ANSWER");
  const [data, setData] = useState<EqualityResponse | null>(null);
  const [reference, setReference] = useState<Reference | null>(null);
  const [error, setError] = useState<ApiError | null>(null);
  const [sent, setSent] = useState("");

  useEffect(() => {
    ops.reference().then(setReference).catch(() => setReference(null));
  }, []);

  useEffect(() => {
    ops
      .equality("ageBand", outcome)
      .then(setData)
      .catch((e: unknown) => setError(asApiError(e)));
  }, [outcome]);

  if (error) return <ErrorSummary error={error} />;
  if (!data || !reference) return <Loading />;

  const { report, rates } = data;

  return (
    <>
      <PageHeader
        title="Outcomes by age"
        lede="How visits end, broken down by the age of the person at the address."
      />

      <Banner
        tone="info"
        title={`Groups of fewer than ${report.k} are not shown`}
      >
        {report.suppressedCells > 0
          ? `${report.suppressedCells} ${report.suppressedCells === 1 ? "group is" : "groups are"} too small to publish, covering ${report.suppressedCount} ${report.suppressedCount === 1 ? "call" : "calls"}.`
          : "Nothing has been held back."}
      </Banner>

      <Section title="Choose an outcome" icon="chart" hue="rose">
        <div className="action-row">
          <Field label="Outcome">
            <select value={outcome} onChange={(e) => setOutcome(e.target.value)}>
              {reference.visitOutcomes.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
          </Field>
        </div>
      </Section>

      <Section title="Share of calls ending this way" icon="chart" hue="sky">
        {!rates || rates.length === 0 ? (
          <Empty>
            Not enough calls recorded to publish a breakdown. {report.totalCount} recorded so far.
          </Empty>
        ) : (
          <table className="data-table">
            <thead>
              <tr>
                <th>Age group</th>
                <th className="numeric">This outcome</th>
                <th className="numeric">All calls</th>
                <th className="numeric">Share</th>
                <th>Compared</th>
              </tr>
            </thead>
            <tbody>
              {rates.map((r) => (
                <tr key={r.value}>
                  <td>{r.value}</td>
                  <td className="numeric">{r.count}</td>
                  <td className="numeric">{r.total}</td>
                  <td className="numeric">{Math.round(r.proportion * 100)}%</td>
                  <td>
                    <div className="bar" aria-hidden="true">
                      <span style={{ width: `${Math.round(r.proportion * 100)}%` }} />
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Section>

      <Section title="Renewal reminders" icon="mail" hue="mint">
        <p className="field-hint">
          Writes to everyone whose licence expires within a month.
        </p>
        {sent && <Banner tone="good" title={sent} />}
        <button
          className="button"
          onClick={() =>
            ops
              .sendRenewalReminders()
              .then((r) => setSent(`${r.sent} ${r.sent === 1 ? "reminder" : "reminders"} sent.`))
              .catch((e: unknown) =>
                setError(asApiError(e)),
              )
          }
        >
          Send reminders
        </button>
      </Section>
    </>
  );
}
