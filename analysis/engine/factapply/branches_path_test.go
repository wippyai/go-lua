package factapply

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

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
	union := typeexpr.Union(intCase, strCase)

	result := symbol.ID(325)
	ch1 := symbol.ID(326)
	resultPath := pathdom.NewPath(result, "result")
	ch1Path := pathdom.NewPath(ch1, "ch1")
	staleValueKey := pathdom.PathKey("sym325@1.value")
	staleValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Number))
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, result, "result")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(result), typevalue.FromType(reg, union)).
		WriteValue(reg, key.SymbolValue(ch1), typevalue.FromType(reg, chanInt)).
		WritePathKey(reg, ks, staleValueKey, staleValue)

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
			Visibility: resolver,
		}),
	})

	assertVariantOriginType(t, reg, got[thenPoint], result, union, intCase)
	assertVariantOriginType(t, reg, got[elsePoint], result, union, strCase)
	assertPathValue(t, reg, ks, got[thenPoint], staleValueKey, product.Bottom(reg))
	assertPathValue(t, reg, ks, got[elsePoint], staleValueKey, product.Bottom(reg))
}

func TestFactsEdgeTransferPathComparisonDerivesConstraintOriginFromWitness(t *testing.T) {
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

	chanInt := typ.NewAlias("__test_WitnessOnlyChanInt", typetable.NewRecord().
		Field("__tag", typ.LiteralString("int")).
		Build())
	chanStr := typ.NewAlias("__test_WitnessOnlyChanStr", typetable.NewRecord().
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
	union := typeexpr.Union(intCase, strCase)

	result := symbol.ID(329)
	ch1 := symbol.ID(330)
	resultPath := pathdom.NewPath(result, "result")
	ch1Path := pathdom.NewPath(ch1, "ch1")
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, result, "result")
	resolver := visibility.NewResolver(visibilityBuilder.Build())

	witnessOnlyChannel := product.Set(reg, typevalue.WithWitness(reg, product.Top(), chanInt), variantorigin.Key, variantorigin.Top())
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(result), typevalue.FromType(reg, union)).
		WriteValue(reg, key.SymbolValue(ch1), witnessOnlyChannel)

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
			Visibility: resolver,
		}),
	})

	assertVariantOriginType(t, reg, got[thenPoint], result, union, intCase)
	assertVariantOriginType(t, reg, got[elsePoint], result, union, strCase)
}

func TestFactsEdgeTransferPathComparisonEmptyVariantOriginIsUnreachable(t *testing.T) {
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

	chanInt := typ.NewAlias("__test_UnreachableChanInt", typetable.NewRecord().
		Field("__tag", typ.LiteralString("int")).
		Build())
	chanStr := typ.NewAlias("__test_UnreachableChanStr", typetable.NewRecord().
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
	union := typeexpr.Union(intCase, strCase)
	family, cases, ok := variant.OriginOfType(union)
	if !ok || len(cases) != 2 {
		t.Fatalf("union origin = %x/%v/%v, want two cases", family, cases, ok)
	}
	intOriginCase := mustOriginCaseIndex(t, family, intCase)

	result := symbol.ID(331)
	ch1 := symbol.ID(332)
	resultPath := pathdom.NewPath(result, "result")
	ch1Path := pathdom.NewPath(ch1, "ch1")
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, result, "result")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	preciseResult := product.Set(
		reg,
		typevalue.WithWitness(reg, typevalue.FromType(reg, union), union),
		variantorigin.Key,
		variantorigin.Singleton(family, intOriginCase),
	)
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(result), preciseResult).
		WriteValue(reg, key.SymbolValue(ch1), typevalue.FromType(reg, chanInt))

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
			Visibility: resolver,
		}),
	})

	assertVariantOriginType(t, reg, got[thenPoint], result, union, intCase)
	if !stateIsBottom(reg, got[elsePoint]) {
		t.Fatalf("else state = %v, want unreachable bottom", got[elsePoint])
	}
}

