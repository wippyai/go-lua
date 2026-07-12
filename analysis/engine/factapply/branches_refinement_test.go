package factapply

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/typenarrow"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
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

func TestFactsEdgeTransferWritesRootConstraintWhenRootValueAbsent(t *testing.T) {
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

	target := symbol.ID(1301)
	tableValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typetable.BuiltinTopMarker()), typetable.BuiltinTopMarker())
	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						factflow.NewBranchRefinement(pathdom.NewPath(target, "bindings"), factflow.NewValueConstraint(tableValue), true, factflow.ValueRefinement{}, false),
					),
				},
			}),
		}),
	})

	gotValue := got[thenPoint].ReadValue(reg, key.SymbolValue(target))
	gotType, ok := typevalue.TypeOf(reg, gotValue)
	if !ok || !typ.TypeEquals(gotType, typetable.BuiltinTopMarker()) {
		t.Fatalf("then root type = %v/%v, want table constraint written from branch proof", gotType, ok)
	}
	assertValue(t, reg, got[elsePoint], key.SymbolValue(target), product.Bottom(reg))
}

func TestFactsEdgeTransferContradictoryBranchRefinementKillsEdge(t *testing.T) {
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

	target := symbol.ID(312)
	initial := state.State{}.WriteValue(reg, key.SymbolValue(target), typevalue.Nil(reg))
	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						branchWithPresence(pathdom.NewPath(target, "x"), presence.Present(), true, presence.Bottom(), false),
					),
				},
				BranchPathEvidence: map[cfg.Point]factflow.BranchPathEvidenceSet{
					branch: factflow.NewBranchPathEvidenceSet(
						factflow.NewBranchPathTruthyEvidenceWithOppositeOnEdge(pathdom.NewPath(target, "x"), true),
					),
				},
			}),
		}),
	})

	domain := state.Domain(reg)
	if !domain.Equal(got[thenPoint], domain.Bottom()) {
		t.Fatalf("then edge state = %#v, want unreachable bottom", got[thenPoint])
	}
	assertValue(t, reg, got[elsePoint], key.SymbolValue(target), typevalue.Nil(reg))
}

func TestFactsEdgeTransferNegatedRootLiteralKillsOnlyContradictoryEdge(t *testing.T) {
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

	target := symbol.ID(313)
	typeValues := typevalue.NewCache()
	lit := typeValues.FromTypeWithWitness(reg, typ.LiteralString("auto"))
	initial := state.State{}.WriteValue(reg, key.SymbolValue(target), lit)
	notAuto := factflow.NewNegatedLiteralConstraint(lit)
	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						factflow.NewBranchRefinement(pathdom.NewPath(target, "kind"), factflow.ValueRefinement{}, false, notAuto, true),
					),
				},
			}),
		}),
	})

	assertValue(t, reg, got[thenPoint], key.SymbolValue(target), lit)
	domain := state.Domain(reg)
	if !domain.Equal(got[elsePoint], domain.Bottom()) {
		t.Fatalf("else edge state = %#v, want unreachable bottom for proven \"auto\" ~= \"auto\"", got[elsePoint])
	}

	broad := typeValues.FromTypeWithWitness(reg, typ.String)
	got = transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: state.State{}.WriteValue(reg, key.SymbolValue(target), broad),
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						factflow.NewBranchRefinement(pathdom.NewPath(target, "kind"), factflow.ValueRefinement{}, false, notAuto, true),
					),
				},
			}),
		}),
	})
	assertValue(t, reg, got[elsePoint], key.SymbolValue(target), broad)
}

func TestFactsEdgeTransferTruthyGuardOnRequiredFunctionMemberKillsFalseEdge(t *testing.T) {
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

	target := symbol.ID(313)
	rootPath := pathdom.NewPath(target, "handlers")
	initPath := rootPath.Field("__init")
	visibilityBuilder := visibility.NewBuilder()
	version := visibilityBuilder.Define(branch, target, "handlers")
	visibilityBuilder.SetVisible(thenPoint, target, version)
	visibilityBuilder.SetVisible(elsePoint, target, version)
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	handlersType := typetable.NewRecord().
		Field("__init", typ.Func().Build()).
		Build()
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(target), typeValue(reg, handlersType))

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchPathEvidence: map[cfg.Point]factflow.BranchPathEvidenceSet{
					branch: factflow.NewBranchPathEvidenceSet(
						factflow.NewBranchPathTruthyEvidenceWithOppositeOnEdge(initPath, true),
					),
				},
			}),
			Visibility:  resolver,
			ProjectPath: testLuaPathTypeProjector,
		}),
	})

	domain := state.Domain(reg)
	if domain.Equal(got[thenPoint], domain.Bottom()) {
		t.Fatal("true edge was killed, want reachable")
	}
	if !domain.Equal(got[elsePoint], domain.Bottom()) {
		t.Fatalf("false edge state = %#v, want unreachable bottom", got[elsePoint])
	}
}

func TestFactsEdgeTransferKillsUnreachableBranchEdge(t *testing.T) {
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

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: state.State{},
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchEdgeReachability: map[cfg.Point]factflow.BranchEdgeReachability{
					branch: factflow.NewBranchEdgeReachability(false, true),
				},
			}),
		}),
	})

	domain := state.Domain(reg)
	if domain.Equal(got[thenPoint], domain.Bottom()) {
		t.Fatal("true edge was killed, want reachable")
	}
	if !domain.Equal(got[elsePoint], domain.Bottom()) {
		t.Fatalf("false edge state = %#v, want unreachable bottom", got[elsePoint])
	}
}

func TestFactsEdgeTransferKillsDynamicallyFalseConditionEdge(t *testing.T) {
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

	source := factflow.NewNilValueSource(0)
	condition, _ := factflow.NewBranchCondition(source, true)
	trueValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.True), typ.True)
	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: state.State{},
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchConditionSources: map[cfg.Point]factflow.BranchCondition{
					branch: condition,
				},
			}),
			Sources: &recordingSourceValues{values: map[factflow.ValueSource]product.Value{
				source: trueValue,
			}},
		}),
	})

	domain := state.Domain(reg)
	if domain.Equal(got[thenPoint], domain.Bottom()) {
		t.Fatal("true edge was killed, want reachable")
	}
	if !domain.Equal(got[elsePoint], domain.Bottom()) {
		t.Fatalf("false edge state = %#v, want unreachable bottom", got[elsePoint])
	}
}

