package factapply

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestResolvedRootAssignmentPlanFactorCompletionFreshEmptyPathsAreExactDetachedAndAuthorityOwned(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(8801)
	target, sourceContainer, unrelatedTable := symbol.ID(8801), symbol.ID(8802), symbol.ID(8803)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 8804, HasExpr: true}
	facts := factflow.NewFacts(factflow.FactsInput{RootAssignments: map[cfg.Point]factflow.RootAssignment{
		point: factflow.NewRootAssignment(
			factflow.RootAssignmentOrdinaryRootWrite,
			target,
			pathdom.NewPath(target, "target"),
			source,
		),
	}})
	builder := visibility.NewBuilder()
	builder.Define(point, target, "target")
	resolver := visibility.NewResolver(builder.Build())
	targetRoot := pathdom.Path{Symbol: target}
	sourceRoot := pathdom.Path{Symbol: sourceContainer}
	closed := []ClosedDynamicAllValueInvariant{
		{Container: targetRoot, Table: pathdom.Path{Symbol: unrelatedTable}},
		{Container: sourceRoot, Table: targetRoot},
		{Container: sourceRoot, Table: targetRoot},
		{Container: sourceRoot.Field("nested"), Table: targetRoot},
		{Container: pathdom.Path{Symbol: 8890}, Table: pathdom.Path{Symbol: 8891}},
	}
	authority := NewRootAssignmentAuthority(
		NewPathSemanticAuthority(resolver, nil, typevalue.NewCache()), facts, closed, state.RegisteredProductDomain(reg),
	)
	transaction, ok := PlanRootAssignmentTransaction(facts, point)
	if !ok {
		t.Fatal("root-assignment transaction")
	}
	resolved, ok := transaction.Bind(reg, []product.Value{typevalue.LiteralString(reg, "value")})
	if !ok {
		t.Fatal("resolved root-assignment transaction")
	}
	plan, err := authority.PrepareResolvedRootAssignmentPlan(transaction)
	if err != nil {
		t.Fatal(err)
	}
	queries, err := plan.FactorCompletionFreshEmptyPaths()
	if err != nil {
		t.Fatal(err)
	}
	want := []pathdom.Path{targetRoot, sourceRoot}
	if len(queries) != len(want) {
		t.Fatalf("fresh-empty queries = %v, want %v", queries, want)
	}
	for index := range want {
		if !queries[index].Equal(want[index]) {
			t.Fatalf("fresh-empty query %d = %v, want %v", index, queries[index], want[index])
		}
	}
	callQueries, err := authority.CallReceiverCompletionFreshEmptyPaths(resolved)
	if err != nil {
		t.Fatal(err)
	}
	for index := range want {
		if !callQueries[index].Equal(queries[index]) {
			t.Fatalf("call-receiver query %d differs from plan query", index)
		}
	}

	queries[0].Symbol = 0
	queries[1].Segments = append(queries[1].Segments, sourceRoot.Field("mutated").Segments...)
	again, err := plan.FactorCompletionFreshEmptyPaths()
	if err != nil {
		t.Fatal(err)
	}
	for index := range want {
		if !again[index].Equal(want[index]) {
			t.Fatalf("caller mutation changed query %d: %v", index, again[index])
		}
	}
}

func TestResolvedRootAssignmentPlanFactorCompletionFreshEmptyPathsRejectsInvalidPlan(t *testing.T) {
	if _, err := (ResolvedRootAssignmentPlan{}).FactorCompletionFreshEmptyPaths(); err == nil {
		t.Fatal("invalid completion plan was accepted")
	}
}