func TestFactsEdgeTransferTypeNameComparisonUsesSameEdgeDiscriminantNarrowing(t *testing.T) {
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

	intCell := typetable.NewRecord().
		Field("kind", typ.LiteralString("number")).
		Field("raw", typeexpr.Union(typ.Number, typ.String, typ.Boolean)).
		Build()
	textCell := typetable.NewRecord().
		Field("kind", typ.LiteralString("string")).
		Field("raw", typeexpr.Union(typ.Number, typ.String, typ.Boolean)).
		Build()
	flagCell := typetable.NewRecord().
		Field("kind", typ.LiteralString("boolean")).
		Field("raw", typeexpr.Union(typ.Number, typ.String, typ.Boolean)).
		Build()
	cellType := typeexpr.Union(intCell, textCell, flagCell)

	cell := symbol.ID(333)
	cellPath := pathdom.NewPath(cell, "cell")
	kindPath := cellPath.Field("kind")
	rawPath := cellPath.Field("raw")
	visibilityBuilder := visibility.NewBuilder()
	version := visibilityBuilder.Define(branch, cell, "cell")
	visibilityBuilder.SetVisible(thenPoint, cell, version)
	visibilityBuilder.SetVisible(elsePoint, cell, version)
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	kindKey := resolver.KeyForVersion(cell, version.ID, kindPath.Segments)
	rawKey := resolver.KeyForVersion(cell, version.ID, rawPath.Segments)
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(cell), typeValues.FromTypeWithWitness(reg, cellType)).
		WritePathKey(reg, ks, kindKey, typevalue.WithWitness(reg, typevalue.FromType(reg, typ.LiteralString("string")), typ.LiteralString("string"))).
		WritePathKey(reg, ks, rawKey, typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String))

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
				BranchPathRelations: map[cfg.Point]factflow.BranchPathRelationSet{
					branch: factflow.NewBranchPathRelationSet(
						factflow.NewBranchPathTypeMatch(rawPath, kindPath, true, false),
					),
				},
			}),
			Visibility:  resolver,
			ProjectPath: testLuaPathTypeProjector,
			TypeValues:  typeValues,
		}),
	})

	if !stateIsBottom(reg, got[thenPoint]) {
		t.Fatalf("then state = %v, want unreachable because stale raw string contradicts required boolean runtime kind", got[thenPoint])
	}
}

func TestFactsEdgeTransferTypeNameComparisonProjectsNarrowedUnionMember(t *testing.T) {
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

	intCell := typetable.NewRecord().
		Field("kind", typ.LiteralString("number")).
		Field("raw", typeexpr.Union(typ.Number, typ.String, typ.Boolean)).
		Build()
	textCell := typetable.NewRecord().
		Field("kind", typ.LiteralString("string")).
		Field("raw", typeexpr.Union(typ.Number, typ.String, typ.Boolean)).
		Build()
	flagCell := typetable.NewRecord().
		Field("kind", typ.LiteralString("boolean")).
		Field("raw", typeexpr.Union(typ.Number, typ.String, typ.Boolean)).
		Build()
	cellType := typeexpr.Union(intCell, textCell, flagCell)

	cell := symbol.ID(335)
	cellPath := pathdom.NewPath(cell, "cell")
	kindPath := cellPath.Field("kind")
	rawPath := cellPath.Field("raw")
	visibilityBuilder := visibility.NewBuilder()
	version := visibilityBuilder.Define(branch, cell, "cell")
	visibilityBuilder.SetVisible(thenPoint, cell, version)
	visibilityBuilder.SetVisible(elsePoint, cell, version)
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	rawKey := resolver.KeyForVersion(cell, version.ID, rawPath.Segments)
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(cell), typeValues.FromTypeWithWitness(reg, cellType))

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
				BranchPathRelations: map[cfg.Point]factflow.BranchPathRelationSet{
					branch: factflow.NewBranchPathRelationSet(
						factflow.NewBranchPathTypeMatch(rawPath, kindPath, true, false),
					),
				},
			}),
			Visibility:  resolver,
			ProjectPath: testLuaPathTypeProjector,
			TypeValues:  typeValues,
		}),
	})

	gotType, ok := typevalue.TypeOf(reg, got[thenPoint].ReadPathKey(reg, ks, rawKey))
	if !ok || !typ.TypeEquals(gotType, typ.Boolean) {
		t.Fatalf("raw path type = %v/%v, want boolean", gotType, ok)
	}
}

