package factapply

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
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
				RootAssignments: map[cfg.Point]factflow.RootAssignment{
					assign: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, path.NewPath(target, "local"), source),
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
				RootAssignments: map[cfg.Point]factflow.RootAssignment{
					assign: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, target, path.NewPath(target, "ordinary"), source),
				},
			}),
			Sources: resolver,
		}),
	})

	assertValue(t, reg, got[graph.Exit()], key.SymbolValue(target), assigned)
	assertResolverCall(t, resolver, assign, source)
}

func TestFactsNodeTransferRootAssignmentAddsPathEqualityProofForPathSource(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(12)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(12), HasExpr: true}
	target := symbol.ID(112)
	sourceSymbol := symbol.ID(113)
	assigned := presentValue(reg)
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{source: assigned},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "alias")
	visibilityBuilder.Define(point, sourceSymbol, "box")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			RootAssignments: map[cfg.Point]factflow.RootAssignment{
				point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, path.NewPath(target, "alias"), source),
			},
			ExpressionPaths: map[factflow.ExprRef]path.Path{
				source.ExprRef: path.NewPath(sourceSymbol, "box"),
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	proof := pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofPathEqual,
		Path:  mustStateKey(t, ks, path.PathKey("sym112@1")),
		Other: mustStateKey(t, ks, path.PathKey("sym113@1")),
	}
	if !got.HasBranchProof(proof) {
		t.Fatalf("missing path equality proof %#v", proof)
	}
}

func TestFactsNodeTransferRootAssignmentPropagatesNumericFloorThroughIncrement(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(13)
	target := symbol.ID(113)
	targetPath := path.NewPath(target, "i")
	targetKey := targetPath.Key()
	leftRef := factflow.ExprRef(131)
	oneRef := factflow.ExprRef(132)
	incRef := factflow.ExprRef(133)
	left := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: leftRef, HasExpr: true}
	one := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: oneRef, HasExpr: true}
	inc := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: incRef, HasExpr: true}
	op, ok := factflow.NewBinaryExpressionOperation("+", left, one)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation returned false")
	}
	oneType := typ.LiteralInt(1)
	oneValue := typevalue.WithWitness(reg, typevalue.FromType(reg, oneType), oneType)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "i")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{inc: typevalue.FromType(reg, typ.Integer)},
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			RootAssignments: map[cfg.Point]factflow.RootAssignment{
				point: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, target, targetPath, inc),
			},
			ExpressionPaths: map[factflow.ExprRef]path.Path{
				leftRef: targetPath,
			},
			ExpressionValues: map[factflow.ExprRef]product.Value{
				oneRef: oneValue,
			},
			ExpressionOperations: map[factflow.ExprRef]factflow.ExpressionOperation{
				incRef: op,
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.WriteNumFloor(ks, targetKey, 1))

	floor, ok := got.ReadNumFloor(ks, targetKey)
	if !ok || floor != 2 {
		t.Fatalf("numeric floor for i = %d/%v, want 2/true", floor, ok)
	}
}

func TestFactsNodeTransferRootAssignmentClearsNumericFloorWhenSourceIsUnresolved(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(14)
	target := symbol.ID(114)
	targetPath := path.NewPath(target, "i")
	targetKey := targetPath.Key()
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(141), HasExpr: true}
	declared := presentValue(reg)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "i")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	sources := &recordingSourceValues{}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			RootAssignments: map[cfg.Point]factflow.RootAssignment{
				point: factflow.NewRootAssignmentWithDeclaredValue(factflow.RootAssignmentLocalDeclaration, target, targetPath, source, declared),
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.WriteNumFloor(ks, targetKey, 9))

	if floor, ok := got.ReadNumFloor(ks, targetKey); ok || floor != 0 {
		t.Fatalf("numeric floor for i = %d/%v, want 0/false", floor, ok)
	}
	assertValue(t, reg, got, key.SymbolValue(target), declared)
	assertResolverCall(t, sources, point, source)
}

func TestFactsNodeTransferRootAssignmentUsesDeclaredContractBeforeSource(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)

	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(12), HasExpr: true}
	target := symbol.ID(103)
	declared := product.Set(reg, presentValue(reg), runtimekind.Key, runtimekind.Singleton(runtimekind.String))

	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				RootAssignments: map[cfg.Point]factflow.RootAssignment{
					assign: factflow.NewRootAssignmentWithDeclaredContractValue(factflow.RootAssignmentLocalDeclaration, target, path.NewPath(target, "local"), source, declared),
				},
			}),
			Sources: panicSourceValues{},
		}),
	})

	assertValue(t, reg, got[graph.Exit()], key.SymbolValue(target), declared)
	assertRuntimeKind(t, reg, got[graph.Exit()].ReadValue(reg, key.SymbolValue(target)), runtimekind.Singleton(runtimekind.String))
}

func TestFactsNodeTransferRootAssignmentUsesSourceBeforeFallbackDeclaredValue(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)

	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(13), HasExpr: true}
	target := symbol.ID(104)
	declared := product.Set(reg, presentValue(reg), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	assigned := product.Set(reg, presentValue(reg), runtimekind.Key, runtimekind.Singleton(runtimekind.Number))
	resolver := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{source: assigned},
	}

	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				RootAssignments: map[cfg.Point]factflow.RootAssignment{
					assign: factflow.NewRootAssignmentWithDeclaredValue(factflow.RootAssignmentLocalDeclaration, target, path.NewPath(target, "local"), source, declared),
				},
			}),
			Sources: resolver,
		}),
	})

	assertValue(t, reg, got[graph.Exit()], key.SymbolValue(target), assigned)
	assertRuntimeKind(t, reg, got[graph.Exit()].ReadValue(reg, key.SymbolValue(target)), runtimekind.Singleton(runtimekind.Number))
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
					RootAssignments: map[cfg.Point]factflow.RootAssignment{
						point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, path.NewPath(target, "obj"), source),
					},
				}
			},
		},
		{
			name: "ordinary",
			fact: func(point cfg.Point, target symbol.ID, source factflow.ValueSource) factflow.FactsInput {
				return factflow.FactsInput{
					RootAssignments: map[cfg.Point]factflow.RootAssignment{
						point: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, target, path.NewPath(target, "obj"), source),
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
			resolver := visibility.NewResolver(visibilityBuilder.Build())
			ks := resolver.KeySpace()

			got := NewFactsNodeTransfer(FactsNodeTransferConfig{
				Facts:      factflow.NewFacts(tc.fact(point, target, source)),
				Sources:    sources,
				Visibility: resolver,
			})(transfer.NodeContext{
				Registry: reg,
				Point:    point,
			}, state.State{}.
				WritePathKey(reg, ks, rootKey, stale).
				WritePathKey(reg, ks, childKey, stale).
				WritePathKey(reg, ks, deepKey, stale).
				WritePathKey(reg, ks, otherVersionKey, stale).
				WritePathKey(reg, ks, otherSymbolKey, stale))

			assertValue(t, reg, got, key.SymbolValue(target), assigned)
			assertPathValue(t, reg, ks, got, rootKey, product.Bottom(reg))
			assertPathValue(t, reg, ks, got, childKey, product.Bottom(reg))
			assertPathValue(t, reg, ks, got, deepKey, product.Bottom(reg))
			assertPathValue(t, reg, ks, got, otherVersionKey, stale)
			assertPathValue(t, reg, ks, got, otherSymbolKey, stale)
			assertResolverCall(t, sources, point, source)
		})
	}
}
