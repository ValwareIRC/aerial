import { useCallback, useEffect, useState, type FormEvent, type ReactNode } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import {
  api,
  asApiError,
  longDate,
  money,
  ops,
  today,
  type AccountView,
  type ApiError,
  type CancellationReason,
  type LicenceDetail,
  type PersonDetail,
  type PersonSummary,
  type PremisesSummary,
  type Reference,
} from "../api";
import { AddressPicker, DangerAction } from "../fields";
import { Avatar } from "../icons";
import {
  Banner,
  Empty,
  ErrorSummary,
  Field,
  Loading,
  PageHeader,
  Section,
  SummaryList,
  Tabs,
  TabPanel,
  Tag,
  statusTone,
  statusWords,
  usePageTitle,
  type TabDef,
} from "../ui";

export function PeopleSearch() {
  const [term, setTerm] = useState("");
  const [rows, setRows] = useState<PersonSummary[] | null>(null);
  const [error, setError] = useState<ApiError | null>(null);

  usePageTitle("People");

  const run = (value: string) => {
    ops
      .searchPeople(value)
      .then(setRows)
      .catch((e: unknown) => setError(asApiError(e)));
  };

  useEffect(() => run(""), []);

  return (
    <>
      <PageHeader title="People" lede="Search by name, email or customer reference." />
      <ErrorSummary error={error} />

      <form
        className="search-form"
        role="search"
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          run(term);
        }}
      >
        <Field label="Search">
          <input
            value={term}
            onChange={(e) => setTerm(e.target.value)}
            placeholder="e.g. Kaur"
            autoFocus
          />
        </Field>
        <button className="button" type="submit">
          Search
        </button>
      </form>

      {rows === null ? (
        <Loading />
      ) : rows.length === 0 ? (
        <Empty>Nobody matches that.</Empty>
      ) : (
        <table className="data-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Address</th>
              <th>Contact</th>
              <th>Licence</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((p) => (
              <tr key={p.id}>
                <td>
                  <Link to={`/people/${p.id}`}>{p.name}</Link>
                  {p.deceased && <span className="cell-sub">Died {longDate(p.deceased)}</span>}
                </td>
                <td>{p.address || "Not recorded"}</td>
                <td>
                  {p.phone}
                  {p.email && <span className="cell-sub">{p.email}</span>}
                </td>
                <td>
                  {p.licenceHeld ? <Tag tone="good">Holds a licence</Tag> : <Tag>None</Tag>}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  );
}

type TabId = "summary" | "money" | "contact" | "letters" | "evidence";

const TABS: TabDef<TabId>[] = [
  { id: "summary", label: "Summary" },
  { id: "money", label: "Licence and payments" },
  { id: "contact", label: "Contact and address" },
  { id: "letters", label: "Letters and notes" },
  { id: "evidence", label: "Discount evidence" },
];

const isTabId = (v: string | null): v is TabId => TABS.some((t) => t.id === v);

export function PersonRecord() {
  const { id = "" } = useParams();
  const [person, setPerson] = useState<PersonDetail | null>(null);
  const [reference, setReference] = useState<Reference | null>(null);
  const [error, setError] = useState<ApiError | null>(null);
  const [notice, setNotice] = useState("");

  // The open tab lives in the address bar, so a record can be sent to a
  // colleague already open at the payments they were asking about.
  const [params, setParams] = useSearchParams();
  const fromUrl = params.get("tab");
  const tab: TabId = isTabId(fromUrl) ? fromUrl : "summary";
  const setTab = (next: TabId) => {
    const updated = new URLSearchParams(params);
    if (next === "summary") updated.delete("tab");
    else updated.set("tab", next);
    setParams(updated, { replace: true });
  };

  const load = useCallback(() => {
    ops
      .person(id)
      .then(setPerson)
      .catch((e: unknown) => setError(asApiError(e)));
  }, [id]);

  useEffect(load, [load]);
  useEffect(() => {
    ops.reference().then(setReference).catch(() => setReference(null));
  }, []);

  usePageTitle(person ? person.name : "Customer record");

  const done = (message: string) => {
    setNotice(message);
    setError(null);
    load();
  };

  if (error && !person) return <ErrorSummary error={error} />;
  if (!person || !reference) return <Loading />;

  const current = person.addresses.find((a) => a.current);
  const account = person.accounts[0];
  const licence = person.licences[0];

  return (
    <>
      <div className="profile">
        <Avatar name={person.name} size={56} />
        <div className="profile-text">
          <p className="overline">Customer record</p>
          <h1>{person.name}</h1>
          <p className="profile-sub">
            {current ? current.addressLine : "No current address recorded"}
          </p>
        </div>
        <div className="profile-actions">
          {licence ? (
            <Tag tone={statusTone(licence.status)}>{statusWords(licence.status)}</Tag>
          ) : (
            <Tag tone="stop">No licence</Tag>
          )}
        </div>
      </div>

      {person.party.deceased && (
        <Banner tone="stop" title={`Recorded as died on ${longDate(person.party.deceased)}`}>
          <p>No further letters will be sent.</p>
          <DangerAction
            actionLabel="Reverse this"
            heading="Reverse the death record?"
            consequences={[
              "The death record is removed.",
              "Any licence cancelled because of it goes back in force.",
              "Any refund is reversed.",
              "A correction is added to the record. Nothing is deleted.",
            ]}
            onConfirm={() =>
              void guard(setError, async () => {
                await ops.reverseDeath(person.party.id, today());
                done("Death record reversed.");
              })
            }
          />
        </Banner>
      )}
      {notice && <Banner tone="good" title={notice} />}
      <ErrorSummary error={error} />

      <div className="key-facts">
        <Fact label="Licence">
          {licence ? (
            <>
              <Tag tone={statusTone(licence.status)}>{statusWords(licence.status)}</Tag>
              <span className="fact-sub">{licence.number}</span>
            </>
          ) : (
            <Tag tone="stop">None held</Tag>
          )}
        </Fact>
        <Fact label="Balance">
          {account ? money(account.balance) : "—"}
          {account && account.arrears > 0 && (
            <span className="fact-sub warn">{money(account.arrears)} behind</span>
          )}
        </Fact>
        <Fact label="Pays by">
          {account?.methodLabel ?? "—"}
          {account?.nextCollection && (
            <span className="fact-sub">
              next {longDate(account.nextCollection.due)}, {money(account.nextCollection.amount)}
            </span>
          )}
        </Fact>
        <Fact label="Contact">
          {person.party.phone || "No phone"}
          <span className="fact-sub">
            {person.party.paperless ? "Email only" : "By post"}
          </span>
        </Fact>
      </div>

      <Tabs name="record" tabs={TABS} active={tab} onChange={setTab} />

      <TabPanel name="record" id={tab}>
        {tab === "summary" && <SummaryTab person={person} />}
        {tab === "money" && (
          <MoneyTab person={person} reference={reference} onDone={done} onError={setError} />
        )}
        {tab === "contact" && (
          <ContactTab person={person} reference={reference} onDone={done} onError={setError} />
        )}
        {tab === "letters" && <LettersTab person={person} onDone={done} onError={setError} />}
        {tab === "evidence" && (
          <EvidenceTab person={person} reference={reference} onDone={done} onError={setError} />
        )}
      </TabPanel>
    </>
  );
}

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="fact">
      <span className="fact-label">{label}</span>
      <span className="fact-value">{children}</span>
    </div>
  );
}

