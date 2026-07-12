package transformer

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
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
			Invalidation: InvalidatePathConfig{Target: PathEffectTarget(path), Scope: InvalidationScopeDescendants},
			Table:        PathEffectTarget(path), Key: key, Value: value,
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
	_, ok := forward.SpecializeWithEffects(cursor, nil, SpecializationContext{}, func(effects []ResolvedEffect) (EffectResolution, bool) {
		for _, effect := range effects {
			gotOrder = append(gotOrder, effect.Mutation.Site.Ordinal)
		}
		return EffectResolution{Contributions: []EffectContribution{
			{Kind: EffectIndexMutation, BoundaryKinds: []callboundary.BoundaryFactKind{callboundary.BoundaryFactKind(callboundary.LanePathInvalidations)}},
			{Kind: EffectIndexMutation, BoundaryKinds: []callboundary.BoundaryFactKind{callboundary.BoundaryFactKind(callboundary.LanePathInvalidations)}},
		}}, true
	})
	if ok || !reflect.DeepEqual(gotOrder, []uint32{1, 2}) {
		t.Fatalf("empty resolver fragment/order = %v/%v, want [1 2]/false", gotOrder, ok)
	}
	builder.effectCatalog = nil
	if _, err := builder.Build(certificate, []Row{{Guard: builder.Arena().True(), Effects: []EffectTerm{first}}}); err == nil {
		t.Fatal("effectful row admitted without an effect catalog verdict")
	}
}

