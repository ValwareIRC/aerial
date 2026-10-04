package ids

import "testing"

func TestParseUPRNAcceptsValidForms(t *testing.T) {
	for _, s := range []string{"1", "100023336956", "12345", " 100023336956 "} {
		if _, err := ParseUPRN(s); err != nil {
			t.Errorf("ParseUPRN(%q) failed: %v", s, err)
		}
	}
}

func TestParseUPRNRejectsMalformed(t *testing.T) {
	for _, s := range []string{"", "0", "0123", "1234567890123", "12a45", "-1", "1.5", "  "} {
		if _, err := ParseUPRN(s); err == nil {
			t.Errorf("ParseUPRN(%q) should have failed", s)
		}
	}
}

func TestUPRNMarshalsAsStringToSurviveJavaScript(t *testing.T) {
	u := MustParseUPRN("100023336956")
	b, err := u.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `"100023336956"` {
		t.Errorf("MarshalJSON = %s, want a quoted string", b)
	}

	var back UPRN
	if err := back.UnmarshalJSON(b); err != nil {
		t.Fatal(err)
	}
	if back != u {
		t.Errorf("round trip gave %d, want %d", back, u)
	}
}

func TestUnmarshalRejectsBadUPRN(t *testing.T) {
	var u UPRN
	if err := u.UnmarshalJSON([]byte(`"not-a-uprn"`)); err == nil {
		t.Error("unmarshalling junk should fail")
	}
}
