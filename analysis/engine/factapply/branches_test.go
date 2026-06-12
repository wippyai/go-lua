package factapply

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestFactsEdgeTransferAppliesNilRefinementsOnRootValue(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	target := symbol.ID(301)
	initial := state.State{}.WriteValue(reg, key.SymbolValue(target), product.Top())
	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						branchWithPresence(pathdom.NewPath(target, "x"), presence.Absent(), true, presence.Present(), true),
					),
				},
			}),
		}),
	})

	assertValue(t, reg, got[thenPoint], key.SymbolValue(target), absentValue(reg))
	assertValue(t, reg, got[elsePoint], key.SymbolValue(target), presentValue(reg))
}

func TestFactsEdgeTransferAppliesMultipleRefinementsOnSameBranchEdge(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	firstTarget := symbol.ID(313)
	secondTarget := symbol.ID(314)
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(firstTarget), product.Top()).
		WriteValue(reg, key.SymbolValue(secondTarget), product.Top())
	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						branchWithPresence(pathdom.NewPath(firstTarget, "first"), presence.Present(), true, presence.Absent(), true),
						branchWithPresence(pathdom.NewPath(secondTarget, "second"), presence.Present(), true, presence.Absent(), true),
					),
				},
			}),
		}),
	})

	assertValue(t, reg, got[thenPoint], key.SymbolValue(firstTarget), presentValue(reg))
	assertValue(t, reg, got[thenPoint], key.SymbolValue(secondTarget), presentValue(reg))
	assertValue(t, reg, got[elsePoint], key.SymbolValue(firstTarget), absentValue(reg))
	assertValue(t, reg, got[elsePoint], key.SymbolValue(secondTarget), absentValue(reg))
}

func TestFactsEdgeTransferAppliesBranchPresenceRelationFromErrorPath(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	value := symbol.ID(315)
	err := symbol.ID(316)
	valuePath := pathdom.NewPath(value, "value")
	errPath := pathdom.NewPath(err, "err")
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(value), product.Top()).
		WriteValue(reg, key.SymbolValue(err), product.Top())

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						branchWithPresence(errPath, presence.Absent(), true, presence.Present(), true),
					),
				},
				BranchPresenceRelations: map[cfg.Point]factflow.BranchPresenceRelationSet{
					branch: factflow.NewBranchPresenceRelationSet(
						factflow.NewBranchPresenceRelation(errPath, presence.Present(), valuePath, presence.Absent()),
						factflow.NewBranchPresenceRelation(errPath, presence.Absent(), valuePath, presence.Present()),
					),
				},
			}),
		}),
	})

	assertValue(t, reg, got[thenPoint], key.SymbolValue(value), presentValue(reg))
	assertValue(t, reg, got[elsePoint], key.SymbolValue(value), absentValue(reg))
}

func TestFactsEdgeTransferAppliesPathEqualityRelationRootRoot(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	left := symbol.ID(319)
	right := symbol.ID(320)
	numberValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Number))
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(left), numberValue).
		WriteValue(reg, key.SymbolValue(right), product.Top())

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchPathRelations: map[cfg.Point]factflow.BranchPathRelationSet{
					branch: factflow.NewBranchPathRelationSet(
						factflow.NewBranchPathEquality(pathdom.NewPath(left, "left"), pathdom.NewPath(right, "right"), true, false),
					),
				},
			}),
		}),
	})

	assertRuntimeKind(t, reg, got[thenPoint].ReadValue(reg, key.SymbolValue(left)), runtimekind.Singleton(runtimekind.Number))
	assertRuntimeKind(t, reg, got[thenPoint].ReadValue(reg, key.SymbolValue(right)), runtimekind.Singleton(runtimekind.Number))
	assertRuntimeKind(t, reg, got[elsePoint].ReadValue(reg, key.SymbolValue(left)), runtimekind.Singleton(runtimekind.Number))
	assertRuntimeKind(t, reg, got[elsePoint].ReadValue(reg, key.SymbolValue(right)), runtimekind.Top())
}

func TestFactsEdgeTransferAppliesPathInequalityEqualityOnFalseEdge(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	left := symbol.ID(321)
	right := symbol.ID(322)
	functionValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Function))
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(left), functionValue).
		WriteValue(reg, key.SymbolValue(right), product.Top())

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchPathRelations: map[cfg.Point]factflow.BranchPathRelationSet{
					branch: factflow.NewBranchPathRelationSet(
						factflow.NewBranchPathEquality(pathdom.NewPath(left, "left"), pathdom.NewPath(right, "right"), false, true),
					),
				},
			}),
		}),
	})

	assertRuntimeKind(t, reg, got[thenPoint].ReadValue(reg, key.SymbolValue(left)), runtimekind.Singleton(runtimekind.Function))
	assertRuntimeKind(t, reg, got[thenPoint].ReadValue(reg, key.SymbolValue(right)), runtimekind.Top())
	assertRuntimeKind(t, reg, got[elsePoint].ReadValue(reg, key.SymbolValue(left)), runtimekind.Singleton(runtimekind.Function))
	assertRuntimeKind(t, reg, got[elsePoint].ReadValue(reg, key.SymbolValue(right)), runtimekind.Singleton(runtimekind.Function))
}