interface TabProps {
  person: PersonDetail;
  reference: Reference;
  onDone: (message: string) => void;
  onError: (e: ApiError) => void;
}

function guard(onError: (e: ApiError) => void, fn: () => Promise<void>) {
  return fn().catch((e: unknown) => onError(asApiError(e)));
}

function SummaryTab({ person }: { person: PersonDetail }) {
  return (
    <>
      <Section title="Details" icon="people" hue="mint">
        <SummaryList
          rows={[
            { key: "Name", value: person.name },
            { key: "Date of birth", value: longDate(person.party.dateOfBirth) || "Not recorded" },
            { key: "Customer reference", value: person.party.id },
            {
              key: "Discounts allowed",
              value:
                person.concessionsAvailable.length === 0
                  ? "None on file"
                  : person.concessionsAvailable.map(concessionWords).join(", "),
            },
          ]}
        />
      </Section>

      <Section title="Addresses" icon="home" hue="peach">
        <table className="data-table">
          <thead>
            <tr>
              <th>Address</th>
              <th>From</th>
              <th>To</th>
            </tr>
          </thead>
          <tbody>
            {person.addresses.map((a) => (
              <tr key={`${a.premises}-${a.from}`}>
                <td>
                  <Link to={`/addresses/${a.premises}`}>{a.addressLine}</Link>
                  {a.current && <span className="cell-sub">Current</span>}
                </td>
                <td>{longDate(a.from)}</td>
                <td>{a.to ? longDate(a.to) : "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </Section>

      <Section title="Licences" icon="licence" hue="lilac">
        {person.licences.length === 0 ? (
          <Empty>None held.</Empty>
        ) : (
          <table className="data-table">
            <thead>
              <tr>
                <th>Number</th>
                <th>Type</th>
                <th>Period</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              {person.licences.map((l) => (
                <tr key={l.id}>
                  <td>
                    <Link to={`/licences/${l.id}`}>{l.number}</Link>
                  </td>
                  <td>{l.classLabel}</td>
                  <td>
                    {longDate(l.start)}
                    <span className="cell-sub">to {longDate(l.expiry)}</span>
                  </td>
                  <td>
                    <Tag tone={statusTone(l.status)}>{statusWords(l.status)}</Tag>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Section>
    </>
  );
}

function MoneyTab({ person, reference, onDone, onError }: TabProps) {
  const account: AccountView | undefined = person.accounts[0];
  const licence = person.licences[0];

  const [amount, setAmount] = useState("");
  const [method, setMethod] = useState("CARD");
  const [reason, setReason] = useState<CancellationReason>("NO_LONGER_WATCHING");
  const [ddName, setDdName] = useState(person.name);
  const [sortCode, setSortCode] = useState("");
  const [accountNumber, setAccountNumber] = useState("");
  const [day, setDay] = useState(1);

  // Refunds are priced by the server, for every reason, so what is shown
  // before the decision is what the decision pays. The console does no money
  // arithmetic of its own.
  const [detail, setDetail] = useState<LicenceDetail | null>(null);
  const licenceID = licence?.id;
  useEffect(() => {
    if (!licenceID) return;
    api
      .licence(licenceID)
      .then(setDetail)
      .catch(() => setDetail(null));
  }, [licenceID]);

  if (!account || !licence) return <Empty>No licence, so nothing to pay.</Empty>;

  const refund = detail?.refunds[reason];

  return (
    <>
      <Section title="Account" icon="money" hue="teal">
        <SummaryList
          rows={[
            { key: "Licence", value: <Link to={`/licences/${licence.id}`}>{licence.number}</Link> },
            { key: "Charged", value: money(account.charged) },
            { key: "Paid so far", value: money(account.paid) },
            {
              key: "Still to pay",
              value: (
                <>
                  {money(account.balance)}
                  {account.arrears > 0 && (
                    <>
                      {" "}
                      <Tag tone="warning">{money(account.arrears)} behind</Tag>
                    </>
                  )}
                </>
              ),
            },
            { key: "Pays by", value: account.methodLabel },
            {
              key: "Direct Debit",
              value: account.mandate
                ? `${account.mandate.accountName}, ${account.mandateMasked}, collected on the ${account.mandate.collectionDay}th`
                : "None set up",
            },
          ]}
        />
      </Section>

      {account.instalments && account.instalments.length > 0 && (
        <Section title="Payment schedule" icon="calendar" hue="sky">
          <table className="data-table">
            <thead>
              <tr>
                <th>Payment</th>
                <th>Date</th>
                <th className="numeric">Amount</th>
                <th>State</th>
              </tr>
            </thead>
            <tbody>
              {account.instalments.map((i) => (
                <tr key={i.seq}>
                  <td>{i.seq}</td>
                  <td>{longDate(i.due)}</td>
                  <td className="numeric">{money(i.amount)}</td>
                  <td>
                    <Tag
                      tone={
                        i.state === "PAID" ? "good" : i.state === "MISSED" ? "stop" : "neutral"
                      }
                    >
                      {instalmentWords(i.state)}
                    </Tag>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </Section>
      )}

      <Section title="Payments received" icon="check" hue="mint">
        {account.payments.length === 0 ? (
          <Empty>Nothing received yet.</Empty>
        ) : (
          <table className="data-table">
            <thead>
              <tr>
                <th>Date</th>
                <th className="numeric">Amount</th>
                <th>How</th>
                <th>Reference</th>
              </tr>
            </thead>
            <tbody>
              {account.payments.map((p) => (
                <tr key={p.id}>
                  <td>{longDate(p.on)}</td>
                  <td className="numeric">{money(p.amount)}</td>
                  <td>{p.method.replace(/_/g, " ").toLowerCase()}</td>
                  <td>{p.reference}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Section>

      <Section title="Take a payment" icon="card" hue="teal">
        <form
          className="action-form"
          onSubmit={(e) => {
            e.preventDefault();
            void guard(onError, async () => {
              await ops.takePayment(licence.id, {
                amount,
                method,
                on: today(),
                reference: "taken by phone",
              });
              setAmount("");
              onDone(`Payment of £${amount} recorded.`);
            });
          }}
        >
          <div className="action-row">
            <Field label="Amount">
              <input value={amount} onChange={(e) => setAmount(e.target.value)} placeholder="15.00" />
            </Field>
            <Field label="How they paid">
              <select value={method} onChange={(e) => setMethod(e.target.value)}>
                {reference.paymentMethods.map((m) => (
                  <option key={m.value} value={m.value}>
                    {m.label}
                  </option>
                ))}
              </select>
            </Field>
            <button className="button" disabled={!amount}>
              Record payment
            </button>
          </div>
        </form>
      </Section>

      <Section title="Set up a Direct Debit" icon="card" hue="sky">
        <form
          className="action-form"
          onSubmit={(e) => {
            e.preventDefault();
            void guard(onError, async () => {
              await ops.setDirectDebit(licence.id, {
                accountName: ddName,
                sortCode,
                accountNumber,
                collectionDay: day,
                on: today(),
              });
              setSortCode("");
              setAccountNumber("");
              onDone("Direct Debit set up.");
            });
          }}
        >
          <p className="field-hint">
            Only the last four digits are kept. Full bank details are never stored.
          </p>
          <div className="action-row">
            <Field label="Name on the account">
              <input value={ddName} onChange={(e) => setDdName(e.target.value)} />
            </Field>
            <Field label="Sort code">
              <input
                value={sortCode}
                onChange={(e) => setSortCode(e.target.value)}
                placeholder="00-00-00"
                inputMode="numeric"
              />
            </Field>
            <Field label="Account number">
              <input
                value={accountNumber}
                onChange={(e) => setAccountNumber(e.target.value)}
                placeholder="00000000"
                inputMode="numeric"
              />
            </Field>
            <Field label="Collect on">
              <select value={day} onChange={(e) => setDay(Number(e.target.value))}>
                {Array.from({ length: 28 }, (_, i) => i + 1).map((n) => (
                  <option key={n} value={n}>
                    {n}
                  </option>
                ))}
              </select>
            </Field>
            <button className="button">Set up</button>
          </div>
        </form>
      </Section>

      {licence.status === "ACTIVE" && (
        <Section title="Cancel the licence" icon="alert" hue="rose">
          <div className="action-form">
            <div className="action-row">
              <Field label="Why are they cancelling?">
                <select
                  value={reason}
                  onChange={(e) => setReason(e.target.value as CancellationReason)}
                >
                  {reference.cancellationReasons.map((r) => (
                    <option key={r.value} value={r.value}>
                      {r.label}
                    </option>
                  ))}
                </select>
              </Field>
            </div>
            <DangerAction
              actionLabel="Cancel licence"
              heading={`Cancel licence ${licence.number}?`}
              consequences={[
                <>The licence stops covering {licence.addressLine} from today.</>,
                refund === undefined ? (
                  <>The refund due will be worked out when you confirm.</>
                ) : (
                  <>
                    A refund of <strong>{money(refund)}</strong> becomes due.
                  </>
                ),
                <>
                  Reason recorded:{" "}
                  {reference.cancellationReasons.find((r) => r.value === reason)?.label}.
                </>,
                "This can be reversed afterwards if it was recorded in error.",
              ]}
              onConfirm={() =>
                void guard(onError, async () => {
                  const r = await ops.cancel(licence.id, today(), reason);
                  onDone(`Licence cancelled. Refund due: ${money(r.refund)}.`);
                })
              }
            />
          </div>
        </Section>
      )}

      {licence.status === "CANCELLED" && (
        <Section title="Cancelled in error?" icon="undo" hue="sun">
          <div className="action-form">
            <p className="field-hint">
              Putting the licence back adds a correction to the record. Nothing is deleted.
            </p>
            <DangerAction
              actionLabel="Put the licence back"
              heading={`Reinstate licence ${licence.number}?`}
              consequences={[
                "The licence goes back in force for the rest of its term.",
                "Any refund already recorded is reversed.",
                "A correction note is added to the record.",
              ]}
              onConfirm={() =>
                void guard(onError, async () => {
                  await ops.reinstate(licence.id, today());
                  onDone("Licence put back in force.");
                })
              }
            />
          </div>
        </Section>
      )}
    </>
  );
}

function ContactTab({ person, reference, onDone, onError }: TabProps) {
  const currentAddress =
    person.addresses.find((a) => a.current)?.addressLine ?? "their current address";
  const [email, setEmail] = useState(person.party.email ?? "");
  const [phone, setPhone] = useState(person.party.phone ?? "");
  const [paperless, setPaperless] = useState(person.party.paperless);

  const [title, setTitle] = useState(person.party.title ?? "");
  const [forename, setForename] = useState(person.party.forename ?? "");
  const [surname, setSurname] = useState(person.party.surname ?? "");
  const [nameReason, setNameReason] = useState("MARRIAGE");

  const [movingTo, setMovingTo] = useState<PremisesSummary | null>(null);
  const [movingOn, setMovingOn] = useState(today());
  const [deathOn, setDeathOn] = useState(today());

  return (
    <>
      <Section title="How we contact them" icon="mail" hue="sky">
        <form
          className="action-form"
          onSubmit={(e) => {
            e.preventDefault();
            void guard(onError, async () => {
              await ops.updateContact(person.party.id, {
                email,
                phone,
                paperless,
                correspondence: person.party.correspondence ?? null,
              });
              onDone("Contact details updated.");
            });
          }}
        >
          <div className="action-row">
            <Field label="Email">
              <input value={email} onChange={(e) => setEmail(e.target.value)} type="email" />
            </Field>
            <Field label="Phone">
              <input value={phone} onChange={(e) => setPhone(e.target.value)} />
            </Field>
          </div>
          <label className="checkbox">
            <input
              type="checkbox"
              checked={paperless}
              onChange={(e) => setPaperless(e.target.checked)}
            />
            Send everything by email instead of post
          </label>
          <button className="button">Save contact details</button>
        </form>

        {person.party.correspondence && (
          <SummaryList
            rows={[
              {
                key: "Letters go to",
                value: [
                  person.party.correspondence.line1,
                  person.party.correspondence.line2,
                  person.party.correspondence.town,
                  person.party.correspondence.postcode,
                ]
                  .filter(Boolean)
                  .join(", "),
              },
            ]}
          />
        )}
      </Section>

      <Section title="They are moving house" icon="home" hue="peach">
        <div className="action-form">
          <p className="field-hint">
            The licence moves with them and keeps the same expiry date. Nothing is refunded and
            nothing is re-charged.
          </p>
          <AddressPicker label="Where are they moving to?" value={movingTo} onChange={setMovingTo} />
          <div className="action-row">
            <Field label="Moving date">
              <input type="date" value={movingOn} onChange={(e) => setMovingOn(e.target.value)} />
            </Field>
          </div>
          {movingTo && (
            <DangerAction
              actionLabel="Change address"
              heading="Move this customer?"
              consequences={[
                <>
                  They stop being recorded at {currentAddress} from {longDate(movingOn)}.
                </>,
                <>They are recorded at {movingTo.addressLine} from that date.</>,
                "Any licence they hold moves with them and keeps its expiry date.",
                "A confirmation letter is sent.",
              ]}
              onConfirm={() =>
                void guard(onError, async () => {
                  const r = await ops.changeAddress(person.party.id, movingTo.id, movingOn);
                  const n = r.licencesMoved?.length ?? 0;
                  setMovingTo(null);
                  onDone(
                    n > 0
                      ? `Address changed and ${n === 1 ? "the licence" : `${n} licences`} moved with them.`
                      : "Address changed.",
                  );
                })
              }
            />
          )}
        </div>
      </Section>

      <Section title="They have changed their name" icon="tag" hue="lilac">
        <form
          className="action-form"
          onSubmit={(e) => {
            e.preventDefault();
            void guard(onError, async () => {
              await ops.changeName(person.party.id, {
                title,
                forename,
                surname,
                reason: nameReason,
                on: today(),
              });
              onDone("Name changed.");
            });
          }}
        >
          <div className="action-row">
            <Field label="Title">
              <input value={title} onChange={(e) => setTitle(e.target.value)} />
            </Field>
            <Field label="First name">
              <input value={forename} onChange={(e) => setForename(e.target.value)} />
            </Field>
            <Field label="Last name">
              <input value={surname} onChange={(e) => setSurname(e.target.value)} />
            </Field>
            <Field label="Reason">
              <select value={nameReason} onChange={(e) => setNameReason(e.target.value)}>
                {reference.nameChangeReasons.map((r) => (
                  <option key={r.value} value={r.value}>
                    {r.label}
                  </option>
                ))}
              </select>
            </Field>
            <button className="button" disabled={!surname}>
              Change name
            </button>
          </div>
        </form>

        {person.party.nameHistory && person.party.nameHistory.length > 0 && (
          <table className="data-table">
            <thead>
              <tr>
                <th>Was</th>
                <th>Became</th>
                <th>Reason</th>
                <th>When</th>
              </tr>
            </thead>
            <tbody>
              {person.party.nameHistory.map((n, i) => (
                <tr key={i}>
                  <td>{n.from}</td>
                  <td>{n.to}</td>
                  <td>
                    {reference.nameChangeReasons.find((r) => r.value === n.reason)?.label ??
                      n.reason}
                  </td>
                  <td>{longDate(n.on)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Section>

      {!person.party.deceased && (
        <Section title="Report a death" icon="alert" hue="rose">
          <div className="action-form">
            <div className="action-row">
              <Field label="Date of death">
                <input type="date" value={deathOn} onChange={(e) => setDeathOn(e.target.value)} />
              </Field>
            </div>
            <DangerAction
              actionLabel="Record death"
              heading={`Record that ${person.name} died on ${longDate(deathOn)}?`}
              confirmWord={person.party.surname || person.party.organisation || "confirm"}
              consequences={[
                "All letters and reminders stop immediately.",
                "Any licence held is cancelled from that date.",
                "A refund of the complete months remaining becomes due.",
                "This can be reversed, but the correction stays on the record.",
              ]}
              onConfirm={() =>
                void guard(onError, async () => {
                  const r = await ops.recordDeath(person.party.id, deathOn);
                  onDone(
                    r.refund > 0
                      ? `Recorded. Letters stopped and a refund of ${money(r.refund)} is due.`
                      : "Recorded. No further letters will be sent.",
                  );
                })
              }
            />
          </div>
        </Section>
      )}
    </>
  );
}

function LettersTab({
  person,
  onDone,
  onError,
}: {
  person: PersonDetail;
  onDone: (m: string) => void;
  onError: (e: ApiError) => void;
}) {
  const [text, setText] = useState("");

  return (
    <>
      <Section title="Add a note" icon="note" hue="sun">
        <form
          className="action-form"
          onSubmit={(e) => {
            e.preventDefault();
            void guard(onError, async () => {
              await ops.addNote(person.party.id, text);
              setText("");
              onDone("Note added.");
            });
          }}
        >
          <Field label="What happened?" hint="Notes cannot be edited or deleted afterwards.">
            <textarea
              value={text}
              onChange={(e) => setText(e.target.value)}
              rows={3}
              placeholder="e.g. Rang to ask about moving the Direct Debit date."
            />
          </Field>
          <button className="button" disabled={!text.trim()}>
            Add note
          </button>
        </form>
      </Section>

      <Section title="Notes" icon="note" hue="mint">
        {person.notes.length === 0 ? (
          <Empty>No notes yet.</Empty>
        ) : (
          <ul className="note-list">
            {person.notes.map((n) => (
              <li key={n.id}>
                <Avatar name={n.by} size={34} />
                <div>
                  <p className="note-text">{n.text}</p>
                  <p className="note-meta">
                    {n.by} · {new Date(n.at).toLocaleString("en-GB")}
                  </p>
                </div>
              </li>
            ))}
          </ul>
        )}
      </Section>

      <Section title="Letters sent" icon="mail" hue="sky">
        {person.letters.length === 0 ? (
          <Empty>Nothing sent.</Empty>
        ) : (
          <table className="data-table">
            <thead>
              <tr>
                <th>Letter</th>
                <th>Sent</th>
                <th>How</th>
                <th>About</th>
              </tr>
            </thead>
            <tbody>
              {person.letters.map((l) => (
                <tr key={l.id}>
                  <td>{letterWords(l.kind)}</td>
                  <td>{longDate(l.sentOn)}</td>
                  <td>{l.channel === "EMAIL" ? "Email" : "Post"}</td>
                  <td>{l.about}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Section>
    </>
  );
}

function EvidenceTab({ person, reference, onDone, onError }: TabProps) {
  const [kind, setKind] = useState("BLIND_REGISTRATION");
  const [ref, setRef] = useState("");
  const [seenOn, setSeenOn] = useState(today());
  const [expiresOn, setExpiresOn] = useState("");

  return (
    <>
      <Section title="Evidence on file" icon="shield" hue="teal">
        {person.evidence.length === 0 ? (
          <Empty>None recorded. Discounts cannot be applied without evidence.</Empty>
        ) : (
          <table className="data-table">
            <thead>
              <tr>
                <th>Evidence</th>
                <th>Reference</th>
                <th>Seen</th>
                <th>Valid until</th>
              </tr>
            </thead>
            <tbody>
              {person.evidence.map((e) => (
                <tr key={e.id}>
                  <td>
                    {reference.evidenceKinds.find((k) => k.value === e.kind)?.label ?? e.kind}
                  </td>
                  <td>{e.reference}</td>
                  <td>{longDate(e.seenOn)}</td>
                  <td>{e.expiresOn ? longDate(e.expiresOn) : "No end date"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Section>

      <Section title="Record evidence" icon="check" hue="mint">
        <form
          className="action-form"
          onSubmit={(e) => {
            e.preventDefault();
            void guard(onError, async () => {
              await ops.addEvidence(person.party.id, {
                kind,
                reference: ref,
                seenOn,
                expiresOn,
              });
              setRef("");
              onDone("Evidence recorded.");
            });
          }}
        >
          <div className="action-row">
            <Field label="What was seen">
              <select value={kind} onChange={(e) => setKind(e.target.value)}>
                {reference.evidenceKinds.map((k) => (
                  <option key={k.value} value={k.value}>
                    {k.label}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="Reference on the document">
              <input value={ref} onChange={(e) => setRef(e.target.value)} />
            </Field>
            <Field label="Date seen">
              <input type="date" value={seenOn} onChange={(e) => setSeenOn(e.target.value)} />
            </Field>
            <Field label="Valid until" hint="Leave blank if it does not expire">
              <input
                type="date"
                value={expiresOn}
                onChange={(e) => setExpiresOn(e.target.value)}
              />
            </Field>
            <button className="button">Record evidence</button>
          </div>
        </form>
      </Section>
    </>
  );
}

function instalmentWords(state: string): string {
  switch (state) {
    case "PAID":
      return "Paid";
    case "MISSED":
      return "Missed";
    case "FUTURE":
      return "Not due yet";
    default:
      return "Due";
  }
}

function concessionWords(c: string): string {
  switch (c) {
    case "BLIND_50":
      return "Sight impaired — half price";
    case "OVER_75_PENSION_CREDIT":
      return "Over 75 on Pension Credit — free";
    default:
      return c;
  }
}

function letterWords(kind: string): string {
  switch (kind) {
    case "LICENCE_CONFIRMATION":
      return "Licence confirmation";
    case "RENEWAL_REMINDER":
      return "Renewal reminder";
    case "PAYMENT_REMINDER":
      return "Payment reminder";
    case "DECLARATION_RECEIVED":
      return "Confirmation that no licence is needed";
    case "DECLARATION_REVIEW":
      return "Check the declaration is still right";
    case "REFUND_CONFIRMATION":
      return "Refund confirmation";
    case "DETAILS_CHANGED":
      return "Confirmation of changed details";
    default:
      return kind;
  }
}
