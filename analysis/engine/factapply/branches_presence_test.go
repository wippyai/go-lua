package factapply

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

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