func TestFactsEdgeTransferPathComparisonNarrowsParentVariantOrigin(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	chanInt := typ.NewAlias("__test_ChanInt", typetable.NewRecord().
		Field("__tag", typ.LiteralString("int")).
		Build())
	chanStr := typ.NewAlias("__test_ChanStr", typetable.NewRecord().
		Field("__tag", typ.LiteralString("str")).
		Build())
	intCase := typetable.NewRecord().
		Field("channel", chanInt).
		Field("value", typ.Number).
		Build()
	strCase := typetable.NewRecord().
		Field("channel", chanStr).
		Field("value", typ.String).
		Build()
	union := typ.NewUnion(intCase, strCase)

	result := symbol.ID(325)
	ch1 := symbol.ID(326)
	resultPath := pathdom.NewPath(result, "result")
	ch1Path := pathdom.NewPath(ch1, "ch1")
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(result), typevalue.FromType(reg, union)).
		WriteValue(reg, key.SymbolValue(ch1), typevalue.FromType(reg, chanInt))
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, result, "result")

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchPathRelations: map[cfg.Point]factflow.BranchPathRelationSet{
					branch: factflow.NewBranchPathRelationSet(
						factflow.NewBranchPathEquality(resultPath.Field("channel"), ch1Path, true, false),
						factflow.NewBranchPathInequality(resultPath.Field("channel"), ch1Path, false, true),
					),
				},
			}),
			Visibility: visibility.NewResolver(visibilityBuilder.Build()),
		}),
	})

	assertVariantOriginType(t, reg, got[thenPoint], result, union, intCase)
	assertVariantOriginType(t, reg, got[elsePoint], result, union, strCase)
}

func TestFactsEdgeTransferAppliesPathEqualityRelationRootMember(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	root := symbol.ID(323)
	table := symbol.ID(324)
	memberPath := pathdom.NewPath(table, "table").Field("field")
	memberKey := pathdom.PathKey("sym324@1.field")
	stringValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(root), stringValue).
		WritePathKey(reg, memberKey, product.Top())
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, table, "table")

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchPathRelations: map[cfg.Point]factflow.BranchPathRelationSet{
					branch: factflow.NewBranchPathRelationSet(
						factflow.NewBranchPathEquality(pathdom.NewPath(root, "root"), memberPath, true, false),
					),
				},
			}),
			Visibility: visibility.NewResolver(visibilityBuilder.Build()),
		}),
	})

	assertRuntimeKind(t, reg, got[thenPoint].ReadValue(reg, key.SymbolValue(root)), runtimekind.Singleton(runtimekind.String))
	assertRuntimeKind(t, reg, got[thenPoint].ReadPathKey(reg, memberKey), runtimekind.Singleton(runtimekind.String))
	assertRuntimeKind(t, reg, got[elsePoint].ReadValue(reg, key.SymbolValue(root)), runtimekind.Singleton(runtimekind.String))
	assertRuntimeKind(t, reg, got[elsePoint].ReadPathKey(reg, memberKey), runtimekind.Top())
}

func TestFactsEdgeTransferAppliesPathEqualityRelationMemberMember(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	leftRoot := symbol.ID(325)
	rightRoot := symbol.ID(326)
	leftPath := pathdom.NewPath(leftRoot, "left").Field("value")
	rightPath := pathdom.NewPath(rightRoot, "right").Field("value")
	leftKey := pathdom.PathKey("sym325@1.value")
	rightKey := pathdom.PathKey("sym326@1.value")
	numberValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Number))
	initial := state.State{}.
		WritePathKey(reg, leftKey, numberValue).
		WritePathKey(reg, rightKey, product.Top())
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, leftRoot, "left")
	visibilityBuilder.Define(branch, rightRoot, "right")

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchPathRelations: map[cfg.Point]factflow.BranchPathRelationSet{
					branch: factflow.NewBranchPathRelationSet(
						factflow.NewBranchPathEquality(leftPath, rightPath, true, false),
					),
				},
			}),
			Visibility: visibility.NewResolver(visibilityBuilder.Build()),
		}),
	})

	assertRuntimeKind(t, reg, got[thenPoint].ReadPathKey(reg, leftKey), runtimekind.Singleton(runtimekind.Number))
	assertRuntimeKind(t, reg, got[thenPoint].ReadPathKey(reg, rightKey), runtimekind.Singleton(runtimekind.Number))
	assertRuntimeKind(t, reg, got[elsePoint].ReadPathKey(reg, leftKey), runtimekind.Singleton(runtimekind.Number))
	assertRuntimeKind(t, reg, got[elsePoint].ReadPathKey(reg, rightKey), runtimekind.Top())
}

