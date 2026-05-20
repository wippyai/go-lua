package assign

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/abstract/transfer/mutator"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/overlaymut"
	"github.com/wippyai/go-lua/types/typ"
)

// CollectFieldAssignments reduces transfer assignment evidence into field assignments.
// Returns a map: symbolID -> map[fieldName]typ.Type representing fields assigned to each symbol.
// The synth function is used to synthesize field value types.
// If filterSyms is non-nil, only symbols in the filter are collected.
func CollectFieldAssignments(
	assignments []api.AssignmentEvidence,
	synth func(ast.Expr, cfg.Point) typ.Type,
	filterSyms map[cfg.SymbolID]bool,
) map[cfg.SymbolID]map[string]typ.Type {
	return overlaymut.CollectFieldAssignments(assignments, synth, filterSyms)
}

// CollectIndexerAssignments reduces transfer assignment evidence for dynamic index writes.
// Returns a map: symbolID -> []IndexerInfo representing index assignments to each symbol.
func CollectIndexerAssignments(
	assignments []api.AssignmentEvidence,
	synth func(ast.Expr, cfg.Point) typ.Type,
	bindings *bind.BindingTable,
	filterSyms map[cfg.SymbolID]bool,
) map[cfg.SymbolID][]mutator.IndexerInfo {
	return overlaymut.CollectIndexerAssignments(assignments, synth, bindings, filterSyms)
}
