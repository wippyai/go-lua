package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// returnTransactionTerm is immutable N5 syntax. It freezes the complete
// source tuple consumed by the sole concrete return authority.
type returnTransactionTerm struct {
	transaction factapply.ReturnTransaction
	sources     []ValueTerm
}

func (t returnTransactionTerm) clone() returnTransactionTerm {
	return returnTransactionTerm{transaction: t.transaction.Clone(), sources: append([]ValueTerm(nil), t.sources...)}
}

func compileReturnTransactionTerm(ctx planCompileContext, point cfg.Point) (returnTransactionTerm, error) {
	fact, present := ctx.facts.Return(point)
	var sources []factflow.ValueSource
	if present {
		sources = fact.Sources()
	}
	semanticSources := make([]factflow.ValueSource, 0, len(sources))
	for _, source := range sources {
		if !exactBoundaryMemberZeroResultReturn(ctx, source) {
			semanticSources = append(semanticSources, source)
		}
	}
	semanticSources = expandOpenTailReturnSources(ctx, semanticSources)
	transaction, ok := factapply.PlanReturnTransactionSources(ctx.facts, point, semanticSources)
	if !ok {
		return returnTransactionTerm{}, fmt.Errorf("return has no frozen transaction")
	}
	terms := make([]ValueTerm, transaction.SourceCount())
	for index := range terms {
		source, _ := transaction.Source(index)
		term, err := exactReturnTransactionSourceTerm(ctx, source)
		if err != nil {
			return returnTransactionTerm{}, fmt.Errorf("return source %d: %w", index, err)
		}
		terms[index] = term
	}
	return returnTransactionTerm{transaction: transaction.Clone(), sources: terms}, nil
}

// expandOpenTailReturnSources resolves Lua's final-call value-list expansion
// against the lexical function's declared result shape.  It is a compile-time
// normalization into the sole N5 return transaction, not a second return path.
func expandOpenTailReturnSources(ctx planCompileContext, sources []factflow.ValueSource) []factflow.ValueSource {
	resultArity := planReturnArity(ctx.plan)
	if len(sources) == 0 || resultArity == 0 {
		return sources
	}
	tail := sources[len(sources)-1]
	if tail.Kind != factflow.ValueSourceCall || !tail.Final || !tail.Expanded || !tail.OpenTail || tail.Adjusted || tail.ResultIndex < 0 {
		return sources
	}
	start := tail.TargetIndex
	if start < 0 {
		start = len(sources) - 1
	}
	if start < 0 || start >= resultArity {
		return sources
	}
	out := append([]factflow.ValueSource(nil), sources[:len(sources)-1]...)
	for target := start; target < resultArity; target++ {
		source := tail
		source.TargetIndex = target
		source.ResultIndex = tail.ResultIndex + target - start
		out = append(out, source)
	}
	return out
}

func exactReturnTransactionSourceTerm(ctx planCompileContext, source factflow.ValueSource) (ValueTerm, error) {
	return exactReturnSourceTerm(ctx, source)
}

func (t returnTransactionTerm) structurallyValid() bool {
	if !t.transaction.Valid() || len(t.sources) != t.transaction.SourceCount() {
		return false
	}
	for _, source := range t.sources {
		if source == 0 {
			return false
		}
	}
	return true
}

func (t returnTransactionTerm) valid(arena *Arena, shape Shape) bool {
	if !t.structurallyValid() {
		return false
	}
	for _, source := range t.sources {
		if !arena.validValue(source, shape, make(map[ValueTerm]bool)) {
			return false
		}
	}
	return true
}

func (t returnTransactionTerm) framesOwned(arena *Arena, owned map[callFrameTerm]struct{}) bool {
	for _, source := range t.sources {
		if !arena.valueFramesOwned(source, owned, make(map[ValueTerm]bool)) {
			return false
		}
	}
	return true
}

func (t returnTransactionTerm) framesOwnedBits(arena *Arena, owned []uint64) bool {
	for _, source := range t.sources {
		if !valueFramesOwnedBits(arena, source, owned) {
			return false
		}
	}
	return true
}

func (t returnTransactionTerm) resolveInto(reg *axis.Registry, arena *Arena, cursor BindingCursor, context SpecializationContext, values []product.Value) (factapply.ResolvedReturnTransaction, bool) {
	if len(values) != len(t.sources) {
		return factapply.ResolvedReturnTransaction{}, false
	}
	for index, source := range t.sources {
		value, exact := arena.evalValue(source, cursor, context)
		if !exact {
			return factapply.ResolvedReturnTransaction{}, false
		}
		if context.HasEnvironment {
			value = sourcevalue.PreferExactHeapRoot(reg, nil, context.Environment, value)
		}
		values[index] = value
	}
	return t.transaction.Bind(reg, values)
}

func (t returnTransactionTerm) canonical(arena *Arena) string {
	out := fmt.Sprintf("n5:%d", t.transaction.Point())
	for _, source := range t.sources {
		out += ":" + arena.canonicalValue(source)
	}
	return out
}
