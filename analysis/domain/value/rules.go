package value

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/link"
)

func installSourceRules(solver *engine.Solver, source *link.Link, factor *engine.Factor[uint64, product.Value], registry *axis.Registry, authority axis.SchemaIdentity) bool {
	for index := 0; index < source.ShardCount(); index++ {
		shard, ok := source.ShardAt(index)
		if !ok {
			return false
		}
		programValue, ok := source.Program(shard)
		if !ok || programValue == nil || !installShardSourceRules(solver, factor, registry, authority, shard, programValue) {
			return false
		}
	}
	return true
}

func installShardSourceRules(solver *engine.Solver, factor *engine.Factor[uint64, product.Value], registry *axis.Registry, authority axis.SchemaIdentity, shard link.Shard, source *program.Program) bool {
	for index := 0; index < source.NilCount(); index++ {
		term, ok := source.NilAt(index)
		if !ok || !declareSourceLiteralRule(solver, factor, registry, authority, shard, source, term) {
			return false
		}
	}
	for index := 0; index < source.BoolCount(); index++ {
		term, ok := source.BoolAt(index)
		if !ok || !declareSourceLiteralRule(solver, factor, registry, authority, shard, source, term) {
			return false
		}
	}
	for index := 0; index < source.IntegerCount(); index++ {
		term, ok := source.IntegerAt(index)
		if !ok || !declareSourceLiteralRule(solver, factor, registry, authority, shard, source, term) {
			return false
		}
	}
	for index := 0; index < source.FloatCount(); index++ {
		term, ok := source.FloatAt(index)
		if !ok || !declareSourceLiteralRule(solver, factor, registry, authority, shard, source, term) {
			return false
		}
	}
	for index := 0; index < source.StringCount(); index++ {
		term, ok := source.StringAt(index)
		if !ok || !declareSourceLiteralRule(solver, factor, registry, authority, shard, source, term) {
			return false
		}
	}
	return true
}

func declareSourceLiteralRule(solver *engine.Solver, factor *engine.Factor[uint64, product.Value], registry *axis.Registry, authority axis.SchemaIdentity, shard link.Shard, source *program.Program, term program.Term) bool {
	value, ok := sourceLiteral(registry, source, term)
	if !ok {
		return false
	}
	_, ok = engine.DeclareRule(solver, factor, semanticOccurrence(authority, source, term), func(binding *engine.RuleBinding) bool {
		return binding.At(shard, term)
	}, func(access engine.Access[uint64, product.Value]) bool {
		return access.Set(occurrenceKey, value)
	})
	return ok
}

// sourceLiteral projects only the five typed source-literal Program families.
// All other Program terms intentionally remain outside this first oracle.
func sourceLiteral(registry *axis.Registry, source *program.Program, term program.Term) (product.Value, bool) {
	if _, ok := source.Nil(term); ok {
		return typevalue.Nil(registry), true
	}
	if _, value, ok := source.Bool(term); ok {
		return typevalue.LiteralBool(registry, value), true
	}
	if _, value, ok := source.Integer(term); ok {
		return typevalue.LiteralInt(registry, value), true
	}
	if _, value, ok := source.Float(term); ok {
		return typevalue.LiteralNumber(registry, value), true
	}
	if _, value, ok := source.String(term); ok {
		return typevalue.LiteralString(registry, value), true
	}
	return product.Value{}, false
}