func TestFactsEdgeTransferFalsyAbsentDoesNotKillBooleanFalseEdge(t *testing.T) {
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

	target := symbol.ID(315)
	targetPath := pathdom.NewPath(target, "ok")
	boolType := typeexpr.Union(typ.True, typ.False)
	boolValue := typevalue.WithWitness(reg, typevalue.FromType(reg, boolType), boolType)
	absent := product.NewWithPresence(reg, product.ShapeTop, presence.Absent())

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: state.State{}.WriteValue(reg, key.SymbolValue(target), boolValue),
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						factflow.NewBranchRefinement(
							targetPath,
							factflow.NewValueConstraint(product.NewWithPresence(reg, product.ShapeTop, presence.Present())),
							true,
							factflow.NewFalsyAbsentConstraint(absent),
							true,
						),
					),
				},
			}),
		}),
	})

	if state.Domain(reg).Equal(got[elsePoint], state.Domain(reg).Bottom()) {
		t.Fatal("false edge was killed, but boolean false is a valid falsy value")
	}
	assertValue(t, reg, got[elsePoint], key.SymbolValue(target), boolValue)
}

func TestFactsEdgeTransferFalsyAbsentKeepsOptionalNonBooleanElseReachable(t *testing.T) {
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

	target := symbol.ID(316)
	targetPath := pathdom.NewPath(target, "err")
	errValue := product.WithPresence(
		reg,
		typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String),
		presence.Maybe(),
	)
	absent := product.NewWithPresence(reg, product.ShapeTop, presence.Absent())

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: state.State{}.WriteValue(reg, key.SymbolValue(target), errValue),
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						factflow.NewBranchRefinement(
							targetPath,
							factflow.NewValueConstraint(product.NewWithPresence(reg, product.ShapeTop, presence.Present())),
							true,
							factflow.NewFalsyAbsentConstraint(absent),
							true,
						),
					),
				},
			}),
		}),
	})

	if state.Domain(reg).Equal(got[elsePoint], state.Domain(reg).Bottom()) {
		t.Fatal("false edge was killed, but optional string nil arm is reachable")
	}
	refined := got[elsePoint].ReadValue(reg, key.SymbolValue(target))
	if gotPresence := product.PresenceOf(refined); !presence.Equal(gotPresence, presence.Absent()) {
		t.Fatalf("false-edge presence = %s (%s), want absent", gotPresence, formatValue(reg, refined))
	}
}

func TestFactsEdgeTransferFalsyAbsentKillsExactStringEdge(t *testing.T) {
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

	target := symbol.ID(317)
	targetPath := pathdom.NewPath(target, "choice")
	exact := typevalue.FromType(reg, typ.LiteralString("auto"))
	absent := product.NewWithPresence(reg, product.ShapeTop, presence.Absent())

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: state.State{}.WriteValue(reg, key.SymbolValue(target), exact),
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						factflow.NewBranchRefinement(
							targetPath,
							factflow.NewFalsyAbsentConstraint(absent),
							true,
							factflow.NewValueConstraint(product.NewWithPresence(reg, product.ShapeTop, presence.Present())),
							true,
						),
					),
				},
			}),
		}),
	})

	if !stateIsBottom(reg, got[thenPoint]) {
		t.Fatalf("true edge state = %v, want unreachable because exact string cannot be falsy", got[thenPoint])
	}
	if stateIsBottom(reg, got[elsePoint]) {
		t.Fatal("false edge unexpectedly bottom")
	}
	assertValue(t, reg, got[elsePoint], key.SymbolValue(target), exact)
}

func TestFactsEdgeTransferOneWayTruthyEvidenceDoesNotKillOppositeEdge(t *testing.T) {
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

	target := symbol.ID(318)
	targetPath := pathdom.NewPath(target, "choice")
	exact := typevalue.FromType(reg, typ.LiteralString("auto"))

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: state.State{}.WriteValue(reg, key.SymbolValue(target), exact),
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchPathEvidence: map[cfg.Point]factflow.BranchPathEvidenceSet{
					branch: factflow.NewBranchPathEvidenceSet(
						factflow.NewBranchPathTruthyEvidenceOnEdge(targetPath, false),
					),
				},
			}),
		}),
	})

	if stateIsBottom(reg, got[thenPoint]) {
		t.Fatal("true edge was killed by one-way truthy evidence from the false edge")
	}
	if stateIsBottom(reg, got[elsePoint]) {
		t.Fatal("false edge unexpectedly bottom")
	}
	assertValue(t, reg, got[thenPoint], key.SymbolValue(target), exact)
	assertValue(t, reg, got[elsePoint], key.SymbolValue(target), exact)
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

func TestFactsEdgeTransferAppliesFrozenTableEvidenceOnlyOnSelectedEdge(t *testing.T) {
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

	target := symbol.ID(318)
	tableID := testTableIdentity(9, 9)
	rootPath := pathdom.NewPath(target, "t")
	initial := state.State{}.WriteValue(reg, key.SymbolValue(target), product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(tableID)))

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchPathEvidence: map[cfg.Point]factflow.BranchPathEvidenceSet{
					branch: factflow.NewBranchPathEvidenceSet(
						factflow.NewBranchFrozenTableEvidenceOnEdge(rootPath, true),
					),
				},
			}),
		}),
	})

	if !got[thenPoint].IsTableFrozen(tableID) {
		t.Fatalf("then edge state is not frozen for %s", tableID)
	}
	if got[elsePoint].IsTableFrozen(tableID) {
		t.Fatalf("else edge state unexpectedly frozen for %s", tableID)
	}
}

func TestFactsEdgeTransferBareTableRootRefinementPreservesDescendantPathFacts(t *testing.T) {
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
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, target, "r")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(target), product.Top()).
		WritePathKey(reg, ks, childKey, staleChild)

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
			Visibility:  resolver,
			ProjectPath: testLuaPathTypeProjector,
		}),
	})

	assertRuntimeKind(t, reg, got[thenPoint].ReadValue(reg, key.SymbolValue(target)), runtimekind.Singleton(runtimekind.Table))
	assertPathValue(t, reg, ks, got[thenPoint], childKey, staleChild)
	assertPathValue(t, reg, ks, got[elsePoint], childKey, staleChild)
}

