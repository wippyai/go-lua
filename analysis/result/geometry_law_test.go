package result

import (
	"testing"

	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// TestProjectKeepsTypeConformanceOffTheBranchPlane proves the result owner
// classifies a call-argument observation as static geometry. It must not be
// mistaken for a branch-value subject, whose producers carry execution-point
// evidence for guard polarity.
func TestProjectKeepsTypeConformanceOffTheBranchPlane(t *testing.T) {
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
			Site: diagnostic.SiteCallArgument, Call: observationID(4), Argument: observationID(5),
			Declared: observationID(6), Span: observationID(7), Position: 0,
			DeclaredMay: runtimekind.Bit(runtimekind.String), Target: "string",
			Evidence: []identity.ContentID{observationID(8)},
		},
	}
	geometry, projected := Project(resultGeometryID(9), []Mount{mount}, []ValueCoordinate{coordinate}, []anadiag.Observation{observation})
	if !projected || !geometry.Valid() {
		t.Fatal("result geometry projection")
	}
	if len(geometry.BranchObservations) != 0 {
		t.Fatalf("type-conformance occupied branch geometry: %d rows", len(geometry.BranchObservations))
	}
	if len(geometry.StaticObservations) != 1 || geometry.StaticObservations[0].Kind != structure.DiagnosticObservationTypeConformance ||
		!geometry.StaticObservations[0].Conformance.Available() {
		t.Fatalf("static conformance geometry = %#v", geometry.StaticObservations)
	}
}

func resultGeometryID(seed byte) identity.ContentID {
	var id identity.ContentID
	id[0] = seed
	return id
}

func observationID(seed byte) identity.ContentID { return resultGeometryID(seed + 16) }

func resultGeometryMount(t *testing.T) (Mount, identity.ContentID) {
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
	compilation, compilationOK := composite.Global()
	if !compilationOK {
		t.Fatal("program schema")
	}
	compiled, failure := composite.CompileArtifactDetailed(program, compilation)
	if failure.Available() || compiled == nil || !compiled.Available() {
		t.Fatalf("compile geometry artifact: %s", failure.Error())
	}
	vocabulary, vocabularyOK := composite.StructureVocabulary()
	snapshot, lowered := ingress.Lower(compiled, vocabulary)
	if !vocabularyOK || !lowered || snapshot == nil || !snapshot.Available() {
		t.Fatal("lower geometry snapshot")
	}
	mount, mounted := NewMount(snapshot, module)
	if !mounted {
		t.Fatal("mount geometry snapshot")
	}
	return mount, module
}
