package relationoracle

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
)

// ColumnMapping is a logical source-to-output rename. Type is optional when
// present input cells can provide it; it is required to materialize a missing
// source cell in Project or Join.
type ColumnMapping struct {
	source model.ColumnID
	target model.ColumnID
	typeID model.TypeID
}

// NewColumnMapping requires the target TypeID because a wholly absent source
// row cannot otherwise establish a heterogeneous output type.
func NewColumnMapping(source, target model.ColumnID, typeID model.TypeID) ColumnMapping {
	return ColumnMapping{source: source, target: target, typeID: typeID}
}

func (mapping ColumnMapping) Source() model.ColumnID { return mapping.source }
func (mapping ColumnMapping) Target() model.ColumnID { return mapping.target }
func (mapping ColumnMapping) Type() model.TypeID     { return mapping.typeID }

type ProjectSpec struct {
	destination model.RelationID
	mappings    []ColumnMapping
}

func NewProjectSpec(destination model.RelationID, mappings []ColumnMapping) ProjectSpec {
	return ProjectSpec{destination: destination, mappings: append([]ColumnMapping(nil), mappings...)}
}
func (spec ProjectSpec) Destination() model.RelationID { return spec.destination }
func (spec ProjectSpec) Mappings() []ColumnMapping {
	return append([]ColumnMapping(nil), spec.mappings...)
}

// Project is the logical projection/rename operator. Missing source columns
// stay UnprovenMissing, never Present(default); target types are retained in
// every output cell.
func Project(input Relation, spec ProjectSpec) Relation {
	if !input.Available() || !spec.destination.Available() || !validMappings(spec.mappings) {
		return Relation{}
	}
	rows := make([]Row, 0, len(input.rows))
	for _, source := range input.rows {
		cells := make([]Cell, 0, len(spec.mappings))
		for _, mapping := range spec.mappings {
			cell, ok := projectCell(source, mapping)
			if !ok {
				return Relation{}
			}
			cells = append(cells, cell)
		}
		id, ok := deriveRowID(spec.destination, "internal/relationoracle/project-row/v1", source.id)
		if !ok {
			return Relation{}
		}
		row, ok := NewRow(id, source.scope, cells)
		if !ok {
			return Relation{}
		}
		rows = append(rows, row)
	}
	result, ok := NewRelation(spec.destination, rows)
	if !ok {
		return Relation{}
	}
	return result
}

func validMappings(mappings []ColumnMapping) bool {
	seen := make(map[model.ColumnID]struct{}, len(mappings))
	for _, mapping := range mappings {
		if !mapping.source.Available() || !mapping.target.Available() {
			return false
		}
		if _, duplicate := seen[mapping.target]; duplicate {
			return false
		}
		seen[mapping.target] = struct{}{}
		if !mapping.typeID.Available() {
			return false
		}
	}
	return true
}

func projectCell(source Row, mapping ColumnMapping) (Cell, bool) {
	cell, found := source.Cell(mapping.source)
	if !found {
		if !mapping.typeID.Available() {
			return Cell{}, false
		}
		return MissingCell(mapping.target, mapping.typeID)
	}
	typeID := cell.typeID
	if mapping.typeID.Available() {
		typeID = mapping.typeID
	}
	if cell.value.Available() {
		if typeID != cell.value.typeID {
			return Cell{}, false
		}
		return NewCell(mapping.target, typeID, cell.value, cell.presence)
	}
	return NewCell(mapping.target, typeID, ValueToken{}, cell.presence)
}

// JoinSpec describes one oriented generic equijoin. Optional mappings make
// output columns explicit; without mappings source column identities remain.
type JoinSpec struct {
	destination   model.RelationID
	leftColumns   []model.ColumnID
	rightColumns  []model.ColumnID
	leftMappings  []ColumnMapping
	rightMappings []ColumnMapping
	scopes        ScopeAlgebra
	algebras      AlgebraRegistry
}

func NewJoinSpec(destination model.RelationID, leftColumns, rightColumns []model.ColumnID, leftMappings, rightMappings []ColumnMapping, algebras AlgebraRegistry, scopes ScopeAlgebra) JoinSpec {
	return JoinSpec{
		destination: destination,
		leftColumns: append([]model.ColumnID(nil), leftColumns...), rightColumns: append([]model.ColumnID(nil), rightColumns...),
		leftMappings: append([]ColumnMapping(nil), leftMappings...), rightMappings: append([]ColumnMapping(nil), rightMappings...),
		algebras: algebras, scopes: scopes,
	}
}

