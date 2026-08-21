package programdiagnostic

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/internal/framing"
)

// referenceDiagnosticObservationID is an independent fresh-hasher reference
// for the exact diagnostic observation preimage. It intentionally retains the
// Record(1) event so a framing migration cannot silently alter IDs.
func referenceDiagnosticObservationID(programID identity.ContentID, kind structure.DiagnosticObservationKind, location programsource.Span, branch branchConditionIdentityPayload, unresolved unresolvedTypeReferenceIdentityPayload, value unresolvedValueReferenceIdentityPayload, conformance typeConformanceIdentityPayload) identity.ContentID {
	if !programID.Available() || !kind.Available() || !ValidSpan(location) {
		return identity.ContentID{}
	}
	hashValue := sha256.New()
	var writer framing.Writer
	if writer.Reset(hashValue, "program/transformer/diagnostic-observation", 1) != nil || writer.Record(1) != nil ||
		writer.Bytes(programID[:]) != nil || writer.Uint(uint64(kind)) != nil || writer.String(location.File) != nil ||
		writer.Uint(uint64(location.StartLine)) != nil || writer.Uint(uint64(location.StartCol)) != nil ||
		writer.Uint(uint64(location.EndLine)) != nil || writer.Uint(uint64(location.EndCol)) != nil {
		return identity.ContentID{}
	}
	switch kind {
	case structure.DiagnosticObservationBranchCondition:
		if !branch.available() || writer.Bytes(branch.decision[:]) != nil || writer.Bytes(branch.value[:]) != nil ||
			writer.Count(uint64(len(branch.points))) != nil || !writeEvidencePoints(&writer, branch.points) {
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
			writer.Bytes(conformance.owner[:]) != nil || writer.Bytes(conformance.measured[:]) != nil ||
			writer.Bytes(conformance.declared[:]) != nil || writer.Bytes(conformance.span[:]) != nil ||
			writer.Uint(uint64(conformance.position)) != nil || writer.Count(uint64(len(conformance.points))) != nil ||
			!writeEvidencePoints(&writer, conformance.points) {
			return identity.ContentID{}
		}
	default:
		return identity.ContentID{}
	}
	if writer.Finish() != nil {
		return identity.ContentID{}
	}
	var id identity.ContentID
	if sum := hashValue.Sum(id[:0]); len(sum) != len(id) {
		return identity.ContentID{}
	}
	return id
}

func diagnosticObservationIdentityLawFixtures() []struct {
	name        string
	programID   identity.ContentID
	kind        structure.DiagnosticObservationKind
	location    programsource.Span
	branch      branchConditionIdentityPayload
	unresolved  unresolvedTypeReferenceIdentityPayload
	value       unresolvedValueReferenceIdentityPayload
	conformance typeConformanceIdentityPayload
} {
	programID := identity.ContentID{1}
	loc := programsource.Span{File: "a.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 5}
	loc2 := programsource.Span{File: "b.lua", StartLine: 3, StartCol: 2, EndLine: 4, EndCol: 9}
	return []struct {
		name        string
		programID   identity.ContentID
		kind        structure.DiagnosticObservationKind
		location    programsource.Span
		branch      branchConditionIdentityPayload
		unresolved  unresolvedTypeReferenceIdentityPayload
		value       unresolvedValueReferenceIdentityPayload
		conformance typeConformanceIdentityPayload
	}{
		{
			name: "branch-single-point", programID: programID, kind: structure.DiagnosticObservationBranchCondition, location: loc,
			branch: branchConditionIdentityPayload{decision: identity.ContentID{2}, value: identity.ContentID{3}, points: []identity.ContentID{{4}}},
		},
		{
			name: "branch-multi-point", programID: programID, kind: structure.DiagnosticObservationBranchCondition, location: loc2,
			branch: branchConditionIdentityPayload{decision: identity.ContentID{5}, value: identity.ContentID{6}, points: []identity.ContentID{{7}, {8}, {9}}},
		},
		{
			name: "unresolved-type-single", programID: programID, kind: structure.DiagnosticObservationTypeReferenceUnresolved, location: loc,
			unresolved: unresolvedTypeReferenceIdentityPayload{reference: identity.ContentID{10}, path: []string{"foo"}},
		},
		{
			name: "unresolved-type-nested", programID: programID, kind: structure.DiagnosticObservationTypeReferenceUnresolved, location: loc2,
			unresolved: unresolvedTypeReferenceIdentityPayload{reference: identity.ContentID{11}, root: identity.ContentID{12}, path: []string{"foo", "bar", "baz"}},
		},
		{
			name: "unresolved-value", programID: programID, kind: structure.DiagnosticObservationValueReferenceUnresolved, location: loc,
			value: unresolvedValueReferenceIdentityPayload{read: identity.ContentID{13}, cell: identity.ContentID{14}, name: "x"},
		},
		{
			name: "conformance-call-argument", programID: programID, kind: structure.DiagnosticObservationTypeConformance, location: loc2,
			conformance: typeConformanceIdentityPayload{
				site: DiagnosticObservationSiteCallArgument, owner: identity.ContentID{15}, measured: identity.ContentID{16},
				declared: identity.ContentID{17}, span: identity.ContentID{18}, position: 2, points: []identity.ContentID{{19}, {20}},
			},
		},
		{
			name: "conformance-member-absent", programID: programID, kind: structure.DiagnosticObservationTypeConformance, location: loc,
			conformance: typeConformanceIdentityPayload{
				site: DiagnosticObservationSiteMemberAbsent, owner: identity.ContentID{21}, measured: identity.ContentID{22},
				declared: identity.ContentID{23}, span: identity.ContentID{24}, position: 0, points: []identity.ContentID{{25}},
			},
		},
	}
}

func diagnosticObservationIdentityFixtureID(fixture struct {
	name        string
	programID   identity.ContentID
	kind        structure.DiagnosticObservationKind
	location    programsource.Span
	branch      branchConditionIdentityPayload
	unresolved  unresolvedTypeReferenceIdentityPayload
	value       unresolvedValueReferenceIdentityPayload
	conformance typeConformanceIdentityPayload
}) identity.ContentID {
	switch fixture.kind {
	case structure.DiagnosticObservationBranchCondition:
		return BranchConditionIdentity(fixture.programID, fixture.location, fixture.branch.decision, fixture.branch.value, fixture.branch.points)
	case structure.DiagnosticObservationTypeReferenceUnresolved:
		return TypeReferenceUnresolvedIdentity(fixture.programID, fixture.location, fixture.unresolved.reference, fixture.unresolved.root, fixture.unresolved.path)
	case structure.DiagnosticObservationValueReferenceUnresolved:
		return ValueReferenceUnresolvedIdentity(fixture.programID, fixture.location, fixture.value.read, fixture.value.cell, fixture.value.name)
	case structure.DiagnosticObservationTypeConformance:
		return TypeConformanceIdentity(fixture.programID, fixture.location, fixture.conformance.site, fixture.conformance.owner, fixture.conformance.measured, fixture.conformance.declared, fixture.conformance.span, fixture.conformance.position, fixture.conformance.points)
	default:
		return identity.ContentID{}
	}
}

func TestDiagnosticObservationIdentityPreservesFramedPreimage(t *testing.T) {
	for _, fixture := range diagnosticObservationIdentityLawFixtures() {
		t.Run(fixture.name, func(t *testing.T) {
			want := referenceDiagnosticObservationID(fixture.programID, fixture.kind, fixture.location, fixture.branch, fixture.unresolved, fixture.value, fixture.conformance)
			got := diagnosticObservationIdentityFixtureID(fixture)
			if !want.Available() || got != want {
				t.Fatalf("identity diverged from Record(1) reference: got %v want %v", got, want)
			}
		})
	}
}

func TestDiagnosticObservationIdentityPoolIsStableUnderReuse(t *testing.T) {
	fixtures := diagnosticObservationIdentityLawFixtures()
	for iteration := 0; iteration < 50; iteration++ {
		for _, fixture := range fixtures {
			got := diagnosticObservationIdentityFixtureID(fixture)
			want := referenceDiagnosticObservationID(fixture.programID, fixture.kind, fixture.location, fixture.branch, fixture.unresolved, fixture.value, fixture.conformance)
			if got != want {
				t.Fatalf("iteration %d fixture %s: pooled identity diverged: got %v want %v", iteration, fixture.name, got, want)
			}
		}
	}
}

func TestDiagnosticObservationIdentityRejectsDuplicateEvidence(t *testing.T) {
	programID := identity.ContentID{1}
	location := programsource.Span{File: "diagnostic.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}
	if id := BranchConditionIdentity(programID, location, identity.ContentID{2}, identity.ContentID{3}, []identity.ContentID{{4}, {4}}); id.Available() {
		t.Fatal("duplicate evidence points were admitted")
	}
	if id := TypeConformanceIdentity(programID, location, DiagnosticObservationSiteAssignment, identity.ContentID{5}, identity.ContentID{6}, identity.ContentID{7}, identity.ContentID{8}, 0, nil); id.Available() {
		t.Fatal("empty conformance evidence was admitted")
	}
}

func BenchmarkDiagnosticObservationIdentityPool(b *testing.B) {
	fixture := diagnosticObservationIdentityLawFixtures()[len(diagnosticObservationIdentityLawFixtures())-1]
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		diagnosticObservationIdentityFixtureID(fixture)
	}
}

func BenchmarkDiagnosticObservationIdentityReference(b *testing.B) {
	fixture := diagnosticObservationIdentityLawFixtures()[len(diagnosticObservationIdentityLawFixtures())-1]
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		referenceDiagnosticObservationID(fixture.programID, fixture.kind, fixture.location, fixture.branch, fixture.unresolved, fixture.value, fixture.conformance)
	}
}