func TestFactsEdgeTransferTypeNameComparisonUsesPreviousBranchLiteralNarrowing(t *testing.T) {
	reg := standard.Registry()
	typeValues := typevalue.NewCache()
	graph := cfg.New()
	kindBranch := graph.AddNode(cfg.NodeBranch)
	typeBranch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), kindBranch, false)
	graph.AddEdge(kindBranch, typeBranch, true)
	graph.AddEdge(kindBranch, elsePoint, false)
	graph.AddEdge(typeBranch, thenPoint, true)
	graph.AddEdge(typeBranch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	cell := symbol.ID(334)
	cellPath := pathdom.NewPath(cell, "cell")
	kindPath := cellPath.Field("kind")
	rawPath := cellPath.Field("raw")
	cellType := typetable.NewRecord().
		Field("kind", typ.String).
		Field("raw", typ.String).
		Build()
	visibilityBuilder := visibility.NewBuilder()
	version := visibilityBuilder.Define(kindBranch, cell, "cell")
	visibilityBuilder.SetVisible(typeBranch, cell, version)
	visibilityBuilder.SetVisible(thenPoint, cell, version)
	visibilityBuilder.SetVisible(elsePoint, cell, version)
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(cell), typeValues.FromTypeWithWitness(reg, cellType))

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
					kindBranch: factflow.NewBranchRefinementSet(
						factflow.NewBranchRefinement(kindPath, factflow.NewValueConstraint(typeValues.FromTypeWithWitness(reg, typ.LiteralString("boolean"))), true, factflow.ValueRefinement{}, false),
					),
					typeBranch: factflow.NewBranchRefinementSet(
						factflow.NewBranchRefinement(kindPath, factflow.NewValueConstraint(typeValues.FromTypeWithWitness(reg, typ.LiteralString("boolean"))), true, factflow.ValueRefinement{}, false),
					),
				},
				BranchPathRelations: map[cfg.Point]factflow.BranchPathRelationSet{
					typeBranch: factflow.NewBranchPathRelationSet(
						factflow.NewBranchPathTypeMatch(rawPath, kindPath, true, false),
					),
				},
			}),
			Visibility:  resolver,
			ProjectPath: testLuaPathTypeProjector,
			TypeValues:  typeValues,
		}),
	})

	if !stateIsBottom(reg, got[thenPoint]) {
		t.Fatalf("then state = %v, want unreachable because raw is string but type name is boolean", got[thenPoint])
	}
	if stateIsBottom(reg, got[typeBranch]) {
		t.Fatal("kind branch should remain reachable before the raw type comparison")
	}
}

