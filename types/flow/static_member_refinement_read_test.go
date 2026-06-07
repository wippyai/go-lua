package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestStaticMemberRefinementReadsReturnsExistingAndBaseProjection(t *testing.T) {
	const sym = cfg.SymbolID(201)
	path := constraint.NewPath(sym, "entry").Field("meta").Field("id")
	existing := product.FromType(typ.String)
	base := product.FromType(typ.NewRecord().
		Field("meta", typ.NewRecord().Field("id", typ.Number).Build()).
		Build())
	state := PointState{
		StaticMembers: StaticMemberFactsDomain.Top().
			WithAddress(testStableAddressKey(t, SymbolPathKey(sym, path.Segments)), existing),
	}

	got := PointFactsOf(state).StaticMemberRefinementReads(path, base, true)

	if got.Existing.State != StateResolved || !product.Domain.Equal(got.Existing.Value, existing) {
		t.Fatalf("existing source = %#v, want static member string", got.Existing)
	}
	if got.Base.State != StateResolved || !typ.TypeEquals(got.Base.Value.ProjectValue(), typ.Number) {
		t.Fatalf("base source = %#v, want projected number", got.Base)
	}
	if preferred := got.Preferred(); preferred.State != StateResolved || !product.Domain.Equal(preferred.Value, existing) {
		t.Fatalf("preferred source = %#v, want existing", preferred)
	}
}

func TestStaticMemberRefinementReadsFallsBackToBaseProjection(t *testing.T) {
	const sym = cfg.SymbolID(202)
	path := constraint.NewPath(sym, "entry").Field("meta").Field("id")
	base := product.FromType(typ.NewRecord().
		Field("meta", typ.NewRecord().Field("id", typ.Boolean).Build()).
		Build())

	got := PointFactsOf(PointState{}).StaticMemberRefinementReads(path, base, true)

	if got.Existing.State != StateUnknown {
		t.Fatalf("existing source = %#v, want unknown", got.Existing)
	}
	if got.Base.State != StateResolved || !typ.TypeEquals(got.Base.Value.ProjectValue(), typ.Boolean) {
		t.Fatalf("base source = %#v, want projected boolean", got.Base)
	}
	if preferred := got.Preferred(); preferred.State != StateResolved || !typ.TypeEquals(preferred.Value.ProjectValue(), typ.Boolean) {
		t.Fatalf("preferred source = %#v, want base", preferred)
	}
}