func TestFactsEdgeTransferPathEqualityMissingVisibilityNoops(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	root := symbol.ID(327)
	table := symbol.ID(328)
	memberPath := pathdom.NewPath(table, "table").Field("field")
	memberKey := pathdom.PathKey("sym328@1.field")
	numberValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Number))
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(root), numberValue).
		WritePathKey(reg, memberKey, product.Top())

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchPathRelations: map[cfg.Point]factflow.BranchPathRelationSet{
					branch: factflow.NewBranchPathRelationSet(
						factflow.NewBranchPathEquality(pathdom.NewPath(root, "root"), memberPath, true, false),
					),
				},
			}),
		}),
	})

	assertRuntimeKind(t, reg, got[thenPoint].ReadValue(reg, key.SymbolValue(root)), runtimekind.Singleton(runtimekind.Number))
	assertRuntimeKind(t, reg, got[thenPoint].ReadPathKey(reg, memberKey), runtimekind.Top())
	assertRuntimeKind(t, reg, got[elsePoint].ReadValue(reg, key.SymbolValue(root)), runtimekind.Singleton(runtimekind.Number))
	assertRuntimeKind(t, reg, got[elsePoint].ReadPathKey(reg, memberKey), runtimekind.Top())
}

func TestFactsEdgeTransferErrorReturnRelationDoesNotInventFalsyNilBranch(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	value := symbol.ID(317)
	err := symbol.ID(318)
	valuePath := pathdom.NewPath(value, "value")
	errPath := pathdom.NewPath(err, "err")
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(value), product.Top()).
		WriteValue(reg, key.SymbolValue(err), product.Top())

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						branchWithPresence(errPath, presence.Present(), true, presence.Bottom(), false),
					),
				},
				BranchPresenceRelations: map[cfg.Point]factflow.BranchPresenceRelationSet{
					branch: factflow.NewBranchPresenceRelationSet(
						factflow.NewBranchPresenceRelation(errPath, presence.Present(), valuePath, presence.Absent()),
						factflow.NewBranchPresenceRelation(errPath, presence.Absent(), valuePath, presence.Present()),
					),
				},
			}),
		}),
	})

	assertValue(t, reg, got[thenPoint], key.SymbolValue(value), absentValue(reg))
	assertValue(t, reg, got[elsePoint], key.SymbolValue(value), product.Top())
}

func TestFactsEdgeTransferOneSidedTruthyFalsyRefinements(t *testing.T) {
	tests := []struct {
		name      string
		fact      factflow.BranchRefinement
		wantTrue  product.Value
		wantFalse product.Value
	}{
		{
			name:      "truthy refines true edge only",
			fact:      branchWithPresence(pathdom.NewPath(symbol.ID(302), "x"), presence.Present(), true, presence.Bottom(), false),
			wantTrue:  presentValue(standard.Registry()),
			wantFalse: product.Top(),
		},
		{
			name:      "falsy refines false edge only",
			fact:      branchWithPresence(pathdom.NewPath(symbol.ID(303), "x"), presence.Bottom(), false, presence.Present(), true),
			wantTrue:  product.Top(),
			wantFalse: presentValue(standard.Registry()),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := standard.Registry()
			graph := cfg.New()
			branch := graph.AddNode(cfg.NodeBranch)
			thenPoint := graph.AddNode(cfg.NodeNoop)
			elsePoint := graph.AddNode(cfg.NodeNoop)
			graph.AddEdge(graph.Entry(), branch, false)
			graph.AddEdge(branch, thenPoint, true)
			graph.AddEdge(branch, elsePoint, false)
			graph.AddEdge(thenPoint, graph.Exit(), false)
			graph.AddEdge(elsePoint, graph.Exit(), false)

			target := tc.fact.TargetPath().Symbol
			initial := state.State{}.WriteValue(reg, key.SymbolValue(target), product.Top())
			got := transfer.Run(transfer.Config{
				Graph:      graph,
				Registry:   reg,
				EntryState: initial,
				EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
					Facts: factflow.NewFacts(factflow.FactsInput{
						BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
							branch: factflow.NewBranchRefinementSet(tc.fact),
						},
					}),
				}),
			})

			assertValue(t, reg, got[thenPoint], key.SymbolValue(target), tc.wantTrue)
			assertValue(t, reg, got[elsePoint], key.SymbolValue(target), tc.wantFalse)
		})
	}
}