func TestFactsEdgeTransferRuntimeKindRootRefinementNarrowsUnionWitness(t *testing.T) {
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
	rootPath := pathdom.NewPath(target, "x")
	visibilityBuilder := visibility.NewBuilder()
	version := visibilityBuilder.Define(branch, target, "x")
	visibilityBuilder.SetVisible(thenPoint, target, version)
	visibilityBuilder.SetVisible(elsePoint, target, version)
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	mapType := typetable.NewMap(typ.String, typ.String)
	typeValues := typevalue.NewCache()
	valueType := typeexpr.Union(typ.String, mapType)
	initialValue := typeValues.FromTypeWithWitness(reg, valueType)
	initialValue = product.Set(reg, initialValue, identity.Key, identity.Singleton(testTableIdentity(1, 1)))
	initial := state.State{}.WriteValue(reg, key.SymbolValue(target), initialValue)

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						branchWithRuntimeKind(rootPath, runtimekind.Singleton(runtimekind.Table), true, runtimekind.Top().Without(runtimekind.Table), true),
					),
				},
			}),
			Visibility:  resolver,
			ProjectPath: testLuaPathTypeProjector,
			TypeValues:  typeValues,
		}),
	})

	if state.Domain(reg).Equal(got[thenPoint], state.Domain(reg).Bottom()) {
		t.Fatal("runtime-kind table branch collapsed compatible union witness to unreachable")
	}
	thenValue := got[thenPoint].ReadValue(reg, key.SymbolValue(target))
	thenType, ok := typevalue.TypeOf(reg, thenValue)
	if !ok || !typ.TypeEquals(thenType, mapType) {
		t.Fatalf("then type = %v/%v, want %v", thenType, ok, mapType)
	}
	elseValue := got[elsePoint].ReadValue(reg, key.SymbolValue(target))
	elseType, ok := typevalue.TypeOf(reg, elseValue)
	if !ok || !typ.TypeEquals(elseType, typ.String) {
		t.Fatalf("else type = %v/%v, want string", elseType, ok)
	}
}

func TestFactsEdgeTransferAppliesBranchDiffConstraintRelationGraphKeys(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	xs := symbol.ID(331)
	i := symbol.ID(332)
	j := symbol.ID(333)
	xsPath := pathdom.NewPath(xs, "xs")
	iPath := pathdom.NewPath(i, "i")
	jPath := pathdom.NewPath(j, "j")
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, xs, "xs")
	visibilityBuilder.Define(branch, i, "i")
	visibilityBuilder.Define(branch, j, "j")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	diff := factflow.NewBranchScaledConstraintOnEdge(1, iPath, false, 1, jPath, false, xsPath, true, 0, true)
	edgeTransfer := NewFactsEdgeTransfer(FactsEdgeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
				branch: factflow.NewBranchRefinementSet().WithDiffConstraints(diff),
			},
		}),
		Visibility: resolver,
	})

	trueOut := edgeTransfer(transfer.EdgeContext{
		Graph:    graph,
		Registry: reg,
		Edge:     cfg.Edge{From: branch, To: thenPoint, Cond: true},
		HasCond:  true,
	}, state.State{})
	falseOut := edgeTransfer(transfer.EdgeContext{
		Graph:    graph,
		Registry: reg,
		Edge:     cfg.Edge{From: branch, To: elsePoint, Cond: false},
		HasCond:  true,
	}, state.State{})

	xsKey, xsOK := visibility.RootOrVisibleStateKeyAt(resolver, branch, xsPath)
	iKey, iOK := visibility.RootOrVisibleStateKeyAt(resolver, branch, iPath)
	jKey, jOK := visibility.RootOrVisibleStateKeyAt(resolver, branch, jPath)
	if !xsOK || !iOK || !jOK {
		t.Fatalf("RootOrVisibleStateKeyAt failed for xs=%v i=%v j=%v", xsOK, iOK, jOK)
	}
	constraints := trueOut.RelConstraints().Constraints
	if len(constraints) != 1 {
		t.Fatalf("true-edge relational constraints = %#v, want one relation", constraints)
	}
	constraint := constraints[0]
	if constraint.CoA != 1 || constraint.CoB != 1 || constraint.K != 0 ||
		!((constraint.A == state.RelValueOperand(iKey) && constraint.B == state.RelValueOperand(jKey)) ||
			(constraint.A == state.RelValueOperand(jKey) && constraint.B == state.RelValueOperand(iKey))) ||
		constraint.C != state.RelLengthOperand(xsKey) {
		t.Fatalf("true-edge relation = %#v, want i+j-len(xs)<=0 under relation graph keys", constraint)
	}
	if constraints := falseOut.RelConstraints().Constraints; len(constraints) != 0 {
		t.Fatalf("false-edge relational constraints = %#v, want no true-edge relation leak", constraints)
	}
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
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, target, "r")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(target), product.Top()).
		WritePathKey(reg, ks, childKey, staleChild)

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
			Visibility: resolver,
		}),
	})

	assertRuntimeKind(t, reg, got[thenPoint].ReadValue(reg, key.SymbolValue(target)), runtimekind.Singleton(runtimekind.Table))
	assertRuntimeKind(t, reg, got[thenPoint].ReadPathKey(reg, ks, childKey), runtimekind.Singleton(runtimekind.Number))
	assertPathValue(t, reg, ks, got[elsePoint], childKey, staleChild)
}

func TestFactsEdgeTransferMetadataRootRefinementKeepsIndependentChildGuard(t *testing.T) {
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

	target := symbol.ID(331)
	rootPath := pathdom.NewPath(target, "raw")
	idPath := rootPath.Field("id")
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, target, "raw")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	idKey := resolver.KeyAt(branch, idPath)
	childGuard := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	rootMetadata := product.Set(reg, product.Top(), variantorigin.Key, variantorigin.Of(99, []int{1}))

	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		EntryState: state.State{}.
			WriteValue(reg, key.SymbolValue(target), product.Top()).
			WritePathKey(reg, ks, idKey, childGuard),
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						factflow.NewBranchRefinement(rootPath, factflow.NewValueConstraint(rootMetadata), true, factflow.ValueRefinement{}, false),
					),
				},
			}),
			Visibility: resolver,
		}),
	})

	assertRuntimeKind(t, reg, got[thenPoint].ReadPathKey(reg, ks, idKey), runtimekind.Singleton(runtimekind.String))
	assertPathValue(t, reg, ks, got[elsePoint], idKey, childGuard)
}

func TestFactsEdgeTransferMetadataRootRefinementDropsExplicitAnyChildFact(t *testing.T) {
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

	target := symbol.ID(332)
	rootPath := pathdom.NewPath(target, "raw")
	idPath := rootPath.Field("id")
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, target, "raw")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	idKey := resolver.KeyAt(branch, idPath)
	taintedChild := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	taintedChild = product.Set(reg, taintedChild, evidence.Key, evidence.ExplicitTop())
	rootMetadata := product.Set(reg, product.Top(), variantorigin.Key, variantorigin.Of(99, []int{1}))

	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		EntryState: state.State{}.
			WriteValue(reg, key.SymbolValue(target), product.Top()).
			WritePathKey(reg, ks, idKey, taintedChild),
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						factflow.NewBranchRefinement(rootPath, factflow.NewValueConstraint(rootMetadata), true, factflow.ValueRefinement{}, false),
					),
				},
			}),
			Visibility: resolver,
		}),
	})

	assertPathValue(t, reg, ks, got[thenPoint], idKey, product.Bottom(reg))
	assertPathValue(t, reg, ks, got[elsePoint], idKey, taintedChild)
}

