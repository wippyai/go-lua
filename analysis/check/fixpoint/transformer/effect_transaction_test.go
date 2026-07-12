package transformer

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestRowEffectsPreserveNonCommutingMutationOrderAndFailClosedWithoutResolver(t *testing.T) {
	reg := standard.Registry()
	shape := Shape{Params: 1}
	builder, certificate := emptyBuilder(t, reg, shape, nil)
	path := builder.Arena().Path(Root{Kind: RootParam, Index: 0})
	key := builder.Arena().Constant(typevalue.LiteralString(reg, "key"))
	firstValue := builder.Arena().Constant(typevalue.LiteralString(reg, "first"))
	secondValue := builder.Arena().Constant(typevalue.LiteralString(reg, "second"))
	makeMutation := func(value ValueTerm, ordinal uint32) EffectTerm {
		t.Helper()
		term, err := builder.EffectArena().IndexMutation(IndexMutationConfig{
			Invalidation: InvalidatePathConfig{Target: path, Scope: InvalidationScopeDescendants},
			Table:        path, Key: key, Value: value,
			Admission: dynamicindex.AdmissionAdmitted,
			Readback:  factflow.DynamicIndexReadbackKeyAndValue,
			Site:      EffectSite{Owner: 7, Ordinal: ordinal},
		})
		if err != nil {
			t.Fatal(err)
		}
		return term
	}
	first, second := makeMutation(firstValue, 1), makeMutation(secondValue, 2)
	sequence := []EffectTerm{first, second}
	forward, err := builder.Build(certificate, []Row{{Guard: builder.Arena().True(), Effects: sequence}})
	if err != nil {
		t.Fatal(err)
	}
	sequence[0], sequence[1] = sequence[1], sequence[0]
	reverse, err := builder.Build(certificate, []Row{{Guard: builder.Arena().True(), Effects: sequence}})
	if err != nil {
		t.Fatal(err)
	}
	if EqualRelation(forward, reverse) {
		t.Fatal("non-commuting mutation sequences compared equal")
	}
	cursor, err := NewBindingCursor(shape, []product.Value{product.Top()}, []pathdom.Path{{Root: "table"}})
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := forward.SpecializeWithContext(cursor, nil, SpecializationContext{}); ok || !reflect.DeepEqual(got, summary.Summary{}) {
		t.Fatal("effectful row specialized without resolver")
	}
	var gotOrder []uint32
	_, ok := forward.SpecializeWithEffects(cursor, nil, SpecializationContext{}, func(effects []ResolvedEffect) (summary.Summary, bool) {
		for _, effect := range effects {
			gotOrder = append(gotOrder, effect.Mutation.Site.Ordinal)
		}
		return summary.Summary{}, true
	})
	if !ok || !reflect.DeepEqual(gotOrder, []uint32{1, 2}) {
		t.Fatalf("resolved mutation order = %v/%v, want [1 2]", gotOrder, ok)
	}
	builder.effectCatalog = nil
	if _, err := builder.Build(certificate, []Row{{Guard: builder.Arena().True(), Effects: []EffectTerm{first}}}); err == nil {
		t.Fatal("effectful row admitted without an effect catalog verdict")
	}
}

func TestRebaseEffectDAGsIsAtomicAndPreservesSequence(t *testing.T) {
	reg := standard.Registry()
	shape := Shape{Params: 1}
	calleeTerms, callerTerms := NewArena(reg), NewArena(reg)
	callee, caller := NewEffectArena(calleeTerms), NewEffectArena(callerTerms)
	root := Root{Kind: RootParam, Index: 0}
	calleePath := calleeTerms.Path(root)
	key := calleeTerms.Constant(typevalue.LiteralString(reg, "key"))
	value := calleeTerms.Constant(typevalue.LiteralString(reg, "value"))
	makeMutation := func(ordinal uint32) EffectTerm {
		term, err := callee.IndexMutation(IndexMutationConfig{
			Invalidation: InvalidatePathConfig{Target: calleePath, Scope: InvalidationScopeDescendants},
			Table:        calleePath, Key: key, Value: value,
			Admission: dynamicindex.AdmissionAdmitted, Readback: factflow.DynamicIndexReadbackKeyAndValue,
			Site: EffectSite{Owner: 9, Ordinal: ordinal},
		})
		if err != nil {
			t.Fatal(err)
		}
		return term
	}
	first, second := makeMutation(1), makeMutation(2)
	bindings, err := NewTermRootBindings(shape, shape,
		[]ValueTerm{callerTerms.Root(root)}, []PathTerm{callerTerms.Path(root)})
	if err != nil {
		t.Fatal(err)
	}
	got, err := RebaseEffectDAGs(caller, callee, bindings, []EffectTerm{second, first})
	if err != nil {
		t.Fatal(err)
	}
	if caller.nodes[got.Effects[0]].site.Ordinal != 2 || caller.nodes[got.Effects[1]].site.Ordinal != 1 {
		t.Fatalf("rebase reordered effects: %#v", got.Effects)
	}
	valuesBefore, pathsBefore, effectsBefore := len(callerTerms.values), len(callerTerms.paths), len(caller.nodes)
	if got, err := RebaseEffectDAGs(caller, callee, bindings, []EffectTerm{first, EffectTerm(999)}); err == nil || len(got.Effects) != 0 {
		t.Fatal("malformed sequence did not fail closed")
	}
	if len(callerTerms.values) != valuesBefore || len(callerTerms.paths) != pathsBefore || len(caller.nodes) != effectsBefore {
		t.Fatal("failed effect rebase mutated destination arenas")
	}
}
