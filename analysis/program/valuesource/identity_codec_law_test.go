package valuesource

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
)

func TestIdentityCodecsPreserveTheirFramedPreimages(t *testing.T) {
	path := fixedValueSourceID(0x11)
	bodyPath := fixedValueSourceID(0x21)
	bodyID := fixedValueSourceID(0x31)
	anchorID := fixedValueSourceID(0x41)
	programID := fixedValueSourceID(0x51)
	entryID := fixedValueSourceID(0x61)
	finishID := fixedValueSourceID(0x71)

	anchor, ok := valueSourceAnchorIdentity(true, path)
	if !ok {
		t.Fatal("valueSourceAnchorIdentity failed")
	}
	wantAnchor := valueSourceSemanticPreimage(t, "program/transformer/value-source-anchor", func(writer *framing.Writer) error {
		if err := writer.Bool(true); err != nil {
			return err
		}
		return writer.Bytes(path[:])
	})
	if anchor != wantAnchor {
		t.Fatalf("anchor identity preimage changed: got %s want %s", anchor, wantAnchor)
	}

	source, ok := valueSourceIdentity(6, bodyPath, bodyID, anchorID)
	if !ok {
		t.Fatal("valueSourceIdentity failed")
	}
	wantSource := valueSourceSemanticPreimage(t, "program/transformer/value-source-occurrence", func(writer *framing.Writer) error {
		if err := writer.Uint(6); err != nil {
			return err
		}
		if err := writer.Bytes(bodyPath[:]); err != nil {
			return err
		}
		if err := writer.Bytes(bodyID[:]); err != nil {
			return err
		}
		return writer.Bytes(anchorID[:])
	})
	if source != wantSource {
		t.Fatalf("source identity preimage changed: got %s want %s", source, wantSource)
	}

	authored := keyspace.MakeTerm(keyspace.FamilyString, 7)
	span, ok := valueSourceSpanIdentity(programID, authored, entryID, finishID)
	if !ok {
		t.Fatal("valueSourceSpanIdentity failed")
	}
	wantSpan := valueSourceRolePreimage(t, "program/transformer/span", programID, func(writer *framing.Writer) error {
		if err := writer.Uint(uint64(keyspace.FamilyString)); err != nil {
			return err
		}
		if err := writer.Uint(7); err != nil {
			return err
		}
		if err := writer.Bytes(entryID[:]); err != nil {
			return err
		}
		return writer.Bytes(finishID[:])
	})
	if span != wantSpan {
		t.Fatalf("span identity preimage changed: got %s want %s", span, wantSpan)
	}
}

func TestValueSourceCodeAndIdentityCodecsFailClosed(t *testing.T) {
	for family, want := range map[keyspace.Family]uint8{
		keyspace.FamilyNil:       1,
		keyspace.FamilyBool:      2,
		keyspace.FamilyInteger:   3,
		keyspace.FamilyFloat:     4,
		keyspace.FamilyString:    5,
		keyspace.FamilyTypeValue: 6,
	} {
		got, ok := valueSourceCode(family)
		if !ok || got != want {
			t.Fatalf("valueSourceCode(%d) = %d/%v, want %d/true", family, got, ok, want)
		}
	}
	if code, ok := valueSourceCode(keyspace.FamilyInvalid); ok || code != 0 {
		t.Fatalf("invalid valueSourceCode = %d/%v", code, ok)
	}

	available := fixedValueSourceID(0x81)
	zero := identity.ContentID{}
	if id, ok := valueSourceAnchorIdentity(false, zero); ok || id.Available() {
		t.Fatalf("zero anchor input admitted: %s/%v", id, ok)
	}
	if id, ok := valueSourceIdentity(0, available, available, available); ok || id.Available() {
		t.Fatalf("zero source code admitted: %s/%v", id, ok)
	}
	if id, ok := valueSourceIdentity(7, available, available, available); ok || id.Available() {
		t.Fatalf("out-of-range source code admitted: %s/%v", id, ok)
	}
	if id, ok := valueSourceIdentity(1, zero, available, available); ok || id.Available() {
		t.Fatalf("zero source field admitted: %s/%v", id, ok)
	}
	if id, ok := valueSourceSpanIdentity(zero, keyspace.MakeTerm(keyspace.FamilyString, 1), available, available); ok || id.Available() {
		t.Fatalf("zero span owner admitted: %s/%v", id, ok)
	}
	if id, ok := valueSourceSpanIdentity(available, keyspace.Term(1), available, available); ok || id.Available() {
		t.Fatalf("malformed span term admitted: %s/%v", id, ok)
	}
	if id, ok := valueSourceSpanIdentity(available, keyspace.MakeTerm(keyspace.FamilyString, 1), zero, available); ok || id.Available() {
		t.Fatalf("zero span entry admitted: %s/%v", id, ok)
	}
}

func fixedValueSourceID(seed byte) identity.ContentID {
	var id identity.ContentID
	for index := range id {
		id[index] = seed + byte(index)
	}
	return id
}

func valueSourceSemanticPreimage(t *testing.T, domain string, write func(*framing.Writer) error) identity.ContentID {
	t.Helper()
	hash := sha256.New()
	var writer framing.Writer
	if err := writer.Reset(hash, domain, 1); err != nil {
		t.Fatal(err)
	}
	if err := writer.Record(1); err != nil {
		t.Fatal(err)
	}
	if err := write(&writer); err != nil {
		t.Fatal(err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}

func valueSourceRolePreimage(t *testing.T, domain string, owner identity.ContentID, write func(*framing.Writer) error) identity.ContentID {
	t.Helper()
	hash := sha256.New()
	var writer framing.Writer
	if err := writer.Reset(hash, domain, 1); err != nil {
		t.Fatal(err)
	}
	if err := writer.Record(1); err != nil {
		t.Fatal(err)
	}
	if err := writer.Bytes(owner[:]); err != nil {
		t.Fatal(err)
	}
	if err := write(&writer); err != nil {
		t.Fatal(err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}