func TestFactsEdgeTransferScalarChildRuntimeGuardClearsExplicitAnyEvidence(t *testing.T) {
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

	root := symbol.ID(333)
	kindPath := pathdom.NewPath(root, "raw").Field("kind")
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, root, "raw")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	kindKey := resolver.KeyAt(branch, kindPath)
	taintedChild := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	taintedChild = product.Set(reg, taintedChild, evidence.Key, evidence.ExplicitTop())

	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		EntryState: state.State{}.
			WriteValue(reg, key.SymbolValue(root), product.Set(reg, product.Top(), evidence.Key, evidence.ExplicitTop())).
			WritePathKey(reg, ks, kindKey, taintedChild),
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						factflow.NewBranchRefinement(kindPath, typenarrow.MatchRefinement(reg, runtimekind.String), true, factflow.ValueRefinement{}, false),
					),
				},
			}),
			Visibility: resolver,
		}),
	})

	gotChild := got[thenPoint].ReadPathKey(reg, ks, kindKey)
	assertRuntimeKind(t, reg, gotChild, runtimekind.Singleton(runtimekind.String))
	if gotEvidence := product.Get(reg, gotChild, evidence.Key); !evidence.Equal(gotEvidence, evidence.Top()) {
		t.Fatalf("then child evidence = %s in %s, want trusted top from scalar runtime guard", gotEvidence, formatValue(reg, gotChild))
	}
	assertPathValue(t, reg, ks, got[elsePoint], kindKey, taintedChild)
}

func TestFactsEdgeTransferTableChildRuntimeGuardPreservesExplicitAnyRootEvidence(t *testing.T) {
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

	root := symbol.ID(334)
	itemsPath := pathdom.NewPath(root, "block").Field("items")
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, root, "block")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	itemsKey := resolver.KeyAt(branch, itemsPath)
	child := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Table))

	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		EntryState: state.State{}.
			WriteValue(reg, key.SymbolValue(root), product.Set(reg, product.Top(), evidence.Key, evidence.ExplicitTop())).
			WritePathKey(reg, ks, itemsKey, child),
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						factflow.NewBranchRefinement(itemsPath, typenarrow.MatchRefinement(reg, runtimekind.Table), true, factflow.ValueRefinement{}, false),
					),
				},
			}),
			Visibility: resolver,
		}),
	})

	gotChild := got[thenPoint].ReadPathKey(reg, ks, itemsKey)
	assertRuntimeKind(t, reg, gotChild, runtimekind.Singleton(runtimekind.Table))
	if gotEvidence := product.Get(reg, gotChild, evidence.Key); !evidence.Equal(gotEvidence, evidence.ExplicitTop()) {
		t.Fatalf("then child evidence = %s in %s, want explicit-any origin preserved for broad table guard", gotEvidence, formatValue(reg, gotChild))
	}
	assertPathValue(t, reg, ks, got[elsePoint], itemsKey, child)
}

func TestFactsEdgeTransferDescendantTableRefinementWritesStaticWitness(t *testing.T) {
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

	target := symbol.ID(331)
	rootPath := pathdom.NewPath(target, "bindings")
	childPath := rootPath.Field("checkpoint")
	visibilityBuilder := visibility.NewBuilder()
	version := visibilityBuilder.Define(branch, target, "bindings")
	visibilityBuilder.SetVisible(thenPoint, target, version)
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	childKey := resolver.KeyForVersion(target, version.ID, childPath.Segments)
	tableValue := typeValue(reg, typetable.BuiltinTopMarker())

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: state.State{}.WriteValue(reg, key.SymbolValue(target), product.Top()),
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						factflow.NewBranchRefinement(
							childPath,
							factflow.NewValueConstraint(tableValue),
							true,
							factflow.ValueRefinement{},
							false,
						),
					),
				},
			}),
			Visibility: resolver,
		}),
	})

	gotValue := got[thenPoint].ReadPathKey(reg, ks, childKey)
	gotType, ok := typevalue.TypeOf(reg, gotValue)
	if !ok || !typ.TypeEquals(gotType, typetable.BuiltinTopMarker()) {
		t.Fatalf("descendant table refinement type = %v/%v in %s, want builtin table marker", gotType, ok, formatValue(reg, gotValue))
	}
}

func TestFactsEdgeTransferDescendantTruthyNarrowsRootOriginFromFlowType(t *testing.T) {
	reg := standard.Registry()
	profile := typetable.NewRecord().
		Field("id", typ.String).
		Field("name", typ.String).
		Build()
	resultType, valueCase, _ := resultTypeFixture(profile)

	tests := []struct {
		name      string
		rootValue product.Value
	}{
		{
			name:      "type witness",
			rootValue: typevalue.WithWitness(reg, typevalue.FromType(reg, resultType), resultType),
		},
		{
			name:      "variant origin",
			rootValue: typevalue.FromType(reg, resultType),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			graph := cfg.New()
			branch := graph.AddNode(cfg.NodeBranch)
			thenPoint := graph.AddNode(cfg.NodeNoop)
			elsePoint := graph.AddNode(cfg.NodeNoop)
			graph.AddEdge(graph.Entry(), branch, false)
			graph.AddEdge(branch, thenPoint, true)
			graph.AddEdge(branch, elsePoint, false)
			graph.AddEdge(thenPoint, graph.Exit(), false)
			graph.AddEdge(elsePoint, graph.Exit(), false)

			target := symbol.ID(331)
			rootPath := pathdom.NewPath(target, "result")
			okPath := rootPath.Field("ok")
			valuePath := rootPath.Field("value")
			visibilityBuilder := visibility.NewBuilder()
			version := visibilityBuilder.Define(branch, target, "result")
			visibilityBuilder.SetVisible(thenPoint, target, version)
			visibilityBuilder.SetVisible(elsePoint, target, version)
			resolver := visibility.NewResolver(visibilityBuilder.Build())
			ks := resolver.KeySpace()
			okKey := resolver.KeyForVersion(target, version.ID, okPath.Segments)
			valueKey := resolver.KeyForVersion(target, version.ID, valuePath.Segments)
			staleValue := typevalue.FromType(reg, typeexpr.Optional(profile))
			facts := factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						branchWithPresence(okPath, presence.Present(), true, presence.Bottom(), false),
					),
				},
			})
			initial := state.State{}.
				WriteValue(reg, key.SymbolValue(target), tc.rootValue).
				WritePathKey(reg, ks, valueKey, staleValue)

			got := transfer.Run(transfer.Config{
				Graph:      graph,
				Registry:   reg,
				EntryState: initial,
				EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
					Facts:       facts,
					Visibility:  resolver,
					ProjectPath: testLuaPathTypeProjector,
				}),
			})

			thenState := got[thenPoint]
			assertVariantOriginType(t, reg, thenState, target, resultType, valueCase)
			resolved, ok := resolvePathValueAt(reg, resolver, thenPoint, thenState, valuePath, testLuaPathTypeProjector)
			if !ok {
				t.Fatal("result.value did not resolve after result.ok guard")
			}
			gotType, ok := typevalue.TypeOf(reg, resolved.value)
			if !ok || !typ.TypeEquals(gotType, profile) {
				t.Fatalf("result.value type = %v/%v, want %v", gotType, ok, profile)
			}
			assertPathValue(t, reg, ks, thenState, valueKey, product.Bottom(reg))
			assertPathPresence(t, reg, ks, thenState, okKey, presence.Present())
			assertPathValue(t, reg, ks, got[elsePoint], valueKey, staleValue)
		})
	}
}

