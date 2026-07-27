package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
)

// TestCanonicalCarrierPrecisionProof pins the minimum algebra a reusable
// lexical relation must retain. The body syntax is built and sealed once;
// distinct caller bindings specialize the same immutable terms. No State,
// CFG, route, or body-solver callback participates in specialization.
func TestCanonicalCarrierPrecisionProof(t *testing.T) {
	reg := standard.Registry()
	shape := Shape{Params: 2}
	terms := NewArena(reg)
	effects := NewEffectArena(terms)

	valueParam := terms.Root(Root{Kind: RootParam, Index: 0})
	storedParam := terms.Root(Root{Kind: RootParam, Index: 1})
	valuePath := terms.Path(Root{Kind: RootParam, Index: 0})
	storedPath := terms.Path(Root{Kind: RootParam, Index: 1})
	nilValue := terms.Constant(typevalue.Nil(reg))
	guardedReturn := terms.SelectValue(terms.Truthy(valueParam), valueParam, nilValue)
	store, err := effects.PathStore(PathStoreConfig{
		HasAssignment: true,
		Assignment: PathStoreWriteConfig{
			Target: valuePath, Value: storedParam, SourcePath: storedPath, HasSourcePath: true,
		},
		Site: EffectSite{Owner: 1, Ordinal: 1},
	})
	if err != nil || valueParam == 0 || guardedReturn == 0 || store == 0 {
		t.Fatalf("carrier construction: identity=%d guarded=%d effect=%d err=%v", valueParam, guardedReturn, store, err)
	}
	terms.Seal()
	effects.Seal()
	valueNodes, guardNodes, effectNodes := len(terms.values), len(terms.guards), len(effects.nodes)

	type invocation struct {
		name              string
		value, stored     product.Value
		valuePath, source pathdom.Path
		wantGuarded       product.Value
	}
	invocations := []invocation{
		{
			name: "truthy", value: typevalue.LiteralString(reg, "first"), stored: typevalue.LiteralInt(reg, 11),
			valuePath: pathdom.NewPlaceholder(0), source: pathdom.NewPlaceholder(1),
			wantGuarded: typevalue.LiteralString(reg, "first"),
		},
		{
			name: "falsy", value: typevalue.LiteralBool(reg, false), stored: typevalue.LiteralString(reg, "second"),
			valuePath: pathdom.NewPlaceholder(2), source: pathdom.NewPlaceholder(3),
			wantGuarded: typevalue.Nil(reg),
		},
	}

	for _, invocation := range invocations {
		cursor, cursorErr := NewBindingCursor(shape,
			[]product.Value{invocation.value, invocation.stored},
			[]pathdom.Path{invocation.valuePath, invocation.source},
		)
		if cursorErr != nil {
			t.Fatalf("%s cursor: %v", invocation.name, cursorErr)
		}

		// OUT(result)=IN(param) is a term edge, not a copied Top value.
		identity, exact := terms.evalValue(valueParam, cursor, SpecializationContext{})
		if !exact || !product.Equal(reg, identity, invocation.value) {
			t.Fatalf("%s identity result = %#v/%v", invocation.name, identity, exact)
		}
		correlated, exact := terms.evalValue(guardedReturn, cursor, SpecializationContext{})
		if !exact || !product.Equal(reg, correlated, invocation.wantGuarded) {
			t.Fatalf("%s guarded result = %#v/%v", invocation.name, correlated, exact)
		}

		resolved, exact := effects.resolve(store, cursor, SpecializationContext{})
		if !exact || resolved.Kind != EffectPathStore || !resolved.PathStore.HasAssignment ||
			!resolved.PathStore.Assignment.Target.Equal(invocation.valuePath) ||
			!resolved.PathStore.Assignment.SourcePath.Equal(invocation.source) ||
			!product.Equal(reg, resolved.PathStore.Assignment.Value, invocation.stored) {
			t.Fatalf("%s heap/effect result = %#v/%v", invocation.name, resolved, exact)
		}
	}

	if len(terms.values) != valueNodes || len(terms.guards) != guardNodes || len(effects.nodes) != effectNodes {
		t.Fatalf("specialization grew sealed body syntax: values %d->%d guards %d->%d effects %d->%d",
			valueNodes, len(terms.values), guardNodes, len(terms.guards), effectNodes, len(effects.nodes))
	}
}