func TestApplyPathOriginRelationKeepsSingletonAliasFieldMatch(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(1)
	chanInt := typ.NewAlias("__test_DirectChanInt", typetable.NewRecord().
		Field("__tag", typ.LiteralString("int")).
		Build())
	chanStr := typ.NewAlias("__test_DirectChanStr", typetable.NewRecord().
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
	union := typeexpr.Union(intCase, strCase)
	family, cases, ok := variant.OriginOfType(union)
	if !ok || len(cases) != 2 {
		t.Fatalf("union origin = %x/%v/%v, want two cases", family, cases, ok)
	}
	intOriginCase := mustOriginCaseIndex(t, family, intCase)

	result := symbol.ID(1331)
	ch1 := symbol.ID(1332)
	resultPath := pathdom.NewPath(result, "result")
	ch1Path := pathdom.NewPath(ch1, "ch1")
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, result, "result")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(result), product.Set(
			reg,
			typevalue.WithWitness(reg, typevalue.FromType(reg, union), union),
			variantorigin.Key,
			variantorigin.Singleton(family, intOriginCase),
		)).
		WriteValue(reg, key.SymbolValue(ch1), typevalue.FromType(reg, chanInt))
	if stateIsBottom(reg, initial) {
		t.Fatal("initial state unexpectedly bottom")
	}
	rootOrigin, ok := typevalue.VariantOriginOfValue(reg, nil, initial.ReadValue(reg, key.SymbolValue(result)))
	if !ok {
		t.Fatal("root origin missing")
	}
	if rootOrigin.Family() != family || rootOrigin.CasesLen() != 1 || rootOrigin.CaseAt(0) != intOriginCase {
		t.Fatalf("root origin = %v, want singleton case %d in family %d", rootOrigin, intOriginCase, family)
	}
	constraintType, ok := typevalue.TypeOf(reg, initial.ReadValue(reg, key.SymbolValue(ch1)))
	if !ok {
		t.Fatal("constraint type missing")
	}
	if narrowedCases, ok := variant.NarrowOriginByPathType(
		family,
		[]int{intOriginCase},
		[]segment.Segment{{Kind: segment.SegmentField, Name: "channel"}},
		constraintType,
		true,
	); ok && len(narrowedCases) == 0 {
		t.Fatalf("singleton field type %s was classified incompatible with %s", chanInt, constraintType)
	}
	constraintOrigin, ok := typevalue.VariantOriginOfValue(reg, nil, initial.ReadValue(reg, key.SymbolValue(ch1)))
	if !ok {
		t.Fatal("constraint origin missing")
	}
	if narrowedCases, ok := variant.NarrowOriginByPath(
		family,
		[]int{intOriginCase},
		[]segment.Segment{{Kind: segment.SegmentField, Name: "channel"}},
		constraintOrigin.Family(),
		constraintOrigin.CasesRef(),
		true,
	); ok {
		if len(narrowedCases) == 0 {
			t.Fatalf("singleton field origin was classified incompatible with constraint origin %v", constraintOrigin)
		}
		t.Fatalf("singleton field origin produced strict narrowing cases %v, want no-op", narrowedCases)
	}
	resolvedConstraint, ok := resolvePathValueAt(reg, resolver, point, initial, ch1Path, nil)
	if !ok {
		t.Fatal("constraint path did not resolve")
	}
	if !product.Equal(reg, resolvedConstraint.value, initial.ReadValue(reg, key.SymbolValue(ch1))) {
		t.Fatalf("resolved constraint = %s, want root slot value", formatValue(reg, resolvedConstraint.value))
	}
	invalidated := invalidateRootDescendantsAt(resolver, point, initial, resultPath.RootOnly())
	if stateIsBottom(reg, invalidated) {
		t.Fatal("root descendant invalidation collapsed reachable state")
	}
	if narrowedCases, ok := narrowOriginCasesByPathConstraint(
		nil,
		reg,
		rootOrigin,
		resultPath.Field("channel").Segments,
		resolvedConstraint.value,
		true,
	); ok {
		if len(narrowedCases) == 0 {
			t.Fatal("origin helper classified matching singleton as unreachable")
		}
		t.Fatalf("origin helper returned strict cases %v for matching singleton, want no-op", narrowedCases)
	}

	got := applyPathOriginRelation(nil, reg, resolver, nil, point, initial, resultPath.Field("channel"), ch1Path, true)
	if stateIsBottom(reg, got) {
		t.Fatal("matching singleton field equality made state unreachable")
	}
	assertVariantOriginType(t, reg, got, result, union, intCase)
}

func TestApplyPathOriginRelationRejectsSingletonAliasFieldMismatch(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(1)
	chanMsg := typ.NewAlias("__test_DirectChanMsg", typetable.NewRecord().
		Field("__tag", typ.LiteralString("msg")).
		Build())
	chanEvent := typ.NewAlias("__test_DirectChanEvent", typetable.NewRecord().
		Field("__tag", typ.LiteralString("event")).
		Build())
	msgCase := typetable.NewRecord().
		Field("channel", chanMsg).
		Field("value", typetable.NewRecord().Field("_topic", typ.String).Build()).
		Build()
	eventCase := typetable.NewRecord().
		Field("channel", chanEvent).
		Field("value", typetable.NewRecord().Field("kind", typ.String).Build()).
		Build()
	union := typeexpr.Union(msgCase, eventCase)
	family, cases, ok := variant.OriginOfType(union)
	if !ok || len(cases) != 2 {
		t.Fatalf("union origin = %x/%v/%v, want two cases", family, cases, ok)
	}
	msgOriginCase := mustOriginCaseIndex(t, family, msgCase)

	result := symbol.ID(1333)
	events := symbol.ID(1334)
	resultPath := pathdom.NewPath(result, "result")
	eventsPath := pathdom.NewPath(events, "events")
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, result, "result")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(result), product.Set(
			reg,
			typevalue.WithWitness(reg, typevalue.FromType(reg, union), union),
			variantorigin.Key,
			variantorigin.Singleton(family, msgOriginCase),
		)).
		WriteValue(reg, key.SymbolValue(events), typevalue.FromType(reg, chanEvent))

	got := applyPathOriginRelation(nil, reg, resolver, nil, point, initial, resultPath.Field("channel"), eventsPath, true)
	if !stateIsBottom(reg, got) {
		t.Fatalf("mismatched singleton field equality state = %v, want unreachable", got)
	}
}