func TestFactsEdgeTransferDescendantTruthyNarrowsOptionalUnionRootByPresentField(t *testing.T) {
	reg := standard.Registry()
	typeValues := typevalue.NewCache()
	template := typetable.NewRecord().
		Field("kind", typ.LiteralString("template")).
		Field("id", typ.String).
		Field("data_func", typeexpr.Optional(typ.String)).
		Field("template_set", typ.String).
		Build()
	component := typetable.NewRecord().
		Field("kind", typ.LiteralString("component")).
		Field("id", typ.String).
		Field("url", typ.String).
		Build()
	pageType := typeexpr.Optional(typeexpr.Union(template, component))

	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	target := symbol.ID(334)
	rootPath := pathdom.NewPath(target, "page")
	dataFuncPath := rootPath.Field("data_func")
	visibilityBuilder := visibility.NewBuilder()
	version := visibilityBuilder.Define(branch, target, "page")
	visibilityBuilder.SetVisible(thenPoint, target, version)
	visibilityBuilder.SetVisible(elsePoint, target, version)
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	dataFuncKey := resolver.KeyForVersion(target, version.ID, dataFuncPath.Segments)
	staleDataFunc := typeValues.FromType(reg, typeexpr.Optional(typ.String))
	facts := factflow.NewFacts(factflow.FactsInput{
		BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
			branch: factflow.NewBranchRefinementSet(
				branchWithPresence(rootPath, presence.Present(), true, presence.Absent(), true),
				branchWithPresence(dataFuncPath, presence.Present(), true, presence.Absent(), true),
			),
		},
	})
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(target), typeValues.FromTypeWithWitness(reg, pageType)).
		WritePathKey(reg, ks, dataFuncKey, staleDataFunc)

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts:       facts,
			Visibility:  resolver,
			ProjectPath: testLuaPathTypeProjector,
			TypeValues:  typeValues,
		}),
	})

	thenState := got[thenPoint]
	assertVariantOriginType(t, reg, thenState, target, pageType, template)
	resolved, ok := resolvePathValueAt(reg, resolver, thenPoint, thenState, dataFuncPath, testLuaPathTypeProjector)
	if !ok {
		t.Fatal("page.data_func did not resolve after page.data_func guard")
	}
	gotType, ok := typevalue.TypeOf(reg, resolved.value)
	if !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("page.data_func type = %v/%v in %s, want string", gotType, ok, formatValue(reg, resolved.value))
	}
	assertPathPresence(t, reg, ks, thenState, dataFuncKey, presence.Present())
}

func TestFactsEdgeTransferTypedRootRefinementPreservesCompatibleDescendantPresence(t *testing.T) {
	reg := standard.Registry()
	typeValues := typevalue.NewCache()
	template := typetable.NewRecord().
		Field("kind", typ.LiteralString("template")).
		Field("id", typ.String).
		Field("data_func", typeexpr.Optional(typ.String)).
		Field("template_set", typ.String).
		Build()
	component := typetable.NewRecord().
		Field("kind", typ.LiteralString("component")).
		Field("id", typ.String).
		Field("url", typ.String).
		Build()
	pageType := typeexpr.Optional(typeexpr.Union(template, component))

	graph := cfg.New()
	presentBranch := graph.AddNode(cfg.NodeBranch)
	rootNarrowBranch := graph.AddNode(cfg.NodeBranch)
	after := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), presentBranch, false)
	graph.AddEdge(presentBranch, graph.Exit(), true)
	graph.AddEdge(presentBranch, rootNarrowBranch, false)
	graph.AddEdge(rootNarrowBranch, graph.Exit(), true)
	graph.AddEdge(rootNarrowBranch, after, false)
	graph.AddEdge(after, graph.Exit(), false)

	target := symbol.ID(335)
	rootPath := pathdom.NewPath(target, "page")
	dataFuncPath := rootPath.Field("data_func")
	visibilityBuilder := visibility.NewBuilder()
	version := visibilityBuilder.Define(presentBranch, target, "page")
	visibilityBuilder.SetVisible(rootNarrowBranch, target, version)
	visibilityBuilder.SetVisible(after, target, version)
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	dataFuncKey := resolver.KeyForVersion(target, version.ID, dataFuncPath.Segments)
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(target), typeValues.FromTypeWithWitness(reg, pageType)).
		WritePathKey(reg, ks, dataFuncKey, typeValues.FromType(reg, typeexpr.Optional(typ.String)))

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					presentBranch: factflow.NewBranchRefinementSet(
						branchWithPresence(dataFuncPath, presence.Bottom(), false, presence.Present(), true),
					),
					rootNarrowBranch: factflow.NewBranchRefinementSet(
						factflow.NewBranchRefinement(
							rootPath,
							factflow.ValueRefinement{}, false,
							factflow.NewValueConstraint(typeValues.FromTypeWithWitness(reg, template)), true,
						),
					),
				},
			}),
			Visibility:  resolver,
			ProjectPath: testLuaPathTypeProjector,
			TypeValues:  typeValues,
		}),
	})

	assertPathPresence(t, reg, ks, got[after], dataFuncKey, presence.Present())
	resolved, ok := resolvePathValueAt(reg, resolver, after, got[after], dataFuncPath, testLuaPathTypeProjector)
	if !ok {
		t.Fatal("page.data_func did not resolve after compatible root narrowing")
	}
	gotType, ok := typevalue.TypeOf(reg, resolved.value)
	if !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("page.data_func type = %v/%v in %s, want string", gotType, ok, formatValue(reg, resolved.value))
	}
}

