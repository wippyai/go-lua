package factapply

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
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
