import { useEffect, useRef, type ReactNode } from "react";
import { Link } from "react-router-dom";
import { ApiError } from "./api";
import { IconChip, type Hue } from "./icons";

const SERVICE_NAME = "Aerial — licence administration";

/**
 * Names the tab after the page. In a single-page app the title otherwise
 * never changes, which leaves a screen reader announcing nothing on
 * navigation and makes a browser history list useless.
 */
export function usePageTitle(title: string | undefined) {
  useEffect(() => {
    document.title = title ? `${title} — ${SERVICE_NAME}` : SERVICE_NAME;
  }, [title]);
}

export function PageHeader({
  overline,
  title,
  lede,
  actions,
}: {
  overline?: string;
  title: string;
  lede?: string;
  actions?: ReactNode;
}) {
  const heading = useRef<HTMLHeadingElement>(null);
  usePageTitle(title);

  useEffect(() => {
    // Move focus to the heading when the page changes, so keyboard and
    // screen-reader users start at the top of the new record rather than
    // wherever the link they followed happened to be.
    heading.current?.focus();
  }, [title]);

  return (
    <div className="page-header">
      <div>
        {overline && <p className="overline">{overline}</p>}
        <h1 ref={heading} tabIndex={-1}>
          {title}
        </h1>
        {lede && <p className="lede">{lede}</p>}
      </div>
      {actions && <div className="page-actions">{actions}</div>}
    </div>
  );
}

export function Section({
  title,
  icon,
  hue = "sky",
  action,
  children,
}: {
  title: string;
  icon?: string;
  hue?: Hue;
  action?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section className="section">
      <div className="section-head">
        <h2>
          {icon && <IconChip name={icon} hue={hue} size={28} />}
          {title}
        </h2>
        {action}
      </div>
      {children}
    </section>
  );
}

export interface Row {
  key: string;
  value: ReactNode;
}

export function SummaryList({ rows }: { rows: Row[] }) {
  return (
    <dl className="summary-list">
      {rows.map((r) => (
        <div key={r.key} className="summary-row">
          <dt>{r.key}</dt>
          <dd>{r.value}</dd>
        </div>
      ))}
    </dl>
  );
}

export type Tone = "neutral" | "good" | "warning" | "stop" | "info";

export function Tag({ tone = "neutral", children }: { tone?: Tone; children: ReactNode }) {
  return <span className={`tag tag-${tone}`}>{children}</span>;
}

export function Banner({ tone = "info", title, children }: { tone?: Tone; title: string; children?: ReactNode }) {
  return (
    <div className={`banner banner-${tone}`} role={tone === "stop" ? "alert" : undefined}>
      <p className="banner-title">{title}</p>
      {children && <div className="banner-body">{children}</div>}
    </div>
  );
}

export function ErrorSummary({ error }: { error: ApiError | Error | null }) {
  const box = useRef<HTMLDivElement>(null);
  const message = error?.message ?? "";

  useEffect(() => {
    // Take focus when a problem appears, which is what makes an error
    // summary more than decoration: the next thing read is the problem.
    if (message) box.current?.focus();
  }, [message]);

  if (!error) return null;
  const reason = error instanceof ApiError ? error.reason : "";
  return (
    <div className="error-summary" role="alert" tabIndex={-1} ref={box}>
      <h2>There is a problem</h2>
      <p>{error.message}</p>
      {reason && <p className="error-detail">{reason}</p>}
    </div>
  );
}

export interface TabDef<T extends string> {
  id: T;
  label: string;
}

/**
 * The ARIA tab pattern: one stop in the tab order, left and right to move
 * between tabs, Home and End to jump. Without the keyboard handling, a
 * tablist built from buttons is harder to use than the links it replaced.
 */
export function Tabs<T extends string>({
  name,
  tabs,
  active,
  onChange,
}: {
  name: string;
  tabs: TabDef<T>[];
  active: T;
  onChange: (id: T) => void;
}) {
  const list = useRef<HTMLDivElement>(null);

  const move = (delta: number) => {
    const index = tabs.findIndex((t) => t.id === active);
    const next = tabs[(index + delta + tabs.length) % tabs.length];
    if (next) {
      onChange(next.id);
      list.current?.querySelector<HTMLButtonElement>(`#${tabId(name, next.id)}`)?.focus();
    }
  };

  const jump = (to: TabDef<T> | undefined) => {
    if (!to) return;
    onChange(to.id);
    list.current?.querySelector<HTMLButtonElement>(`#${tabId(name, to.id)}`)?.focus();
  };

  return (
    <div className="tabs" role="tablist" ref={list} aria-label={`${name} sections`}>
      {tabs.map((t) => (
        <button
          key={t.id}
          id={tabId(name, t.id)}
          type="button"
          role="tab"
          aria-selected={t.id === active}
          aria-controls={panelId(name, t.id)}
          tabIndex={t.id === active ? 0 : -1}
          className={`tab${t.id === active ? " tab-active" : ""}`}
          onClick={() => onChange(t.id)}
          onKeyDown={(e) => {
            switch (e.key) {
              case "ArrowRight":
                e.preventDefault();
                move(1);
                break;
              case "ArrowLeft":
                e.preventDefault();
                move(-1);
                break;
              case "Home":
                e.preventDefault();
                jump(tabs[0]);
                break;
              case "End":
                e.preventDefault();
                jump(tabs[tabs.length - 1]);
                break;
            }
          }}
        >
          {t.label}
        </button>
      ))}
    </div>
  );
}

export function TabPanel({
  name,
  id,
  children,
}: {
  name: string;
  id: string;
  children: ReactNode;
}) {
  return (
    <div role="tabpanel" id={panelId(name, id)} aria-labelledby={tabId(name, id)} tabIndex={0}>
      {children}
    </div>
  );
}

const tabId = (name: string, id: string) => `${name}-tab-${id}`;
const panelId = (name: string, id: string) => `${name}-panel-${id}`;

export function Empty({ children }: { children: ReactNode }) {
  return <p className="empty">{children}</p>;
}

export function Loading() {
  return <p className="empty">Loading…</p>;
}

export function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <label className="field">
      <span className="field-label">{label}</span>
      {hint && <span className="field-hint">{hint}</span>}
      {children}
    </label>
  );
}

export function RecordLink({ to, children }: { to: string; children: ReactNode }) {
  return (
    <Link className="record-link" to={to}>
      {children}
    </Link>
  );
}

export function statusTone(status: string): Tone {
  switch (status) {
    case "Licensed":
    case "ACTIVE":
      return "good";
    case "No licence":
      return "stop";
    case "No licence needed":
      return "info";
    case "EXPIRED":
    case "CANCELLED":
      return "neutral";
    case "PENDING":
      return "warning";
    default:
      return "neutral";
  }
}

export function statusWords(status: string): string {
  switch (status) {
    case "ACTIVE":
      return "In force";
    case "EXPIRED":
      return "Expired";
    case "CANCELLED":
      return "Cancelled";
    case "PENDING":
      return "Not started";
    default:
      return status;
  }
}