func TestEffectResolverFragmentCannotExceedOriginatingDescriptor(t *testing.T) {
	reg := standard.Registry()
	shape := Shape{Params: 1}
	builder, certificate := emptyBuilder(t, reg, shape, nil)
	table := builder.Arena().Path(Root{Kind: RootParam, Index: 0})
	scalar := builder.Arena().Constant(typevalue.LiteralString(reg, "value"))
	effect, err := builder.EffectArena().IndexMutation(IndexMutationConfig{
		Invalidation: InvalidatePathConfig{Target: PathEffectTarget(table), Scope: InvalidationScopeDescendants},
		Table:        PathEffectTarget(table), Key: scalar, Value: scalar,
		Admission: dynamicindex.AdmissionAdmitted, Readback: factflow.DynamicIndexReadbackKeyAndValue,
		Site: EffectSite{Owner: 17, Ordinal: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	relation, err := builder.Build(certificate, []Row{{Guard: builder.Arena().True(), Effects: []EffectTerm{effect}}})
	if err != nil {
		t.Fatal(err)
	}
	cursor, err := NewBindingCursor(shape, []product.Value{product.Top()}, []pathdom.Path{pathdom.NewPlaceholder(0)})
	if err != nil {
		t.Fatal(err)
	}

	adversarial := []summary.Summary{
		{MaySuspend: true},
		{Returns: []product.Value{product.Top()}},
		{ProtectedCallTypestate: callboundary.ProtectedCallTypestate{Normal: typestate.Empty(), HasNormal: true}},
	}
	for i, injected := range adversarial {
		got, ok := relation.SpecializeWithEffects(cursor, nil, SpecializationContext{}, func([]ResolvedEffect) (EffectResolution, bool) {
			return EffectResolution{Summary: injected, Contributions: []EffectContribution{{
				Kind: EffectIndexMutation, BoundaryKinds: summary.PresentFactKinds(injected),
			}}}, true
		})
		if ok || !reflect.DeepEqual(got, summary.Summary{}) {
			t.Fatalf("adversarial resolver fragment %d escaped descriptor: ok=%v got=%#v", i, ok, got)
		}
	}

	allowed := summary.Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		PathInvalidations: []callboundary.PathInvalidationFact{{Path: pathdom.NewPlaceholder(0)}},
	}}
	got, ok := relation.SpecializeWithEffects(cursor, nil, SpecializationContext{}, func([]ResolvedEffect) (EffectResolution, bool) {
		return EffectResolution{Summary: allowed, Contributions: []EffectContribution{{
			Kind:          EffectIndexMutation,
			BoundaryKinds: []callboundary.BoundaryFactKind{callboundary.BoundaryFactKind(callboundary.LanePathInvalidations)},
		}}}, true
	})
	if !ok || !summary.Equal(reg, got, summary.Normalize(reg, allowed)) {
		t.Fatalf("canonical descriptor fragment rejected: ok=%v got=%#v", ok, got)
	}
}

func TestEffectResolutionRequiresOrderedPerEffectAuthorityAndExactUnion(t *testing.T) {
	reg := standard.Registry()
	shape := Shape{Params: 2}
	builder, certificate := emptyBuilder(t, reg, shape, nil)
	firstPathTerm := builder.Arena().Path(Root{Kind: RootParam, Index: 0})
	secondPathTerm := builder.Arena().Path(Root{Kind: RootParam, Index: 1})
	standalone, err := builder.EffectArena().InvalidatePath(InvalidatePathConfig{
		Target: PathEffectTarget(firstPathTerm), Scope: InvalidationScopeDescendants,
	})
	if err != nil {
		t.Fatal(err)
	}
	scalar := builder.Arena().Constant(typevalue.LiteralString(reg, "value"))
	mutation, err := builder.EffectArena().IndexMutation(IndexMutationConfig{
		Invalidation: InvalidatePathConfig{Target: PathEffectTarget(secondPathTerm), Scope: InvalidationScopeDescendants},
		Table:        PathEffectTarget(secondPathTerm), Key: scalar, Value: scalar,
		Admission: dynamicindex.AdmissionAdmitted, Readback: factflow.DynamicIndexReadbackKeyAndValue,
		Site: EffectSite{Owner: 31, Ordinal: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	relation, err := builder.Build(certificate, []Row{{
		Guard: builder.Arena().True(), Effects: []EffectTerm{standalone, mutation},
	}})
	if err != nil {
		t.Fatal(err)
	}
	firstPath, secondPath := pathdom.NewPlaceholder(0), pathdom.NewPlaceholder(1)
	cursor, err := NewBindingCursor(shape, []product.Value{product.Top(), product.Top()}, []pathdom.Path{firstPath, secondPath})
	if err != nil {
		t.Fatal(err)
	}
	fragment := summary.Summary{NormalReturnFacts: callboundary.NormalReturnFacts{
		PathInvalidations: []callboundary.PathInvalidationFact{{Path: firstPath}},
		DynamicIndexFacts: []callboundary.DynamicIndexFact{{Table: secondPath}},
	}}
	pathKind := callboundary.BoundaryFactKind(callboundary.LanePathInvalidations)
	dynamicKind := callboundary.BoundaryFactKind(callboundary.LaneDynamicIndexFacts)
	valid := EffectResolution{Summary: fragment, Contributions: []EffectContribution{
		{Kind: EffectInvalidatePath, BoundaryKinds: []callboundary.BoundaryFactKind{pathKind}},
		{Kind: EffectIndexMutation, BoundaryKinds: []callboundary.BoundaryFactKind{dynamicKind}},
	}}
	got, ok := relation.SpecializeWithEffects(cursor, nil, SpecializationContext{}, func([]ResolvedEffect) (EffectResolution, bool) {
		return valid, true
	})
	if !ok || !summary.Equal(reg, got, summary.Normalize(reg, fragment)) {
		t.Fatalf("valid mixed effect authority rejected: ok=%v got=%#v", ok, got)
	}

	tests := []struct {
		name       string
		resolution EffectResolution
	}{
		{name: "missing contribution", resolution: EffectResolution{Summary: fragment, Contributions: valid.Contributions[:1]}},
		{name: "extra contribution", resolution: EffectResolution{Summary: fragment, Contributions: append(append([]EffectContribution(nil), valid.Contributions...), valid.Contributions[1])}},
		{name: "wrong ordered kind", resolution: EffectResolution{Summary: fragment, Contributions: []EffectContribution{valid.Contributions[1], valid.Contributions[0]}}},
		{name: "cross-effect smuggling", resolution: EffectResolution{Summary: fragment, Contributions: []EffectContribution{
			{Kind: EffectInvalidatePath, BoundaryKinds: []callboundary.BoundaryFactKind{dynamicKind}},
			{Kind: EffectIndexMutation, BoundaryKinds: []callboundary.BoundaryFactKind{pathKind}},
		}}},
		{name: "empty semantic effect", resolution: EffectResolution{Summary: fragment, Contributions: []EffectContribution{
			{Kind: EffectInvalidatePath}, valid.Contributions[1],
		}}},
		{name: "missing populated lane", resolution: EffectResolution{Summary: fragment, Contributions: []EffectContribution{
			valid.Contributions[0],
			{Kind: EffectIndexMutation, BoundaryKinds: []callboundary.BoundaryFactKind{pathKind}},
		}}},
		{name: "unpopulated claimed lane", resolution: EffectResolution{Summary: fragment, Contributions: []EffectContribution{
			valid.Contributions[0],
			{Kind: EffectIndexMutation, BoundaryKinds: []callboundary.BoundaryFactKind{dynamicKind, callboundary.BoundaryFactKind(callboundary.LaneKeyMemberships)}},
		}}},
		{name: "duplicate contribution lane", resolution: EffectResolution{Summary: fragment, Contributions: []EffectContribution{
			valid.Contributions[0],
			{Kind: EffectIndexMutation, BoundaryKinds: []callboundary.BoundaryFactKind{dynamicKind, dynamicKind}},
		}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := relation.SpecializeWithEffects(cursor, nil, SpecializationContext{}, func([]ResolvedEffect) (EffectResolution, bool) {
				return test.resolution, true
			})
			if ok || !reflect.DeepEqual(got, summary.Summary{}) {
				t.Fatalf("invalid authority partially published: ok=%v got=%#v", ok, got)
			}
		})
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
			Invalidation: InvalidatePathConfig{Target: PathEffectTarget(calleePath), Scope: InvalidationScopeDescendants},
			Table:        PathEffectTarget(calleePath), Key: key, Value: value,
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
