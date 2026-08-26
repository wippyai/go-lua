package relcompile_test

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// assertApplySlotSources is the census certificate for the Apply ABI. It
// replays only the sealed tuple layout of the closed relational expression
// grammar, then proves every semantic slot reaches the exact child cell whose
// source row owns the signature relation and whose column is the signature
// column. The test intentionally has no nominal ColumnID lookup: that would
// mask the self-join defect this certificate exists to prevent.
func assertApplySlotSources(t testing.TB, compiled plan.ExecutionSchema) {
	t.Helper()
	relations := make(map[model.RelationID]model.RelationSchema, len(compiled.Relations()))
	for _, relation := range compiled.Relations() {
		relations[relation.ID()] = relation
	}
	signatures := make(map[signature.Identity]signature.Signature, len(compiled.Signatures()))
	for _, semantic := range compiled.Signatures() {
		signatures[semantic.Identity()] = semantic
	}
	for _, entry := range compiled.Expressions() {
		_ = assertApplySlotsExpression(t, entry.Expression(), "expression", relations, signatures)
	}
}

type slotSourceShape struct {
	sources []model.RelationID
	cells   []slotSourceCell
}

type slotSourceCell struct {
	relation model.RelationID
	column   model.ColumnID
	source   uint32
}

func assertApplySlotsExpression(t testing.TB, expression algebra.Expression, path string, relations map[model.RelationID]model.RelationSchema, signatures map[signature.Identity]signature.Signature) slotSourceShape {
	t.Helper()
	switch value := expression.(type) {
	case algebra.Input:
		return slotSourceRelationShape(t, value.Relation(), value.Columns(), relations, path)
	case algebra.Select:
		return assertApplySlotsExpression(t, value.Child(), path+".select", relations, signatures)
	case algebra.Complete:
		child := assertApplySlotsExpression(t, value.Child(), path+".complete.child", relations, signatures)
		return completeSlotSourceShape(t, child, value.Denominator(), relations, path+".complete")
	case algebra.Group:
		return assertApplySlotsExpression(t, value.Child(), path+".group", relations, signatures)
	case algebra.ColumnProject:
		child := assertApplySlotsExpression(t, value.Child(), path+".column-project.child", relations, signatures)
		slots := value.Contract().Slots()
		if len(slots) == 0 {
			t.Fatalf("%s: ColumnProject has no slots", path)
		}
		result := slotSourceShape{sources: append([]model.RelationID(nil), child.sources...), cells: make([]slotSourceCell, 0, len(slots))}
		seen := make(map[model.ColumnID]struct{}, len(slots))
		for index, slot := range slots {
			if int(slot.Cell()) >= len(child.cells) {
				t.Fatalf("%s: ColumnProject slot %d cell=%d outside child width %d", path, index, slot.Cell(), len(child.cells))
			}
			if _, duplicate := seen[slot.Column()]; duplicate {
				t.Fatalf("%s: ColumnProject slot %d repeats an output column", path, index)
			}
			cell := child.cells[slot.Cell()]
			if cell.column != slot.Column() {
				t.Fatalf("%s: ColumnProject slot %d does not retain its sealed column", path, index)
			}
			seen[slot.Column()] = struct{}{}
			result.cells = append(result.cells, cell)
		}
		return result
	case algebra.Project:
		_ = assertApplySlotsExpression(t, value.Child(), path+".project.child", relations, signatures)
		return slotSourceRelationShape(t, value.Contract().Target(), nil, relations, path+".project")
	case algebra.Join:
		left := assertApplySlotsExpression(t, value.Left(), path+".join.left", relations, signatures)
		right := assertApplySlotsExpression(t, value.Right(), path+".join.right", relations, signatures)
		return joinSlotSourceShapes(left, right)
	case algebra.Expand:
		// Expand is a dependent keyed extension: the child supplies C rows and
		// the sealed reader relation supplies the appended R row. P is sealed
		// owner evidence, not a runtime slot, so neither it nor its coordinate
		// vector appears in this logical slot certificate.
		child := assertApplySlotsExpression(t, value.Child(), path+".expand.child", relations, signatures)
		reader, ok := relations[value.Contract().Reader()]
		if !ok {
			t.Fatalf("%s: Expand reader relation is not declared", path)
		}
		return joinSlotSourceShapes(child, slotSourceRelationShape(t, reader.ID(), nil, relations, path+".expand.reader"))
	case algebra.Merge:
		inputs := value.Inputs()
		if len(inputs) == 0 {
			t.Fatalf("%s: Merge has no inputs", path)
		}
		result := assertApplySlotsExpression(t, inputs[0], path+".merge[0]", relations, signatures)
		for index := 1; index < len(inputs); index++ {
			other := assertApplySlotsExpression(t, inputs[index], fmt.Sprintf("%s.merge[%d]", path, index), relations, signatures)
			if !sameSlotSourceShape(result, other) {
				t.Fatalf("%s: Merge input %d changes sealed tuple layout", path, index)
			}
		}
		return result
	case algebra.Apply:
		inputs := value.Inputs()
		children := make([]slotSourceShape, len(inputs))
		for index, input := range inputs {
			children[index] = assertApplySlotsExpression(t, input, fmt.Sprintf("%s.apply.child[%d]", path, index), relations, signatures)
		}
		semantic, ok := signatures[value.Contract().Operation()]
		if !ok {
			t.Fatalf("%s: Apply operation has no sealed signature", path)
		}
		slots := value.Contract().SlotSource()
		if len(slots) != semantic.InputLen() {
			t.Fatalf("%s: Apply slots=%d, semantic inputs=%d", path, len(slots), semantic.InputLen())
		}
		for index, source := range slots {
			if int(source.Child()) >= len(children) {
				t.Fatalf("%s: slot %d child=%d outside %d children", path, index, source.Child(), len(children))
			}
			child := children[source.Child()]
			if int(source.Cell()) >= len(child.cells) {
				t.Fatalf("%s: slot %d cell=%d outside child width %d", path, index, source.Cell(), len(child.cells))
			}
			cell := child.cells[source.Cell()]
			if int(cell.source) >= len(child.sources) {
				t.Fatalf("%s: slot %d cell source=%d outside source layout", path, index, cell.source)
			}
			input, inputOK := semantic.InputAt(index)
			if !inputOK || child.sources[cell.source] != input.Relation || cell.relation != input.Relation || cell.column != input.Column {
				t.Fatalf("%s: slot %d does not reference its sealed semantic input", path, index)
			}
		}
		outputs := semantic.Outputs()
		if len(outputs) == 0 {
			t.Fatalf("%s: Apply signature has no output", path)
		}
		return slotSourceOutputShape(t, outputs, path+".apply.output")
	case algebra.Publish:
		return assertApplySlotsExpression(t, value.Child(), path+".publish", relations, signatures)
	default:
		t.Fatalf("%s: unsupported expression %T in slot-source certificate", path, expression)
		return slotSourceShape{}
	}
}

