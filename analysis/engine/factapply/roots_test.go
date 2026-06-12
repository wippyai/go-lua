package factapply

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestFactsNodeTransferAppliesLocalAssignmentThroughResolver(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)

	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(10), HasExpr: true}
	target := symbol.ID(101)
	assigned := presentValue(reg)
	resolver := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{source: assigned},
	}

	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				LocalAssignments: map[cfg.Point]factflow.RootAssignment{
					assign: factflow.NewRootAssignment(target, path.NewPath(target, "local"), source),
				},
			}),
			Sources: resolver,
		}),
	})

	assertValue(t, reg, got[assign], key.SymbolValue(target), product.Bottom(reg))
	assertValue(t, reg, got[graph.Exit()], key.SymbolValue(target), assigned)
	assertResolverCall(t, resolver, assign, source)
}

func TestFactsNodeTransferAppliesOrdinaryAssignmentThroughResolver(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)

	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(11), HasExpr: true}
	target := symbol.ID(102)
	assigned := absentValue(reg)
	resolver := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{source: assigned},
	}

	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				OrdinaryAssignments: map[cfg.Point]factflow.RootAssignment{
					assign: factflow.NewRootAssignment(target, path.NewPath(target, "ordinary"), source),
				},
			}),
			Sources: resolver,
		}),
	})

	assertValue(t, reg, got[graph.Exit()], key.SymbolValue(target), assigned)
	assertResolverCall(t, resolver, assign, source)
}

func TestFactsNodeTransferRootAssignmentInvalidatesVisiblePathSubtree(t *testing.T) {
	tests := []struct {
		name string
		fact func(cfg.Point, symbol.ID, factflow.ValueSource) factflow.FactsInput
	}{
		{
			name: "local",
			fact: func(point cfg.Point, target symbol.ID, source factflow.ValueSource) factflow.FactsInput {
				return factflow.FactsInput{
					LocalAssignments: map[cfg.Point]factflow.RootAssignment{
						point: factflow.NewRootAssignment(target, path.NewPath(target, "obj"), source),
					},
				}
			},
		},
		{
			name: "ordinary",
			fact: func(point cfg.Point, target symbol.ID, source factflow.ValueSource) factflow.FactsInput {
				return factflow.FactsInput{
					OrdinaryAssignments: map[cfg.Point]factflow.RootAssignment{
						point: factflow.NewRootAssignment(target, path.NewPath(target, "obj"), source),
					},
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := standard.Registry()
			point := cfg.Point(60)
			target := symbol.ID(120)
			source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(60), HasExpr: true}
			assigned := absentValue(reg)
			stale := presentValue(reg)
			rootKey := path.PathKey("sym120@1")
			childKey := path.PathKey("sym120@1.field")
			deepKey := path.PathKey("sym120@1.field.deep")
			otherVersionKey := path.PathKey("sym120@2.field")
			otherSymbolKey := path.PathKey("sym121@1.field")
			sources := &recordingSourceValues{
				values: map[factflow.ValueSource]product.Value{source: assigned},
			}
			visibilityBuilder := visibility.NewBuilder()
			visibilityBuilder.Define(point, target, "obj")

			got := NewFactsNodeTransfer(FactsNodeTransferConfig{
				Facts:      factflow.NewFacts(tc.fact(point, target, source)),
				Sources:    sources,
				Visibility: visibility.NewResolver(visibilityBuilder.Build()),
			})(transfer.NodeContext{
				Registry: reg,
				Point:    point,
			}, state.State{}.
				WritePathKey(reg, rootKey, stale).
				WritePathKey(reg, childKey, stale).
				WritePathKey(reg, deepKey, stale).
				WritePathKey(reg, otherVersionKey, stale).
				WritePathKey(reg, otherSymbolKey, stale))

			assertValue(t, reg, got, key.SymbolValue(target), assigned)
			assertPathValue(t, reg, got, rootKey, product.Bottom(reg))
			assertPathValue(t, reg, got, childKey, product.Bottom(reg))
			assertPathValue(t, reg, got, deepKey, product.Bottom(reg))
			assertPathValue(t, reg, got, otherVersionKey, stale)
			assertPathValue(t, reg, got, otherSymbolKey, stale)
			assertResolverCall(t, sources, point, source)
		})
	}
}
