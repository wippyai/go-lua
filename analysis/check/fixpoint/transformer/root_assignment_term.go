package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// rootAssignmentTerm is immutable N4 syntax: one frozen factapply descriptor
// plus the complete symbolic source tuple it consumes. It owns no State and no
// source callback.
type rootAssignmentTerm struct {
	transaction factapply.RootAssignmentTransaction
	sources     []ValueTerm
}

func compileRootAssignmentTerm(ctx planCompileContext, point cfg.Point) (rootAssignmentTerm, error) {
	transaction, ok := factapply.PlanRootAssignmentTransaction(ctx.facts, point)
	if !ok {
		return rootAssignmentTerm{}, fmt.Errorf("root assignment has no frozen transaction")
	}
	sources := make([]ValueTerm, transaction.SourceCount())
	for index := range sources {
		source, _ := transaction.Source(index)
		term, err := exactRootAssignmentSourceTerm(ctx, source)
		if err != nil {
			return rootAssignmentTerm{}, fmt.Errorf("root assignment source %d: %w", index, err)
		}
		sources[index] = term
	}
	return rootAssignmentTerm{transaction: transaction.Clone(), sources: sources}, nil
}

func exactRootAssignmentSourceTerm(ctx planCompileContext, source factflow.ValueSource) (ValueTerm, error) {
	return exactCompilerSourceTerm(ctx, source)
}

func (t rootAssignmentTerm) valid(arena *Arena, shape Shape) bool {
	if !t.structurallyValid() {
		return false
	}
	for _, source := range t.sources {
		if source == 0 || !arena.validValue(source, shape, make(map[ValueTerm]bool)) {
			return false
		}
	}
	return true
}

func (t rootAssignmentTerm) structurallyValid() bool {
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

func (t rootAssignmentTerm) framesOwned(arena *Arena, owned map[callFrameTerm]struct{}) bool {
	for _, source := range t.sources {
		if !arena.valueFramesOwned(source, owned, make(map[ValueTerm]bool)) {
			return false
		}
	}
	return true
}

func (t rootAssignmentTerm) framesOwnedBits(arena *Arena, owned []uint64) bool {
	for _, source := range t.sources {
		if !valueFramesOwnedBits(arena, source, owned) {
			return false
		}
	}
	return true
}

func (t rootAssignmentTerm) canonical(arena *Arena) string {
	out := fmt.Sprintf("n4:%d:%d", t.transaction.Point(), t.transaction.TargetSymbol())
	for _, source := range t.sources {
		out += ":" + arena.canonicalValue(source)
	}
	return out
}
