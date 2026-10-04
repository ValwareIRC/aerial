package domain

import (
	"errors"
	"testing"

	"github.com/aerial-project/aerial/internal/ids"
	"github.com/aerial-project/aerial/internal/money"
	"github.com/aerial-project/aerial/internal/timeline"
)

func d(s string) timeline.Date { return timeline.MustParseDate(s) }

func standardLicence(t *testing.T, id ids.LicenceID, premises ids.PremisesID, start string) *Licence {
	t.Helper()
	l, err := Issue(id, "HOLDER", premises, ClassColour, ConcessionNone, d(start), DefaultTermMonths, money.MustFromPounds("180.00"))
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestIssueSetsATwelveMonthTerm(t *testing.T) {
	l := standardLicence(t, "L1", "P1", "2026-04-01")
	if l.Expiry().String() != "2027-04-01" {
		t.Errorf("expiry = %s, want 2027-04-01", l.Expiry())
	}
	if l.TermMonths() != 12 {
		t.Errorf("term = %d months, want 12", l.TermMonths())
	}
}

func TestStatusTransitions(t *testing.T) {
	l := standardLicence(t, "L1", "P1", "2026-04-01")
	cases := map[string]LicenceStatus{
		"2026-03-31": LicencePending,
		"2026-04-01": LicenceActive,
		"2027-03-31": LicenceActive,
		"2027-04-01": LicenceExpired,
	}
	for on, want := range cases {
		if got := l.StatusOn(d(on)); got != want {
			t.Errorf("status on %s = %s, want %s", on, got, want)
		}
	}
}

func TestIssueRejectsBadInput(t *testing.T) {
	fee := money.MustFromPounds("180.00")
	if _, err := Issue("L1", "H", "P1", "HOLOGRAM", ConcessionNone, d("2026-04-01"), 12, fee); err == nil {
		t.Error("unknown class should be rejected")
	}
	if _, err := Issue("L1", "H", "P1", ClassColour, ConcessionNone, d("2026-04-01"), 0, fee); err == nil {
		t.Error("a zero-month term should be rejected")
	}
	if _, err := Issue("L1", "H", "P1", ClassColour, ConcessionNone, d("2026-04-01"), 12, money.FromPence(-1)); err == nil {
		t.Error("a negative fee should be rejected")
	}
}

func TestTransferMovesCoverageAndKeepsExpiry(t *testing.T) {
	r := NewLicenceRegister()
	l := standardLicence(t, "L1", "P1", "2026-04-01")
	if err := r.Add(l); err != nil {
		t.Fatal(err)
	}
	if err := r.Transfer("L1", "P2", d("2026-10-01")); err != nil {
		t.Fatal(err)
	}

	if l.Expiry().String() != "2027-04-01" {
		t.Errorf("transfer changed expiry to %s", l.Expiry())
	}
	if !l.CoversPremisesOn("P1", d("2026-09-30")) {
		t.Error("old premises should be covered up to the transfer date")
	}
	if l.CoversPremisesOn("P1", d("2026-10-01")) {
		t.Error("old premises should not be covered from the transfer date")
	}
	if !l.CoversPremisesOn("P2", d("2026-10-01")) {
		t.Error("new premises should be covered from the transfer date")
	}
	if !l.CoversPremisesOn("P2", d("2027-03-31")) {
		t.Error("new premises should be covered to expiry")
	}
	if got, ok := r.LicensedOn("P2", d("2027-01-01")); !ok || got.ID != "L1" {
		t.Error("register should find the licence at the new premises")
	}
}

func TestTransferRejectsInvalidMoves(t *testing.T) {
	r := NewLicenceRegister()
	l := standardLicence(t, "L1", "P1", "2026-04-01")
	if err := r.Add(l); err != nil {
		t.Fatal(err)
	}

	if err := r.Transfer("L1", "P1", d("2026-10-01")); err == nil {
		t.Error("transferring to the same premises should fail")
	}
	if err := r.Transfer("L1", "P2", d("2027-06-01")); err == nil {
		t.Error("transferring after expiry should fail")
	}
	if err := r.Transfer("NOPE", "P2", d("2026-10-01")); err == nil {
		t.Error("transferring an unknown licence should fail")
	}
}

func TestRefundPaysCompleteMonthsOnly(t *testing.T) {
	cases := []struct {
		cancelOn string
		want     string
	}{
		{"2026-09-01", "£105.00"}, // 7 whole months left
		{"2026-09-15", "£90.00"},  // 6 whole months, the part month is not refunded
		{"2026-04-01", "£180.00"}, // cancelled the day it starts
		{"2027-03-15", "£0.00"},   // inside the final month
		{"2027-03-31", "£0.00"},
	}
	for _, c := range cases {
		l := standardLicence(t, "L1", "P1", "2026-04-01")
		if got := l.RefundDue(d(c.cancelOn), CancelNoLongerWatch); got.String() != c.want {
			t.Errorf("refund on %s = %s, want %s", c.cancelOn, got, c.want)
		}
	}
}

func TestRefundOnAZeroFeeLicenceIsZero(t *testing.T) {
	l, err := Issue("L1", "H", "P1", ClassColour, ConcessionOver75PensionCredit, d("2026-04-01"), 12, money.Zero)
	if err != nil {
		t.Fatal(err)
	}
	if got := l.RefundDue(d("2026-09-01"), CancelNoLongerWatch); !got.IsZero() {
		t.Errorf("refund on a free licence = %s, want £0.00", got)
	}
}

func TestCancelIsOnce(t *testing.T) {
	l := standardLicence(t, "L1", "P1", "2026-04-01")
	refund, err := l.Cancel(d("2026-09-01"), CancelNoLongerWatch)
	if err != nil {
		t.Fatal(err)
	}
	if refund.String() != "£105.00" {
		t.Errorf("refund = %s, want £105.00", refund)
	}
	if _, err := l.Cancel(d("2026-10-01"), CancelNoLongerWatch); !errors.Is(err, ErrLicenceState) {
		t.Errorf("second cancel = %v, want ErrLicenceState", err)
	}
}

func TestCancelRejectsExpiredLicence(t *testing.T) {
	l := standardLicence(t, "L1", "P1", "2026-04-01")
	if _, err := l.Cancel(d("2027-06-01"), CancelNoLongerWatch); !errors.Is(err, ErrLicenceState) {
		t.Errorf("cancelling an expired licence = %v, want ErrLicenceState", err)
	}
}

func TestRegisterRejectsDoubleCover(t *testing.T) {
	r := NewLicenceRegister()
	if err := r.Add(standardLicence(t, "L1", "P1", "2026-04-01")); err != nil {
		t.Fatal(err)
	}
	err := r.Add(standardLicence(t, "L2", "P1", "2026-10-01"))
	if !errors.Is(err, ErrOverlappingLicence) {
		t.Errorf("overlapping licence = %v, want ErrOverlappingLicence", err)
	}
}

func TestRegisterAllowsAbuttingRenewal(t *testing.T) {
	r := NewLicenceRegister()
	if err := r.Add(standardLicence(t, "L1", "P1", "2026-04-01")); err != nil {
		t.Fatal(err)
	}
	if err := r.Add(standardLicence(t, "L2", "P1", "2027-04-01")); err != nil {
		t.Errorf("a renewal starting the day the previous licence ends should be allowed: %v", err)
	}
}

func TestCancellationFreesTheRemainingPeriod(t *testing.T) {
	r := NewLicenceRegister()
	first := standardLicence(t, "L1", "P1", "2026-04-01")
	if err := r.Add(first); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Cancel(d("2026-10-01"), CancelNoLongerWatch); err != nil {
		t.Fatal(err)
	}
	if err := r.Add(standardLicence(t, "L2", "P1", "2026-10-01")); err != nil {
		t.Errorf("a new licence from the cancellation date should be allowed: %v", err)
	}
}

func TestRegisterRejectsDuplicateID(t *testing.T) {
	r := NewLicenceRegister()
	if err := r.Add(standardLicence(t, "L1", "P1", "2026-04-01")); err != nil {
		t.Fatal(err)
	}
	if err := r.Add(standardLicence(t, "L1", "P2", "2026-04-01")); err == nil {
		t.Error("registering the same licence id twice should fail")
	}
}

func TestLicensedOnFindsNothingWhenUnlicensed(t *testing.T) {
	r := NewLicenceRegister()
	if err := r.Add(standardLicence(t, "L1", "P1", "2026-04-01")); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.LicensedOn("P1", d("2027-05-01")); ok {
		t.Error("after expiry the premises should read as unlicensed")
	}
	if _, ok := r.LicensedOn("P9", d("2026-05-01")); ok {
		t.Error("an unknown premises should read as unlicensed")
	}
}

func TestCancellationReasonDrivesRefund(t *testing.T) {
	issuedInError := standardLicence(t, "L1", "P1", "2026-04-01")
	if got := issuedInError.RefundDue(d("2027-03-15"), CancelIssuedInError); got.String() != "£180.00" {
		t.Errorf("issued in error should refund the whole fee, got %s", got)
	}
	normal := standardLicence(t, "L2", "P2", "2026-04-01")
	if got := normal.RefundDue(d("2027-03-15"), CancelNoLongerWatch); got.String() != "£0.00" {
		t.Errorf("a part month should refund nothing, got %s", got)
	}
}

func TestCancelRejectsUnknownReason(t *testing.T) {
	l := standardLicence(t, "L1", "P1", "2026-04-01")
	if _, err := l.Cancel(d("2026-09-01"), "BECAUSE"); err == nil {
		t.Error("an unknown cancellation reason should be rejected")
	}
}
