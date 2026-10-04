package domain

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aerial-project/aerial/internal/ids"
	"github.com/aerial-project/aerial/internal/money"
	"github.com/aerial-project/aerial/internal/payment"
	"github.com/aerial-project/aerial/internal/timeline"
)

type PaymentMethod string

const (
	MethodCard        PaymentMethod = "CARD"
	MethodDirectDebit PaymentMethod = "DIRECT_DEBIT"
	MethodPaymentCard PaymentMethod = "PAYMENT_CARD"
	MethodCash        PaymentMethod = "CASH"
	MethodCheque      PaymentMethod = "CHEQUE"
	MethodRefund      PaymentMethod = "REFUND"
)

func (m PaymentMethod) Label() string {
	switch m {
	case MethodCard:
		return "Debit or credit card"
	case MethodDirectDebit:
		return "Direct Debit"
	case MethodPaymentCard:
		return "TV Licensing payment card"
	case MethodCash:
		return "Cash"
	case MethodCheque:
		return "Cheque"
	case MethodRefund:
		return "Refund"
	}
	return string(m)
}

type Payment struct {
	ID        string        `json:"id"`
	Licence   ids.LicenceID `json:"licence"`
	On        timeline.Date `json:"on"`
	Amount    money.Money   `json:"amount"`
	Method    PaymentMethod `json:"method"`
	Reference string        `json:"reference,omitempty"`
	TakenBy   string        `json:"takenBy,omitempty"`
}

// DirectDebit is a mandate. Only the last four digits of the account number are
// held; the sort code is stored masked.
type DirectDebit struct {
	AccountName   string         `json:"accountName"`
	SortCodeLast  string         `json:"sortCodeLast"`
	AccountLast4  string         `json:"accountLast4"`
	CollectionDay int            `json:"collectionDay"`
	SetUpOn       timeline.Date  `json:"setUpOn"`
	Cancelled     *timeline.Date `json:"cancelled,omitempty"`
}

func (d DirectDebit) ActiveOn(on timeline.Date) bool {
	return d.Cancelled == nil || on.Before(*d.Cancelled)
}

func (d DirectDebit) Masked() string {
	return fmt.Sprintf("**-**-%s  ****%s", d.SortCodeLast, d.AccountLast4)
}

