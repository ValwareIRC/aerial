import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import {
  api,
  asApiError,
  longDate,
  money,
  ops,
  today,
  type ApiError,
  type CancellationReason,
  type LicenceDetail,
  type LicenceSummary,
  type PremisesSummary,
  type Reference,
} from "../api";
import { AddressPicker, DangerAction } from "../fields";
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

export function LicenceList() {
  const [rows, setRows] = useState<LicenceSummary[] | null>(null);
  const [error, setError] = useState<ApiError | null>(null);

  useEffect(() => {
    api
      .licences()
      .then(setRows)
      .catch((e: unknown) => setError(asApiError(e)));
  }, []);

  if (error) return <ErrorSummary error={error} />;
  if (!rows) return <Loading />;

  return (
    <>
      <PageHeader title="Licences" lede={`${rows.length} on record.`} />
      {rows.length === 0 ? (
        <Empty>No licences have been issued.</Empty>
      ) : (
        <table className="data-table">
          <thead>
            <tr>
              <th>Number</th>
              <th>Holder</th>
              <th>Address</th>
              <th>Type</th>
              <th>Expires</th>
              <th>Status</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((l) => (
              <tr key={l.id}>
                <td>
                  <Link to={`/licences/${l.id}`}>{l.number}</Link>
                </td>
                <td>{l.holderName}</td>
                <td>{l.addressLine}</td>
                <td>{l.classLabel}</td>
                <td>{longDate(l.expiry)}</td>
                <td>
                  <Tag tone={statusTone(l.status)}>{statusWords(l.status)}</Tag>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  );
}

export function LicenceRecord() {
  const { id = "" } = useParams();
  const [data, setData] = useState<LicenceDetail | null>(null);
  const [error, setError] = useState<ApiError | null>(null);
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [moveTo, setMoveTo] = useState<PremisesSummary | null>(null);
  const [moveOn, setMoveOn] = useState(today());
  const [reason, setReason] = useState<CancellationReason>("NO_LONGER_WATCHING");
  const [reference, setReference] = useState<Reference | null>(null);

  const load = () => {
    api
      .licence(id)
      .then(setData)
      .catch((e: unknown) => setError(asApiError(e)));
  };

  useEffect(load, [id]);
  useEffect(() => {
    ops.reference().then(setReference).catch(() => setReference(null));
  }, []);

  if (error && !data) return <ErrorSummary error={error} />;
  if (!data) return <Loading />;

  const { summary } = data;
  // The refund for the reason actually chosen, priced by the server.
  const refund = data.refunds[reason] ?? data.refund;

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
    <>
      <PageHeader overline="Licence" title={summary.number} lede={summary.addressLine} />

      {notice && <Banner tone="good" title={notice} />}
      <ErrorSummary error={error} />

      <Section title="Details" icon="licence" hue="lilac">
        <SummaryList
          rows={[
            { key: "Holder", value: summary.holderName },
            { key: "Address", value: <Link to={`/addresses/${summary.premises}`}>{summary.addressLine}</Link> },
            { key: "Type", value: summary.classLabel },
            { key: "Discount", value: concessionWords(summary.concession) },
            { key: "Period", value: `${longDate(summary.start)} to ${longDate(summary.expiry)}` },
            { key: "Paid", value: money(summary.feePaid) },
            { key: "Status", value: <Tag tone={statusTone(summary.status)}>{statusWords(summary.status)}</Tag> },
          ]}
        />
      </Section>

      {summary.status === "ACTIVE" && (
        <Section title="What do you need to do?" icon="check" hue="mint">
          <div className="action-form">
            <h3>The holder is moving house</h3>
            <p className="field-hint">
              The licence moves with them and keeps the same expiry date. Nothing is refunded and
              nothing is re-charged.
            </p>
            <AddressPicker label="Where are they moving to?" value={moveTo} onChange={setMoveTo} />
            <div className="action-row">
              <Field label="Moving date">
                <input type="date" value={moveOn} onChange={(e) => setMoveOn(e.target.value)} />
              </Field>
            </div>
            {moveTo && (
              <DangerAction
                actionLabel="Move licence"
                heading="Move this licence?"
                consequences={[
                  <>It stops covering {summary.addressLine} from {longDate(moveOn)}.</>,
                  <>It covers {moveTo.addressLine} from that date until {longDate(summary.expiry)}.</>,
                  "Nothing is refunded and nothing is re-charged.",
                ]}
                busy={busy}
                onConfirm={() =>
                  void guard(async () => {
                    await api.transferLicence(id, moveTo.id, moveOn);
                    setMoveTo(null);
                    setNotice("Licence moved to the new address.");
                    load();
                  })
                }
              />
            )}
          </div>

          <div className="action-form">
            <h3>Cancel this licence</h3>
            <p className="field-hint">
              Complete months left are refunded; a part month is not. A licence issued in error
              refunds the whole fee.
            </p>
            <div className="action-row">
              <Field label="Why are they cancelling?">
                <select
                  value={reason}
                  onChange={(e) => setReason(e.target.value as CancellationReason)}
                >
                  {(reference?.cancellationReasons ?? []).map((r) => (
                    <option key={r.value} value={r.value}>
                      {r.label}
                    </option>
                  ))}
                </select>
              </Field>
            </div>
            <p className="field-hint">
              Refund for that reason, today: <strong>{money(refund)}</strong>.
            </p>
            <DangerAction
              actionLabel="Cancel licence"
              heading={`Cancel licence ${summary.number}?`}
              consequences={[
                <>It stops covering {summary.addressLine} from today.</>,
                <>
                  A refund of <strong>{money(refund)}</strong> becomes due.
                </>,
                <>
                  Reason recorded:{" "}
                  {reference?.cancellationReasons.find((r) => r.value === reason)?.label}.
                </>,
                "This can be reversed afterwards if it was recorded in error.",
              ]}
              busy={busy || !reference}
              onConfirm={() =>
                void guard(async () => {
                  const r = await ops.cancel(id, today(), reason);
                  setNotice(`Licence cancelled. Refund due: ${money(r.refund)}.`);
                  load();
                })
              }
            />
          </div>
        </Section>
      )}

      {summary.status === "CANCELLED" && (
        <Section title="Cancelled in error?" icon="undo" hue="sun">
          <div className="action-form">
            <p className="field-hint">
              Putting the licence back adds a correction to the record. Nothing is deleted.
            </p>
            <DangerAction
              actionLabel="Put the licence back"
              heading={`Reinstate licence ${summary.number}?`}
              consequences={[
                "The licence goes back in force for the rest of its term.",
                "Any refund already recorded is reversed.",
                "A correction note is added to the record.",
              ]}
              busy={busy}
              onConfirm={() =>
                void guard(async () => {
                  await ops.reinstate(id, today());
                  setNotice("Licence put back in force.");
                  load();
                })
              }
            />
          </div>
        </Section>
      )}
    </>
  );
}

export function concessionWords(c: string): string {
  switch (c) {
    case "NONE":
      return "None";
    case "BLIND_50":
      return "Sight impaired — half price";
    case "OVER_75_PENSION_CREDIT":
      return "Over 75 on Pension Credit — free";
    default:
      return c;
  }
}
