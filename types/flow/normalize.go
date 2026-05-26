package flow

import (
	"sort"

	"github.com/wippyai/go-lua/types/constraint"
)

// Normalize enforces deterministic ordering for slice-based inputs.
//
// This does not change semantics; it only stabilizes iteration order for
// reproducible analysis and diagnostics.
func (in *Inputs) Normalize() {
	if in == nil {
		return
	}
	in.normalizeAssignments()
	in.normalizeEdgeConditions()
	in.normalizeEdgeNumericConstraints()
	in.normalizeVariantFieldOrigins()
	in.normalizeMapMutatorAssignments()
	in.normalizeTableMutatorAssignments()
	in.normalizeContainerMutatorAssignments()
}

func (in *Inputs) normalizeAssignments() {
	if len(in.Assignments) > 1 {
		sort.Slice(in.Assignments, func(i, j int) bool {
			return assignmentLess(in.Assignments[i], in.Assignments[j])
		})
	}
}

func (in *Inputs) normalizeEdgeConditions() {
	if len(in.EdgeConditions) > 1 {
		sort.Slice(in.EdgeConditions, func(i, j int) bool {
			return edgeConditionLess(in.EdgeConditions[i], in.EdgeConditions[j])
		})
	}
}

func (in *Inputs) normalizeEdgeNumericConstraints() {
	if len(in.EdgeNumericConstraints) > 1 {
		sort.Slice(in.EdgeNumericConstraints, func(i, j int) bool {
			return edgeNumericConstraintLess(in.EdgeNumericConstraints[i], in.EdgeNumericConstraints[j])
		})
	}
}

func (in *Inputs) normalizeVariantFieldOrigins() {
	if len(in.VariantFieldOrigins) > 1 {
		sort.Slice(in.VariantFieldOrigins, func(i, j int) bool {
			return variantFieldOriginLess(in.VariantFieldOrigins[i], in.VariantFieldOrigins[j])
		})
	}
}

func (in *Inputs) normalizeMapMutatorAssignments() {
	if len(in.MapMutatorAssignments) > 1 {
		sort.Slice(in.MapMutatorAssignments, func(i, j int) bool {
			return mapMutatorAssignmentLess(in.MapMutatorAssignments[i], in.MapMutatorAssignments[j])
		})
	}
}

func (in *Inputs) normalizeTableMutatorAssignments() {
	if len(in.TableMutatorAssignments) > 1 {
		sort.Slice(in.TableMutatorAssignments, func(i, j int) bool {
			return tableMutatorAssignmentLess(in.TableMutatorAssignments[i], in.TableMutatorAssignments[j])
		})
	}
}

func (in *Inputs) normalizeContainerMutatorAssignments() {
	if len(in.ContainerMutatorAssignments) > 1 {
		sort.Slice(in.ContainerMutatorAssignments, func(i, j int) bool {
			return containerMutatorAssignmentLess(in.ContainerMutatorAssignments[i], in.ContainerMutatorAssignments[j])
		})
	}
}

func assignmentLess(a, b UnifiedAssignment) bool {
	if a.Point != b.Point {
		return a.Point < b.Point
	}
	if pathLess(a.TargetPath, b.TargetPath) {
		return true
	}
	if pathLess(b.TargetPath, a.TargetPath) {
		return false
	}
	return assignmentSourceLess(a.Source, b.Source)
}

func edgeConditionLess(a, b EdgeCondition) bool {
	if a.From != b.From {
		return a.From < b.From
	}
	if a.To != b.To {
		return a.To < b.To
	}
	return false
}

func edgeNumericConstraintLess(a, b EdgeNumericConstraint) bool {
	if a.From != b.From {
		return a.From < b.From
	}
	if a.To != b.To {
		return a.To < b.To
	}
	return false
}

func variantFieldOriginLess(a, b VariantFieldOrigin) bool {
	if pathLess(a.Target, b.Target) {
		return true
	}
	if pathLess(b.Target, a.Target) {
		return false
	}
	if a.Field != b.Field {
		return a.Field < b.Field
	}
	if pathLess(a.Source, b.Source) {
		return true
	}
	if pathLess(b.Source, a.Source) {
		return false
	}
	if a.DiscriminatorField != b.DiscriminatorField {
		return a.DiscriminatorField < b.DiscriminatorField
	}
	if a.DiscriminatorValue == nil || b.DiscriminatorValue == nil {
		return a.DiscriminatorValue != nil
	}
	return a.DiscriminatorValue.Hash() < b.DiscriminatorValue.Hash()
}