func TestApplyPathEqualityRejectsDisjointResolvedValues(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(1)
	chanMsg := typ.NewAlias("__test_EqualityChanMsg", typetable.NewRecord().
		Field("__tag", typ.LiteralString("msg")).
		Build())
	chanEvent := typ.NewAlias("__test_EqualityChanEvent", typetable.NewRecord().
		Field("__tag", typ.LiteralString("event")).
		Build())
	msgResult := typetable.NewRecord().
		Field("channel", chanMsg).
		Field("value", typetable.NewRecord().Field("_topic", typ.String).Build()).
		Build()

	result := symbol.ID(1335)
	events := symbol.ID(1336)
	resultPath := pathdom.NewPath(result, "result")
	eventsPath := pathdom.NewPath(events, "events")
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, result, "result")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(result), typevalue.WithWitness(reg, typevalue.FromType(reg, msgResult), msgResult)).
		WriteValue(reg, key.SymbolValue(events), typevalue.FromType(reg, chanEvent))

	got := applyPathEqualityAtCached(nil, reg, resolver, testLuaPathTypeProjector, point, initial, resultPath.Field("channel"), eventsPath)
	if !stateIsBottom(reg, got) {
		t.Fatalf("disjoint equality state = %v, want unreachable", got)
	}
}

func TestFactsEdgeTransferEqualityEvidenceRejectsDisjointResolvedValues(t *testing.T) {
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

	chanMsg := typ.NewAlias("__test_EvidenceChanMsg", typetable.NewRecord().
		Field("__tag", typ.LiteralString("msg")).
		Build())
	chanEvent := typ.NewAlias("__test_EvidenceChanEvent", typetable.NewRecord().
		Field("__tag", typ.LiteralString("event")).
		Build())
	msgResult := typetable.NewRecord().
		Field("channel", chanMsg).
		Field("value", typetable.NewRecord().Field("_topic", typ.String).Build()).
		Build()

	result := symbol.ID(1337)
	events := symbol.ID(1338)
	resultPath := pathdom.NewPath(result, "result")
	eventsPath := pathdom.NewPath(events, "events")
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, result, "result")
	visibilityBuilder.Define(branch, events, "events")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(result), typevalue.WithWitness(reg, typevalue.FromType(reg, msgResult), msgResult)).
		WriteValue(reg, key.SymbolValue(events), typevalue.FromType(reg, chanEvent))

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchPathEvidence: map[cfg.Point]factflow.BranchPathEvidenceSet{
					branch: factflow.NewBranchPathEvidenceSet(
						factflow.NewBranchPathEqualityEvidenceOnEdge(resultPath.Field("channel"), eventsPath, true),
					),
				},
			}),
			Visibility:  resolver,
			ProjectPath: testLuaPathTypeProjector,
			TypeValues:  typevalue.NewCache(),
		}),
	})

	if !stateIsBottom(reg, got[thenPoint]) {
		t.Fatalf("then state = %v, want unreachable from disjoint equality evidence", got[thenPoint])
	}
	if stateIsBottom(reg, got[elsePoint]) {
		t.Fatal("else state should remain reachable")
	}
}

