package factapply

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestFactsNodeTransferAppliesExpressionRefinementsToAssignments(t *testing.T) {
	tests := []struct {
		name         string
		innerAbsent  bool
		refinement   product.Value
		wantKind     runtimekind.Value
		wantPresence presence.Value
	}{
		{
			name:         "runtime kind refinement attaches",
			refinement:   runtimeKindConstraint(runtimekind.Singleton(runtimekind.Table)),
			wantKind:     runtimekind.Singleton(runtimekind.Table),
			wantPresence: presence.Present(),
		},
		{
			name:         "runtime kind refinement keeps absent presence",
			innerAbsent:  true,
			refinement:   runtimeKindConstraint(runtimekind.Singleton(runtimekind.Function)),
			wantKind:     runtimekind.Singleton(runtimekind.Function),
			wantPresence: presence.Absent(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := standard.Registry()
			point := cfg.Point(51)
			target := symbol.ID(151)
			inner := factflow.ExprRef(1510)
			outer := factflow.ExprRef(1511)
			innerSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: inner, HasExpr: true}
			outerSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: outer, HasExpr: true}
			innerValue := presentValue(reg)
			if tc.innerAbsent {
				innerValue = absentValue(reg)
			}
			sources := sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
				Registry: reg,
				ExpressionValues: map[factflow.ExprRef]product.Value{
					inner: innerValue,
				},
			})

			got := NewFactsNodeTransfer(FactsNodeTransferConfig{
				Facts: factflow.NewFacts(factflow.FactsInput{
					RootAssignments: map[cfg.Point]factflow.RootAssignment{
						point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, path.NewPath(target, "value"), outerSource),
					},
					ExpressionRefinements: map[factflow.ExprRef]factflow.ExpressionRefinement{
						outer: factflow.NewExpressionRefinement(innerSource, tc.refinement),
					},
				}),
				Sources: sources,
			})(transfer.NodeContext{
				Registry: reg,
				Point:    point,
			}, state.State{})

			assigned := got.ReadValue(reg, key.SymbolValue(target))
			if gotPresence := product.PresenceOf(assigned); !presence.Equal(gotPresence, tc.wantPresence) {
				t.Fatalf("assigned presence = %s, want %s", gotPresence, tc.wantPresence)
			}
			if gotKind := product.Get(reg, assigned, runtimekind.Key); !runtimekind.Equal(gotKind, tc.wantKind) {
				t.Fatalf("assigned runtime kind = %s, want %s", gotKind, tc.wantKind)
			}
		})
	}
}

func TestFactsNodeTransferMissingResolverValueLeavesStateUnchanged(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(12)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(12), HasExpr: true}
	target := symbol.ID(103)
	unchangedValue := presentValue(reg)
	in := state.State{}.WriteValue(reg, key.SymbolValue(target), unchangedValue)

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			RootAssignments: map[cfg.Point]factflow.RootAssignment{
				point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, path.NewPath(target, "local"), source),
			},
		}),
		Sources: &recordingSourceValues{},
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	assertStateEqual(t, reg, got, in)
}

func TestFactsNodeTransferAbsentFactsAndNilResolverLeaveStateUnchanged(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(13)
	target := symbol.ID(104)
	in := state.State{}.WriteValue(reg, key.SymbolValue(target), presentValue(reg))

	gotNoResolver := NewFactsNodeTransfer(FactsNodeTransferConfig{Facts: factflow.Facts{}})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, in)
	assertStateEqual(t, reg, gotNoResolver, in)

	gotNoFacts := NewFactsNodeTransfer(FactsNodeTransferConfig{Sources: panicSourceValues{}})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, in)
	assertStateEqual(t, reg, gotNoFacts, in)
}

func TestFactsNodeTransferIgnoresNonRootAssignmentFacts(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(14)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(14), HasExpr: true}
	target := symbol.ID(105)
	in := state.State{}.WriteValue(reg, key.SymbolValue(target), presentValue(reg))
	resolver := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{source: absentValue(reg)},
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			RootAssignments: map[cfg.Point]factflow.RootAssignment{
				point: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, target, path.NewPath(target, "ordinary").Field("member"), source),
			},
		}),
		Sources: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	assertStateEqual(t, reg, got, in)
	if len(resolver.calls) != 0 {
		t.Fatalf("non-root assignment resolved source %d times, want zero", len(resolver.calls))
	}
}
