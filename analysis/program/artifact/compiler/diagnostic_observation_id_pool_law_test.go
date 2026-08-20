package compiler

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/internal/framing"
)

// referenceDiagnosticObservationID is the pre-pooling preimage: a fresh
// sha256.New() per call, framed exactly as diagnosticObservationID frames
// it. It exists only in this test as the old-path reference the pooled
// hasher's byte-for-byte identity law is checked against.
func referenceDiagnosticObservationID(owner identity.ContentID, kind structure.DiagnosticObservationKind, location programsource.Span, branch diagnosticBranchConditionRow, unresolved diagnosticUnresolvedTypeReferenceRow, value diagnosticUnresolvedValueReferenceRow, conformance diagnosticTypeConformanceRow) identity.ContentID {
	if !owner.Available() || !kind.Available() || !validDiagnosticSpan(location) {
		return identity.ContentID{}
	}
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, "program/transformer/diagnostic-observation", 1) != nil || writer.Record(1) != nil ||
		writer.Bytes(owner[:]) != nil || writer.Uint(uint64(kind)) != nil || writer.String(location.File) != nil ||
		writer.Uint(uint64(location.StartLine)) != nil || writer.Uint(uint64(location.StartCol)) != nil ||
		writer.Uint(uint64(location.EndLine)) != nil || writer.Uint(uint64(location.EndCol)) != nil {
		return identity.ContentID{}
	}
	switch kind {
	case structure.DiagnosticObservationBranchCondition:
		if !branch.available() || writer.Bytes(branch.decision[:]) != nil || writer.Bytes(branch.value[:]) != nil ||
			writer.Count(uint64(len(branch.points))) != nil || !writeDiagnosticEvidencePoints(&writer, branch.points) {
			return identity.ContentID{}
		}
	case structure.DiagnosticObservationTypeReferenceUnresolved:
		if !unresolved.available() || writer.Bytes(unresolved.reference[:]) != nil || writer.Bytes(unresolved.root[:]) != nil ||
			writer.Count(uint64(len(unresolved.path))) != nil {
			return identity.ContentID{}
		}
		for _, component := range unresolved.path {
			if writer.String(component) != nil {
				return identity.ContentID{}
			}
		}
	case structure.DiagnosticObservationValueReferenceUnresolved:
		if !value.available() || writer.Bytes(value.read[:]) != nil || writer.Bytes(value.cell[:]) != nil || writer.String(value.name) != nil {
			return identity.ContentID{}
		}
	case structure.DiagnosticObservationTypeConformance:
		if !conformance.available() || writer.Uint(uint64(conformance.site)) != nil ||
			writer.Bytes(conformance.owner[:]) != nil || writer.Bytes(conformance.value[:]) != nil ||
			writer.Bytes(conformance.declared[:]) != nil || writer.Bytes(conformance.span[:]) != nil ||
			writer.Uint(uint64(conformance.position)) != nil || writer.Count(uint64(len(conformance.points))) != nil ||
			!writeDiagnosticEvidencePoints(&writer, conformance.points) {
			return identity.ContentID{}
		}
	default:
		return identity.ContentID{}
	}
	if writer.Finish() != nil {
		return identity.ContentID{}
	}
	var id identity.ContentID
	if sum := hash.Sum(id[:0]); len(sum) != len(id) {
		return identity.ContentID{}
	}
	return id
}

