package references

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
)

const (
	sectionDomain  = "program/static/references-law"
	sectionVersion = 1
)

func ledgerCounts() [keyspace.FamilyCount]uint32 {
	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyTypeRef] = 3
	counts[keyspace.FamilyTypeAlias] = 1
	counts[keyspace.FamilyTypeInterface] = 1
	counts[keyspace.FamilyCell] = 2
	return counts
}

func term(family keyspace.Family, ordinal uint32) keyspace.Term {
	return keyspace.MakeTerm(family, ordinal)
}

// ledgerInput carries one row of every binder disposition, both qualification
// forms, and both key columns.
func ledgerInput() Input {
	return Input{TypeRef: []TypeRef{
		{Resolution: Declaration, Source: []keyspace.Key{1}, Target: term(keyspace.FamilyTypeAlias, 1)},
		{
			Resolution: CanonicalPath, Source: []keyspace.Key{2, 3},
			Root: term(keyspace.FamilyCell, 1), Canonical: []keyspace.Key{7, 8},
		},
		{Resolution: Unresolved, Source: []keyspace.Key{4, 5}, Root: term(keyspace.FamilyCell, 1)},
	}}
}

func sectionBytes(t *testing.T, input Input) []byte {
	t.Helper()
	table, err := Build(input, ledgerCounts())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var data bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&data, sectionDomain, sectionVersion); err != nil {
		t.Fatal(err)
	}
	if err := WriteContent(&writer, table); err != nil {
		t.Fatalf("WriteContent: %v", err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), data.Bytes()...)
}

func sectionReader(t *testing.T, data []byte) *framing.Reader {
	t.Helper()
	reader, err := framing.NewReader(data, len(data))
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Header(sectionDomain, sectionVersion); err != nil {
		t.Fatal(err)
	}
	return reader
}

// TestAuthoredDistinctionsReachTheSection proves the section byte stream, which
// is the same schema the Static ContentID digests, separates every authored
// field, arity, and order distinction this vertical retains.
func TestAuthoredDistinctionsReachTheSection(t *testing.T) {
	for _, test := range []struct {
		name    string
		perturb func(*Input)
	}{
		// The disposition carries its exclusive anchors with it, so the
		// perturbation replaces the whole row rather than one field.
		{"reference.resolution", func(in *Input) {
			in.TypeRef[0] = TypeRef{Resolution: Unresolved, Source: []keyspace.Key{1}}
		}},
		{"reference.target", func(in *Input) {
			in.TypeRef[0].Target = term(keyspace.FamilyTypeInterface, 1)
		}},
		{"reference.root", func(in *Input) { in.TypeRef[1].Root = term(keyspace.FamilyCell, 2) }},
		{"reference.source-key", func(in *Input) { in.TypeRef[0].Source[0] = 77 }},
		{"reference.source-arity", func(in *Input) {
			in.TypeRef[1].Source = append(in.TypeRef[1].Source, 9)
		}},
		{"reference.source-order", func(in *Input) {
			in.TypeRef[1].Source[0], in.TypeRef[1].Source[1] = in.TypeRef[1].Source[1], in.TypeRef[1].Source[0]
		}},
		{"reference.canonical-key", func(in *Input) { in.TypeRef[1].Canonical[0] = 77 }},
		{"reference.canonical-arity", func(in *Input) {
			in.TypeRef[1].Canonical = in.TypeRef[1].Canonical[:1]
		}},
		{"reference.canonical-order", func(in *Input) {
			in.TypeRef[1].Canonical[0], in.TypeRef[1].Canonical[1] = in.TypeRef[1].Canonical[1], in.TypeRef[1].Canonical[0]
		}},
		{"reference.row-order", func(in *Input) {
			in.TypeRef[0], in.TypeRef[2] = in.TypeRef[2], in.TypeRef[0]
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			base := sectionBytes(t, ledgerInput())
			perturbed := ledgerInput()
			test.perturb(&perturbed)
			if bytes.Equal(base, sectionBytes(t, perturbed)) {
				t.Fatal("authored distinction is absent from the section stream")
			}
		})
	}
}

// TestSectionRoundTripPreservesEveryAuthoredRow proves the section decoder
// recovers exactly the authored input the writer emitted.
func TestSectionRoundTripPreservesEveryAuthoredRow(t *testing.T) {
	encoded := sectionBytes(t, ledgerInput())
	reader := sectionReader(t, encoded)
	decoded, err := Decode(reader)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if err := reader.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if !bytes.Equal(encoded, sectionBytes(t, decoded)) {
		t.Fatal("round-tripped input did not reproduce the section stream")
	}
}

// TestScanValidatesWithoutRetainingRows proves the preflight half consumes the
// same stream shape as Decode.
func TestScanValidatesWithoutRetainingRows(t *testing.T) {
	encoded := sectionBytes(t, ledgerInput())
	reader := sectionReader(t, encoded)
	if err := Scan(reader); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if err := reader.Finish(); err != nil {
		t.Fatalf("Scan left the stream unconsumed: %v", err)
	}
}

// TestResolvedIsTheExactPublishableDisposition proves the column siblings
// consume admits exactly the two dispositions that mean something beyond the
// local spelling, so no consumer restates the test as a family check.
func TestResolvedIsTheExactPublishableDisposition(t *testing.T) {
	table, err := Build(ledgerInput(), ledgerCounts())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for ordinal, want := range map[uint32]bool{1: true, 2: true, 3: false} {
		row, ok := table.Ref(term(keyspace.FamilyTypeRef, ordinal))
		if !ok {
			t.Fatalf("Ref(%d) is absent", ordinal)
		}
		if row.Resolved() != want {
			t.Fatalf("Ref(%d).Resolved() = %v, want %v", ordinal, row.Resolved(), want)
		}
	}
	if row, ok := table.Ref(term(keyspace.FamilyTypeRef, 4)); ok || row.Resolution != 0 {
		t.Fatal("Ref admitted a term past the sealed denominator")
	}
	if row, ok := table.Ref(term(keyspace.FamilyTypeAlias, 1)); ok || row.Resolution != 0 {
		t.Fatal("Ref admitted a foreign-family term")
	}
}
