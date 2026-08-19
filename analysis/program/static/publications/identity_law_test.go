package publications

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	"github.com/wippyai/go-lua/internal/framing"
)

const (
	sectionDomain  = "program/static/publications-law"
	sectionVersion = 1
)

func ledgerCounts() [keyspace.FamilyCount]uint32 {
	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyAssign] = 2
	counts[keyspace.FamilyTypeRef] = 3
	counts[keyspace.FamilyTypeAlias] = 1
	counts[keyspace.FamilyCell] = 1
	counts[keyspace.FamilyTypePublication] = 2
	return counts
}

func term(family keyspace.Family, ordinal uint32) keyspace.Term {
	return keyspace.MakeTerm(family, ordinal)
}

// ledgerRefs supplies the sealed References stage input. Its third row is
// deliberately unresolved so the admission law has a negative case.
func ledgerRefs(t *testing.T) staticrefs.Table {
	t.Helper()
	table, err := staticrefs.Build(staticrefs.Input{TypeRef: []staticrefs.TypeRef{
		{Resolution: staticrefs.Declaration, Source: []keyspace.Key{1}, Target: term(keyspace.FamilyTypeAlias, 1)},
		{
			Resolution: staticrefs.CanonicalPath, Source: []keyspace.Key{2, 3},
			Root: term(keyspace.FamilyCell, 1), Canonical: []keyspace.Key{7},
		},
		{Resolution: staticrefs.Unresolved, Source: []keyspace.Key{4}},
	}}, ledgerCounts())
	if err != nil {
		t.Fatalf("references.Build: %v", err)
	}
	return table
}

func ledgerInput() Input {
	return Input{Type: []Publication{
		{Assign: term(keyspace.FamilyAssign, 1), Pair: 0, Target: term(keyspace.FamilyTypeRef, 1)},
		{Assign: term(keyspace.FamilyAssign, 2), Pair: 1, Target: term(keyspace.FamilyTypeRef, 2)},
	}}
}

func sectionBytes(t *testing.T, input Input) []byte {
	t.Helper()
	table, err := Build(input, ledgerCounts(), ledgerRefs(t))
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
// field and order distinction this vertical retains.
func TestAuthoredDistinctionsReachTheSection(t *testing.T) {
	for _, test := range []struct {
		name    string
		perturb func(*Input)
	}{
		{"publication.assign", func(in *Input) { in.Type[0].Assign = term(keyspace.FamilyAssign, 2) }},
		{"publication.pair", func(in *Input) { in.Type[0].Pair = 5 }},
		{"publication.target", func(in *Input) { in.Type[0].Target = term(keyspace.FamilyTypeRef, 2) }},
		{"publication.row-order", func(in *Input) { in.Type[0], in.Type[1] = in.Type[1], in.Type[0] }},
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

// TestBuildAdmitsOnlyResolvedTargets proves an unresolved name is never made
// public merely because an Assign happens to contain it. The admission comes
// from the References owner's published disposition, not from a family test
// restated here.
func TestBuildAdmitsOnlyResolvedTargets(t *testing.T) {
	input := ledgerInput()
	input.Type[0].Target = term(keyspace.FamilyTypeRef, 3)
	if _, err := Build(input, ledgerCounts(), ledgerRefs(t)); err == nil {
		t.Fatal("Build published an unresolved type reference")
	}
}

// TestBuildRefusesARepeatedWritePair proves one Assign position carries at
// most one publication.
func TestBuildRefusesARepeatedWritePair(t *testing.T) {
	input := ledgerInput()
	input.Type[1].Assign = input.Type[0].Assign
	input.Type[1].Pair = input.Type[0].Pair
	if _, err := Build(input, ledgerCounts(), ledgerRefs(t)); err == nil {
		t.Fatal("Build admitted two publications for one write pair")
	}
}
