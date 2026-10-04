import { useEffect, useState } from "react";
import { api, asApiError, type ApiError, type AuditPage } from "../api";
import { Empty, ErrorSummary, Loading, PageHeader, Tag } from "../ui";
import { roleWords } from "../Shell";

const ACTION_WORDS: Record<string, string> = {
  "session.sign_in": "Signed in",
  "enforcement.snapshot": "Looked up an address",
  "enforcement.visit_check": "Checked whether a call is permitted",
  "service.party_history": "Looked up a person's history",
};

function actionWords(action: string): string {
  if (ACTION_WORDS[action]) return ACTION_WORDS[action];
  if (action.startsWith("http.")) return `Tried to open ${action.split(" ").pop()}`;
  return action;
}

const PAGE_SIZE = 200;

export function AccessLog() {
  const [page, setPage] = useState<AuditPage | null>(null);
  const [error, setError] = useState<ApiError | null>(null);
  const [onlyRefused, setOnlyRefused] = useState(false);

  // The server filters and orders the log. Narrowing to refusals asks a
  // narrower question rather than downloading everything and hiding most of
  // it, which matters once a log is a day old.
  useEffect(() => {
    setError(null);
    api
      .audit({ refused: onlyRefused, limit: PAGE_SIZE })
      .then(setPage)
      .catch((e: unknown) => setError(asApiError(e)));
  }, [onlyRefused]);

  if (error) return <ErrorSummary error={error} />;
  if (!page) return <Loading />;

  const shown = page.entries ?? [];

  return (
    <>
      <PageHeader title="Access log" lede="Every record anyone opened, and everything refused." />

      <label className="checkbox">
        <input
          type="checkbox"
          checked={onlyRefused}
          onChange={(e) => setOnlyRefused(e.target.checked)}
        />
        Show only refused attempts
      </label>

      <p className="result-count">
        {page.matched === page.total
          ? `${page.total} ${page.total === 1 ? "entry" : "entries"}`
          : `${page.matched} of ${page.total} entries match`}
        {page.returned < page.matched && <> · showing the most recent {page.returned}</>}
      </p>

      {shown.length === 0 ? (
        <Empty>Nothing recorded.</Empty>
      ) : (
        <table className="data-table">
          <thead>
            <tr>
              <th>Time</th>
              <th>Who</th>
              <th>What they did</th>
              <th>Record</th>
              <th>Result</th>
            </tr>
          </thead>
          <tbody>
            {shown.map((e, i) => (
              <tr key={`${e.at}-${i}`}>
                <td>{new Date(e.at).toLocaleString("en-GB")}</td>
                <td>
                  {e.actor.id}
                  <span className="cell-sub">{roleWords(e.actor.role)}</span>
                </td>
                <td>{actionWords(e.action)}</td>
                <td>{e.subject}</td>
                <td>
                  {e.allowed ? <Tag tone="good">Allowed</Tag> : <Tag tone="stop">Refused</Tag>}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  );
}
