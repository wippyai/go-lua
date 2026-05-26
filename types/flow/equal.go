package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/typ"
)

// InputsEqual reports whether two flow input products describe the same
// abstract-interpreter problem. Runtime helpers such as decomposers are owned by
// the caller and are intentionally not part of the equality law.
func InputsEqual(a, b *Inputs) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Graph == b.Graph &&
		symbolTypeMapEqual(a.DeclaredTypes, b.DeclaredTypes) &&
		symbolBoolMapEqual(a.AnnotatedVars, b.AnnotatedVars) &&
		assignmentsEqual(a.Assignments, b.Assignments) &&
		constValuesEqual(a.ConstValues, b.ConstValues) &&
		edgeConditionsEqual(a.EdgeConditions, b.EdgeConditions) &&
		edgeNumericConstraintsEqual(a.EdgeNumericConstraints, b.EdgeNumericConstraints) &&
		typeKeyMapEqual(a.TypeKeys, b.TypeKeys) &&
		returnKindMapEqual(a.ReturnKinds, b.ReturnKinds) &&
		returnConstraintsEqual(a.ReturnConstraints, b.ReturnConstraints) &&
		predicateLinksEqual(a.PredicateLinks, b.PredicateLinks) &&
		siblingAssignmentsEqual(a.SiblingAssignments, b.SiblingAssignments) &&
		variantFieldOriginsEqual(a.VariantFieldOrigins, b.VariantFieldOrigins) &&
		mapMutatorAssignmentsEqual(a.MapMutatorAssignments, b.MapMutatorAssignments) &&
		tableMutatorAssignmentsEqual(a.TableMutatorAssignments, b.TableMutatorAssignments) &&
		containerMutatorAssignmentsEqual(a.ContainerMutatorAssignments, b.ContainerMutatorAssignments) &&
		pointBoolMapEqual(a.DeadPoints, b.DeadPoints) &&
		symbolStringMapEqual(a.ModuleAliases, b.ModuleAliases) &&
		symbolSymbolMapEqual(a.FunctionAliases, b.FunctionAliases) &&
		symbolTypeMapEqual(a.SiblingTypes, b.SiblingTypes) &&
		symbolTypeMapEqual(a.LiteralTypes, b.LiteralTypes) &&
		symbolSymbolMapEqual(a.KeysProvenance, b.KeysProvenance)
}

// SolutionEqual reports whether two solved flow products expose the same
// semantic state. Scratch buffers, query caches, epochs, and iteration counters
// are not semantic state.
func SolutionEqual(a, b *Solution) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return stringValueMapEqual(a.values, b.values) &&
		nestedStringTypeMapEqual(a.fieldOverlayIndex, b.fieldOverlayIndex) &&
		pointStringValueMapEqual(a.mutableValues, b.mutableValues) &&
		edgeConditionMapEqual(a.edgeConditions, b.edgeConditions) &&
		symbolSliceEqual(a.declaredSyms, b.declaredSyms) &&
		edgeNumericConstraintMapEqual(a.edgeNumericConstraints, b.edgeNumericConstraints) &&
		edgeBoolMapEqual(a.unsatEdges, b.unsatEdges) &&
		pointConditionMapEqual(a.pointConditions, b.pointConditions) &&
		numericStateMapEqual(a.numericStates, b.numericStates) &&
		stringStringMapEqual(a.pathAliases, b.pathAliases)
}

func typeEqual(a, b typ.Type) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return value.FactTypeEqual(a, b)
}

func symbolTypeMapEqual(a, b map[cfg.SymbolID]typ.Type) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		if !typeEqual(av, b[k]) {
			return false
		}
	}
	return true
}

func stringTypeMapEqual(a, b map[string]typ.Type) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		if !typeEqual(av, b[k]) {
			return false
		}
	}
	return true
}

func nestedStringTypeMapEqual(a, b map[string]map[string]typ.Type) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		if !stringTypeMapEqual(av, b[k]) {
			return false
		}
	}
	return true
}

func stringValueMapEqual(a, b map[string]product.AbstractValue) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok || !av.Equal(bv) {
			return false
		}
	}
	return true
}

func pointStringValueMapEqual(a, b map[cfg.Point]map[string]product.AbstractValue) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		if !stringValueMapEqual(av, b[k]) {
			return false
		}
	}
	return true
}

func symbolBoolMapEqual(a, b map[cfg.SymbolID]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		if av != b[k] {
			return false
		}
	}
	return true
}

func pointBoolMapEqual(a, b map[cfg.Point]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		if av != b[k] {
			return false
		}
	}
	return true
}