func TestFactsEdgeTransferDescendantLiteralRootNarrowingPreservesCompatibleDescendantPresence(t *testing.T) {
	reg := standard.Registry()
	typeValues := typevalue.NewCache()
	template := typetable.NewRecord().
		Field("kind", typ.LiteralString("template")).
		Field("id", typ.String).
		Field("data_func", typeexpr.Optional(typ.String)).
		Field("template_set", typ.String).
		Build()
	component := typetable.NewRecord().
		Field("kind", typ.LiteralString("component")).
		Field("id", typ.String).
		Field("url", typ.String).
		Build()
	pageType := typeexpr.Optional(typeexpr.Union(template, component))

	graph := cfg.New()
	presentBranch := graph.AddNode(cfg.NodeBranch)
	literalBranch := graph.AddNode(cfg.NodeBranch)
	after := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), presentBranch, false)
	graph.AddEdge(presentBranch, graph.Exit(), true)
	graph.AddEdge(presentBranch, literalBranch, false)
	graph.AddEdge(literalBranch, graph.Exit(), true)
	graph.AddEdge(literalBranch, after, false)
	graph.AddEdge(after, graph.Exit(), false)

	target := symbol.ID(336)
	rootPath := pathdom.NewPath(target, "page")
	dataFuncPath := rootPath.Field("data_func")
	visibilityBuilder := visibility.NewBuilder()
	version := visibilityBuilder.Define(presentBranch, target, "page")
	visibilityBuilder.SetVisible(literalBranch, target, version)
	visibilityBuilder.SetVisible(after, target, version)
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	dataFuncKey := resolver.KeyForVersion(target, version.ID, dataFuncPath.Segments)
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(target), typeValues.FromTypeWithWitness(reg, pageType)).
		WritePathKey(reg, ks, dataFuncKey, typeValues.FromType(reg, typeexpr.Optional(typ.String)))
	notEmpty := factflow.NewNegatedLiteralConstraint(typeValues.FromTypeWithWitness(reg, typ.LiteralString("")))

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					presentBranch: factflow.NewBranchRefinementSet(
						branchWithPresence(dataFuncPath, presence.Bottom(), false, presence.Present(), true),
					),
					literalBranch: factflow.NewBranchRefinementSet(
						factflow.NewBranchRefinement(dataFuncPath, factflow.ValueRefinement{}, false, notEmpty, true),
					),
				},
			}),
			Visibility:  resolver,
			ProjectPath: testLuaPathTypeProjector,
			TypeValues:  typeValues,
		}),
	})

	assertPathPresence(t, reg, ks, got[after], dataFuncKey, presence.Present())
	resolved, ok := resolvePathValueAt(reg, resolver, after, got[after], dataFuncPath, testLuaPathTypeProjector)
	if !ok {
		t.Fatal("page.data_func did not resolve after descendant literal root narrowing")
	}
	gotType, ok := typevalue.TypeOf(reg, resolved.value)
	if !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("page.data_func type = %v/%v in %s, want string", gotType, ok, formatValue(reg, resolved.value))
	}
}

func TestFactsEdgeTransferDescendantFalsyNarrowsRootOriginFromFlowType(t *testing.T) {
	reg := standard.Registry()
	profile := typetable.NewRecord().
		Field("id", typ.String).
		Field("name", typ.String).
		Build()
	resultType, _, errorCase := resultTypeFixture(profile)

	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	target := symbol.ID(332)
	rootPath := pathdom.NewPath(target, "result")
	okPath := rootPath.Field("ok")
	errorPath := rootPath.Field("error")
	visibilityBuilder := visibility.NewBuilder()
	version := visibilityBuilder.Define(branch, target, "result")
	visibilityBuilder.SetVisible(thenPoint, target, version)
	visibilityBuilder.SetVisible(elsePoint, target, version)
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	errorKey := resolver.KeyForVersion(target, version.ID, errorPath.Segments)
	staleError := typevalue.FromType(reg, typeexpr.Optional(typ.String))
	falseLiteral := typ.LiteralBool(false)
	falseRefinement := factflow.NewValueConstraint(typevalue.WithWitness(reg, typevalue.FromType(reg, falseLiteral), falseLiteral))
	facts := factflow.NewFacts(factflow.FactsInput{
		BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
			branch: factflow.NewBranchRefinementSet(
				factflow.NewBranchRefinement(okPath, falseRefinement, true, factflow.ValueRefinement{}, false),
			),
		},
	})
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(target), typevalue.WithWitness(reg, typevalue.FromType(reg, resultType), resultType)).
		WritePathKey(reg, ks, errorKey, staleError)

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts:      facts,
			Visibility: resolver,
		}),
	})

	thenState := got[thenPoint]
	assertVariantOriginType(t, reg, thenState, target, resultType, errorCase)
	assertPathValue(t, reg, ks, thenState, errorKey, product.Bottom(reg))
	assertPathValue(t, reg, ks, got[elsePoint], errorKey, staleError)
}

func TestFactsEdgeTransferDescendantTruthyFalseEdgeNarrowsRootOriginFromFlowType(t *testing.T) {
	reg := standard.Registry()
	profile := typetable.NewRecord().
		Field("id", typ.String).
		Field("name", typ.String).
		Build()
	resultType, _, errorCase := resultTypeFixture(profile)

	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	target := symbol.ID(333)
	rootPath := pathdom.NewPath(target, "result")
	okPath := rootPath.Field("ok")
	errorPath := rootPath.Field("error")
	visibilityBuilder := visibility.NewBuilder()
	version := visibilityBuilder.Define(branch, target, "result")
	visibilityBuilder.SetVisible(thenPoint, target, version)
	visibilityBuilder.SetVisible(elsePoint, target, version)
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	errorKey := resolver.KeyForVersion(target, version.ID, errorPath.Segments)
	staleError := typevalue.FromType(reg, typeexpr.Optional(typ.String))
	facts := factflow.NewFacts(factflow.FactsInput{
		BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
			branch: factflow.NewBranchRefinementSet(
				branchWithPresence(okPath, presence.Present(), true, presence.Bottom(), false),
			),
		},
		BranchPathEvidence: map[cfg.Point]factflow.BranchPathEvidenceSet{
			branch: factflow.NewBranchPathEvidenceSet(
				factflow.NewBranchPathTruthyEvidenceWithOppositeOnEdge(okPath, true),
			),
		},
	})
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(target), typevalue.WithWitness(reg, typevalue.FromType(reg, resultType), resultType)).
		WritePathKey(reg, ks, errorKey, staleError)

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts:      facts,
			Visibility: resolver,
		}),
	})

	assertVariantOriginType(t, reg, got[elsePoint], target, resultType, errorCase)
	assertPathValue(t, reg, ks, got[elsePoint], errorKey, product.Bottom(reg))
	assertPathValue(t, reg, ks, got[thenPoint], errorKey, product.Bottom(reg))
}

