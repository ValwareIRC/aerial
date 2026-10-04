package domain

import (
	"errors"
	"testing"

	"github.com/aerial-project/aerial/internal/ids"
	"github.com/aerial-project/aerial/internal/timeline"
)

const testPremises ids.PremisesID = "P1"

func TestNLNGetsAReviewDateAndSurvivesIt(t *testing.T) {
	made := timeline.MustParseDate("2026-01-01")
	d, err := NewDeclaration("D1", testPremises, DeclarationNLN, "PARTY1", made)
	if err != nil {
		t.Fatal(err)
	}
	if d.ReviewDue == nil || d.ReviewDue.String() != "2028-01-01" {
		t.Fatalf("review due = %v, want 2028-01-01", d.ReviewDue)
	}

	if got := d.StateOn(timeline.MustParseDate("2027-12-31")); got != StateActive {
		t.Errorf("before review date state = %s, want ACTIVE", got)
	}
	if got := d.StateOn(timeline.MustParseDate("2028-01-01")); got != StateDueForReview {
		t.Errorf("on review date state = %s, want DUE_FOR_REVIEW", got)
	}
	if !d.InForceOn(timeline.MustParseDate("2040-01-01")) {
		t.Error("an unreviewed NLN must remain in force, not lapse")
	}
}

func TestDueForReviewIsStillADeclaration(t *testing.T) {
	r := NewRegister()
	d, _ := NewDeclaration("D1", testPremises, DeclarationNLN, "PARTY1", timeline.MustParseDate("2026-01-01"))
	if err := r.Record(d); err != nil {
		t.Fatal(err)
	}

	declared, due := r.NoLicenceDeclared(testPremises, timeline.MustParseDate("2029-06-01"))
	if !declared {
		t.Error("an NLN past review must still read as declared")
	}
	if !due {
		t.Error("it should be flagged as due for review")
	}
}

func TestWithdrawalOfAccessBlocksVisitsIndefinitely(t *testing.T) {
	r := NewRegister()
	d, err := NewDeclaration("W1", testPremises, DeclarationWOIRA, "PARTY1", timeline.MustParseDate("2026-01-01"))
	if err != nil {
		t.Fatal(err)
	}
	if d.ReviewDue != nil {
		t.Error("a withdrawal of access must not carry a review date")
	}
	if err := r.Record(d); err != nil {
		t.Fatal(err)
	}

	for _, on := range []string{"2026-01-01", "2030-01-01", "2099-01-01"} {
		err := r.VisitPermitted(testPremises, timeline.MustParseDate(on))
		if !errors.Is(err, ErrVisitProhibited) {
			t.Errorf("visit on %s: got %v, want ErrVisitProhibited", on, err)
		}
	}
}

func TestWithdrawalOfAccessRejectsAReviewDate(t *testing.T) {
	r := NewRegister()
	due := timeline.MustParseDate("2028-01-01")
	d := Declaration{
		ID: "W1", Premises: testPremises, Kind: DeclarationWOIRA,
		MadeBy: "PARTY1", MadeOn: timeline.MustParseDate("2026-01-01"), ReviewDue: &due,
	}
	if err := r.Record(d); err == nil {
		t.Error("recording a withdrawal of access with a review date should fail")
	}
}

func TestOnlyTheMakerMayRevokeAWithdrawalOfAccess(t *testing.T) {
	r := NewRegister()
	d, _ := NewDeclaration("W1", testPremises, DeclarationWOIRA, "PARTY1", timeline.MustParseDate("2026-01-01"))
	if err := r.Record(d); err != nil {
		t.Fatal(err)
	}

	on := timeline.MustParseDate("2027-01-01")
	if err := r.Revoke(testPremises, "W1", on, "ENFORCEMENT_OFFICER"); err == nil {
		t.Error("a third party must not be able to revoke a withdrawal of access")
	}
	if err := r.Revoke(testPremises, "W1", on, "PARTY1"); err != nil {
		t.Fatalf("the maker should be able to revoke: %v", err)
	}
	if err := r.VisitPermitted(testPremises, timeline.MustParseDate("2027-06-01")); err != nil {
		t.Errorf("after revocation visits should be permitted, got %v", err)
	}
}

func TestRevokedDeclarationStopsCounting(t *testing.T) {
	r := NewRegister()
	d, _ := NewDeclaration("D1", testPremises, DeclarationNLN, "PARTY1", timeline.MustParseDate("2026-01-01"))
	if err := r.Record(d); err != nil {
		t.Fatal(err)
	}
	if err := r.Revoke(testPremises, "D1", timeline.MustParseDate("2026-06-01"), "PARTY1"); err != nil {
		t.Fatal(err)
	}

	if declared, _ := r.NoLicenceDeclared(testPremises, timeline.MustParseDate("2026-07-01")); declared {
		t.Error("a revoked NLN must not read as declared")
	}
	if declared, _ := r.NoLicenceDeclared(testPremises, timeline.MustParseDate("2026-05-01")); !declared {
		t.Error("before revocation it should still read as declared")
	}
}

func TestRevokeRejectsUnknownAndDoubleRevocation(t *testing.T) {
	r := NewRegister()
	d, _ := NewDeclaration("D1", testPremises, DeclarationNLN, "PARTY1", timeline.MustParseDate("2026-01-01"))
	if err := r.Record(d); err != nil {
		t.Fatal(err)
	}
	on := timeline.MustParseDate("2026-06-01")

	if err := r.Revoke(testPremises, "NOPE", on, "PARTY1"); err == nil {
		t.Error("revoking an unknown declaration should fail")
	}
	if err := r.Revoke(testPremises, "D1", on, "PARTY1"); err != nil {
		t.Fatal(err)
	}
	if err := r.Revoke(testPremises, "D1", on, "PARTY1"); err == nil {
		t.Error("revoking twice should fail")
	}
}