func edgeBoolMapEqual(a, b map[edgeKey]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		if av != b[k] {
			return false
		}
	}
	return true
}

func symbolStringMapEqual(a, b map[cfg.SymbolID]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		if av != b[k] {
			return false
		}
	}
	return true
}

func symbolSymbolMapEqual(a, b map[cfg.SymbolID]cfg.SymbolID) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		if av != b[k] {
			return false
		}
	}
	return true
}

func stringStringMapEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		if av != b[k] {
			return false
		}
	}
	return true
}

func symbolSliceEqual(a, b []cfg.SymbolID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func pathEqual(a, b constraint.Path) bool {
	if a.Root != b.Root || a.Symbol != b.Symbol || a.Version != b.Version || len(a.Segments) != len(b.Segments) {
		return false
	}
	for i := range a.Segments {
		if a.Segments[i] != b.Segments[i] {
			return false
		}
	}
	return true
}

func pathsEqual(a, b []constraint.Segment) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func assignmentSourceEqual(a, b AssignmentSource) bool {
	return a.Kind == b.Kind &&
		a.ProjectionKind == b.ProjectionKind &&
		typeEqual(a.ProjectedType, b.ProjectedType) &&
		pathEqual(a.Path, b.Path) &&
		a.IteratorKind == b.IteratorKind &&
		a.VarIndex == b.VarIndex &&
		pathEqual(a.ContainerPath, b.ContainerPath) &&
		pathEqual(a.MapPath, b.MapPath) &&
		a.KeySymbol == b.KeySymbol &&
		a.KeyVar == b.KeyVar &&
		a.Offset == b.Offset &&
		pathEqual(a.CalleePath, b.CalleePath) &&
		pathEqual(a.ReceiverPath, b.ReceiverPath) &&
		a.Method == b.Method &&
		a.ReturnIndex == b.ReturnIndex
}

func assignmentsEqual(a, b []UnifiedAssignment) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Point != b[i].Point ||
			!pathEqual(a[i].TargetPath, b[i].TargetPath) ||
			!typeEqual(a[i].Type, b[i].Type) ||
			!assignmentSourceEqual(a[i].Source, b[i].Source) {
			return false
		}
	}
	return true
}

func constValuesEqual(a, b map[cfg.SymbolID]map[cfg.Point]*ConstValue) bool {
	if len(a) != len(b) {
		return false
	}
	for sym, av := range a {
		if !pointConstMapEqual(av, b[sym]) {
			return false
		}
	}
	return true
}

func pointConstMapEqual(a, b map[cfg.Point]*ConstValue) bool {
	if len(a) != len(b) {
		return false
	}
	for p, av := range a {
		bv := b[p]
		if av == nil || bv == nil {
			if av != nil || bv != nil {
				return false
			}
			continue
		}
		if *av != *bv {
			return false
		}
	}
	return true
}

func conditionEqual(a, b constraint.Condition) bool {
	return a.Equals(b)
}

func edgeConditionsEqual(a, b []EdgeCondition) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].From != b[i].From || a[i].To != b[i].To || !conditionEqual(a[i].Condition, b[i].Condition) {
			return false
		}
	}
	return true
}

func edgeConditionMapEqual(a, b map[edgeKey]constraint.Condition) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		if !conditionEqual(av, b[k]) {
			return false
		}
	}
	return true
}

func pointConditionMapEqual(a, b map[cfg.Point]constraint.Condition) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		if !conditionEqual(av, b[k]) {
			return false
		}
	}
	return true
}

func numericConstraintEqual(a, b constraint.NumericConstraint) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equals(b)
}

func numericConstraintsEqual(a, b []constraint.NumericConstraint) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !numericConstraintEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func edgeNumericConstraintsEqual(a, b []EdgeNumericConstraint) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].From != b[i].From || a[i].To != b[i].To || !numericConstraintsEqual(a[i].Constraints, b[i].Constraints) {
			return false
		}
	}
	return true
}

func edgeNumericConstraintMapEqual(a, b map[edgeKey][]constraint.NumericConstraint) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		if !numericConstraintsEqual(av, b[k]) {
			return false
		}
	}
	return true
}

func typeKeyMapEqual(a, b map[uint64]typ.Type) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		if !typeEqual(av, b[k]) {
			return false
		}
	}
	return true
}

func returnKindMapEqual(a, b map[cfg.Point]ReturnKind) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		if av != b[k] {
			return false
		}
	}
	return true
}

func returnConstraintsEqual(a, b map[cfg.Point]ReturnExprConstraints) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok ||
			!conditionEqual(av.OnReturn, bv.OnReturn) ||
			!conditionEqual(av.OnTrue, bv.OnTrue) ||
			!conditionEqual(av.OnFalse, bv.OnFalse) {
			return false
		}
	}
	return true
}

