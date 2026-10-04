import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import {
  api,
  asApiError,
  longDate,
  type ApiError,
  type Overview as OverviewData,
  type PremisesSummary,
} from "../api";
import { Empty, ErrorSummary, Loading, PageHeader, Section, Tag } from "../ui";

export function Overview() {
  const [data, setData] = useState<OverviewData | null>(null);
  const [error, setError] = useState<ApiError | null>(null);

  // One request. The server decides what "outstanding" means and sends the
  // addresses behind each count, so the page is not re-deriving the rules
  // from a download of the whole register.
  useEffect(() => {
    api
      .overview()
      .then(setData)
      .catch((e: unknown) => setError(asApiError(e)));
  }, []);

  if (error) return <ErrorSummary error={error} />;
  if (!data) return <Loading />;

  return (
    <>
      <PageHeader title="Overview" lede="Addresses on this caseload today." />

      <div className="stat-strip">
        <Figure label="Addresses" value={data.premises} />
        <Figure label="Licensed" value={data.licensed} />
        <Figure label="No licence needed" value={data.noLicenceNeeded} />
        <Figure label="No licence" value={data.unlicensed} emphasis={data.unlicensed > 0} />
      </div>

      {data.expiringSoon > 0 && (
        <p className="result-count">
          {data.expiringSoon} {data.expiringSoon === 1 ? "licence expires" : "licences expire"} within
          two months.
        </p>
      )}

      <Section title="No licence and none declared" icon="alert" hue="peach">
        {data.noLicenceList.length === 0 ? (
          <Empty>Nothing outstanding.</Empty>
        ) : (
          <AddressTable rows={data.noLicenceList} />
        )}
      </Section>

      <Section title="Declarations due to be checked" icon="clock" hue="sun">
        {data.reviewList.length === 0 ? (
          <Empty>No declarations are due.</Empty>
        ) : (
          <AddressTable rows={data.reviewList} note="Still valid. Confirm when convenient." />
        )}
      </Section>

      {data.doNotVisitList.length > 0 && (
        <Section title="Do not visit" icon="shield" hue="rose">
          <AddressTable
            rows={data.doNotVisitList}
            note="The occupier has withdrawn permission to call."
          />
        </Section>
      )}
    </>
  );
}

function Figure({ label, value, emphasis }: { label: string; value: number; emphasis?: boolean }) {
  return (
    <div className={`figure-block${emphasis ? " figure-emphasis" : ""}`}>
      <span className="figure-value">{value}</span>
      <span className="figure-label">{label}</span>
    </div>
  );
}

function AddressTable({ rows, note }: { rows: PremisesSummary[]; note?: string }) {
  return (
    <>
      {note && <p className="table-note">{note}</p>}
      <table className="data-table">
        <thead>
          <tr>
            <th>Address</th>
            <th>Occupier</th>
            <th>Licence</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((p) => (
            <tr key={p.id}>
              <td>
                <Link to={`/addresses/${p.id}`}>{p.addressLine}</Link>
              </td>
              <td>{p.occupier || "Not recorded"}</td>
              <td>
                {p.licensed ? (
                  <>
                    <Tag tone="good">Licensed</Tag>
                    <span className="cell-sub">to {longDate(p.expiry)}</span>
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
  );
}
