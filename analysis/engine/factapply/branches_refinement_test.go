package factapply

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
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

func TestFactsEdgeTransferRootRefinementInvalidatesDescendantPathFacts(t *testing.T) {
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

	target := symbol.ID(329)
	rootPath := pathdom.NewPath(target, "r")
	childKey := pathdom.PathKey("sym329@1.value")
	staleChild := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(target), product.Top()).
		WritePathKey(reg, childKey, staleChild)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, target, "r")

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						branchWithRuntimeKind(rootPath, runtimekind.Singleton(runtimekind.Table), true, runtimekind.Value{}, false),
					),
				},
			}),
			Visibility: visibility.NewResolver(visibilityBuilder.Build()),
		}),
	})

	assertRuntimeKind(t, reg, got[thenPoint].ReadValue(reg, key.SymbolValue(target)), runtimekind.Singleton(runtimekind.Table))
	assertPathValue(t, reg, got[thenPoint], childKey, product.Bottom(reg))
	assertPathValue(t, reg, got[elsePoint], childKey, staleChild)
}

func TestFactsEdgeTransferRootRefinementAllowsLaterChildRepublish(t *testing.T) {
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

	target := symbol.ID(330)
	rootPath := pathdom.NewPath(target, "r")
	childPath := rootPath.Field("value")
	childKey := pathdom.PathKey("sym330@1.value")
	staleChild := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(target), product.Top()).
		WritePathKey(reg, childKey, staleChild)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, target, "r")

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						branchWithRuntimeKind(rootPath, runtimekind.Singleton(runtimekind.Table), true, runtimekind.Value{}, false),
						branchWithRuntimeKind(childPath, runtimekind.Singleton(runtimekind.Number), true, runtimekind.Value{}, false),
					),
				},
			}),
			Visibility: visibility.NewResolver(visibilityBuilder.Build()),
		}),
	})

	assertRuntimeKind(t, reg, got[thenPoint].ReadValue(reg, key.SymbolValue(target)), runtimekind.Singleton(runtimekind.Table))
	assertRuntimeKind(t, reg, got[thenPoint].ReadPathKey(reg, childKey), runtimekind.Singleton(runtimekind.Number))
	assertPathValue(t, reg, got[elsePoint], childKey, staleChild)
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