func TestFactsEdgeTransferDescendantLiteralRefinesExactPathWithoutVariantOrigin(t *testing.T) {
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

	target := symbol.ID(334)
	rootPath := pathdom.NewPath(target, "item")
	kindPath := rootPath.Field("kind")
	visibilityBuilder := visibility.NewBuilder()
	version := visibilityBuilder.Define(branch, target, "item")
	visibilityBuilder.SetVisible(thenPoint, target, version)
	visibilityBuilder.SetVisible(elsePoint, target, version)
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	kindKey := resolver.KeyForVersion(target, version.ID, kindPath.Segments)
	lit := typ.LiteralString("ready")
	refinement := factflow.NewValueConstraint(typeValue(reg, lit))
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(target), typeValue(reg,
			typetable.NewRecord().Field("kind", typ.String).Build()))

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						factflow.NewBranchRefinement(kindPath, refinement, true, factflow.ValueRefinement{}, false),
					),
				},
			}),
			Visibility: resolver,
		}),
	})

	assertPathValue(t, reg, ks, got[thenPoint], kindKey, typeValue(reg, lit))
	assertPathValue(t, reg, ks, got[elsePoint], kindKey, product.Bottom(reg))
}

func TestFactsEdgeTransferDescendantLiteralContradictionMakesEdgeUnreachable(t *testing.T) {
	reg := standard.Registry()
	typeValues := typevalue.NewCache()
	aCase := typetable.NewRecord().
		Field("tag", typ.LiteralString("a")).
		Field("value", typ.String).
		Build()
	bCase := typetable.NewRecord().
		Field("tag", typ.LiteralString("b")).
		Field("value", typ.Number).
		Build()
	union := typeexpr.Union(aCase, bCase)
	family, _, ok := variant.OriginOfType(union)
	if !ok {
		t.Fatal("union has no variant origin")
	}
	aOriginCase := mustOriginCaseIndex(t, family, aCase)

	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	target := symbol.ID(335)
	rootPath := pathdom.NewPath(target, "r")
	tagPath := rootPath.Field("tag")
	valuePath := rootPath.Field("value")
	visibilityBuilder := visibility.NewBuilder()
	version := visibilityBuilder.Define(branch, target, "r")
	visibilityBuilder.SetVisible(thenPoint, target, version)
	visibilityBuilder.SetVisible(elsePoint, target, version)
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	valueKey := resolver.KeyForVersion(target, version.ID, valuePath.Segments)
	aValue := product.Set(
		reg,
		typeValues.FromTypeWithWitness(reg, aCase),
		variantorigin.Key,
		variantorigin.Singleton(family, aOriginCase),
	)
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(target), aValue).
		WritePathKey(reg, ks, valueKey, typeValues.FromTypeWithWitness(reg, typ.LiteralString("x")))
	litA := typeValues.FromTypeWithWitness(reg, typ.LiteralString("a"))
	notA := factflow.NewNegatedLiteralConstraint(litA)

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						factflow.NewBranchRefinement(tagPath, factflow.NewValueConstraint(litA), true, notA, true),
					),
				},
			}),
			Visibility:  resolver,
			ProjectPath: testLuaPathTypeProjector,
			TypeValues:  typeValues,
		}),
	})

	assertVariantOriginType(t, reg, got[thenPoint], target, union, aCase)
	if !stateIsBottom(reg, got[elsePoint]) {
		t.Fatalf("else state = %v, want unreachable because proven a-tag value cannot satisfy tag ~= \"a\"", got[elsePoint])
	}
}

func TestFactsEdgeTransferPresentAliasLiteralRefinesOptionalUnionDescendant(t *testing.T) {
	reg := standard.Registry()
	text := typetable.NewRecord().
		Field("kind", typ.LiteralString("text")).
		Field("value", typ.String).
		Build()
	group := typetable.NewRecord().
		Field("kind", typ.LiteralString("group")).
		Field("children", typ.NewArray(typ.Unknown)).
		Build()
	union := typeexpr.Union(text, group)
	optionalUnion := typeexpr.Optional(union)
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	first := symbol.ID(380)
	firstPath := pathdom.NewPath(first, "first")
	kindPath := firstPath.Field("kind")
	valuePath := firstPath.Field("value")
	visibilityBuilder := visibility.NewBuilder()
	version := visibilityBuilder.Define(branch, first, "first")
	visibilityBuilder.SetVisible(thenPoint, first, version)
	visibilityBuilder.SetVisible(elsePoint, first, version)
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	lit := typ.LiteralString("text")
	litValue := typevalue.WithWitness(reg, typevalue.FromType(reg, lit), lit)
	facts := factflow.NewFacts(factflow.FactsInput{
		BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
			branch: factflow.NewBranchRefinementSet(
				branchWithPresence(firstPath, presence.Present(), true, presence.Bottom(), false),
				factflow.NewBranchRefinement(kindPath, factflow.NewValueConstraint(litValue), true, factflow.ValueRefinement{}, false),
			),
		},
	})
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(first), typevalue.WithWitness(reg, typevalue.FromType(reg, optionalUnion), optionalUnion))

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts:      facts,
			Visibility: resolver,
		}),
	})

	root := got[thenPoint].ReadValue(reg, key.SymbolValue(first))
	if gotPresence := product.PresenceOf(root); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("first presence = %s in %s, want present", gotPresence, formatValue(reg, root))
	}
	resolved, ok := resolvePathValueAt(reg, resolver, thenPoint, got[thenPoint], valuePath, testLuaPathTypeProjector)
	if !ok {
		t.Fatal("first.value did not resolve")
	}
	gotType, ok := typevalue.TypeOf(reg, resolved.value)
	if !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("first.value type = %v/%v, want string", gotType, ok)
	}
}

func TestFactsEdgeTransferDescendantLiteralNarrowsRootOriginFromFlowType(t *testing.T) {
	reg := standard.Registry()
	box := typetable.NewRecord().
		Field("kind", typ.LiteralString("box")).
		Field("node", typetable.NewRecord().
			Field("left", typ.String).
			Field("right", typ.Number).
			Build()).
		Build()
	stream := typetable.NewRecord().
		Field("kind", typ.LiteralString("stream")).
		Field("router", typ.String).
		Build()
	rootType := typeexpr.Union(box, stream)

	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	target := symbol.ID(335)
	rootPath := pathdom.NewPath(target, "payload")
	kindPath := rootPath.Field("kind")
	visibilityBuilder := visibility.NewBuilder()
	version := visibilityBuilder.Define(branch, target, "payload")
	visibilityBuilder.SetVisible(thenPoint, target, version)
	visibilityBuilder.SetVisible(elsePoint, target, version)
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	lit := typ.LiteralString("box")
	refinement := factflow.NewValueConstraint(typeValue(reg, lit))
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(target), typeValue(reg, rootType))

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						factflow.NewBranchRefinement(kindPath, refinement, true, factflow.ValueRefinement{}, false),
					),
				},
			}),
			Visibility: resolver,
		}),
	})

	assertVariantOriginType(t, reg, got[thenPoint], target, rootType, box)
	assertPathValue(t, reg, ks, got[thenPoint], resolver.KeyForVersion(target, version.ID, rootPath.Field("node").Field("left").Segments), product.Bottom(reg))
}

