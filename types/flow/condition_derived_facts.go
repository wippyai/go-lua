package flow

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/narrow"
)

// StaticMemberReduction is a proven point-local value for an exact static path.
type StaticMemberReduction struct {
	Path  constraint.Path
	Value product.AbstractValue
}

// ConditionReducer asks flow for all point-state reductions implied by one
// condition fact. Transfer supplies only policy boundaries that flow cannot own:
// lexical symbol storage, declared-type bases, and callable field resolution.
type ConditionReducer struct {
	State                       PointState
	Fact                        constraint.Condition
	VariantCaseFieldProjections []VariantCaseFieldProjection
	SymbolValue                 SymbolProductReader
	ProductBase                 ProductConditionBaseReader
	Resolver                    narrow.Resolver
}

// ConditionReductions is the flow-owned result of interpreting a condition
// fact. Callers apply symbol reductions through their storage boundary and
// static-member reductions through flow writers.
type ConditionReductions struct {
	SymbolValues  []SymbolValueReduction
	StaticMembers []StaticMemberReduction
}

// Reductions interprets condition-derived facts and product-domain narrowings
// through one canonical flow-domain route.
func (q ConditionReducer) Reductions() ConditionReductions {
	out := ConditionReductions{
		StaticMembers: VariantCaseFieldProjectionReductions(q.State, q.Fact, q.VariantCaseFieldProjections),
	}
	out.SymbolValues = append(out.SymbolValues, VariantOriginConditionReducer{
		SymbolValue: q.SymbolValue,
	}.Reductions(q.Fact)...)
	out.SymbolValues = append(out.SymbolValues, ProductConditionReducer{
		Fact:     q.Fact,
		Facts:    PointFactsOf(q.State),
		Resolver: q.Resolver,
		Base:     q.ProductBase,
	}.Reductions()...)
	return out
}

// HasReductions reports whether the batch contains any concrete fact updates.
func (r ConditionReductions) HasReductions() bool {
	return len(r.SymbolValues) > 0 || len(r.StaticMembers) > 0
}

// ApplyStaticMemberReductions materializes static member reductions into point
// state. It is separate from Reductions so transfer can preserve its own symbol
// storage policy while still using flow's static-member writer.
func ApplyStaticMemberReductions(out *PointState, reductions []StaticMemberReduction) bool {
	if out == nil || len(reductions) == 0 {
		return false
	}
	changed := false
	for _, reduction := range reductions {
		changed = SetStaticMemberPath(out, reduction.Path, reduction.Value) || changed
	}
	return changed
}