func TestFactsEdgeTransferStopsAfterEqualityEvidenceMakesEdgeUnreachable(t *testing.T) {
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

	chanMsg := typ.NewAlias("__test_StopEvidenceChanMsg", typetable.NewRecord().
		Field("__tag", typ.LiteralString("msg")).
		Build())
	chanEvent := typ.NewAlias("__test_StopEvidenceChanEvent", typetable.NewRecord().
		Field("__tag", typ.LiteralString("event")).
		Build())
	msgResult := typetable.NewRecord().
		Field("channel", chanMsg).
		Field("value", typetable.NewRecord().Field("_topic", typ.String).Build()).
		Build()

	result := symbol.ID(1339)
	events := symbol.ID(1340)
	resultPath := pathdom.NewPath(result, "result")
	eventsPath := pathdom.NewPath(events, "events")
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, result, "result")
	visibilityBuilder.Define(branch, events, "events")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(result), typevalue.WithWitness(reg, typevalue.FromType(reg, msgResult), msgResult)).
		WriteValue(reg, key.SymbolValue(events), typevalue.FromType(reg, chanEvent))

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchPathEvidence: map[cfg.Point]factflow.BranchPathEvidenceSet{
					branch: factflow.NewBranchPathEvidenceSet(
						factflow.NewBranchPathEqualityEvidenceOnEdge(resultPath.Field("channel"), eventsPath, true),
						factflow.NewBranchPathPresenceEvidenceOnEdge(resultPath.Field("value"), presence.Present(), true),
					),
				},
			}),
			Visibility:  resolver,
			ProjectPath: testLuaPathTypeProjector,
			TypeValues:  typevalue.NewCache(),
		}),
	})

	if !stateIsBottom(reg, got[thenPoint]) {
		t.Fatalf("then state = %v, want unreachable after contradictory equality evidence", got[thenPoint])
	}
	if stateIsBottom(reg, got[elsePoint]) {
		t.Fatal("else state should remain reachable")
	}
}

func mustOriginCaseIndex(t *testing.T, family uint64, want typ.Type) int {
	t.Helper()
	cases, ok := variant.OriginCases(family)
	if !ok {
		t.Fatalf("origin family %d has no cases", family)
	}
	for _, c := range cases {
		if typ.TypeEquals(c.Type, want) {
			return c.Index
		}
	}
	t.Fatalf("origin family %d has no case for %s", family, want)
	return 0
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
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, table, "table")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(root), stringValue).
		WritePathKey(reg, ks, memberKey, product.Top())

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
			Visibility: resolver,
		}),
	})

	assertRuntimeKind(t, reg, got[thenPoint].ReadValue(reg, key.SymbolValue(root)), runtimekind.Singleton(runtimekind.String))
	assertRuntimeKind(t, reg, got[thenPoint].ReadPathKey(reg, ks, memberKey), runtimekind.Singleton(runtimekind.String))
	assertRuntimeKind(t, reg, got[elsePoint].ReadValue(reg, key.SymbolValue(root)), runtimekind.Singleton(runtimekind.String))
	assertRuntimeKind(t, reg, got[elsePoint].ReadPathKey(reg, ks, memberKey), runtimekind.Top())
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
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, leftRoot, "left")
	visibilityBuilder.Define(branch, rightRoot, "right")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	initial := state.State{}.
		WritePathKey(reg, ks, leftKey, numberValue).
		WritePathKey(reg, ks, rightKey, product.Top())

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
			Visibility: resolver,
		}),
	})

	assertRuntimeKind(t, reg, got[thenPoint].ReadPathKey(reg, ks, leftKey), runtimekind.Singleton(runtimekind.Number))
	assertRuntimeKind(t, reg, got[thenPoint].ReadPathKey(reg, ks, rightKey), runtimekind.Singleton(runtimekind.Number))
	assertRuntimeKind(t, reg, got[elsePoint].ReadPathKey(reg, ks, leftKey), runtimekind.Singleton(runtimekind.Number))
	assertRuntimeKind(t, reg, got[elsePoint].ReadPathKey(reg, ks, rightKey), runtimekind.Top())
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
	ks := keyspace.New()
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(root), numberValue).
		WritePathKey(reg, ks, memberKey, product.Top())

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
	assertRuntimeKind(t, reg, got[thenPoint].ReadPathKey(reg, ks, memberKey), runtimekind.Top())
	assertRuntimeKind(t, reg, got[elsePoint].ReadValue(reg, key.SymbolValue(root)), runtimekind.Singleton(runtimekind.Number))
	assertRuntimeKind(t, reg, got[elsePoint].ReadPathKey(reg, ks, memberKey), runtimekind.Top())
}