func mapMutatorAssignmentLess(a, b MapMutatorAssignment) bool {
	if a.Point != b.Point {
		return a.Point < b.Point
	}
	if pathLess(a.Target, b.Target) {
		return true
	}
	if pathLess(b.Target, a.Target) {
		return false
	}
	if a.ValueMode != b.ValueMode {
		return a.ValueMode < b.ValueMode
	}
	if a.KeySymbol != b.KeySymbol {
		return a.KeySymbol < b.KeySymbol
	}
	if a.KeyVar != b.KeyVar {
		return a.KeyVar < b.KeyVar
	}
	if pathLess(a.ValuePath, b.ValuePath) {
		return true
	}
	if pathLess(b.ValuePath, a.ValuePath) {
		return false
	}
	return valueTemplateLess(a.Value, b.Value)
}

func tableMutatorAssignmentLess(a, b TableMutatorAssignment) bool {
	if a.Point != b.Point {
		return a.Point < b.Point
	}
	if pathLess(a.Target, b.Target) {
		return true
	}
	if pathLess(b.Target, a.Target) {
		return false
	}
	if a.KeySymbol != b.KeySymbol {
		return a.KeySymbol < b.KeySymbol
	}
	if a.KeyVar != b.KeyVar {
		return a.KeyVar < b.KeyVar
	}
	if pathLess(a.ValuePath, b.ValuePath) {
		return true
	}
	if pathLess(b.ValuePath, a.ValuePath) {
		return false
	}
	return valueTemplateLess(a.Value, b.Value)
}

func containerMutatorAssignmentLess(a, b ContainerMutatorAssignment) bool {
	if a.Point != b.Point {
		return a.Point < b.Point
	}
	if pathLess(a.Target, b.Target) {
		return true
	}
	if pathLess(b.Target, a.Target) {
		return false
	}
	if pathLess(a.ValuePath, b.ValuePath) {
		return true
	}
	if pathLess(b.ValuePath, a.ValuePath) {
		return false
	}
	return valueTemplateLess(a.Value, b.Value)
}

func valueTemplateLess(a, b ValueTemplate) bool {
	if len(a.Slots) != len(b.Slots) {
		return len(a.Slots) < len(b.Slots)
	}
	for i := range a.Slots {
		if segmentsLess(a.Slots[i].Segments, b.Slots[i].Segments) {
			return true
		}
		if segmentsLess(b.Slots[i].Segments, a.Slots[i].Segments) {
			return false
		}
		if assignmentSourceLess(a.Slots[i].Source, b.Slots[i].Source) {
			return true
		}
		if assignmentSourceLess(b.Slots[i].Source, a.Slots[i].Source) {
			return false
		}
	}
	return false
}

func assignmentSourceLess(a, b AssignmentSource) bool {
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	if pathLess(a.Path, b.Path) {
		return true
	}
	if pathLess(b.Path, a.Path) {
		return false
	}
	if pathLess(a.ContainerPath, b.ContainerPath) {
		return true
	}
	if pathLess(b.ContainerPath, a.ContainerPath) {
		return false
	}
	if pathLess(a.MapPath, b.MapPath) {
		return true
	}
	if pathLess(b.MapPath, a.MapPath) {
		return false
	}
	if pathLess(a.CalleePath, b.CalleePath) {
		return true
	}
	if pathLess(b.CalleePath, a.CalleePath) {
		return false
	}
	if pathLess(a.ReceiverPath, b.ReceiverPath) {
		return true
	}
	if pathLess(b.ReceiverPath, a.ReceiverPath) {
		return false
	}
	if a.IteratorKind != b.IteratorKind {
		return a.IteratorKind < b.IteratorKind
	}
	if a.VarIndex != b.VarIndex {
		return a.VarIndex < b.VarIndex
	}
	if a.KeySymbol != b.KeySymbol {
		return a.KeySymbol < b.KeySymbol
	}
	if a.KeyVar != b.KeyVar {
		return a.KeyVar < b.KeyVar
	}
	if a.Offset != b.Offset {
		return a.Offset < b.Offset
	}
	if a.Method != b.Method {
		return a.Method < b.Method
	}
	return a.ReturnIndex < b.ReturnIndex
}

func pathLess(a, b constraint.Path) bool {
	if a.Symbol != b.Symbol {
		return a.Symbol < b.Symbol
	}
	if a.Root != b.Root {
		return a.Root < b.Root
	}
	if a.Version != b.Version {
		return a.Version < b.Version
	}
	return segmentsLess(a.Segments, b.Segments)
}

func segmentsLess(a, b []constraint.Segment) bool {
	min := len(a)
	if len(b) < min {
		min = len(b)
	}
	for i := 0; i < min; i++ {
		if a[i].Kind != b[i].Kind {
			return a[i].Kind < b[i].Kind
		}
		if a[i].Name != b[i].Name {
			return a[i].Name < b[i].Name
		}
		if a[i].Index != b[i].Index {
			return a[i].Index < b[i].Index
		}
	}
	return len(a) < len(b)
}
