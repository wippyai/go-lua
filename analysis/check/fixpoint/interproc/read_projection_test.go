package interproc

import (
	"errors"
	"testing"
)

func TestReadProjectionSelectsOnlyCertifiedEntryCoordinates(t *testing.T) {
	certificate, err := NewReadProjectionCertificate("normal-return", ReadCertificateInputs{
		Semantic: []EntrySelector{"argument"},
		Guards:   []EntrySelector{"branch"},
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := NewEntryBinding([]EntryValue{
		{Selector: "argument", Encoding: []byte("same")},
		{Selector: "branch", Encoding: []byte("true")},
		{Selector: "unread", Encoding: []byte("one")},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewEntryBinding([]EntryValue{
		{Selector: "argument", Encoding: []byte("same")},
		{Selector: "branch", Encoding: []byte("true")},
		{Selector: "unread", Encoding: []byte("two")},
	})
	if err != nil {
		t.Fatal(err)
	}
	left, err := certificate.Project(first)
	if err != nil {
		t.Fatal(err)
	}
	right, err := certificate.Project(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(left.CanonicalBytes()) != string(right.CanonicalBytes()) {
		t.Fatal("non-read entry coordinate changed projection")
	}

	changedGuard, err := NewEntryBinding([]EntryValue{
		{Selector: "argument", Encoding: []byte("same")},
		{Selector: "branch", Encoding: []byte("false")},
		{Selector: "unread", Encoding: []byte("two")},
	})
	if err != nil {
		t.Fatal(err)
	}
	changed, err := certificate.Project(changedGuard)
	if err != nil {
		t.Fatal(err)
	}
	if string(left.CanonicalBytes()) == string(changed.CanonicalBytes()) {
		t.Fatal("guard selector was omitted from projection")
	}
	if err := ValidateEntryProjectionCanonicalBytes(left.CanonicalBytes()); err != nil {
		t.Fatalf("canonical projection rejected: %v", err)
	}
	if err := ValidateEntryProjectionCanonicalBytes(append(left.CanonicalBytes(), 0)); err == nil {
		t.Fatal("projection with trailing bytes was accepted")
	}
}

func TestReadProjectionAuditFailsClosed(t *testing.T) {
	certificate, err := NewReadProjectionCertificate("normal-return", ReadCertificateInputs{Semantic: []EntrySelector{"argument"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := certificate.VerifyReadAudit([]ReadObservation{{Role: ReadSemantic, Selector: "argument"}}); err != nil {
		t.Fatalf("declared read rejected: %v", err)
	}
	for _, observed := range []ReadObservation{
		{Role: ReadSemantic, Selector: "hidden"},
		{Role: ReadProviderCallback, Selector: "argument"},
	} {
		err := certificate.VerifyReadAudit([]ReadObservation{observed})
		var incomplete *IncompleteReadCertificateError
		if !errors.As(err, &incomplete) || !IsIncompleteReadCertificateError(err) {
			t.Fatalf("audit error = %v, want typed incomplete certificate error", err)
		}
	}

	entry, err := NewEntryBinding([]EntryValue{{Selector: "other", Encoding: []byte("value")}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = certificate.Project(entry)
	var incomplete *IncompleteReadCertificateError
	if !errors.As(err, &incomplete) {
		t.Fatalf("missing projected coordinate error = %v, want typed incomplete certificate error", err)
	}
}

func TestReadCertificateCanonicalizesCategoryOrder(t *testing.T) {
	left, err := NewReadProjectionCertificate("normal-return", ReadCertificateInputs{
		Diagnostic: []EntrySelector{"diagnostic-b", "diagnostic-a"},
		Semantic:   []EntrySelector{"value-b", "value-a"},
	})
	if err != nil {
		t.Fatal(err)
	}
	right, err := NewReadProjectionCertificate("normal-return", ReadCertificateInputs{
		Diagnostic: []EntrySelector{"diagnostic-a", "diagnostic-b"},
		Semantic:   []EntrySelector{"value-a", "value-b"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(left.CanonicalBytes()) != string(right.CanonicalBytes()) || left.ContentID() != right.ContentID() {
		t.Fatal("certificate identity depends on declaration order")
	}
}