func (spec JoinSpec) Destination() model.RelationID { return spec.destination }
func (spec JoinSpec) LeftColumns() []model.ColumnID {
	return append([]model.ColumnID(nil), spec.leftColumns...)
}
func (spec JoinSpec) RightColumns() []model.ColumnID {
	return append([]model.ColumnID(nil), spec.rightColumns...)
}

// Join is deliberately a nested-loop logical equijoin. It has no physical
// arrangement, local ordinal, or storage coordinate.
func Join(left, right Relation, spec JoinSpec) Relation {
	if !left.Available() || !right.Available() || !spec.destination.Available() || !validJoinSpec(spec) {
		return Relation{}
	}
	scopeAlgebra := spec.scopes
	if scopeAlgebra == nil {
		scopeAlgebra = ExactScope{}
	}
	rows := make([]Row, 0)
	for _, leftRow := range left.rows {
		for _, rightRow := range right.rows {
			if !joinKeysEqual(leftRow, rightRow, spec) {
				continue
			}
			scope := scopeAlgebra.Conjoin(leftRow.scope, rightRow.scope)
			if !scope.Available() {
				return Relation{}
			}
			cells, ok := joinedCells(leftRow, rightRow, spec)
			if !ok {
				return Relation{}
			}
			id, ok := deriveRowID(spec.destination, "internal/relationoracle/join-row/v1", leftRow.id, rightRow.id)
			if !ok {
				return Relation{}
			}
			row, ok := NewRow(id, scope, cells)
			if !ok {
				return Relation{}
			}
			rows = append(rows, row)
		}
	}
	result, ok := NewRelation(spec.destination, rows)
	if !ok {
		return Relation{}
	}
	return result
}

func validJoinSpec(spec JoinSpec) bool {
	if len(spec.leftColumns) == 0 || len(spec.leftColumns) != len(spec.rightColumns) || !spec.algebras.Available() {
		return false
	}
	for index := range spec.leftColumns {
		if !spec.leftColumns[index].Available() || !spec.rightColumns[index].Available() {
			return false
		}
	}
	return (len(spec.leftMappings) == 0 || validMappings(spec.leftMappings)) && (len(spec.rightMappings) == 0 || validMappings(spec.rightMappings))
}

func joinKeysEqual(left, right Row, spec JoinSpec) bool {
	for index := range spec.leftColumns {
		leftCell, leftOK := left.Cell(spec.leftColumns[index])
		rightCell, rightOK := right.Cell(spec.rightColumns[index])
		if !leftOK || !rightOK || !leftCell.presence.Is(model.Present) || !rightCell.presence.Is(model.Present) || leftCell.typeID != rightCell.typeID {
			return false
		}
		algebra, ok := spec.algebras.Lookup(leftCell.typeID)
		if !ok {
			return false
		}
		if !algebra.Equal(leftCell.value, rightCell.value) {
			return false
		}
	}
	return true
}

func joinedCells(left, right Row, spec JoinSpec) ([]Cell, bool) {
	if len(spec.leftMappings) == 0 && len(spec.rightMappings) == 0 {
		cells := append(left.Cells(), right.Cells()...)
		return deduplicateJoinedCells(cells)
	}
	cells := make([]Cell, 0, len(spec.leftMappings)+len(spec.rightMappings))
	for _, mapping := range append(append([]ColumnMapping(nil), spec.leftMappings...), spec.rightMappings...) {
		source := left
		if !containsMapping(spec.leftMappings, mapping) {
			source = right
		}
		cell, ok := projectCell(source, mapping)
		if !ok {
			return nil, false
		}
		cells = append(cells, cell)
	}
	return deduplicateJoinedCells(cells)
}

func containsMapping(mappings []ColumnMapping, target ColumnMapping) bool {
	for _, mapping := range mappings {
		if mapping == target {
			return true
		}
	}
	return false
}

func deduplicateJoinedCells(cells []Cell) ([]Cell, bool) {
	sort.SliceStable(cells, func(left, right int) bool { return columnLess(cells[left].column, cells[right].column) })
	result := cells[:0]
	for _, cell := range cells {
		if len(result) != 0 && result[len(result)-1].column == cell.column {
			continue
		}
		result = append(result, cell)
	}
	return result, true
}

