package reachability

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/link"
)

func installProgramRules(solver *engine.Solver, source *link.Link, factor *engine.Factor[uint64, Value]) bool {
	for index := 0; index < source.ShardCount(); index++ {
		shard, ok := source.ShardAt(index)
		if !ok {
			return false
		}
		programValue, ok := source.Program(shard)
		if !ok || programValue == nil || !installShardRules(solver, factor, shard, programValue) {
			return false
		}
	}
	return true
}

func installShardRules(solver *engine.Solver, factor *engine.Factor[uint64, Value], shard link.Shard, source *program.Program) bool {
	entry, ok := source.Entry()
	if !ok || !declareEntryRule(solver, factor, shard, source, entry) {
		return false
	}
	activations, ok := activations(source, entry)
	if !ok {
		return false
	}
	for _, activation := range activations {
		count, ok := source.ActivationEdgeCount(activation)
		if !ok {
			return false
		}
		for index := 0; index < count; index++ {
			edge, ok := source.ActivationEdgeAt(activation, index)
			if !ok || !declareTransferRule(solver, factor, shard, source, activation, index, edge) {
				return false
			}
		}
	}
	return true
}

func activations(source *program.Program, entry program.Term) ([]program.Term, bool) {
	result := []program.Term{entry}
	seen := map[program.Term]struct{}{entry: {}}
	for index := 0; index < source.FunctionCount(); index++ {
		function, ok := source.FunctionAt(index)
		if !ok {
			return nil, false
		}
		_, body, _, ok := source.Function(function)
		if !ok {
			return nil, false
		}
		if _, exists := seen[body]; exists {
			continue
		}
		seen[body] = struct{}{}
		result = append(result, body)
	}
	return result, true
}

func declareEntryRule(solver *engine.Solver, factor *engine.Factor[uint64, Value], shard link.Shard, source *program.Program, entry program.Term) bool {
	_, ok := engine.DeclareRule(solver, factor, semanticProgram("entry", source, uint64(entry)), func(binding *engine.RuleBinding) bool {
		return binding.At(shard, entry)
	}, func(access engine.Access[uint64, Value]) bool {
		return access.Set(key, Reachable)
	})
	return ok
}

func declareTransferRule(solver *engine.Solver, factor *engine.Factor[uint64, Value], shard link.Shard, source *program.Program, activation program.Term, ordinal int, edge program.Edge) bool {
	var input engine.ReadRef[uint64, Value]
	rule, ok := engine.DeclareRule(solver, factor, semanticProgram("transfer", source,
		uint64(activation), uint64(ordinal)), func(binding *engine.RuleBinding) bool {
		return binding.From(shard, edge)
	}, func(access engine.Access[uint64, Value]) bool {
		return engine.Carry(access, input)
	})
	if !ok {
		return false
	}
	input, ok = engine.Read(rule, 0, factor)
	return ok
}
