package sourcevalue

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestPreferExactHeapRootUsesSameIdentityStructuralRefinement(t *testing.T) {
	reg := standard.Registry()
	id := identity.ID{Kind: "test.table", Site: "heap-root", Index: 1}
	currentType := typetable.BuiltinTopMarker()
	current := product.Set(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, currentType), currentType), identity.Key, identity.Singleton(id))
	wantType := typ.NewArray(typ.String)
	want := product.Set(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, wantType), wantType), identity.Key, identity.Singleton(id))
	in := state.State{}.WriteHeapTableObject(reg, id, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: want}))

	got := PreferExactHeapRoot(reg, nil, in, current)
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, wantType) {
		t.Fatalf("preferred heap root = %v/%v, want %v", gotType, ok, wantType)
	}
}