// NewDirectDebit validates and masks bank details. Full numbers are never kept.
func NewDirectDebit(accountName, sortCode, accountNumber string, collectionDay int, on timeline.Date) (DirectDebit, error) {
	digitsOnly := func(s string) string {
		var b strings.Builder
		for _, r := range s {
			if r >= '0' && r <= '9' {
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	sc, an := digitsOnly(sortCode), digitsOnly(accountNumber)

	if strings.TrimSpace(accountName) == "" {
		return DirectDebit{}, fmt.Errorf("domain: the name on the account is required")
	}
	if len(sc) != 6 {
		return DirectDebit{}, fmt.Errorf("domain: a sort code must be six digits")
	}
	if len(an) != 8 {
		return DirectDebit{}, fmt.Errorf("domain: an account number must be eight digits")
	}
	if collectionDay < 1 || collectionDay > 28 {
		return DirectDebit{}, fmt.Errorf("domain: the collection day must be between 1 and 28")
	}
	return DirectDebit{
		AccountName:   strings.TrimSpace(accountName),
		SortCodeLast:  sc[4:],
		AccountLast4:  an[4:],
		CollectionDay: collectionDay,
		SetUpOn:       on,
	}, nil
}

type InstalmentState string

const (
	InstalmentPaid   InstalmentState = "PAID"
	InstalmentDue    InstalmentState = "DUE"
	InstalmentMissed InstalmentState = "MISSED"
	InstalmentFuture InstalmentState = "FUTURE"
)

type InstalmentView struct {
	Seq    int             `json:"seq"`
	Due    timeline.Date   `json:"due"`
	Amount money.Money     `json:"amount"`
	State  InstalmentState `json:"state"`
}

// Account is the money side of one licence: what was charged, what has been
// paid, and the plan it is being paid under.
type Account struct {
	Licence  ids.LicenceID `json:"licence"`
	Charged  money.Money   `json:"charged"`
	Method   PaymentMethod `json:"method"`
	Plan     *payment.Plan `json:"plan,omitempty"`
	Mandate  *DirectDebit  `json:"mandate,omitempty"`
	Payments []Payment     `json:"payments"`
}

func (a *Account) Paid() money.Money {
	amounts := make([]money.Money, 0, len(a.Payments))
	for _, p := range a.Payments {
		amounts = append(amounts, p.Amount)
	}
	return money.Sum(amounts)
}

func (a *Account) Balance() money.Money { return a.Charged.Sub(a.Paid()) }

// DueBy is the total the plan expects to have been collected by a date.
func (a *Account) DueBy(on timeline.Date) money.Money {
	if a.Plan == nil {
		return a.Charged
	}
	return a.Plan.Paid(on)
}

// Arrears is what the plan expected by now, less what has been paid.
func (a *Account) Arrears(on timeline.Date) money.Money {
	arrears := a.DueBy(on).Sub(a.Paid())
	if arrears.IsNegative() {
		return money.Zero
	}
	return arrears
}

// Instalments marks each scheduled payment against what has actually been
// paid. Money is allocated to instalments in order, and allocation stops at the
// first one it cannot cover: a later, cheaper instalment is never reported as
// paid while an earlier one is outstanding.
func (a *Account) Instalments(on timeline.Date) []InstalmentView {
	if a.Plan == nil {
		return nil
	}
	remaining := a.Paid()
	covered := true
	out := make([]InstalmentView, 0, len(a.Plan.Instalments))
	for _, i := range a.Plan.Instalments {
		view := InstalmentView{Seq: i.Seq, Due: i.Due, Amount: i.Amount}
		switch {
		case covered && !remaining.Less(i.Amount):
			view.State = InstalmentPaid
			remaining = remaining.Sub(i.Amount)
		case i.Due.After(on):
			covered = false
			view.State = InstalmentFuture
		case i.Due.Equal(on):
			covered = false
			view.State = InstalmentDue
		default:
			covered = false
			view.State = InstalmentMissed
		}
		out = append(out, view)
	}
	return out
}

// NextCollection is the first instalment not yet covered by payments.
func (a *Account) NextCollection(on timeline.Date) *InstalmentView {
	for _, i := range a.Instalments(on) {
		if i.State == InstalmentFuture || i.State == InstalmentMissed || i.State == InstalmentDue {
			view := i
			return &view
		}
	}
	return nil
}

type Ledger struct {
	accounts map[ids.LicenceID]*Account
	seq      int
}

func NewLedger() *Ledger {
	return &Ledger{accounts: map[ids.LicenceID]*Account{}}
}

func (l *Ledger) Open(licence ids.LicenceID, charged money.Money, method PaymentMethod) *Account {
	a := &Account{Licence: licence, Charged: charged, Method: method, Payments: []Payment{}}
	l.accounts[licence] = a
	return a
}

func (l *Ledger) Get(licence ids.LicenceID) (*Account, bool) {
	a, ok := l.accounts[licence]
	return a, ok
}

func (l *Ledger) SetPlan(licence ids.LicenceID, p payment.Plan, method PaymentMethod) error {
	a, ok := l.accounts[licence]
	if !ok {
		return fmt.Errorf("domain: no account for licence %s", licence)
	}
	a.Plan = &p
	a.Method = method
	return nil
}

func (l *Ledger) SetMandate(licence ids.LicenceID, d DirectDebit) error {
	a, ok := l.accounts[licence]
	if !ok {
		return fmt.Errorf("domain: no account for licence %s", licence)
	}
	a.Mandate = &d
	a.Method = MethodDirectDebit
	return nil
}

func (l *Ledger) Record(licence ids.LicenceID, amount money.Money, method PaymentMethod, on timeline.Date, reference, takenBy string) (Payment, error) {
	a, ok := l.accounts[licence]
	if !ok {
		return Payment{}, fmt.Errorf("domain: no account for licence %s", licence)
	}
	if amount.IsZero() {
		return Payment{}, fmt.Errorf("domain: a payment cannot be zero")
	}
	l.seq++
	p := Payment{
		ID: fmt.Sprintf("PMT%d", l.seq), Licence: licence, On: on,
		Amount: amount, Method: method, Reference: reference, TakenBy: takenBy,
	}
	a.Payments = append(a.Payments, p)
	sort.Slice(a.Payments, func(i, j int) bool { return a.Payments[i].On.Before(a.Payments[j].On) })
	return p, nil
}

func (l *Ledger) Accounts() []*Account {
	out := make([]*Account, 0, len(l.accounts))
	for _, a := range l.accounts {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Licence < out[j].Licence })
	return out
}
