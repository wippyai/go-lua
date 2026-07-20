package factapply

import (
	"context"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestPlanCallResultTransactionOwnsN0N3N5Order(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(17)
	left := pathdom.NewPath(symbol.ID(10), "left")
	right := pathdom.NewPath(symbol.ID(11), "right")
	target := pathdom.NewPath(symbol.ID(12), "target")
	facts := factflow.NewFacts(factflow.FactsInput{
		CallResultValues: map[cfg.Point]factflow.CallResultValueSet{
			point: factflow.NewCallResultValueSet(factflow.NewCallResultValue(0, typevalue.LiteralString(reg, "ready"))),
		},
		PostconditionRefinements: map[cfg.Point]factflow.PostconditionRefinementSet{
			point: factflow.NewPostconditionRefinementSet(factflow.NewPostconditionRefinement(
				target, factflow.NewValueConstraint(product.NewWithPresence(reg, product.ShapeTop, presence.Present())),
			)),
		},
		PostconditionPathRelations: map[cfg.Point][]factflow.PostconditionPathRelation{
			point: {factflow.NewPostconditionPathEquality(left, right)},
		},
		ReturnPresenceRelations: map[cfg.Point]factflow.ReturnPresenceRelationSet{
			point: factflow.NewReturnPresenceRelationSet(
				factflow.NewReturnPresenceRelation(1, presence.Present(), 0, presence.Absent()),
			),
		},
	})
	transaction := PlanCallResultTransaction(facts, point)
	want := []CallResultStepKind{
		CallResultStepValue,
		CallResultStepPostconditionRefinement,
		CallResultStepPostconditionPathRelation,
		CallResultStepReturnPresenceRelation,
	}
	if !transaction.Valid(reg) || transaction.Len() != len(want) || !transaction.HasStateSteps() || !transaction.HasPublicationSteps() {
		t.Fatalf("transaction valid/len/state/publication = %t/%d/%t/%t", transaction.Valid(reg), transaction.Len(), transaction.HasStateSteps(), transaction.HasPublicationSteps())
	}
	for index, kind := range want {
		step, ok := transaction.Step(index)
		if !ok || step.Kind() != kind {
			t.Fatalf("step %d = %v/%t, want %v", index, step.Kind(), ok, kind)
		}
	}
	publicationStep, _ := transaction.Step(3)
	publication, ok := publicationStep.ReturnPresenceRelation()
	if !ok || publication.TriggerIndex() != 1 || !presence.Equal(publication.TriggerPresence(), presence.Present()) ||
		publication.TargetIndex() != 0 || !presence.Equal(publication.TargetPresence(), presence.Absent()) {
		t.Fatal("N5 return-presence publication changed while freezing")
	}
	frozen := transaction.Clone()
	refinementStep, _ := frozen.Step(1)
	refinement, _ := refinementStep.PostconditionRefinement()
	mutated := refinement.TargetPath()
	mutated.Segments = append(mutated.Segments, target.Field("mutated").Segments...)
	again, _ := frozen.Step(1)
	againRefinement, _ := again.PostconditionRefinement()
	if !againRefinement.TargetPath().Equal(target) {
		t.Fatal("sealed call-result transaction exposed mutable postcondition path storage")
	}
}

func TestConcreteCallResultTransactionKeepsMaterializeAndPostconditionBarriers(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(23)
	leftID, rightID, targetID := symbol.ID(20), symbol.ID(21), symbol.ID(22)
	left := pathdom.NewPath(leftID, "left")
	right := pathdom.NewPath(rightID, "right")
	target := pathdom.NewPath(targetID, "target")
	ready := typevalue.LiteralString(reg, "ready")
	facts := factflow.NewFacts(factflow.FactsInput{
		CallResultValues: map[cfg.Point]factflow.CallResultValueSet{
			point: factflow.NewCallResultValueSet(factflow.NewCallResultValue(0, ready)),
		},
		PostconditionRefinements: map[cfg.Point]factflow.PostconditionRefinementSet{
			point: factflow.NewPostconditionRefinementSet(factflow.NewPostconditionRefinement(
				target, factflow.NewValueConstraint(product.NewWithPresence(reg, product.ShapeTop, presence.Present())),
			)),
		},
		PostconditionPathRelations: map[cfg.Point][]factflow.PostconditionPathRelation{
			point: {factflow.NewPostconditionPathEquality(left, right)},
		},
		ReturnPresenceRelations: map[cfg.Point]factflow.ReturnPresenceRelationSet{
			point: factflow.NewReturnPresenceRelationSet(factflow.NewReturnPresenceRelation(1, presence.Present(), 0, presence.Absent())),
		},
	})
	transaction := PlanCallResultTransaction(facts, point)
	resultSlot := key.CallResult(uint32(point), 0)
	input := state.Reachable(state.State{}).
		WriteValue(reg, resultSlot, product.Bottom(reg)).
		WriteValue(reg, key.SymbolValue(leftID), product.Top()).
		WriteValue(reg, key.SymbolValue(rightID), ready).
		WriteValue(reg, key.SymbolValue(targetID), product.Top())
	ctx := transfer.NodeContext{Registry: reg, Point: point}
	builder := visibility.NewBuilder()
	builder.Define(point, leftID, "left")
	builder.Define(point, rightID, "right")
	builder.Define(point, targetID, "target")
	authority := NewPathSemanticAuthority(visibility.NewResolver(builder.Build()), nil, nil)

	materialized := ApplyConcreteCallResultTransaction(ConcreteCallResultRequest{
		Context: ctx, Transaction: transaction, Phase: ConcreteCallResultPhaseMaterialize, Output: input,
	})
	if materialized.Canceled || !product.Equal(reg, materialized.Output.ReadValue(reg, resultSlot), ready) {
		t.Fatal("N0 did not materialize the fixed call result")
	}
	if !product.Equal(reg, materialized.Output.ReadValue(reg, key.SymbolValue(leftID)), product.Top()) {
		t.Fatal("N0 crossed the N3 postcondition barrier")
	}

	post := ApplyConcreteCallResultTransaction(ConcreteCallResultRequest{
		Context: ctx, Resolver: authority.resolver, Transaction: transaction, Phase: ConcreteCallResultPhasePostconditions, Output: materialized.Output,
	})
	if post.Canceled || !product.Equal(reg, post.Output.ReadValue(reg, key.SymbolValue(leftID)), ready) {
		t.Fatal("N3 path equality did not refine the left result")
	}
	if !presence.Equal(product.PresenceOf(post.Output.ReadValue(reg, key.SymbolValue(targetID))), presence.Present()) {
		t.Fatal("N3 value postcondition did not refine presence")
	}

	authorityMaterialized, err := authority.ApplyCallResultPhase(context.Background(), reg, transaction.Clone(), ConcreteCallResultPhaseMaterialize, input)
	if err != nil {
		t.Fatal(err)
	}
	authorityPost, err := authority.ApplyCallResultPhase(context.Background(), reg, transaction.Clone(), ConcreteCallResultPhasePostconditions, authorityMaterialized)
	if err != nil {
		t.Fatal(err)
	}
	wantAuthorityPost := ApplyConcreteCallResultTransaction(ConcreteCallResultRequest{
		Context: transfer.NodeContext{Registry: reg, Point: point}, Resolver: authority.resolver,
		Transaction: transaction, Phase: ConcreteCallResultPhasePostconditions, Output: materialized.Output,
	})
	if !state.Domain(reg).Equal(authorityPost, wantAuthorityPost.Output) {
		t.Fatal("callback-free call-result authority differs from exact N0/N3 phase execution")
	}
}

func TestCallResultTransactionCancellationRollsBackPhaseInput(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(31)
	transaction := PlanCallResultTransaction(factflow.NewFacts(factflow.FactsInput{
		CallResultValues: map[cfg.Point]factflow.CallResultValueSet{
			point: factflow.NewCallResultValueSet(factflow.NewCallResultValue(0, typevalue.LiteralString(reg, "ready"))),
		},
	}), point)
	builder := visibility.NewBuilder()
	authority := NewPathSemanticAuthority(visibility.NewResolver(builder.Build()), nil, nil)
	input := state.Reachable(state.State{}).WriteValue(reg, key.CallResult(uint32(point), 0), product.Top())
	ctx, session := cancellation.Attach(context.Background())
	session.Token().Cancel(context.Canceled)
	rolledBack, err := authority.ApplyCallResultPhase(ctx, reg, transaction, ConcreteCallResultPhaseMaterialize, input)
	if err == nil {
		t.Fatal("pre-canceled call-result authority did not report cancellation")
	}
	if !state.Domain(reg).Equal(rolledBack, input) {
		t.Fatal("canceled N0 call-result phase published a partial slot edit")
	}
}
