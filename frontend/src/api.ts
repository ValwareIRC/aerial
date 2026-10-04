export type Role = "SERVICE" | "ENFORCEMENT" | "ADMIN" | "SUBJECT";
export type LicenceClass = "COLOUR" | "MONOCHROME" | "ARC";
export type Concession = "NONE" | "BLIND_50" | "OVER_75_PENSION_CREDIT";
export type LicenceStatus = "PENDING" | "ACTIVE" | "EXPIRED" | "CANCELLED";

export interface Session {
  userId: string;
  username: string;
  displayName: string;
  role: Role;
  expires: string;
}

export interface Address {
  line1: string;
  line2?: string;
  town: string;
  postcode: string;
}

export interface PremisesSummary {
  id: string;
  uprn: string;
  kind: string;
  kindLabel: string;
  address: Address;
  addressLine: string;
  occupier: string;
  licensed: boolean;
  licenceNumber?: string;
  licenceId?: string;
  expiry?: string;
  declared: boolean;
  dueForReview: boolean;
  accessWithdrawn: boolean;
  coveredByParent: boolean;
}

export interface Party {
  id: string;
  title?: string;
  forename?: string;
  surname?: string;
  organisation?: string;
}

export interface Declaration {
  id: string;
  premises: string;
  kind: "NO_LICENCE_NEEDED" | "LEGALLY_LICENCE_FREE" | "ACCESS_WITHDRAWN";
  madeBy: string;
  madeOn: string;
  reviewDue?: string;
  revokedOn?: string;
}

export interface LicenceSummary {
  id: string;
  number: string;
  holder: string;
  holderName: string;
  class: LicenceClass;
  classLabel: string;
  concession: Concession;
  status: LicenceStatus;
  premises: string;
  addressLine: string;
  start: string;
  expiry: string;
  feePaid: number;
}

export interface PremisesDetail {
  premises: PremisesSummary & { parent?: string };
  summary: PremisesSummary;
  licences: LicenceSummary[];
  declarations: Declaration[];
  occupants: Party[];
  parent?: PremisesSummary;
}

/** Every reason a licence can be surrendered for, as the server defines them. */
export type CancellationReason =
  | "NO_LONGER_WATCHING"
  | "MOVED_ABROAD"
  | "LICENCE_HOLDER_DIED"
  | "NOW_ELIGIBLE_FREE"
  | "ISSUED_IN_ERROR";

export interface LicenceDetail {
  summary: LicenceSummary;
  coverage: { premises: string; period: { Start: string; End?: string } }[] | null;
  account: AccountView;
  /** What a surrender today pays back for the ordinary reason. */
  refund: number;
  /**
   * The same figure for every reason. Refunds are worked out by the server so
   * the number shown before a decision is the number the decision produces.
   */
  refunds: Partial<Record<CancellationReason, number>>;
}

export interface Overview {
  premises: number;
  licensed: number;
  noLicenceNeeded: number;
  unlicensed: number;
  dueForReview: number;
  accessWithdrawn: number;
  expiringSoon: number;
  noLicenceList: PremisesSummary[];
  reviewList: PremisesSummary[];
  doNotVisitList: PremisesSummary[];
}

export interface WorklistItem {
  id: string;
  addressLine: string;
  postcode: string;
  status: string;
  visitPermitted: boolean;
  dueForReview: boolean;
}

export interface EnforcementView {
  premises: string;
  on: string;
  licensed: boolean;
  licenceExpiry?: string;
  noLicenceDeclared: boolean;
  dueForReview: boolean;
  visitPermitted: boolean;
  occupantTokens: string[] | null;
}

export interface Quote {
  class: LicenceClass;
  concession: Concession;
  on: string;
  base: number;
  payable: number;
}

export interface RateCoverage {
  class: LicenceClass;
  earliestKnown: string;
  hasOpenEnded: boolean;
  recordCount: number;
}

export interface Instalment {
  seq: number;
  due: string;
  amount: number;
}

export interface Plan {
  kind: string;
  total: number;
  start: string;
  instalments: Instalment[];
}

export interface AuditEntry {
  at: string;
  actor: { id: string; role: string };
  action: string;
  subject: string;
  purpose: string;
  allowed: boolean;
  detail?: string;
}

/** One page of the access log, newest first, with the totals it was drawn from. */
export interface AuditPage {
  entries: AuditEntry[] | null;
  /** How many entries matched the filter, before the limit was applied. */
  matched: number;
  returned: number;
  /** Everything in the log, filter or no filter. */
  total: number;
}

export interface LicenceNotice {
  service: string;
  version: string;
  copyright: string;
  licence: string;
  summary: string;
  openSource: string;
}