// slotSourceOutputShape mirrors Apply's semantic output vector, which is
// deliberately narrower than the destination relation's physical row when a
// publication writes only one fact column.  Destination address/key authority
// remains in the sealed source layout of a carried row, never fabricated as
// an Apply result cell.
func slotSourceOutputShape(t testing.TB, outputs []signature.Output, path string) slotSourceShape {
	t.Helper()
	if len(outputs) == 0 || !outputs[0].Relation.Available() {
		t.Fatalf("%s: Apply has no available output relation", path)
	}
	relation := outputs[0].Relation
	result := slotSourceShape{sources: []model.RelationID{relation}, cells: make([]slotSourceCell, 0, len(outputs))}
	seen := make(map[model.ColumnID]struct{}, len(outputs))
	for index, output := range outputs {
		if !output.Available() || output.Relation != relation {
			t.Fatalf("%s: Apply output %d does not form one relation row", path, index)
		}
		if _, duplicate := seen[output.Column]; duplicate {
			t.Fatalf("%s: Apply output %d repeats a semantic column", path, index)
		}
		seen[output.Column] = struct{}{}
		result.cells = append(result.cells, slotSourceCell{relation: relation, column: output.Column, source: 0})
	}
	return result
}

func slotSourceRelationShape(t testing.TB, relationID model.RelationID, exactColumns []model.ColumnID, relations map[model.RelationID]model.RelationSchema, path string) slotSourceShape {
	t.Helper()
	relation, ok := relations[relationID]
	if !ok {
		t.Fatalf("%s: relation is not declared", path)
	}
	columns := relation.Columns()
	// Compiler-emitted Inputs carry an occurrence-local exact projection. The
	// certificate must replay that sealed child width instead of silently
	// widening it back to the authored relation row.
	// (Direct algebra specimens still use the full relation shape because their
	// Input is the explicit AllColumns form.)
	if exactColumns != nil {
		columns = exactColumns
	}
	result := slotSourceShape{sources: []model.RelationID{relationID}, cells: make([]slotSourceCell, len(columns))}
	for index, column := range columns {
		result.cells[index] = slotSourceCell{relation: relationID, column: column, source: 0}
	}
	return result
}

