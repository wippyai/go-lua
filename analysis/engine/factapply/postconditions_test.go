package factapply

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestFactsNodeTransferAppliesTruthyAssertPostconditionOnNormalContinuation(t *testing.T) {
	reg := standard.Registry()
	graph, call, after := callContinuationGraph()
	target := symbol.ID(401)
	initial := state.State{}.WriteValue(reg, key.SymbolValue(target), product.Top())

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				PostconditionRefinements: map[cfg.Point]factflow.PostconditionRefinementSet{
					call: factflow.NewPostconditionRefinementSet(
						factflow.NewPostconditionRefinement(
							pathdom.NewPath(target, "x"),
							factflow.NewValueConstraint(product.NewWithPresence(reg, product.ShapeTop, presence.Present())),
						),
					),
				},
			}),
		}),
	})

	assertValue(t, reg, got[after], key.SymbolValue(target), presentValue(reg))
}

func TestFactsNodeTransferAppliesTypeAssertPostconditionOnNormalContinuation(t *testing.T) {
	reg := standard.Registry()
	graph, call, after := callContinuationGraph()
	target := symbol.ID(402)
	numberValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Number))
	want := product.Meet(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), numberValue)
	refinement := factflow.NewValueRefinement().
		WithConstraint(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present())).
		WithConstraint(reg, numberValue)

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: state.State{}.WriteValue(reg, key.SymbolValue(target), product.Top()),
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				PostconditionRefinements: map[cfg.Point]factflow.PostconditionRefinementSet{
					call: factflow.NewPostconditionRefinementSet(
						factflow.NewPostconditionRefinement(pathdom.NewPath(target, "x"), refinement),
					),
				},
			}),
		}),
	})

	assertValue(t, reg, got[after], key.SymbolValue(target), want)
}

func callContinuationGraph() (*cfg.CFG, cfg.Point, cfg.Point) {
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	after := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, after, false)
	graph.AddEdge(after, graph.Exit(), false)
	return graph, call, after
}