// Merge combines alternatives by logical row ID. Each column's TypeID is
// resolved independently through the registry; values of different types
// therefore coexist without a caller-defined sum type.
func Merge(inputs []Relation, algebras AlgebraRegistry) Relation {
	if len(inputs) == 0 || !algebras.Available() || !inputs[0].Available() {
		return Relation{}
	}
	relationID := inputs[0].id
	byID := make(map[model.RowID][]Row)
	for _, input := range inputs {
		if !input.Available() || input.id != relationID {
			return Relation{}
		}
		for _, row := range input.rows {
			byID[row.id] = append(byID[row.id], row)
		}
	}
	ids := make([]model.RowID, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return rowIDLess(ids[left], ids[right]) })
	rows := make([]Row, 0, len(ids))
	for _, id := range ids {
		row, ok := mergeRows(byID[id], algebras)
		if !ok {
			return Relation{}
		}
		rows = append(rows, row)
	}
	result, ok := NewRelation(relationID, rows)
	if !ok {
		return Relation{}
	}
	return result
}

func mergeRows(rows []Row, algebras AlgebraRegistry) (Row, bool) {
	if len(rows) == 0 {
		return Row{}, false
	}
	scope := rows[0].scope
	for _, row := range rows[1:] {
		if compareScope(row.scope, scope) < 0 {
			scope = row.scope
		}
	}
	columns := make(map[model.ColumnID][]Cell)
	for _, row := range rows {
		for _, cell := range row.cells {
			columns[cell.column] = append(columns[cell.column], cell)
		}
	}
	columnIDs := make([]model.ColumnID, 0, len(columns))
	for column := range columns {
		columnIDs = append(columnIDs, column)
	}
	sort.Slice(columnIDs, func(left, right int) bool { return columnLess(columnIDs[left], columnIDs[right]) })
	cells := make([]Cell, 0, len(columnIDs))
	for _, column := range columnIDs {
		cell, ok := mergeCells(columns[column], algebras)
		if !ok {
			return Row{}, false
		}
		cells = append(cells, cell)
	}
	return NewRow(rows[0].id, scope, cells)
}

func mergeCells(cells []Cell, algebras AlgebraRegistry) (Cell, bool) {
	if len(cells) == 0 {
		return Cell{}, false
	}
	typeID := cells[0].typeID
	for _, cell := range cells[1:] {
		if cell.typeID != typeID {
			return Cell{}, false
		}
	}
	var value ValueToken
	present, opaque, provenAbsent, missing := false, false, false, false
	for _, cell := range cells {
		switch {
		case cell.presence.Is(model.Present), cell.presence.Is(model.AuthenticatedOpaque):
			cellValue, ok := cell.Value()
			if !ok {
				return Cell{}, false
			}
			if !present && !opaque {
				value = cellValue
				present = cell.presence.Is(model.Present)
				opaque = cell.presence.Is(model.AuthenticatedOpaque)
			} else {
				algebra, ok := algebras.Lookup(typeID)
				if !ok {
					return Cell{}, false
				}
				value = algebra.Join(value, cellValue)
				if !value.Available() || value.typeID != typeID {
					return Cell{}, false
				}
				present = true
				opaque = false
			}
		case cell.presence.Is(model.ProvenAbsent):
			provenAbsent = true
		case cell.presence.Is(model.UnprovenMissing):
			missing = true
		}
	}
	if present {
		return NewCell(cells[0].column, typeID, value, presence(model.Present))
	}
	if opaque {
		return NewCell(cells[0].column, typeID, value, presence(model.AuthenticatedOpaque))
	}
	if provenAbsent {
		return NewCell(cells[0].column, typeID, ValueToken{}, presence(model.ProvenAbsent))
	}
	if missing {
		return NewCell(cells[0].column, typeID, ValueToken{}, presence(model.UnprovenMissing))
	}
	return cells[0], true
}

func presence(kind model.PresenceKind) model.Presence {
	value, ok := model.NewPresence(kind)
	if !ok {
		panic("relationoracle: invalid presence")
	}
	return value
}