func TestFactsEdgeTransferDescendantLiteralDoesNotCloseUntrustedTopRoot(t *testing.T) {
	reg := standard.Registry()
	image := typetable.NewRecord().
		Field("type", typ.LiteralString("image")).
		Build()
	call := typetable.NewRecord().
		Field("type", typ.LiteralString("function_call")).
		Field("arguments", typ.String).
		Build()
	rootType := typeexpr.Union(image, call)

	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(thenPoint, graph.Exit(), false)

	target := symbol.ID(336)
	rootPath := pathdom.NewPath(target, "payload")
	typePath := rootPath.Field("type")
	visibilityBuilder := visibility.NewBuilder()
	version := visibilityBuilder.Define(branch, target, "payload")
	visibilityBuilder.SetVisible(thenPoint, target, version)
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	lit := typ.LiteralString("image")
	refinement := factflow.NewValueConstraint(typeValue(reg, lit))
	rootValue := typeValue(reg, rootType)
	rootValue = product.Set(reg, rootValue, evidence.Key, evidence.ExplicitTop())
	initial := state.State{}.WriteValue(reg, key.SymbolValue(target), rootValue)

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						factflow.NewBranchRefinement(typePath, refinement, true, factflow.ValueRefinement{}, false),
					),
				},
			}),
			Visibility: resolver,
		}),
	})

	root := got[thenPoint].ReadValue(reg, key.SymbolValue(target))
	if gotEvidence := product.Get(reg, root, evidence.Key); !evidence.Equal(gotEvidence, evidence.ExplicitTop()) {
		t.Fatalf("root evidence = %s in %s, want explicit top", gotEvidence, formatValue(reg, root))
	}
	gotType, ok := typevalue.TypeOf(reg, root)
	if !ok || !typ.TypeEquals(gotType, rootType) {
		t.Fatalf("root type = %v/%v in %s, want broad union %s", gotType, ok, formatValue(reg, root), rootType)
	}
}

func TestFactsEdgeTransferDescendantTruthyPreservesHeapRootIdentity(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(thenPoint, graph.Exit(), false)

	batch := symbol.ID(1337)
	rootPath := pathdom.NewPath(batch, "batch")
	itemPath := rootPath.Field("items").IndexStr("route-1")
	batchID := identity.ID{Kind: "lua.table", Site: "branch-refinement", Index: 1}
	itemsID := identity.ID{Kind: "lua.table", Site: "branch-refinement", Index: 2}
	batchType := typetable.NewRecord().
		Field("items", typetable.NewMap(typ.String, typ.String)).
		Build()
	batchValue := product.Set(reg, typeValue(reg, batchType), identity.Key, identity.Singleton(batchID))
	itemsValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(itemsID))
	visibilityBuilder := visibility.NewBuilder()
	version := visibilityBuilder.Define(branch, batch, "batch")
	visibilityBuilder.SetVisible(thenPoint, batch, version)
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	itemsMemberKey, ok := heapidentity.StaticMemberSuffixKey(ks, fieldSuffix("items").Segments)
	if !ok {
		t.Fatal("missing items suffix key")
	}
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(batch), batchValue).
		WritePlacement(batchID, placement.Stack).
		WritePlacement(itemsID, placement.Stack).
		WriteHeapTableObject(reg, batchID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root:          batchValue,
			StaticMembers: map[keyspace.Key]product.Value{itemsMemberKey: itemsValue},
		})).
		WriteHeapTableObject(reg, itemsID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: itemsValue}))

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						factflow.NewBranchRefinement(itemPath, factflow.NewValueConstraint(product.NewWithPresence(reg, product.ShapeTop, presence.Present())), true, factflow.ValueRefinement{}, false),
					),
				},
			}),
			Visibility: resolver,
		}),
	})

	root := got[thenPoint].ReadValue(reg, key.SymbolValue(batch))
	if gotID, ok := product.Get(reg, root, identity.Key).ID(); !ok || gotID != batchID {
		t.Fatalf("root identity = %v/%v, want %v in %s", gotID, ok, batchID, formatValue(reg, root))
	}
	if gotPlacement := got[thenPoint].ReadPlacement(batchID); gotPlacement != placement.Stack {
		t.Fatalf("batch placement = %s, want stack", gotPlacement)
	}
	if gotPlacement := got[thenPoint].ReadPlacement(itemsID); gotPlacement != placement.Stack {
		t.Fatalf("items placement = %s, want stack", gotPlacement)
	}
}

func TestFactsEdgeTransferDescendantLiteralContradictsSpecializedRecord(t *testing.T) {
	reg := standard.Registry()
	typeValues := typevalue.NewCache()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	cell := symbol.ID(337)
	cellPath := pathdom.NewPath(cell, "cell")
	kindPath := cellPath.Field("kind")
	textCell := typetable.NewRecord().
		Field("kind", typ.LiteralString("string")).
		Field("raw", typ.String).
		Build()
	visibilityBuilder := visibility.NewBuilder()
	version := visibilityBuilder.Define(branch, cell, "cell")
	visibilityBuilder.SetVisible(thenPoint, cell, version)
	visibilityBuilder.SetVisible(elsePoint, cell, version)
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(cell), typeValues.FromTypeWithWitness(reg, textCell))

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					branch: factflow.NewBranchRefinementSet(
						factflow.NewBranchRefinement(kindPath, factflow.NewValueConstraint(typeValues.FromTypeWithWitness(reg, typ.LiteralString("boolean"))), true, factflow.ValueRefinement{}, false),
					),
				},
			}),
			Visibility:  resolver,
			ProjectPath: testLuaPathTypeProjector,
			TypeValues:  typeValues,
		}),
	})

	if !stateIsBottom(reg, got[thenPoint]) {
		t.Fatalf("then state = %v, want unreachable because specialized cell.kind is string", got[thenPoint])
	}
	if stateIsBottom(reg, got[elsePoint]) {
		t.Fatal("else state unexpectedly bottom")
	}
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
	trueRefinement := factflow.NewValueConstraint(constraint)
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

func resultTypeFixture(payload typ.Type) (typ.Type, typ.Type, typ.Type) {
	tp := typ.NewTypeParam("T", nil)
	result := typ.NewGeneric("Result", []*typ.TypeParam{tp}, typeexpr.Union(
		typetable.NewRecord().
			Field("ok", typ.LiteralBool(true)).
			Field("value", tp).
			Build(),
		typetable.NewRecord().
			Field("ok", typ.LiteralBool(false)).
			Field("error", typ.String).
			Build(),
	))
	valueCase := typetable.NewRecord().
		Field("ok", typ.LiteralBool(true)).
		Field("value", payload).
		Build()
	errorCase := typetable.NewRecord().
		Field("ok", typ.LiteralBool(false)).
		Field("error", typ.String).
		Build()
	return typ.Instantiate(result, payload), valueCase, errorCase
}
