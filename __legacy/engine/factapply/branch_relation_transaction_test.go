package factapply

import (
	"context"
	"errors"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestPlanBranchRelationTransactionOwnsCanonicalOrderAndEdgeOccurrence(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(19)
	trigger := pathdom.NewPath(symbol.ID(1), "trigger")
	target := pathdom.NewPath(symbol.ID(2), "target")
	other := pathdom.NewPath(symbol.ID(3), "other")
	facts := factflow.NewFacts(factflow.FactsInput{
		BranchPresenceRelations: map[cfg.Point]factflow.BranchPresenceRelationSet{
			point: factflow.NewBranchPresenceRelationSet(
				factflow.NewBranchPresenceRelation(trigger, presence.Present(), target, presence.Absent()),
			),
		},
		BranchPathRelations: map[cfg.Point]factflow.BranchPathRelationSet{
			point: factflow.NewBranchPathRelationSet(
				factflow.NewBranchPathEquality(target, other, true, false),
				factflow.NewBranchPathInequality(target, other, false, true),
			),
		},
		BranchSufficientLiteralCases: map[cfg.Point]factflow.BranchSufficientLiteralCaseSet{
			point: factflow.NewBranchSufficientLiteralCaseSet(
				factflow.NewBranchSufficientLiteralCase(trigger, typevalue.LiteralString(reg, "yes"), true),
				factflow.NewBranchSufficientLiteralCase(trigger, typevalue.LiteralString(reg, "no"), false),
			),
		},
	})

	transaction := PlanBranchRelationTransaction(facts, point, true)
	if transaction.Point() != point || !transaction.Cond() || transaction.Len() != 4 ||
		!transaction.HasStateSteps() || !transaction.HasSufficientLiteralCases() || !transaction.ValidForRegistry(reg) {
		t.Fatalf("transaction = point %d cond %t len %d", transaction.Point(), transaction.Cond(), transaction.Len())
	}
	want := []BranchRelationStepKind{
		BranchRelationStepPresence,
		BranchRelationStepPath,
		BranchRelationStepSufficientLiteralCase,
		BranchRelationStepSufficientLiteralCase,
	}
	for index, kind := range want {
		step, ok := transaction.Step(index)
		if !ok || step.Kind() != kind {
			t.Fatalf("step %d = %v/%t, want %v", index, step.Kind(), ok, kind)
		}
	}
	pathStep, _ := transaction.Step(1)
	relation, ok := pathStep.PathRelation()
	if !ok || relation.Kind() != factflow.BranchPathRelationEqual || !relation.ActiveOnEdge(true) || relation.ActiveOnEdge(false) {
		t.Fatalf("selected path relation = %#v/%t", relation, ok)
	}
	caseStep, _ := transaction.Step(2)
	literalCase, ok := caseStep.SufficientLiteralCase()
	if !ok || !literalCase.Edge() || !literalCase.TargetPath().Equal(trigger) {
		t.Fatalf("selected sufficient case = %#v/%t", literalCase, ok)
	}
	oppositeStep, _ := transaction.Step(3)
	opposite, ok := oppositeStep.SufficientLiteralCase()
	if !ok || opposite.Edge() {
		t.Fatalf("opposite sufficient case = %#v/%t", opposite, ok)
	}
	if _, ok := transaction.Step(4); ok {
		t.Fatal("transaction exposed an out-of-range step")
	}
}

func applyPreparedBranchFactorsForTest(ctx context.Context, authority *PathSemanticAuthority, domain state.ProductDomain, transaction BranchRelationTransaction, input state.State) (state.State, error) {
	inventory, err := authority.SealCoordinateFactorInventory(domain, nil)
	if err != nil {
		return input, err
	}
	factors, err := authority.PrepareBranchRelationFactors(domain, transaction, inventory)
	if err != nil {
		return input, err
	}
	edge := transfer.EdgeContext{
		Context: ctx, Session: cancellation.FromContext(ctx), Registry: domain.Registry(),
		Edge: cfg.Edge{From: transaction.Point(), Cond: transaction.Cond()}, HasCond: true,
	}
	original, out := input, input
	for _, stage := range factors.Stages() {
		for _, factor := range stage.Factors() {
			if _, present := factors.PresenceImplicationDependencyPlan(factor); present {
				return original, errors.New("test transaction requires a separately bound coordinate consequence program")
			}
			result := factors.ApplyFactor(factor, edge, original, out)
			if result.Err != nil {
				return original, result.Err
			}
			if result.Canceled {
				if err := ctx.Err(); err != nil {
					return original, err
				}
				return original, context.Canceled
			}
			out = result.Output
		}
	}
	return out, nil
}

func TestBranchRelationTransactionAccessorsDoNotExposePathStorage(t *testing.T) {
	point := cfg.Point(7)
	left := pathdom.NewPath(symbol.ID(8), "left").Field("member")
	right := pathdom.NewPath(symbol.ID(9), "right").Field("member")
	facts := factflow.NewFacts(factflow.FactsInput{BranchPathRelations: map[cfg.Point]factflow.BranchPathRelationSet{
		point: factflow.NewBranchPathRelationSet(factflow.NewBranchPathEquality(left, right, true, false)),
	}})
	transaction := PlanBranchRelationTransaction(facts, point, true)
	step, _ := transaction.Step(0)
	first, _ := step.PathRelation()
	mutated := first.LeftPath()
	mutated.Segments[0].Name = "mutated"
	second, _ := step.PathRelation()
	if !second.LeftPath().Equal(left) {
		t.Fatal("branch transaction exposed mutable path storage")
	}
}

func TestBranchRelationDynamicPresenceUsesExactKeyAndFailsClosedForBroadKey(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(23)
	tableSymbol := symbol.ID(41)
	table := pathdom.NewPath(tableSymbol, "table").Field("references")
	builder := visibility.NewBuilder()
	builder.Define(point, tableSymbol, "table")
	resolver := visibility.NewResolver(builder.Build())
	authority := NewPathSemanticAuthority(resolver, nil, typevalue.NewCache())

	base, ok := PlanBranchRelationTransaction(factflow.NewFacts(factflow.FactsInput{}), point, true).WithDynamicPresenceProof(table)
	if !ok || !base.RequiresDynamicPresenceKey() {
		t.Fatal("dynamic presence step was not frozen as an unresolved E3 input")
	}
	if _, duplicate := base.WithDynamicPresenceProof(table.IndexStr("other")); duplicate {
		t.Fatal("one branch transaction admitted two dynamic keys into one binding slot")
	}
	exact, ok := base.BindDynamicPresenceKey(reg, typevalue.LiteralString(reg, "present"))
	if !ok || exact.RequiresDynamicPresenceKey() {
		t.Fatal("exact dynamic key was not bound transactionally")
	}
	got, err := applyPreparedBranchFactorsForTest(context.Background(), authority, state.RegisteredProductDomain(reg), exact, state.State{})
	if err != nil {
		t.Fatal(err)
	}
	wantPath, pathOK := resolver.VisibleLocalKeyspaceKeyAt(point, table.IndexStr("present"))
	want := pathevidence.BranchProof{
		Kind: pathevidence.BranchProofPathPresence, Path: wantPath, Presence: presence.Present(),
	}
	if !pathOK || !got.HasBranchProof(want) {
		t.Fatalf("dynamic presence proof missing: got %#v, want %#v", got.BranchProofsSnapshot(resolver.KeySpace()), want)
	}

	broadValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	broad, ok := base.BindDynamicPresenceKey(reg, broadValue)
	if !ok {
		t.Fatal("broad dynamic key is a valid bound abstract value")
	}
	broadState, err := applyPreparedBranchFactorsForTest(context.Background(), authority, state.RegisteredProductDomain(reg), broad, state.State{})
	if err != nil {
		t.Fatal(err)
	}
	if broadState.HasBranchProofKind(pathevidence.BranchProofPathPresence) || product.Equal(reg, broadValue, product.Bottom(reg)) {
		t.Fatal("broad dynamic key published an invented concrete presence proof")
	}
}

func TestBranchRelationTransactionCancellationRollsBackSelectedEdgeInput(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(29)
	leftID, rightID := symbol.ID(51), symbol.ID(52)
	left := pathdom.NewPath(leftID, "left")
	right := pathdom.NewPath(rightID, "right")
	transaction := PlanBranchRelationTransaction(factflow.NewFacts(factflow.FactsInput{
		BranchPathRelations: map[cfg.Point]factflow.BranchPathRelationSet{
			point: factflow.NewBranchPathRelationSet(factflow.NewBranchPathEquality(left, right, true, false)),
		},
	}), point, true)
	if !transaction.Clone().ValidForRegistry(reg) {
		t.Fatal("deeply frozen branch transaction lost registry ownership")
	}
	builder := visibility.NewBuilder()
	builder.Define(point, leftID, "left")
	builder.Define(point, rightID, "right")
	authority := NewPathSemanticAuthority(visibility.NewResolver(builder.Build()), nil, nil)
	input := state.Reachable(state.State{})
	ctx, session := cancellation.Attach(context.Background())
	session.Token().Cancel(context.Canceled)
	rolledBack, err := applyPreparedBranchFactorsForTest(ctx, authority, state.RegisteredProductDomain(reg), transaction, input)
	if err == nil {
		t.Fatal("pre-canceled branch authority did not report cancellation")
	}
	if !state.Domain(reg).Equal(rolledBack, input) {
		t.Fatal("canceled branch transaction published an E1/E3 prefix")
	}
}
