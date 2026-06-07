package flow

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
)

// StaticMemberReduction is a proven point-local value for an exact static path.
type StaticMemberReduction struct {
	Path  constraint.Path
	Value product.AbstractValue
}

// ConditionDerivedFacts asks flow for all point facts derivable directly from a
// condition fact, independent of transfer's lexical storage policy.
type ConditionDerivedFacts struct {
	State                       PointState
	Fact                        constraint.Condition
	VariantCaseFieldProjections []VariantCaseFieldProjection
	SymbolValue                 SymbolProductReader
}

// ConditionDerivedFactReductions is the flow-owned result of interpreting a
// condition fact. Callers apply reductions through their storage boundary.
type ConditionDerivedFactReductions struct {
	SymbolValues  []SymbolValueReduction
	StaticMembers []StaticMemberReduction
}

// Reductions interprets condition-derived facts that do not require caller
// policy about declared types or lexical storage.
func (q ConditionDerivedFacts) Reductions() ConditionDerivedFactReductions {
	return ConditionDerivedFactReductions{
		SymbolValues: VariantOriginConditionReducer{
			SymbolValue: q.SymbolValue,
		}.Reductions(q.Fact),
		StaticMembers: VariantCaseFieldProjectionReductions(q.State, q.Fact, q.VariantCaseFieldProjections),
	}
}

// HasReductions reports whether the batch contains any concrete fact updates.
func (r ConditionDerivedFactReductions) HasReductions() bool {
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
