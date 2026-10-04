import { useEffect, useState } from "react";
import {
  api,
  asApiError,
  type ApiError,
  longDate,
  money,
  today,
  type Concession,
  type LicenceClass,
  type Plan,
  type Quote,
  type RateCoverage,
} from "../api";
import { Empty, ErrorSummary, Field, Loading, PageHeader, Section, SummaryList } from "../ui";

const CLASS_WORDS: Record<LicenceClass, string> = {
  COLOUR: "Standard",
  MONOCHROME: "Black and white",
  ARC: "Care home resident",
};

const PLANS = [
  { value: "ANNUAL", label: "All at once" },
  { value: "QUARTERLY_DD", label: "Every 3 months by Direct Debit" },
  { value: "MONTHLY_DD", label: "Monthly by Direct Debit" },
  { value: "FORTNIGHTLY_CASH", label: "Every 2 weeks by payment card" },
  { value: "WEEKLY_CASH", label: "Weekly by payment card" },
];

export function Fees() {
  const [cls, setCls] = useState<LicenceClass>("COLOUR");
  const [concession, setConcession] = useState<Concession>("NONE");
  // Today, not a date hard-coded when the page was written: the fee somebody
  // is asking about is almost always the one in force now.
  const [on, setOn] = useState(today());
  const [quote, setQuote] = useState<Quote | null>(null);
  const [quoteError, setQuoteError] = useState<ApiError | null>(null);

  const [planKind, setPlanKind] = useState("MONTHLY_DD");
  const [plan, setPlan] = useState<Plan | null>(null);

  const [coverage, setCoverage] = useState<RateCoverage[] | null>(null);

  useEffect(() => {
    setQuoteError(null);
    api
      .quote(cls, concession, on)
      .then(setQuote)
      .catch((e: unknown) => {
        setQuote(null);
        setQuoteError(asApiError(e));
      });
  }, [cls, concession, on]);

  useEffect(() => {
    api.coverage().then(setCoverage).catch(() => setCoverage([]));
  }, []);

  useEffect(() => {
    if (!quote) {
      setPlan(null);
      return;
    }
    const pounds = (quote.payable / 100).toFixed(2);
    api
      .plan(planKind, pounds, on)
      .then(setPlan)
      .catch(() => setPlan(null));
  }, [quote, planKind, on]);

  return (
    <>
      <PageHeader title="Fees and payment plans" lede="What a licence costs, and how it can be paid." />

      <Section title="Work out a fee" icon="money" hue="teal">
        <div className="action-row">
          <Field label="Licence type">
            <select value={cls} onChange={(e) => setCls(e.target.value as LicenceClass)}>
              {Object.entries(CLASS_WORDS).map(([k, v]) => (
                <option key={k} value={k}>
                  {v}
                </option>
              ))}
            </select>
          </Field>
          <Field label="Discount">
            <select value={concession} onChange={(e) => setConcession(e.target.value as Concession)}>
              <option value="NONE">None</option>
              <option value="BLIND_50">Sight impaired — half price</option>
              <option value="OVER_75_PENSION_CREDIT">Over 75 on Pension Credit — free</option>
            </select>
          </Field>
          <Field label="Date the licence starts">
            <input type="date" value={on} onChange={(e) => setOn(e.target.value)} />
          </Field>
        </div>

        <ErrorSummary error={quoteError} />

        {quote && (
          <div className="quote-result">
            <p className="quote-amount">{money(quote.payable)}</p>
            <p className="quote-working">
              Full price {money(quote.base)}
              {quote.base !== quote.payable && <> · discount {money(quote.base - quote.payable)}</>}
            </p>
          </div>
        )}
      </Section>

      {quote && (
        <Section title="Ways to pay" icon="card" hue="sky">
          <div className="action-row">
            <Field label="Payment plan">
              <select value={planKind} onChange={(e) => setPlanKind(e.target.value)}>
                {PLANS.map((p) => (
                  <option key={p.value} value={p.value}>
                    {p.label}
                  </option>
                ))}
              </select>
            </Field>
          </div>

          {plan === null ? (
            <Empty>This plan is not available for this licence.</Empty>
          ) : (
            <>
              <SummaryList
                rows={[
                  { key: "Number of payments", value: String(plan.instalments.length) },
                  {
                    key: "Each payment",
                    value: largestAndSmallest(plan),
                  },
                  { key: "First payment", value: longDate(plan.instalments[0]?.due) },
                  { key: "Total", value: money(plan.total) },
                ]}
              />
              <details className="details">
                <summary>See every payment date</summary>
                <table className="data-table">
                  <thead>
                    <tr>
                      <th>Payment</th>
                      <th>Date</th>
                      <th className="numeric">Amount</th>
                    </tr>
                  </thead>
                  <tbody>
                    {plan.instalments.map((i) => (
                      <tr key={i.seq}>
                        <td>{i.seq}</td>
                        <td>{longDate(i.due)}</td>
                        <td className="numeric">{money(i.amount)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </details>
            </>
          )}
        </Section>
      )}

      <Section title="Fees held on record" icon="log" hue="sand">
        {coverage === null ? (
          <Loading />
        ) : (
          <table className="data-table">
            <thead>
              <tr>
                <th>Licence type</th>
                <th>Earliest fee held</th>
                <th className="numeric">Rates on record</th>
                <th>A fee applies from now on</th>
              </tr>
            </thead>
            <tbody>
              {coverage.map((c) => (
                <tr key={c.class}>
                  <td>{CLASS_WORDS[c.class] ?? c.class}</td>
                  <td>{longDate(c.earliestKnown)}</td>
                  <td className="numeric">{c.recordCount}</td>
                  <td>{c.hasOpenEnded ? "Yes" : <strong>No — the data stops</strong>}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        <p className="field-hint">
          Dates before these cannot be quoted. Aerial will say so rather than give a figure it
          cannot stand behind.
        </p>
      </Section>
    </>
  );
}

function largestAndSmallest(plan: Plan): string {
  const amounts = plan.instalments.map((i) => i.amount);
  const max = Math.max(...amounts);
  const min = Math.min(...amounts);
  return max === min ? money(max) : `${money(max)}, last one ${money(min)}`;
}