export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
    readonly reason = "",
    /** The parsed response body, for the few failures that carry detail. */
    readonly body: unknown = null,
  ) {
    super(message);
  }
}

/** Turns anything thrown by fetch or a handler into an ApiError. */
export function asApiError(e: unknown): ApiError {
  if (e instanceof ApiError) return e;
  if (e instanceof TypeError) return new ApiError(0, "Could not reach the service. Check your connection.");
  return new ApiError(0, e instanceof Error ? e.message : String(e));
}

type UnauthorizedHandler = () => void;

let onUnauthorized: UnauthorizedHandler = () => {};

/**
 * Called whenever the service says the session has gone. The app signs out
 * rather than leaving the user clicking at a screen that will never load.
 */
export function setUnauthorizedHandler(h: UnauthorizedHandler) {
  onUnauthorized = h;
}

/** Requests that are allowed to see a 401 without ending the session. */
const SESSION_PATHS = new Set(["/api/session"]);

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    credentials: "same-origin",
    ...init,
    headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) },
  });

  const text = await res.text();
  let body: unknown = null;
  if (text) {
    try {
      body = JSON.parse(text);
    } catch {
      // A proxy or a crash can answer with HTML. Say so plainly rather than
      // surfacing a JSON parser's complaint about "<".
      throw new ApiError(res.status, "The service sent something unreadable", text.slice(0, 200));
    }
  }

  if (!res.ok) {
    const e = body as { error?: string; reason?: string } | null;
    if (res.status === 401 && !SESSION_PATHS.has(path.split("?")[0] ?? path)) {
      onUnauthorized();
    }
    throw new ApiError(res.status, e?.error ?? res.statusText, e?.reason ?? "", body);
  }
  return body as T;
}

const q = (params: Record<string, string | undefined>) => {
  const s = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) if (v) s.set(k, v);
  const out = s.toString();
  return out ? `?${out}` : "";
};

export const api = {
  signIn: (username: string, password: string) =>
    request<Session>("/api/session", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    }),
  signOut: () => request<null>("/api/session", { method: "DELETE" }),
  whoAmI: () => request<Session>("/api/session"),

  overview: (on?: string) => request<Overview>(`/api/overview${q({ on })}`),
  searchPremises: (query: string, on?: string) =>
    request<PremisesSummary[]>(`/api/premises${q({ q: query, on })}`),
  premises: (id: string, on?: string) =>
    request<PremisesDetail>(`/api/premises/${encodeURIComponent(id)}${q({ on })}`),

  licences: (on?: string) => request<LicenceSummary[]>(`/api/licences${q({ on })}`),
  licence: (id: string, on?: string) =>
    request<LicenceDetail>(`/api/licences/${encodeURIComponent(id)}${q({ on })}`),
  transferLicence: (id: string, premises: string, on: string) =>
    request<LicenceSummary>(`/api/licences/${encodeURIComponent(id)}/transfer`, {
      method: "POST",
      body: JSON.stringify({ premises, on }),
    }),
  issueLicence: (body: {
    holder: string;
    premises: string;
    class: LicenceClass;
    concession: Concession;
    start: string;
  }) => request<{ licence: { id: string; number: string } }>("/api/licences", {
    method: "POST",
    body: JSON.stringify(body),
  }),
  declare: (premises: string, kind: string, by: string, on: string) =>
    request<Declaration>("/api/declarations", {
      method: "POST",
      body: JSON.stringify({ premises, kind, by, on }),
    }),

  worklist: (query: string, on?: string) =>
    request<WorklistItem[]>(`/api/worklist${q({ q: query, on })}`),
  enforcementView: (id: string, on?: string) =>
    request<EnforcementView>(`/api/premises/${encodeURIComponent(id)}/enforcement${q({ on })}`),

  quote: (cls: LicenceClass, concession: Concession, on: string) =>
    request<Quote>(`/api/fees/quote${q({ class: cls, concession, on })}`),
  coverage: () => request<RateCoverage[]>("/api/fees/coverage"),
  plan: (kind: string, total: string, start: string) =>
    request<Plan>(`/api/plans${q({ kind, total, start })}`),

  audit: (opts: { refused?: boolean; role?: string; limit?: number } = {}) =>
    request<AuditPage>(
      `/api/audit${q({
        refused: opts.refused ? "true" : undefined,
        role: opts.role,
        limit: opts.limit ? String(opts.limit) : undefined,
      })}`,
    ),

  /** The terms the running service is licensed under. Needs no session. */
  licenceNotice: () => request<LicenceNotice>("/api/licence"),
};

export function money(pence: number): string {
  const negative = pence < 0;
  const abs = Math.abs(pence);
  const pounds = Math.floor(abs / 100).toLocaleString("en-GB");
  return `${negative ? "−" : ""}£${pounds}.${String(abs % 100).padStart(2, "0")}`;
}