func TestFactsEdgeTransferRefinesStaticMemberPathThroughVisibility(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	target := symbol.ID(304)
	targetPath := pathdom.NewPath(target, "t").Field("field")
	pathKey := pathdom.PathKey("sym304@1.field")
	initial := state.State{}.WritePathKey(reg, pathKey, product.Top())
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, target, "t")

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						branchWithPresence(targetPath, presence.Present(), true, presence.Absent(), true),
					),
				},
			}),
			Visibility: visibility.NewResolver(visibilityBuilder.Build()),
		}),
	})

	assertPathValue(t, reg, got[thenPoint], pathKey, presentValue(reg))
	assertPathValue(t, reg, got[elsePoint], pathKey, absentValue(reg))
	assertValue(t, reg, got[thenPoint], key.SymbolValue(target), product.Bottom(reg))
}

func TestFactsEdgeTransferRefinesRuntimeKindOnRootValue(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	target := symbol.ID(308)
	initial := state.State{}.WriteValue(reg, key.SymbolValue(target), product.Top())
	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						branchWithRuntimeKind(pathdom.NewPath(target, "x"), runtimekind.Singleton(runtimekind.Table), true, runtimekind.Value{}, false),
					),
				},
			}),
		}),
	})

	assertRuntimeKind(t, reg, got[thenPoint].ReadValue(reg, key.SymbolValue(target)), runtimekind.Singleton(runtimekind.Table))
	assertRuntimeKind(t, reg, got[elsePoint].ReadValue(reg, key.SymbolValue(target)), runtimekind.Top())
}

func TestFactsEdgeTransferRefinesRuntimeKindOnStaticMemberPath(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	target := symbol.ID(309)
	targetPath := pathdom.NewPath(target, "t").Field("field")
	pathKey := pathdom.PathKey("sym309@1.field")
	initial := state.State{}.WritePathKey(reg, pathKey, product.Top())
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, target, "t")

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						branchWithRuntimeKind(targetPath, runtimekind.Singleton(runtimekind.Function), true, runtimekind.Value{}, false),
					),
				},
			}),
			Visibility: visibility.NewResolver(visibilityBuilder.Build()),
		}),
	})

	assertRuntimeKind(t, reg, got[thenPoint].ReadPathKey(reg, pathKey), runtimekind.Singleton(runtimekind.Function))
	assertRuntimeKind(t, reg, got[elsePoint].ReadPathKey(reg, pathKey), runtimekind.Top())
}

func TestFactsEdgeTransferRuntimeKindContradictionGoesBottom(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	target := symbol.ID(310)
	numberValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Number))
	initial := state.State{}.WriteValue(reg, key.SymbolValue(target), numberValue)
	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						branchWithRuntimeKind(pathdom.NewPath(target, "x"), runtimekind.Singleton(runtimekind.Table), true, runtimekind.Value{}, false),
					),
				},
			}),
		}),
	})

	assertValue(t, reg, got[thenPoint], key.SymbolValue(target), product.Bottom(reg))
	assertRuntimeKind(t, reg, got[elsePoint].ReadValue(reg, key.SymbolValue(target)), runtimekind.Singleton(runtimekind.Number))
}

func TestFactsEdgeTransferAppliesGenericProductConstraintAxis(t *testing.T) {
	reg := wideningRegistry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	target := symbol.ID(312)
	initialValue := wideningValue(reg, wideningExactMax)
	constraint := product.Set(reg, product.Top(), wideningKey, wideningOne)
	trueRefinement := factflow.NewValueRefinement().WithConstraint(reg, constraint)
	initial := state.State{}.WriteValue(reg, key.SymbolValue(target), initialValue)
	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						factflow.NewBranchRefinement(pathdom.NewPath(target, "x"), trueRefinement, true, factflow.ValueRefinement{}, false),
					),
				},
			}),
		}),
	})

	if gotValue := product.Get(reg, got[thenPoint].ReadValue(reg, key.SymbolValue(target)), wideningKey); gotValue != wideningOne {
		t.Fatalf("true edge custom axis = %v, want %v", gotValue, wideningOne)
	}
	if gotValue := product.Get(reg, got[elsePoint].ReadValue(reg, key.SymbolValue(target)), wideningKey); gotValue != wideningExactMax {
		t.Fatalf("false edge custom axis = %v, want %v", gotValue, wideningExactMax)
	}
}