func joinSlotSourceShapes(left, right slotSourceShape) slotSourceShape {
	result := slotSourceShape{
		sources: append([]model.RelationID(nil), left.sources...),
		cells:   append([]slotSourceCell(nil), left.cells...),
	}
	offset := uint32(len(result.sources))
	result.sources = append(result.sources, right.sources...)
	for _, cell := range right.cells {
		cell.source += offset
		result.cells = append(result.cells, cell)
	}
	return result
}

// completeSlotSourceShape independently replays Complete's sealed physical
// output law.  A compiler that leaves a sparse Input under Complete will gain
// the denominator cells here; every subsequent Apply SlotSource must point at
// that expanded tuple rather than the pre-Complete projection.
func completeSlotSourceShape(t testing.TB, child slotSourceShape, denominator model.DenominatorRef, relations map[model.RelationID]model.RelationSchema, path string) slotSourceShape {
	t.Helper()
	relation, relationOK := relations[denominator.Relation()]
	if !denominator.Available() || !relationOK {
		t.Fatalf("%s: Complete denominator is not declared", path)
	}
	cells := make([]algebra.CellLayoutCell, len(child.cells))
	for index, cell := range child.cells {
		cells[index] = algebra.NewCellLayoutCell(cell.column, cell.source)
	}
	layout, layoutOK := algebra.NewCellLayout(child.sources, cells)
	if !layoutOK {
		t.Fatalf("%s: Complete child has no canonical cell layout", path)
	}
	completed, completedOK := algebra.CompleteCellLayout(layout, denominator, relation.Columns())
	if !completedOK {
		// Relcompile deliberately preserves a structurally representable but
		// semantically foreign denominator for the independent typing pass to
		// diagnose. Such a specimen has no executable Complete layout yet; it
		// must not make this compiler-only slot census invent one.
		return child
	}
	result := slotSourceShape{sources: completed.Sources(), cells: make([]slotSourceCell, completed.Len())}
	for index := 0; index < completed.Len(); index++ {
		cell, cellOK := completed.CellAt(index)
		if !cellOK {
			t.Fatalf("%s: Complete cell %d is unavailable", path, index)
		}
		source := int(cell.Source())
		if source < 0 || source >= len(result.sources) {
			t.Fatalf("%s: Complete cell %d source=%d outside %d", path, index, cell.Source(), len(result.sources))
		}
		result.cells[index] = slotSourceCell{relation: result.sources[source], column: cell.Column(), source: cell.Source()}
	}
	return result
}

func sameSlotSourceShape(left, right slotSourceShape) bool {
	if len(left.sources) != len(right.sources) || len(left.cells) != len(right.cells) {
		return false
	}
	for index := range left.sources {
		if left.sources[index] != right.sources[index] {
			return false
		}
	}
	for index := range left.cells {
		if left.cells[index] != right.cells[index] {
			return false
		}
	}
	return true
}
