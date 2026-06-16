package factapply

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
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
				WritePathKey(reg, valueKey, staleValue)

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
			assertPathValue(t, reg, thenState, valueKey, product.Bottom(reg))
			assertPathPresence(t, reg, thenState, okKey, presence.Present())
			assertPathValue(t, reg, got[elsePoint], valueKey, staleValue)
		})
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
		WritePathKey(reg, errorKey, staleError)

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
	assertPathValue(t, reg, thenState, errorKey, product.Bottom(reg))
	assertPathValue(t, reg, got[elsePoint], errorKey, staleError)
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
				factflow.NewBranchPathTruthyEvidenceOnEdge(okPath, true),
			),
		},
	})
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(target), typevalue.WithWitness(reg, typevalue.FromType(reg, resultType), resultType)).
		WritePathKey(reg, errorKey, staleError)

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
	assertPathValue(t, reg, got[elsePoint], errorKey, product.Bottom(reg))
	assertPathValue(t, reg, got[thenPoint], errorKey, product.Bottom(reg))
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

	assertPathValue(t, reg, got[thenPoint], kindKey, typeValue(reg, lit))
	assertPathValue(t, reg, got[elsePoint], kindKey, product.Bottom(reg))
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
	assertPathValue(t, reg, got[thenPoint], resolver.KeyForVersion(target, version.ID, rootPath.Field("node").Field("left").Segments), product.Bottom(reg))
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
