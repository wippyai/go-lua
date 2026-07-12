package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	enginesourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestStaticIndexTermIsCanonicalRebasesAndMatchesSharedKernel(t *testing.T) {
	reg := standard.Registry()
	callee, caller := NewArena(reg), NewArena(reg)
	shape := Shape{Params: 1}
	member := segment.Segment{Kind: segment.SegmentField, Name: "target_name"}
	term := callee.StaticIndexValue(callee.Root(Root{Kind: RootParam}), member)
	if term == 0 || term != callee.StaticIndexValue(callee.Root(Root{Kind: RootParam}), member) {
		t.Fatal("static index term is not hash-consed")
	}
	record := typetable.NewRecord().Field("target_name", typ.String).Build()
	owner := typevalue.WithWitness(reg, typevalue.FromType(reg, record), record)
	owner = product.WithPresence(reg, owner, presence.Present())
	bound := caller.Constant(owner)
	bindings, _ := NewTermRootBindings(shape, Shape{}, []ValueTerm{bound}, nil)
	got, err := RebaseTermDAGs(caller, callee, bindings, TermRebaseInput{Values: []ValueTerm{term}})
	if err != nil || len(got.Values) != 1 || got.Values[0] != caller.StaticIndexValue(bound, member) {
		t.Fatalf("static index rebase = %#v/%v", got, err)
	}
	cursor, _ := NewBindingCursor(Shape{}, nil, nil)
	projected, ok := caller.evalValue(got.Values[0], cursor, SpecializationContext{})
	key := typevalue.LiteralString(reg, "target_name")
	want, wantOK := enginesourcevalue.StaticIndexValue(reg, nil, owner, key)
	if !ok || !wantOK || !product.Equal(reg, projected, want) {
		t.Fatalf("term projection = %#v/%v, shared kernel %#v/%v", projected, ok, want, wantOK)
	}
}

func TestAdjustedDirectBindingAdmitsOnlyIteratorDerivedMemberProjection(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	ref := factflow.ExprRef(71)
	alias := symbol.ID(22)
	plan := operationplan.New(graph, factflow.FactsInput{
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{ref: pathdom.NewPath(alias, "route_entry").Field("target_name")},
	})
	builder := NewBuilder(reg, Shape{}, DefaultOutputCapabilityRegistry(), plan)
	container := builder.Arena().Constant(typevalue.FromType(reg, typ.NewArray(typetable.NewRecord().Field("target_name", typ.String).Build())))
	iterator := iteration.Iterator{Kind: iteration.IterateIndexed, Source: effect.ParamRef{Index: 0}}
	projection := builder.Arena().IteratorProjectionValue(iterator, 1, container)
	ctx := planCompileContext{registry: reg, graph: graph, plan: plan, facts: plan.Facts(), builder: builder, locals: map[symbol.ID]ValueTerm{alias: projection}}
	adjustedShape, _ := factflow.NewValueSourceShape(true, false, true, false)
	source, _ := factflow.NewExpressionValueSource(ref, 0, 0, 0, adjustedShape)
	value, path, err := exactDirectCallSourceBinding(ctx, source)
	if err != nil || value == 0 || path != 0 || builder.Arena().values[value].op != valueStaticIndex {
		t.Fatalf("adjusted member binding = %d/%d/%v", value, path, err)
	}

	malformed := source
	malformed.ResultIndex = factflow.NoValueSourceIndex
	callSource, _ := factflow.NewCallValueSource(ref, 0, 0, 0, 1, adjustedShape)
	varargSource, _ := factflow.NewVarargValueSource(ref, 0, 0, 0, adjustedShape)
	rootPathSource := source
	rootPathSource.ExprRef = ref + 1
	expandedSource := source
	expandedSource.Adjusted = false
	expandedSource.Expanded = true
	openSource := expandedSource
	openSource.OpenTail = true
	malformedExpression := source
	malformedExpression.HasExpr = false
	tests := []struct {
		name   string
		source factflow.ValueSource
	}{
		{name: "malformed-result-slot", source: malformed},
		{name: "call", source: callSource},
		{name: "vararg", source: varargSource},
		{name: "missing-expression-path", source: rootPathSource},
		{name: "expanded", source: expandedSource},
		{name: "open", source: openSource},
		{name: "malformed-expression", source: malformedExpression},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := exactDirectCallSourceBinding(ctx, test.source); err == nil {
				t.Fatal("adjusted non-member source accepted")
			}
		})
	}
}

func TestStaticIndexMalformedKeyFailsBeforeRebasePublication(t *testing.T) {
	reg := standard.Registry()
	callee, caller := NewArena(reg), NewArena(reg)
	shape := Shape{Params: 1}
	root := callee.Root(Root{Kind: RootParam})
	term := callee.StaticIndexValue(root, segment.Segment{Kind: segment.SegmentField, Name: "field"})
	callee.values[term].args[1] = root // adversarial private-DAG corruption
	bound := caller.Root(Root{Kind: RootParam})
	bindings, _ := NewTermRootBindings(shape, shape, []ValueTerm{bound}, nil)
	before := len(caller.values)
	if got, err := RebaseTermDAGs(caller, callee, bindings, TermRebaseInput{Values: []ValueTerm{term}}); err == nil || len(got.Values) != 0 {
		t.Fatalf("malformed static key accepted: %#v/%v", got, err)
	}
	if len(caller.values) != before {
		t.Fatal("malformed static key partially published caller terms")
	}
}
