package testfixture

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	schemaissuance "github.com/wippyai/go-lua/analysis/schema/issuance"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
)

type emptySchemaSurface struct{ kind schema.SurfaceKind }

func (surface emptySchemaSurface) Kind() schema.SurfaceKind { return surface.kind }
func (emptySchemaSurface) Entries() []schema.Entry          { return nil }
func (emptySchemaSurface) Seal(schema.View, schema.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

// EmptyProgramIssuancePlan seals the real Program issuance vocabulary with no
// rule subscriptions. It is the canonical fixture for tests whose subject is
// artifact construction rather than a composed domain rule inventory.
func EmptyProgramIssuancePlan(t testing.TB) schemaissuance.Plan {
	t.Helper()
	entries, ok := programissuance.Entries()
	if !ok {
		t.Fatal("Program issuance declarations unavailable")
	}
	builder := schema.NewBuilder()
	builder.Register(emptySchemaSurface{schema.SurfaceKindStructure})
	builder.Register(emptySchemaSurface{schema.SurfaceKindAxis})
	builder.Register(schemaissuance.NewSurface(entries))
	for kind := schema.SurfaceKindRule; kind <= schema.SurfaceKindObservation; kind++ {
		builder.Register(emptySchemaSurface{kind})
	}
	sealed, failure := builder.Seal()
	if failure.Available() || sealed == nil {
		t.Fatalf("Program issuance schema refused: %+v", failure)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindIssuance)
	table, tableOK := schemaissuance.NewTable(view)
	plan, planOK := schemaissuance.NewPlan(table, nil)
	if !viewOK || !tableOK || !planOK {
		t.Fatal("Program issuance plan unavailable")
	}
	return plan
}