func compareScope(left, right Scope) int {
	if left.Equal(right) {
		return 0
	}
	leftID, rightID := left.Formula(), right.Formula()
	return bytes.Compare(leftID[:], rightID[:])
}

// Grouped is an immutable logical group keyed by an opaque content identity.
type Grouped struct {
	key   identity.ContentID
	rows  []Row
	valid bool
}

func (group Grouped) Available() bool {
	return group.valid && group.key.Available() && group.rows != nil
}
func (group Grouped) Key() identity.ContentID { return group.key }
func (group Grouped) Rows() []Row             { return append([]Row(nil), group.rows...) }

// GroupBy forms immutable groups using an injected logical key extractor. The
// extractor is invoked immediately and is not stored in the result.
func GroupBy(input Relation, key func(Row) identity.ContentID) []Grouped {
	if !input.Available() || key == nil {
		return nil
	}
	byKey := make(map[identity.ContentID][]Row)
	for _, row := range input.rows {
		groupKey := key(row)
		if !groupKey.Available() {
			return nil
		}
		byKey[groupKey] = append(byKey[groupKey], row)
	}
	keys := make([]identity.ContentID, 0, len(byKey))
	for groupKey := range byKey {
		keys = append(keys, groupKey)
	}
	identity.SortContentIDs(keys)
	groups := make([]Grouped, 0, len(keys))
	for _, groupKey := range keys {
		rows := append([]Row(nil), byKey[groupKey]...)
		sort.Slice(rows, func(left, right int) bool { return rowLess(rows[left], rows[right]) })
		groups = append(groups, Grouped{key: groupKey, rows: rows, valid: true})
	}
	return groups
}

func GroupByRowID(input Relation) []Grouped {
	return GroupBy(input, func(row Row) identity.ContentID { return row.id.Content() })
}

// ApplyResult is one bounded injected semantic judgment result. Refusal and
// all other terminal outcomes stay in outcome.Result, never in Cell.Presence.
type ApplyResult struct {
	Outcome outcome.Result
	Cells   []Cell
}

type Judgment interface {
	Judge(row Row) ApplyResult
}

type JudgmentFunc func(row Row) ApplyResult

func (fn JudgmentFunc) Judge(row Row) ApplyResult { return fn(row) }

type Invocation struct {
	input   model.RowID
	outcome outcome.Result
	valid   bool
}

func (invocation Invocation) Available() bool {
	return invocation.valid && invocation.input.Available() && invocation.outcome.Available()
}
func (invocation Invocation) Input() model.RowID      { return invocation.input }
func (invocation Invocation) Outcome() outcome.Result { return invocation.outcome }

type Applied struct {
	relation Relation
	outcomes []Invocation
	valid    bool
}

func (applied Applied) Available() bool        { return applied.valid && applied.relation.Available() }
func (applied Applied) Relation() Relation     { return applied.relation }
func (applied Applied) Rows() []Row            { return applied.relation.Rows() }
func (applied Applied) Outcomes() []Invocation { return append([]Invocation(nil), applied.outcomes...) }

func Apply(input Relation, destination model.RelationID, judgment Judgment) Applied {
	if !input.Available() || !destination.Available() || judgment == nil {
		return Applied{}
	}
	rows := make([]Row, 0)
	outcomes := make([]Invocation, 0, len(input.rows))
	for _, row := range input.rows {
		result := judgment.Judge(row)
		if !result.Outcome.Available() {
			return Applied{}
		}
		outcomes = append(outcomes, Invocation{input: row.id, outcome: result.Outcome, valid: true})
		if result.Outcome.Code != outcome.Produced {
			if len(result.Cells) != 0 {
				return Applied{}
			}
			continue
		}
		id, ok := deriveRowID(destination, "internal/relationoracle/apply-row/v1", row.id)
		if !ok {
			return Applied{}
		}
		output, ok := NewRow(id, row.scope, result.Cells)
		if !ok {
			return Applied{}
		}
		rows = append(rows, output)
	}
	relation, ok := NewRelation(destination, rows)
	if !ok {
		return Applied{}
	}
	return Applied{relation: relation, outcomes: outcomes, valid: true}
}

func Publish(destination, proposals Relation, algebras AlgebraRegistry) Relation {
	if !destination.Available() || !proposals.Available() || destination.id != proposals.id {
		return Relation{}
	}
	return Merge([]Relation{destination, proposals}, algebras)
}
