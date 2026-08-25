package result

import (
	"testing"

	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/identity"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// TestProjectKeepsTypeConformanceOnItsOwnPlane proves the result owner gives a
// conformance observation its own geometry. It is not a branch-value subject,
// whose polarity is judged from the same column but concluded differently, and
// it is not static: it measures a value the solve produced, so it carries the
// producer geometry a static row never has.
func TestProjectKeepsTypeConformanceOnItsOwnPlane(t *testing.T) {
	mount, module := resultGeometryMount(t)
	coordinateID := resultGeometryID(1)
	coordinate, coordinateOK := NewValueCoordinate(coordinateID, module)
	if !coordinateOK {
		t.Fatal("value coordinate")
	}
	artifactID := mount.Snapshot.ArtifactID()
	observation := anadiag.Observation{
		ID: observationID(2), Mount: module, Artifact: artifactID, Local: observationID(3),
		Kind:     structure.DiagnosticObservationTypeConformance,
		Location: source.Span{File: "geometry.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 8},
		Conformance: anadiag.Conformance{
			Site: diagnostic.SiteCallArgument, Owner: observationID(4), Measured: observationID(5),
			Declared: observationID(6), Span: observationID(7), Position: 0,
			ValueID:     coordinateID,
			DeclaredMay: runtimekind.Bit(runtimekind.String), Target: "string", Subject: "x", Callee: "takes_string",
			Evidence: []identity.ContentID{observationID(8)},
			Producers: []anadiag.Producer{{
				Key: "value-transfer", Occurrence: observationID(9), Point: observationID(10), Anchor: observationID(8),
			}},
		},
	}
	geometry, projected := Project(resultGeometryID(9), []programmount.MountedArtifact{mount}, []ValueCoordinate{coordinate}, []anadiag.Observation{observation})
	if !projected || !geometry.Valid() {
		t.Fatal("result geometry projection")
	}
	if len(geometry.BranchObservations) != 0 {
		t.Fatalf("type-conformance occupied branch geometry: %d rows", len(geometry.BranchObservations))
	}
	if len(geometry.StaticObservations) != 0 {
		t.Fatalf("type-conformance occupied static geometry: %d rows", len(geometry.StaticObservations))
	}
	if len(geometry.ConformanceObservations) != 1 || geometry.ConformanceObservations[0].Kind != structure.DiagnosticObservationTypeConformance ||
		!geometry.ConformanceObservations[0].Conformance.Available() {
		t.Fatalf("conformance geometry = %#v", geometry.ConformanceObservations)
	}
}

func resultGeometryID(seed byte) identity.ContentID {
	var id identity.ContentID
	id[0] = seed
	return id
}

func observationID(seed byte) identity.ContentID { return resultGeometryID(seed + 16) }

func resultGeometryMount(t *testing.T) (programmount.MountedArtifact, identity.ContentID) {
	t.Helper()
	const text = "return 1\n"
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := testfixture.SealSource(contract, "geometry.lua", []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	program, programOK := mounts.Program(shard)
	module, moduleOK := linked.Project().ModuleKey(shard)
	if !shardOK || !programOK || !moduleOK || program == nil {
		t.Fatal("geometry fixture mount")
	}
	compilation, compilationOK := composite.Build()
	if !compilationOK {
		t.Fatal("program schema")
	}
	grammar := compilation.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	if !grammar.Available() || !issuanceOK {
		t.Fatal("artifact compiler inputs")
	}
	compiled, failure := artifactcompiler.CompileDetailed(program, grammar, issuance)
	if failure.Available() || compiled == nil || !compiled.Available() {
		t.Fatalf("compile geometry artifact: %s", failure.Error())
	}
	vocabulary, vocabularyOK := composite.StructureVocabulary(compilation)
	snapshot, lowered := ingress.Lower(compiled, vocabulary)
	if !vocabularyOK || !lowered || snapshot == nil || !snapshot.Available() {
		t.Fatal("lower geometry snapshot")
	}
	mountedProgram := programmount.Program{ModuleKey: module, Program: snapshot.Program()}
	mount := programmount.MountedArtifact{Program: mountedProgram, Snapshot: snapshot}
	if !mount.Available() {
		t.Fatal("mount geometry snapshot")
	}
	return mount, module
}

// TestGeometryAddressesValueRowsByMountedPortableIdentity states what replaced
// the dense Value ordinal at this boundary. A diagnostic population carries the
// owner-issued ValueID from the sealed observation site and nothing else, so
// the geometry has to answer for the pair (mount, ValueID) - a positional index
// into a Value-width vector is not available to it and is not reconstructed.
//
// Mount-qualification is the half that a bare identity map would lose: two
// mounts may each issue the same portable ValueID for their own value, and the
// rows they name are different rows. An identity the census never issued has no
// row, and none is invented for it.
func TestGeometryAddressesValueRowsByMountedPortableIdentity(t *testing.T) {
	mount, module := resultGeometryMount(t)
	coordinateID := resultGeometryID(1)
	coordinate, coordinateOK := NewValueCoordinate(coordinateID, module)
	if !coordinateOK {
		t.Fatal("value coordinate")
	}
	geometry, projected := Project(resultGeometryID(9), []programmount.MountedArtifact{mount}, []ValueCoordinate{coordinate}, nil)
	if !projected || !geometry.Valid() {
		t.Fatal("result geometry projection")
	}
	row, rowOK := geometry.valueResultID(module, coordinateID)
	if !rowOK || !row.Available() {
		t.Fatalf("mounted portable identity resolved to %v/%v", row, rowOK)
	}
	if _, ok := geometry.valueResultID(resultGeometryID(2), coordinateID); ok {
		t.Fatal("a foreign mount reached another mount's value row")
	}
	if _, ok := geometry.valueResultID(module, resultGeometryID(3)); ok {
		t.Fatal("an identity the census never issued resolved to a row")
	}
	if _, ok := geometry.valueResultID(identity.ContentID{}, coordinateID); ok {
		t.Fatal("an absent mount resolved to a row")
	}
}
