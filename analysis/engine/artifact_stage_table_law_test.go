package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/rows"
	analysisschema "github.com/wippyai/go-lua/analysis/schema"
	schemaissuance "github.com/wippyai/go-lua/analysis/schema/issuance"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	"github.com/wippyai/go-lua/analysis/schema/seal"
)

type artifactStageEmptySurface struct{ kind analysisschema.SurfaceKind }

func (surface artifactStageEmptySurface) Kind() analysisschema.SurfaceKind { return surface.kind }
func (artifactStageEmptySurface) Entries() []analysisschema.Entry          { return nil }
func (artifactStageEmptySurface) Seal(seal.View, seal.Sealed) analysisschema.SealFailure {
	return analysisschema.SealFailure{}
}

// installArtifactStageTable gives a scalar-law fixture the exact canonical
// Program stage declarations. Production receives this same table from the
// compiled issuance Plan; tests build the owner surface directly so they do
// not restate its stage graph.
func installArtifactStageTable(t testing.TB, spec *rows.ArtifactScalarSpec) {
	t.Helper()
	entries, entriesOK := programissuance.Entries()
	if !entriesOK {
		t.Fatal("Program issuance entries")
	}
	builder := seal.NewBuilder()
	builder.Register(artifactStageEmptySurface{analysisschema.SurfaceKindStructure})
	builder.Register(artifactStageEmptySurface{analysisschema.SurfaceKindAxis})
	builder.Register(schemaissuance.NewSurface(entries))
	for kind := analysisschema.SurfaceKindRule; kind <= analysisschema.SurfaceKindObservation; kind++ {
		builder.Register(artifactStageEmptySurface{kind})
	}
	sealed, failure := builder.Seal()
	if failure.Available() || sealed == nil || !sealed.Available() {
		t.Fatalf("Program issuance seal: %+v", failure)
	}
	view, viewOK := sealed.Surface(analysisschema.SurfaceKindIssuance)
	table, tableOK := schemaissuance.NewTable(view)
	if !viewOK || !tableOK || !spec.InstallStageTable(table) {
		t.Fatal("Program stage table installation")
	}
}
