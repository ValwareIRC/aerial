import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { api, asApiError, ops, type PremisesSummary } from "./api";
import { Field } from "./ui";

interface PickerProps {
  label: string;
  hint?: string;
  value: PremisesSummary | null;
  onChange: (p: PremisesSummary | null) => void;
  allowNew?: boolean;
}

/**
 * Chooses an address that already exists, so the same door is never typed
 * twice. It is a combobox, and it behaves like one: arrow keys move through
 * the suggestions, Enter takes the highlighted one, Escape closes the list,
 * and what is highlighted is announced rather than only coloured.
 */
export function AddressPicker({ label, hint, value, onChange, allowNew = true }: PickerProps) {
  const [term, setTerm] = useState("");
  const [results, setResults] = useState<PremisesSummary[]>([]);
  const [open, setOpen] = useState(false);
  const [adding, setAdding] = useState(false);
  const [highlighted, setHighlighted] = useState(-1);
  const [searching, setSearching] = useState(false);
  const box = useRef<HTMLDivElement>(null);
  const listID = useId();
  const optionID = (i: number) => `${listID}-option-${i}`;

  useEffect(() => {
    if (term.trim().length < 2) {
      setResults([]);
      setOpen(false);
      setHighlighted(-1);
      return;
    }
    let cancelled = false;
    setSearching(true);
    const timer = setTimeout(() => {
      api
        .searchPremises(term)
        .then((r) => {
          if (cancelled) return;
          setResults(r.slice(0, 8));
          setHighlighted(r.length > 0 ? 0 : -1);
          setOpen(true);
        })
        .catch(() => {
          if (!cancelled) setResults([]);
        })
        .finally(() => {
          if (!cancelled) setSearching(false);
        });
    }, 180);
    return () => {
      cancelled = true;
      setSearching(false);
      clearTimeout(timer);
    };
  }, [term]);

  useEffect(() => {
    const away = (e: MouseEvent) => {
      if (box.current && !box.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", away);
    return () => document.removeEventListener("mousedown", away);
  }, []);

  const pick = (p: PremisesSummary | undefined) => {
    if (!p) return;
    onChange(p);
    setOpen(false);
    setTerm("");
    setHighlighted(-1);
  };

  if (value) {
    return (
      <div className="picked">
        <span className="picked-label">{label}</span>
        <p className="picked-address">{value.addressLine}</p>
        <p className="picked-meta">
          {value.kindLabel}
          {value.occupier && <> · {value.occupier}</>}
        </p>
        <button type="button" className="link-inline" onClick={() => onChange(null)}>
          Choose a different address
        </button>
      </div>
    );
  }

  if (adding) {
    return <AddressCreator onCancel={() => setAdding(false)} onCreated={(p) => { setAdding(false); onChange(p); }} />;
  }

  return (
    <div className="picker" ref={box}>
      <Field label={label} hint={hint ?? "Start typing an address or postcode"}>
        <input
          value={term}
          onChange={(e) => setTerm(e.target.value)}
          onFocus={() => results.length > 0 && setOpen(true)}
          onKeyDown={(e) => {
            switch (e.key) {
              case "ArrowDown":
                e.preventDefault();
                if (results.length > 0) {
                  setOpen(true);
                  setHighlighted((i) => (i + 1) % results.length);
                }
                break;
              case "ArrowUp":
                e.preventDefault();
                if (results.length > 0) {
                  setOpen(true);
                  setHighlighted((i) => (i - 1 + results.length) % results.length);
                }
                break;
              case "Enter":
                if (open && highlighted >= 0) {
                  e.preventDefault();
                  pick(results[highlighted]);
                }
                break;
              case "Escape":
                setOpen(false);
                setHighlighted(-1);
                break;
            }
          }}
          placeholder="e.g. Rowan Close or CT16"
          autoComplete="off"
          role="combobox"
          aria-expanded={open}
          aria-controls={listID}
          aria-autocomplete="list"
          {...(open && highlighted >= 0 ? { "aria-activedescendant": optionID(highlighted) } : {})}
        />
      </Field>

      <p className="visually-hidden" role="status">
        {searching
          ? "Searching"
          : open
            ? `${results.length} ${results.length === 1 ? "address" : "addresses"} found`
            : ""}
      </p>

      <ul className="picker-list" id={listID} role="listbox" aria-label={label} hidden={!open}>
        {results.length === 0 ? (
          <li className="picker-empty">No address matches that.</li>
        ) : (
          results.map((p, i) => (
            <li
              key={p.id}
              id={optionID(i)}
              role="option"
              aria-selected={i === highlighted}
              className={i === highlighted ? "picker-option-active" : undefined}
            >
              <button type="button" tabIndex={-1} onMouseEnter={() => setHighlighted(i)} onClick={() => pick(p)}>
                <span className="picker-line">{p.addressLine}</span>
                <span className="picker-sub">
                  {p.kindLabel}
                  {p.occupier && <> · {p.occupier}</>}
                </span>
              </button>
            </li>
          ))
        )}
      </ul>

      {allowNew && (
        <button type="button" className="link-inline" onClick={() => setAdding(true)}>
          The address is not listed
        </button>
      )}
    </div>
  );
}

interface CreatorProps {
  onCancel: () => void;
  onCreated: (p: PremisesSummary) => void;
}

/** Adds an address, refusing an exact duplicate and warning on the same postcode. */
export function AddressCreator({ onCancel, onCreated }: CreatorProps) {
  const [line1, setLine1] = useState("");
  const [line2, setLine2] = useState("");
  const [town, setTown] = useState("");
  const [postcode, setPostcode] = useState("");
  const [kind, setKind] = useState("DWELLING");
  const [near, setNear] = useState<PremisesSummary[]>([]);
  const [error, setError] = useState<string>("");
  const [existing, setExisting] = useState<PremisesSummary[]>([]);
  const [busy, setBusy] = useState(false);

  const submit = async (acknowledgedNear: boolean) => {
    setBusy(true);
    setError("");
    try {
      const result = await ops.createAddress({ line1, line2, town, postcode, kind, acknowledgedNear });
      switch (result.outcome) {
        case "created":
          onCreated(result.premises);
          return;
        case "needs-confirmation":
          setNear(result.near);
          return;
        case "already-recorded":
          setExisting(result.exact);
          setError(result.message);
          return;
      }
    } catch (e) {
      setError(asApiError(e).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="creator">
      <h4>Add an address</h4>

      {error && (
        <div className="inline-problem" role="alert">
          <p>{error}</p>
          {existing.map((p) => (
            <p key={p.id} className="inline-problem-item">
              Already recorded as <strong>{p.addressLine}</strong>
              {p.occupier && <> — {p.occupier}</>}.{" "}
              <button type="button" className="link-inline" onClick={() => onCreated(p)}>
                Use that record
              </button>
            </p>
          ))}
        </div>
      )}

      {near.length > 0 && (
        <div className="inline-warning" role="alert">
          <p>
            {near.length === 1 ? "One address" : `${near.length} addresses`} already exist at this
            postcode. Check this is not the same one recorded differently.
          </p>
          <ul>
            {near.map((p) => (
              <li key={p.id}>
                {p.addressLine}{" "}
                <button type="button" className="link-inline" onClick={() => onCreated(p)}>
                  Use this
                </button>
              </li>
            ))}
          </ul>
          <button type="button" className="button" disabled={busy} onClick={() => void submit(true)}>
            None of these — add it anyway
          </button>
        </div>
      )}

      <div className="action-row">
        <Field label="Address line 1">
          <input value={line1} onChange={(e) => setLine1(e.target.value)} />
        </Field>
        <Field label="Address line 2">
          <input value={line2} onChange={(e) => setLine2(e.target.value)} />
        </Field>
      </div>
      <div className="action-row">
        <Field label="Town">
          <input value={town} onChange={(e) => setTown(e.target.value)} />
        </Field>
        <Field label="Postcode">
          <input
            value={postcode}
            onChange={(e) => setPostcode(e.target.value.toUpperCase())}
            maxLength={8}
          />
        </Field>
        <Field label="Type">
          <select value={kind} onChange={(e) => setKind(e.target.value)}>
            <option value="DWELLING">Home</option>
            <option value="HMO_ROOM">Room in shared house</option>
            <option value="CARE_ROOM">Care home room</option>
            <option value="BUSINESS">Business</option>
            <option value="VEHICLE">Vehicle</option>
          </select>
        </Field>
      </div>

      <p className="field-hint">
        New addresses get a temporary property reference until they are matched to the national
        address register.
      </p>

      <div className="action-row">
        <button
          type="button"
          className="button"
          disabled={busy || !line1 || !postcode}
          onClick={() => void submit(false)}
        >
          Add address
        </button>
        <button type="button" className="button button-quiet" onClick={onCancel}>
          Cancel
        </button>
      </div>
    </div>
  );
}

interface DangerProps {
  actionLabel: string;
  heading: string;
  consequences: ReactNode[];
  /** When set, the exact word must be typed before the action is allowed. */
  confirmWord?: string;
  busy?: boolean;
  onConfirm: () => void;
}

/** Two-step confirmation. Nothing consequential happens on a single click. */
export function DangerAction({
  actionLabel,
  heading,
  consequences,
  confirmWord,
  busy,
  onConfirm,
}: DangerProps) {
  const [open, setOpen] = useState(false);
  const [typed, setTyped] = useState("");

  if (!open) {
    return (
      <button type="button" className="button button-warning" onClick={() => setOpen(true)}>
        {actionLabel}
      </button>
    );
  }

  const ready = !confirmWord || typed.trim().toLowerCase() === confirmWord.toLowerCase();

  return (
    <div className="confirm" role="group" aria-label={heading}>
      <h4>{heading}</h4>
      <ul className="confirm-list">
        {consequences.map((c, i) => (
          <li key={i}>{c}</li>
        ))}
      </ul>

      {confirmWord && (
        <Field label={`Type ${confirmWord} to confirm`}>
          <input value={typed} onChange={(e) => setTyped(e.target.value)} autoFocus />
        </Field>
      )}

      <div className="action-row">
        <button
          type="button"
          className="button button-warning"
          disabled={!ready || busy}
          onClick={() => {
            onConfirm();
            setOpen(false);
            setTyped("");
          }}
        >
          Yes, {actionLabel.toLowerCase()}
        </button>
        <button
          type="button"
          className="button button-quiet"
          onClick={() => {
            setOpen(false);
            setTyped("");
          }}
        >
          No, go back
        </button>
      </div>
    </div>
  );
}