func diagnosticObservationIDLawFixtures() []struct {
	name        string
	owner       identity.ContentID
	kind        structure.DiagnosticObservationKind
	location    programsource.Span
	branch      diagnosticBranchConditionRow
	unresolved  diagnosticUnresolvedTypeReferenceRow
	value       diagnosticUnresolvedValueReferenceRow
	conformance diagnosticTypeConformanceRow
} {
	owner := identity.ContentID{1}
	loc := programsource.Span{File: "a.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 5}
	loc2 := programsource.Span{File: "b.lua", StartLine: 3, StartCol: 2, EndLine: 4, EndCol: 9}
	return []struct {
		name        string
		owner       identity.ContentID
		kind        structure.DiagnosticObservationKind
		location    programsource.Span
		branch      diagnosticBranchConditionRow
		unresolved  diagnosticUnresolvedTypeReferenceRow
		value       diagnosticUnresolvedValueReferenceRow
		conformance diagnosticTypeConformanceRow
	}{
		{
			name: "branch-single-point", owner: owner, kind: structure.DiagnosticObservationBranchCondition, location: loc,
			branch: diagnosticBranchConditionRow{decision: identity.ContentID{2}, value: identity.ContentID{3}, points: []identity.ContentID{{4}}},
		},
		{
			name: "branch-multi-point", owner: owner, kind: structure.DiagnosticObservationBranchCondition, location: loc2,
			branch: diagnosticBranchConditionRow{decision: identity.ContentID{5}, value: identity.ContentID{6}, points: []identity.ContentID{{7}, {8}, {9}}},
		},
		{
			name: "unresolved-type-single", owner: owner, kind: structure.DiagnosticObservationTypeReferenceUnresolved, location: loc,
			unresolved: diagnosticUnresolvedTypeReferenceRow{reference: identity.ContentID{10}, path: []string{"foo"}},
		},
		{
			name: "unresolved-type-nested", owner: owner, kind: structure.DiagnosticObservationTypeReferenceUnresolved, location: loc2,
			unresolved: diagnosticUnresolvedTypeReferenceRow{reference: identity.ContentID{11}, root: identity.ContentID{12}, path: []string{"foo", "bar", "baz"}},
		},
		{
			name: "unresolved-value", owner: owner, kind: structure.DiagnosticObservationValueReferenceUnresolved, location: loc,
			value: diagnosticUnresolvedValueReferenceRow{read: identity.ContentID{13}, cell: identity.ContentID{14}, name: "x"},
		},
		{
			name: "conformance-call-argument", owner: owner, kind: structure.DiagnosticObservationTypeConformance, location: loc2,
			conformance: diagnosticTypeConformanceRow{
				site: diagnosticTypeConformanceSiteCallArgument, owner: identity.ContentID{15}, value: identity.ContentID{16},
				declared: identity.ContentID{17}, span: identity.ContentID{18}, position: 2, points: []identity.ContentID{{19}, {20}},
			},
		},
		{
			name: "conformance-member-absent", owner: owner, kind: structure.DiagnosticObservationTypeConformance, location: loc,
			conformance: diagnosticTypeConformanceRow{
				site: diagnosticTypeConformanceSiteMemberAbsent, owner: identity.ContentID{21}, value: identity.ContentID{22},
				declared: identity.ContentID{23}, span: identity.ContentID{24}, position: 0, points: []identity.ContentID{{25}},
			},
		},
	}
}

// TestDiagnosticObservationIDPoolPreservesPreimage is the W2 gate: pooling
// the SHA-256 hasher through acquireHash/releaseHash must not change one
// bit of the diagnosticObservationID preimage. It compares the pooled path
// against the unpooled reference implementation over a generated row set
// covering every DiagnosticObservationKind and both single- and multi-point
// evidence.
func TestDiagnosticObservationIDPoolPreservesPreimage(t *testing.T) {
	for _, fixture := range diagnosticObservationIDLawFixtures() {
		t.Run(fixture.name, func(t *testing.T) {
			want := referenceDiagnosticObservationID(fixture.owner, fixture.kind, fixture.location, fixture.branch, fixture.unresolved, fixture.value, fixture.conformance)
			if !want.Available() {
				t.Fatal("reference implementation produced no ID for a valid fixture")
			}
			got := diagnosticObservationID(fixture.owner, fixture.kind, fixture.location, fixture.branch, fixture.unresolved, fixture.value, fixture.conformance)
			if got != want {
				t.Fatalf("pooled ID diverged from reference: got %v want %v", got, want)
			}
		})
	}
}

// TestDiagnosticObservationIDPoolIsStableUnderReuse forces the hash pool
// through many acquire/release cycles, interleaving distinct rows, and
// checks every recomputation is both self-consistent and free of
// cross-call state bleed from a reused hash.Hash.
func TestDiagnosticObservationIDPoolIsStableUnderReuse(t *testing.T) {
	fixtures := diagnosticObservationIDLawFixtures()
	for iteration := 0; iteration < 50; iteration++ {
		for _, fixture := range fixtures {
			got := diagnosticObservationID(fixture.owner, fixture.kind, fixture.location, fixture.branch, fixture.unresolved, fixture.value, fixture.conformance)
			want := referenceDiagnosticObservationID(fixture.owner, fixture.kind, fixture.location, fixture.branch, fixture.unresolved, fixture.value, fixture.conformance)
			if got != want {
				t.Fatalf("iteration %d fixture %s: pooled ID diverged from reference: got %v want %v", iteration, fixture.name, got, want)
			}
		}
	}
}

// BenchmarkDiagnosticObservationIDPool isolates diagnosticObservationID's
// own per-call allocation cost from the rest of the compile pipeline: W2
// pools the hasher, so this should show 0 allocs/op instead of 1.
func BenchmarkDiagnosticObservationIDPool(b *testing.B) {
	fixture := diagnosticObservationIDLawFixtures()[len(diagnosticObservationIDLawFixtures())-1]
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		diagnosticObservationID(fixture.owner, fixture.kind, fixture.location, fixture.branch, fixture.unresolved, fixture.value, fixture.conformance)
	}
}

// BenchmarkDiagnosticObservationIDReference runs the pre-pooling reference
// path (fresh sha256.New() per call) for direct comparison against
// BenchmarkDiagnosticObservationIDPool.
func BenchmarkDiagnosticObservationIDReference(b *testing.B) {
	fixture := diagnosticObservationIDLawFixtures()[len(diagnosticObservationIDLawFixtures())-1]
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		referenceDiagnosticObservationID(fixture.owner, fixture.kind, fixture.location, fixture.branch, fixture.unresolved, fixture.value, fixture.conformance)
	}
}
