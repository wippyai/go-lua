package transformer

import (
	"context"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestFormalChannelSelectUsesCanonicalGuardedFactorTransaction(t *testing.T) {
	reg := standard.Registry()
	base := formalRootInputTestProgram(t, reg)
	arena := base.bodies[0].relation.code.terms
	guard := arena.Truthy(arena.Root(Root{Kind: RootParam, Index: 0}))
	point := cfg.Point(41)
	path := pathdom.NewPath(symbol.ID(101), "param").Field("channel")
	payload := typevalue.LiteralString(reg, "payload")
	facts := factflow.NewFacts(factflow.FactsInput{ChannelSelects: map[cfg.Point]factflow.ChannelSelectSet{
		point: factflow.NewChannelSelectSet(factflow.NewChannelSelect(factflow.ChannelSelectConfig{
			SelectID: "formal-select", Kind: factflow.ChannelSelectReceive, Index: 2,
			ResultPath: path, HasResultPath: true, CasePath: path, HasCasePath: true,
			PayloadValue: payload, HasPayloadValue: true,
		})),
	}})
	program := formalRelationExecutorTestProgramFromBase(t, base, []relationNode{
		{},
		{kind: relationNodeSequence, steps: []boundaryStep{{
			kind: boundaryStepChannelSelect, guard: guard,
			channel: factapply.PlanChannelSelectTransaction(facts, point),
		}}, next: 2},
		{kind: relationNodeOutcome, outcome: 1},
	})
	execution, err := executeFormalRelation(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	cell := formalRelationCell{Variable: 1, Kind: formalRelationCellStep, Root: 1, Step: 1}
	tuple := execution.values[cell]
	equation, ok := program.formalTemplate.equation(cell)
	if !ok {
		t.Fatal("ChannelSelect equation")
	}
	span, _, _, _ := execution.algebra.span(1)
	plan := equation.Operator.channelSelect
	wantRead := len(plan.values.members) + len(plan.channel.members)
	if plan.hasPathValues {
		wantRead += len(plan.pathValues.members)
	}
	wantWrite := len(plan.values.members) + len(plan.channel.members)
	if len(plan.readOrdinals) != wantRead || len(plan.writeOrdinals) != wantWrite || len(plan.readOrdinals) >= span.count {
		t.Fatalf("ChannelSelect widths read/write/product = %d/%d/%d, want %d/%d/<product", len(plan.readOrdinals), len(plan.writeOrdinals), span.count, wantRead, wantWrite)
	}
	t.Logf("ChannelSelect correlated width %d -> %d descriptors; publishes %d", span.count, len(plan.readOrdinals), len(plan.writeOrdinals))
	operands, err := partitionFormalRelationStepOperands(equation)
	if err != nil {
		t.Fatal(err)
	}
	input := execution.values[operands.Flow.Source.cell]
	condition, err := execution.algebra.decisionForGuard(1, 0, arena, guard)
	if err != nil {
		t.Fatal(err)
	}
	complement, err := formalDecisionBooleanNot(execution.algebra, condition)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		care decisionRef
		want int
	}{{"selected", condition, 1}, {"not-selected", complement, 0}} {
		t.Run(test.name, func(t *testing.T) {
			guarded, restrictErr := execution.algebra.restrictTupleCare(tuple, test.care)
			if restrictErr != nil {
				t.Fatal(restrictErr)
			}
			regions, regionsErr := execution.algebra.tupleLeafRegions(guarded)
			if regionsErr != nil || len(regions) != 1 {
				t.Fatalf("regions = %d, %v", len(regions), regionsErr)
			}
			factor, factorErr := regions[0].evaluator.laneFactor(equation.Operator.channelSelect.channel)
			if factorErr != nil {
				t.Fatal(factorErr)
			}
			residual, composeErr := regions[0].evaluator.authority.product.ComposeSparse([]state.LaneFactor{factor})
			if composeErr != nil {
				t.Fatal(composeErr)
			}
			snapshot := residual.ChannelSelectFactsSnapshot()
			if len(snapshot.Facts) != test.want {
				t.Fatalf("facts = %d, want %d", len(snapshot.Facts), test.want)
			}
			if test.want != 0 {
				result, resultOK := span.keys.FromStateKey(snapshot.Facts[0].Result.PathKey())
				selected, selectedOK := span.keys.FromStateKey(snapshot.Facts[0].Case.PathKey())
				if !resultOK || !selectedOK {
					t.Fatal("formal ChannelSelect StateKeys did not round-trip")
				}
				if _, typed := span.keys.DescribeFormalRoot(result); !typed {
					t.Fatal("formal result path lost typed root identity")
				}
				if _, typed := span.keys.DescribeFormalRoot(selected); !typed {
					t.Fatal("formal case path lost typed root identity")
				}
				guardedInput, inputErr := execution.algebra.restrictTupleCare(input, test.care)
				if inputErr != nil {
					t.Fatal(inputErr)
				}
				inputRegions, inputErr := execution.algebra.tupleLeafRegions(guardedInput)
				if inputErr != nil || len(inputRegions) != 1 {
					t.Fatalf("input regions = %d, %v", len(inputRegions), inputErr)
				}
				for _, group := range span.groupDescriptors() {
					if group.kind == equation.Operator.channelSelect.channel.kind && group.lane == equation.Operator.channelSelect.channel.lane {
						continue
					}
					if group.kind == formalFiberGroupValues {
						before, beforeErr := inputRegions[0].evaluator.valuesFactor()
						after, afterErr := regions[0].evaluator.valuesFactor()
						if beforeErr != nil || afterErr != nil || !state.ValueFactorLattice[FormalSlot](reg).Equal(before, after) {
							t.Fatalf("ChannelSelect changed Values: %v/%v", beforeErr, afterErr)
						}
						continue
					}
					before, beforeErr := inputRegions[0].evaluator.laneFactor(group)
					after, afterErr := regions[0].evaluator.laneFactor(group)
					equal, equalErr := regions[0].evaluator.authority.product.LaneEqual(before, after)
					if beforeErr != nil || afterErr != nil || equalErr != nil || !equal {
						t.Fatalf("ChannelSelect changed unrelated lane %s: %v/%v/%v", group.lane.ID(), beforeErr, afterErr, equalErr)
					}
				}
			}
		})
	}
}
