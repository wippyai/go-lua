package program

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/evaluated"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/observation"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestObserverProgramKeepsTwoCallContextsCorrelated(t *testing.T) {
	stmts := parseChunk(t, `
local function leaf(value: string) return value end
local first = leaf("first")
local second = leaf(false)
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	reg := standard.Registry()
	stats := &Stats{}
	artifact, err := runEvaluatedBoundChunk(context.Background(), stmts, bindings, Config{
		Check: body.Config{
			Registry: reg, Signatures: signaturelookup.Source{IncludeStdlib: true},
			UnitNamespace: lexicalidentity.UnitNamespaceFromContent([]byte("observer-two-context")),
		},
		Stats: stats,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.program.instances) != 3 || stats.EvaluatedRelationEquationApplications != 2 ||
		stats.EvaluatedRootProjections != 3 || stats.EvaluatedObserverInstanceProjections != 3 ||
		stats.EvaluatedObserverEntryProjections != 1 || stats.EvaluatedObserverTermApplications == 0 {
		t.Fatalf("instances/equations/projections/instance/entry/terms = %d/%d/%d/%d/%d/%d, want 3/2/3/3/1/>0",
			len(artifact.program.instances), stats.EvaluatedRelationEquationApplications, stats.EvaluatedRootProjections,
			stats.EvaluatedObserverInstanceProjections, stats.EvaluatedObserverEntryProjections,
			stats.EvaluatedObserverTermApplications)
	}
	entry, ok, err := artifact.Entry(context.Background(), reg)
	if err != nil || !ok {
		t.Fatalf("entry materialization = ok:%v err:%v", ok, err)
	}
	var first, invalid bool
	for _, slot := range entry.Observations() {
		for _, item := range slot.Observed {
			if item.Kind != observation.CallArgument || !item.HasExpected || !product.Equal(reg, item.Expected, typevalue.String(reg)) {
				continue
			}
			first = first || product.Equal(reg, item.Actual, typevalue.LiteralString(reg, "first"))
			invalid = invalid || product.Equal(reg, item.Actual, typevalue.LiteralBool(reg, false))
		}
	}
	if !first || !invalid {
		t.Fatalf("entry lost correlated actual/expected call evidence: first=%v invalid=%v", first, invalid)
	}
	// Both contexts share the lexical template but retain distinct canonical
	// boundaries and independently sealed local evidence.
	left := artifact.program.instances[1]
	right := artifact.program.instances[2]
	if left.template != right.template || string(left.boundary.values[0].Bytes()) == string(right.boundary.values[0].Bytes()) {
		t.Fatal("two call contexts were merged or assigned different lexical templates")
	}
	for _, instance := range artifact.program.instances {
		if _, err := instance.local.Materialize(context.Background(), reg); err != nil {
			t.Fatalf("instance %d local artifact: %v", instance.id, err)
		}
	}
}

func TestObserverProgramRecursiveWorldsAreEdgeLocalConjuncts(t *testing.T) {
	// Deliberately use the same DecisionID in two different proofs. Treating the
	// integers as one global ROBDD would merge the guards. The observer program
	// instead interprets the path as ingress.guard AND recursive.guard.
	artifact := observerProgramArtifact{proofs: []observerProgramProofArtifact{
		{decisions: []evaluated.Decision{{ID: 2, Low: evaluated.DecisionFalse, High: evaluated.DecisionTrue}}},
		{decisions: []evaluated.Decision{{ID: 2, Low: evaluated.DecisionFalse, High: evaluated.DecisionTrue}}},
	}}
	ingress := observerProgramParentArtifact{proof: 1, worlds: evaluated.WorldSet{Root: 2}}
	self := observerProgramParentArtifact{proof: 2, worlds: evaluated.WorldSet{Root: 2}, backedge: true}
	conjunction, err := artifact.observerWorldConjunction(ingress, self)
	if err != nil {
		t.Fatal(err)
	}
	if len(conjunction) != 2 || conjunction[0].proof != 1 || conjunction[1].proof != 2 ||
		conjunction[0].worlds.Root != 2 || conjunction[1].worlds.Root != 2 {
		t.Fatalf("self-recursive conjunction collapsed proof namespaces: %#v", conjunction)
	}

	// Mutual recursion adds another owner-local conjunct but still reuses the
	// structural graph nodes. No DecisionID comparison crosses proof ownership.
	artifact.proofs = append(artifact.proofs,
		observerProgramProofArtifact{decisions: []evaluated.Decision{{ID: 2, Low: evaluated.DecisionFalse, High: evaluated.DecisionTrue}}})
	mutual := observerProgramParentArtifact{proof: 3, worlds: evaluated.WorldSet{Root: 2}, backedge: true}
	conjunction, err = artifact.observerWorldConjunction(ingress, self, mutual)
	if err != nil {
		t.Fatal(err)
	}
	if len(conjunction) != 3 || conjunction[2].proof != 3 {
		t.Fatalf("mutual-recursive conjunction lost local guard: %#v", conjunction)
	}
}

func TestObserverProgramRecursiveTupleWidensAtomicallyAndSealsMuPath(t *testing.T) {
	reg := standard.Registry()
	previous, err := concreteObserverBoundaryTuple(
		[]product.Value{typevalue.LiteralString(reg, "left"), typevalue.LiteralBool(reg, false)},
		[]pathdom.Path{pathdom.NewPlaceholder(0), pathdom.NewPlaceholder(1)},
	)
	if err != nil {
		t.Fatal(err)
	}
	next, err := concreteObserverBoundaryTuple(
		[]product.Value{typevalue.LiteralString(reg, "right"), typevalue.LiteralBool(reg, true)},
		[]pathdom.Path{pathdom.NewPlaceholder(0).Field("next"), pathdom.NewPlaceholder(1)},
	)
	if err != nil {
		t.Fatal(err)
	}
	var anchor, target lexicalObserverTemplateRef
	anchor.Cell.Function, target.Cell.Function = 1, 2
	anchor.Body[len(anchor.Body)-1], target.Body[len(target.Body)-1] = 1, 2
	mu := lexicalObserverMuRef{Anchor: anchor, Target: target}
	widened, changed, err := widenObserverBoundaryTuple(reg, previous, next, mu)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || !product.LessOrEq(reg, previous.values[0], widened.values[0]) ||
		!product.LessOrEq(reg, next.values[0], widened.values[0]) ||
		!product.LessOrEq(reg, previous.values[1], widened.values[1]) ||
		!product.LessOrEq(reg, next.values[1], widened.values[1]) {
		t.Fatal("recursive tuple widening is not one product-wise upper bound")
	}
	if widened.paths[0].mu == nil || widened.paths[0].mu.mu != mu || widened.paths[1].mu != nil {
		t.Fatalf("recursive paths were unfolded or unrelated paths collapsed: %#v", widened.paths)
	}
	again, changed, err := widenObserverBoundaryTuple(reg, widened, next, mu)
	if err != nil || changed || !observerBoundaryTupleLessOrEq(reg, again, widened) || !observerBoundaryTupleLessOrEq(reg, widened, again) {
		t.Fatalf("recursive tuple did not reach structural equality: changed=%v err=%v", changed, err)
	}

	// A self recurrence updates its one ingress alternative. Figure-eight and
	// mutual edges with incomparable edge-local proof namespaces remain distinct.
	environment := observerBoundaryEnvironment{}
	self := observerMuIngressKey{anchor: anchor, target: target, parent: 1, proof: 1, worlds: evaluated.WorldSet{Root: 1}}
	if changed, err := environment.merge(reg, self, previous); err != nil || !changed {
		t.Fatalf("seed self ingress: changed=%v err=%v", changed, err)
	}
	if changed, err := environment.merge(reg, self, next); err != nil || !changed {
		t.Fatalf("widen self ingress: changed=%v err=%v", changed, err)
	}
	left := self
	left.proof = 2
	right := self
	right.proof = 3
	right.target = anchor
	for _, ingress := range []observerMuIngressKey{left, right} {
		if changed, err := environment.merge(reg, ingress, previous); err != nil || !changed {
			t.Fatalf("seed incomparable ingress %#v: changed=%v err=%v", ingress, changed, err)
		}
	}
	if len(environment.alternatives) != 3 {
		t.Fatalf("figure-eight/mutual alternatives = %d, want three distinct guarded tuples", len(environment.alternatives))
	}
}