func TestFactsEdgeTransferNoopsWithoutBranchConditionOrVisibility(t *testing.T) {
	t.Run("non-branch edge", func(t *testing.T) {
		reg := standard.Registry()
		graph := cfg.New()
		mid := graph.AddNode(cfg.NodeNoop)
		graph.AddEdge(graph.Entry(), mid, false)
		graph.AddEdge(mid, graph.Exit(), false)

		target := symbol.ID(305)
		initial := state.State{}.WriteValue(reg, key.SymbolValue(target), product.Top())
		got := transfer.Run(transfer.Config{
			Graph:      graph,
			Registry:   reg,
			EntryState: initial,
			EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
				Facts: factflow.NewFacts(factflow.FactsInput{
					BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
						graph.Entry(): factflow.NewBranchRefinementSet(
							branchWithPresence(pathdom.NewPath(target, "x"), presence.Absent(), true, presence.Present(), true),
						),
					},
				}),
			}),
		})

		assertValue(t, reg, got[mid], key.SymbolValue(target), product.Top())
	})

	t.Run("missing visibility for static path", func(t *testing.T) {
		reg := standard.Registry()
		graph := cfg.New()
		branch := graph.AddNode(cfg.NodeBranch)
		thenPoint := graph.AddNode(cfg.NodeNoop)
		elsePoint := graph.AddNode(cfg.NodeNoop)
		graph.AddEdge(graph.Entry(), branch, false)
		graph.AddEdge(branch, thenPoint, true)
		graph.AddEdge(branch, elsePoint, false)
		graph.AddEdge(thenPoint, graph.Exit(), false)
		graph.AddEdge(elsePoint, graph.Exit(), false)

		target := symbol.ID(306)
		targetPath := pathdom.NewPath(target, "t").Field("field")
		pathKey := pathdom.PathKey("sym306@1.field")
		initial := state.State{}.WritePathKey(reg, pathKey, product.Top())
		got := transfer.Run(transfer.Config{
			Graph:      graph,
			Registry:   reg,
			EntryState: initial,
			EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
				Facts: factflow.NewFacts(factflow.FactsInput{
					BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
						branch: factflow.NewBranchRefinementSet(
							branchWithPresence(targetPath, presence.Present(), true, presence.Absent(), true),
						),
					},
				}),
			}),
		})

		assertPathValue(t, reg, got[thenPoint], pathKey, product.Top())
		assertPathValue(t, reg, got[elsePoint], pathKey, product.Top())
	})
}

func TestFactsEdgeTransferJoinRestoresMaybePresence(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	join := graph.AddNode(cfg.NodeJoin)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, join, false)
	graph.AddEdge(elsePoint, join, false)
	graph.AddEdge(join, graph.Exit(), false)

	target := symbol.ID(307)
	initial := state.State{}.WriteValue(reg, key.SymbolValue(target), product.Top())
	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						branchWithPresence(pathdom.NewPath(target, "x"), presence.Absent(), true, presence.Present(), true),
					),
				},
			}),
		}),
	})

	assertValue(t, reg, got[thenPoint], key.SymbolValue(target), absentValue(reg))
	assertValue(t, reg, got[elsePoint], key.SymbolValue(target), presentValue(reg))
	assertValue(t, reg, got[join], key.SymbolValue(target), product.Top())
}

func TestFactsEdgeTransferJoinRestoresRuntimeKindUnion(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	join := graph.AddNode(cfg.NodeJoin)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, join, false)
	graph.AddEdge(elsePoint, join, false)
	graph.AddEdge(join, graph.Exit(), false)

	target := symbol.ID(311)
	initial := state.State{}.WriteValue(reg, key.SymbolValue(target), product.Top())
	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						branchWithRuntimeKind(
							pathdom.NewPath(target, "x"),
							runtimekind.Singleton(runtimekind.Table), true,
							runtimekind.Singleton(runtimekind.Function), true,
						),
					),
				},
			}),
		}),
	})

	tableKind := runtimekind.Singleton(runtimekind.Table)
	functionKind := runtimekind.Singleton(runtimekind.Function)
	assertRuntimeKind(t, reg, got[thenPoint].ReadValue(reg, key.SymbolValue(target)), tableKind)
	assertRuntimeKind(t, reg, got[elsePoint].ReadValue(reg, key.SymbolValue(target)), functionKind)
	assertRuntimeKind(t, reg, got[join].ReadValue(reg, key.SymbolValue(target)), runtimekind.Join(tableKind, functionKind))
}