func predicateLinksEqual(a, b map[string]PredicateLink) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok || !conditionEqual(av.OnTruthy, bv.OnTruthy) || !conditionEqual(av.OnFalsy, bv.OnFalsy) {
			return false
		}
	}
	return true
}

func returnCorrelationsEqual(a, b []ReturnCorrelation) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func guardedCorrelationsEqual(a, b []GuardedTypeCorrelation) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].GuardIndex != b[i].GuardIndex ||
			a[i].TargetIndex != b[i].TargetIndex ||
			a[i].GuardOnTruthy != b[i].GuardOnTruthy ||
			!typeEqual(a[i].TargetType, b[i].TargetType) {
			return false
		}
	}
	return true
}

func siblingAssignmentEqual(a, b *SiblingAssignment) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if !symbolSliceEqual(a.Symbols, b.Symbols) ||
		len(a.Names) != len(b.Names) ||
		len(a.Types) != len(b.Types) ||
		!returnCorrelationsEqual(a.Correlations, b.Correlations) ||
		!returnCorrelationsEqual(a.CoCorrelations, b.CoCorrelations) ||
		!guardedCorrelationsEqual(a.GuardedCorrelations, b.GuardedCorrelations) {
		return false
	}
	for i := range a.Names {
		if a.Names[i] != b.Names[i] {
			return false
		}
	}
	for i := range a.Types {
		if !typeEqual(a.Types[i], b.Types[i]) {
			return false
		}
	}
	return true
}

func siblingAssignmentsEqual(a, b map[SiblingKey]*SiblingAssignment) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		if !siblingAssignmentEqual(av, b[k]) {
			return false
		}
	}
	return true
}

func variantFieldOriginsEqual(a, b []VariantFieldOrigin) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !pathEqual(a[i].Target, b[i].Target) ||
			a[i].Field != b[i].Field ||
			!pathEqual(a[i].Source, b[i].Source) ||
			a[i].DiscriminatorField != b[i].DiscriminatorField ||
			!typeEqual(a[i].DiscriminatorValue, b[i].DiscriminatorValue) {
			return false
		}
	}
	return true
}

func mapMutatorAssignmentsEqual(a, b []MapMutatorAssignment) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Point != b[i].Point ||
			!pathEqual(a[i].Target, b[i].Target) ||
			a[i].ValueMode != b[i].ValueMode ||
			a[i].KeyVar != b[i].KeyVar ||
			a[i].KeySymbol != b[i].KeySymbol ||
			!typeEqual(a[i].KeyType, b[i].KeyType) ||
			!pathEqual(a[i].ValuePath, b[i].ValuePath) ||
			!typeEqual(a[i].ValueType, b[i].ValueType) ||
			!valueTemplateEqual(a[i].Value, b[i].Value) {
			return false
		}
	}
	return true
}

func tableMutatorAssignmentsEqual(a, b []TableMutatorAssignment) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Point != b[i].Point ||
			!pathEqual(a[i].Target, b[i].Target) ||
			a[i].KeyVar != b[i].KeyVar ||
			a[i].KeySymbol != b[i].KeySymbol ||
			!typeEqual(a[i].KeyType, b[i].KeyType) ||
			!pathEqual(a[i].ValuePath, b[i].ValuePath) ||
			!typeEqual(a[i].ValueType, b[i].ValueType) ||
			!valueTemplateEqual(a[i].Value, b[i].Value) {
			return false
		}
	}
	return true
}

func containerMutatorAssignmentsEqual(a, b []ContainerMutatorAssignment) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Point != b[i].Point ||
			!pathEqual(a[i].Target, b[i].Target) ||
			!pathEqual(a[i].ValuePath, b[i].ValuePath) ||
			!typeEqual(a[i].ValueType, b[i].ValueType) ||
			!valueTemplateEqual(a[i].Value, b[i].Value) {
			return false
		}
	}
	return true
}

func valueTemplateEqual(a, b ValueTemplate) bool {
	if len(a.Slots) != len(b.Slots) {
		return false
	}
	for i := range a.Slots {
		if !pathsEqual(a.Slots[i].Segments, b.Slots[i].Segments) ||
			!assignmentSourceEqual(a.Slots[i].Source, b.Slots[i].Source) {
			return false
		}
	}
	return true
}

func numericStateMapEqual(a, b map[cfg.Point]*numeric.State) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv := b[k]
		if av == nil || bv == nil {
			if av != nil || bv != nil {
				return false
			}
			continue
		}
		if !av.Equals(bv) {
			return false
		}
	}
	return true
}