const MONTHS = [
  "January", "February", "March", "April", "May", "June",
  "July", "August", "September", "October", "November", "December",
];

/** "2026-04-01" to "1 April 2026". Anything unexpected is passed through. */
export function longDate(iso: string | undefined): string {
  if (!iso) return "";
  const [y, m, d] = iso.split("-");
  if (!y || !m || !d) return iso;
  const month = MONTHS[Number(m) - 1];
  if (!month || Number.isNaN(Number(d))) return iso;
  return `${Number(d)} ${month} ${y}`;
}

export function today(): string {
  return new Date().toISOString().slice(0, 10);
}

// --- people and operations -------------------------------------------------

export interface NameChange {
  on: string;
  from: string;
  to: string;
  reason: string;
  by: string;
}

export interface PersonRecord {
  id: string;
  title?: string;
  forename?: string;
  surname?: string;
  organisation?: string;
  dateOfBirth?: string;
  deceased?: string;
  email?: string;
  phone?: string;
  paperless: boolean;
  correspondence?: Address;
  nameHistory?: NameChange[];
}

export interface PersonSummary {
  id: string;
  name: string;
  sortName: string;
  address: string;
  email?: string;
  phone?: string;
  deceased?: string;
  licenceHeld: boolean;
}

export interface AddressHistoryItem {
  premises: string;
  addressLine: string;
  from: string;
  to?: string;
  current: boolean;
}

export interface InstalmentView {
  seq: number;
  due: string;
  amount: number;
  state: "PAID" | "DUE" | "MISSED" | "FUTURE";
}

export interface DirectDebit {
  accountName: string;
  collectionDay: number;
  setUpOn: string;
}

export interface AccountView {
  licence: string;
  licenceNumber: string;
  method: string;
  methodLabel: string;
  charged: number;
  paid: number;
  balance: number;
  arrears: number;
  mandate?: DirectDebit;
  mandateMasked?: string;
  nextCollection?: InstalmentView;
  instalments?: InstalmentView[];
  payments: PaymentRecord[];
}

export interface PaymentRecord {
  id: string;
  on: string;
  amount: number;
  method: string;
  reference?: string;
  takenBy?: string;
}

export interface Letter {
  id: string;
  kind: string;
  channel: "POST" | "EMAIL";
  sentOn: string;
  about?: string;
  address?: string;
}

export interface Note {
  id: string;
  subject: string;
  text: string;
  by: string;
  at: string;
}

export interface Evidence {
  id: string;
  kind: string;
  reference?: string;
  seenOn: string;
  expiresOn?: string;
  seenBy: string;
}

export interface PersonDetail {
  party: PersonRecord;
  name: string;
  addresses: AddressHistoryItem[];
  licences: LicenceSummary[];
  accounts: AccountView[];
  letters: Letter[];
  notes: Note[];
  evidence: Evidence[];
  concessionsAvailable: string[];
}

export interface Visit {
  id: string;
  premises: string;
  on: string;
  officer: string;
  outcome: string;
  note?: string;
}

export interface Option {
  value: string;
  label: string;
}

export interface NewAddress {
  line1: string;
  line2: string;
  town: string;
  postcode: string;
  kind: string;
  acknowledgedNear: boolean;
}

export interface DuplicateCheck {
  exact: PremisesSummary[] | null;
  near: PremisesSummary[] | null;
}

export type AddressCreated =
  | { outcome: "created"; premises: PremisesSummary }
  | { outcome: "needs-confirmation"; near: PremisesSummary[] }
  | { outcome: "already-recorded"; message: string; exact: PremisesSummary[] };

export interface Reference {
  paymentMethods: Option[];
  cancellationReasons: Option[];
  visitOutcomes: Option[];
  evidenceKinds: Option[];
  nameChangeReasons: Option[];
  letterKinds: Option[];
}

export interface EqualityCell {
  value: string;
  outcome: string;
  count: number;
}

export interface EqualityRate {
  value: string;
  outcome: string;
  count: number;
  total: number;
  proportion: number;
}

export interface EqualityResponse {
  report: {
    attribute: string;
    k: number;
    cells: EqualityCell[] | null;
    suppressedCells: number;
    suppressedCount: number;
    totalCount: number;
  };
  rates?: EqualityRate[];
}

export const ops = {
  searchPeople: (query: string, on?: string) =>
    request<PersonSummary[]>(`/api/people${q({ q: query, on })}`),
  person: (id: string, on?: string) =>
    request<PersonDetail>(`/api/people/${encodeURIComponent(id)}${q({ on })}`),
  changeName: (id: string, body: { title: string; forename: string; surname: string; reason: string; on: string }) =>
    request<unknown>(`/api/people/${encodeURIComponent(id)}/name`, { method: "POST", body: JSON.stringify(body) }),
  updateContact: (id: string, body: { email: string; phone: string; paperless: boolean; correspondence: Address | null }) =>
    request<unknown>(`/api/people/${encodeURIComponent(id)}/contact`, { method: "POST", body: JSON.stringify(body) }),
  changeAddress: (id: string, premises: string, on: string) =>
    request<{ licencesMoved: string[] | null }>(`/api/people/${encodeURIComponent(id)}/address`, {
      method: "POST",
      body: JSON.stringify({ premises, on }),
    }),
  recordDeath: (id: string, on: string) =>
    request<{ refund: number }>(`/api/people/${encodeURIComponent(id)}/deceased`, {
      method: "POST",
      body: JSON.stringify({ on }),
    }),
  addEvidence: (id: string, body: { kind: string; reference: string; seenOn: string; expiresOn: string }) =>
    request<Evidence>(`/api/people/${encodeURIComponent(id)}/evidence`, { method: "POST", body: JSON.stringify(body) }),
  subjectAccess: (id: string) => request<unknown>(`/api/people/${encodeURIComponent(id)}/export`),

  addNote: (subject: string, text: string) =>
    request<Note>("/api/notes", { method: "POST", body: JSON.stringify({ subject, text }) }),

  account: (licence: string, on?: string) =>
    request<AccountView>(`/api/licences/${encodeURIComponent(licence)}/account${q({ on })}`),
  takePayment: (licence: string, body: { amount: string; method: string; on: string; reference: string }) =>
    request<PaymentRecord>(`/api/licences/${encodeURIComponent(licence)}/payments`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  setPlan: (licence: string, kind: string, method: string, on: string) =>
    request<unknown>(`/api/licences/${encodeURIComponent(licence)}/plan`, {
      method: "POST",
      body: JSON.stringify({ kind, method, on }),
    }),
  setDirectDebit: (licence: string, body: { accountName: string; sortCode: string; accountNumber: string; collectionDay: number; on: string }) =>
    request<unknown>(`/api/licences/${encodeURIComponent(licence)}/direct-debit`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  renew: (licence: string, method: string, plan: string) =>
    request<{ number: string }>(`/api/licences/${encodeURIComponent(licence)}/renew`, {
      method: "POST",
      body: JSON.stringify({ method, plan }),
    }),
  cancel: (licence: string, on: string, reason: string) =>
    request<{ refund: number }>(`/api/licences/${encodeURIComponent(licence)}/cancel`, {
      method: "POST",
      body: JSON.stringify({ on, reason }),
    }),

  reinstate: (licence: string, on: string) =>
    request<unknown>(`/api/licences/${encodeURIComponent(licence)}/reinstate`, {
      method: "POST",
      body: JSON.stringify({ on }),
    }),
  reverseDeath: (id: string, on: string) =>
    request<unknown>(`/api/people/${encodeURIComponent(id)}/deceased/reverse`, {
      method: "POST",
      body: JSON.stringify({ on }),
    }),

  recordVisit: (body: { premises: string; on: string; outcome: string; note: string }) =>
    request<Visit>("/api/visits", { method: "POST", body: JSON.stringify(body) }),
  visits: (premises: string) => request<Visit[]>(`/api/premises/${encodeURIComponent(premises)}/visits`),

  /**
   * Adds an address. The server answers three ways: created, "there are
   * others at this postcode, are you sure", or a refusal because this exact
   * address is already recorded. All three come back as one result rather
   * than as an exception, because none of them is a surprise.
   */
  createAddress: async (body: NewAddress): Promise<AddressCreated> => {
    try {
      const created = await request<PremisesSummary & { needsConfirmation?: boolean; duplicate?: DuplicateCheck }>(
        "/api/premises",
        { method: "POST", body: JSON.stringify(body) },
      );
      if (created.needsConfirmation) {
        return { outcome: "needs-confirmation", near: created.duplicate?.near ?? [] };
      }
      return { outcome: "created", premises: created };
    } catch (e) {
      const err = asApiError(e);
      if (err.status === 409) {
        const payload = err.body as { duplicate?: DuplicateCheck } | null;
        return {
          outcome: "already-recorded",
          message: err.message,
          exact: payload?.duplicate?.exact ?? [],
        };
      }
      throw err;
    }
  },

  reference: () => request<Reference>("/api/reference"),
  equality: (attribute: string, outcome?: string) =>
    request<EqualityResponse>(`/api/equality/${encodeURIComponent(attribute)}${q({ outcome })}`),
  sendRenewalReminders: () => request<{ sent: number }>("/api/renewal-reminders", { method: "POST" }),
};
