package relationoracle

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
)

type lawFixture struct {
	owner                              model.OwnerID
	left, right, joined                model.RelationID
	leftKey, leftValue                 model.ColumnID
	rightKey, rightValue               model.ColumnID
	projectKey, joinedKey, joinedValue model.ColumnID
	integer                            model.TypeID
	scope, otherScope                  Scope
	registry                           AlgebraRegistry
}

func lawContent(t *testing.T, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("relationoracle-law", []byte(label))
	if !ok {
		t.Fatalf("derive %q", label)
	}
	return value
}

func lawFixtureFor(t *testing.T) lawFixture {
	t.Helper()
	owner, ok := model.IssueOwnerID(lawContent(t, "owner"))
	if !ok {
		t.Fatal("owner")
	}
	issueRelation := func(label string) model.RelationID {
		relation, valid := model.IssueRelationID(owner, lawContent(t, label))
		if !valid {
			t.Fatalf("relation %s", label)
		}
		return relation
	}
	left, right, joined := issueRelation("left"), issueRelation("right"), issueRelation("joined")
	issueColumn := func(relation model.RelationID, label string) model.ColumnID {
		column, valid := model.IssueColumnID(relation, lawContent(t, label))
		if !valid {
			t.Fatalf("column %s", label)
		}
		return column
	}
	integer, ok := model.IssueTypeID(owner, lawContent(t, "integer"))
	if !ok {
		t.Fatal("type")
	}
	scope, ok := NewScope(lawContent(t, "scope"))
	if !ok {
		t.Fatal("scope")
	}
	otherScope, ok := NewScope(lawContent(t, "other-scope"))
	if !ok {
		t.Fatal("other scope")
	}
	entry, ok := NewAlgebraEntry(integer, IdentityAlgebra{})
	if !ok {
		t.Fatal("algebra entry")
	}
	registry, ok := NewAlgebraRegistry([]AlgebraEntry{entry})
	if !ok {
		t.Fatal("algebra registry")
	}
	return lawFixture{
		owner: owner, left: left, right: right, joined: joined,
		leftKey: issueColumn(left, "left/key"), leftValue: issueColumn(left, "left/value"),
		rightKey: issueColumn(right, "right/key"), rightValue: issueColumn(right, "right/value"),
		projectKey: issueColumn(joined, "joined/key"), joinedKey: issueColumn(joined, "joined/join-key"), joinedValue: issueColumn(joined, "joined/value"), integer: integer,
		scope: scope, otherScope: otherScope, registry: registry,
	}
}

func lawRowID(t *testing.T, relation model.RelationID, label string) model.RowID {
	t.Helper()
	row, ok := model.IssueRowID(relation, lawContent(t, "row/"+label))
	if !ok {
		t.Fatalf("row %s", label)
	}
	return row
}

func lawValue(t *testing.T, fixture lawFixture, label string) ValueToken {
	t.Helper()
	value, ok := NewValueToken(fixture.integer, lawContent(t, "value/"+label))
	if !ok {
		t.Fatal("value")
	}
	return value
}

func lawPresent(t *testing.T, column model.ColumnID, typeID model.TypeID, value ValueToken) Cell {
	t.Helper()
	cell, ok := PresentCell(column, typeID, value)
	if !ok {
		t.Fatal("present cell")
	}
	return cell
}

func lawMissing(t *testing.T, column model.ColumnID, typeID model.TypeID) Cell {
	t.Helper()
	cell, ok := MissingCell(column, typeID)
	if !ok {
		t.Fatal("missing cell")
	}
	return cell
}

func lawAbsent(t *testing.T, column model.ColumnID, typeID model.TypeID) Cell {
	t.Helper()
	cell, ok := AbsentCell(column, typeID)
	if !ok {
		t.Fatal("absent cell")
	}
	return cell
}

func lawRow(t *testing.T, id model.RowID, scope Scope, cells ...Cell) Row {
	t.Helper()
	row, ok := NewRow(id, scope, cells)
	if !ok {
		t.Fatal("row")
	}
	return row
}

func lawRelation(t *testing.T, id model.RelationID, rows ...Row) Relation {
	t.Helper()
	relation, ok := NewRelation(id, rows)
	if !ok {
		t.Fatal("relation")
	}
	return relation
}

func mustCell(t *testing.T, relation Relation, rowID model.RowID, column model.ColumnID) Cell {
	t.Helper()
	row, ok := relation.Row(rowID)
	if !ok {
		t.Fatalf("row missing")
	}
	cell, ok := row.Cell(column)
	if !ok {
		t.Fatalf("cell missing")
	}
	return cell
}

func TestLogicalRowsArePermutationInvariantAndImmutable(t *testing.T) {
	fixture := lawFixtureFor(t)
	first := lawRow(t, lawRowID(t, fixture.left, "first"), fixture.scope, lawPresent(t, fixture.leftKey, fixture.integer, lawValue(t, fixture, "first")))
	second := lawRow(t, lawRowID(t, fixture.left, "second"), fixture.otherScope, lawPresent(t, fixture.leftKey, fixture.integer, lawValue(t, fixture, "second")))
	forward := lawRelation(t, fixture.left, first, second)
	reverse := lawRelation(t, fixture.left, second, first)
	if !reflect.DeepEqual(forward.Rows(), reverse.Rows()) {
		t.Fatal("row permutation changed canonical relation")
	}
	rows := forward.Rows()
	rows[0] = Row{}
	if !forward.Rows()[0].Available() {
		t.Fatal("relation exposed mutable row slice")
	}
	cells := forward.Rows()[0].Cells()
	cells[0] = Cell{}
	if !forward.Rows()[0].Cells()[0].Available() {
		t.Fatal("row exposed mutable cell slice")
	}
}

func TestExactScopeConjunctionIsCommutativeAndIdempotent(t *testing.T) {
	fixture := lawFixtureFor(t)
	exact := ExactScope{}
	left := exact.Conjoin(fixture.scope, fixture.otherScope)
	right := exact.Conjoin(fixture.otherScope, fixture.scope)
	if !left.Equal(right) {
		t.Fatal("scope conjunction depends on operand order")
	}
	if !exact.Conjoin(fixture.scope, fixture.scope).Equal(fixture.scope) {
		t.Fatal("scope conjunction is not idempotent")
	}
}

func TestPresenceDoesNotCollapseDefaultMissingAndProvenAbsence(t *testing.T) {
	fixture := lawFixtureFor(t)
	presentID := lawRowID(t, fixture.left, "present")
	missingID := lawRowID(t, fixture.left, "missing")
	absentID := lawRowID(t, fixture.left, "absent")
	present := lawRow(t, presentID, fixture.scope, lawPresent(t, fixture.leftValue, fixture.integer, lawValue(t, fixture, "default")))
	missing := lawRow(t, missingID, fixture.scope, lawMissing(t, fixture.leftValue, fixture.integer))
	absent := lawRow(t, absentID, fixture.scope, lawAbsent(t, fixture.leftValue, fixture.integer))
	relation := lawRelation(t, fixture.left, present, missing, absent)
	for _, testCase := range []struct {
		id   model.RowID
		want model.PresenceKind
	}{{presentID, model.Present}, {missingID, model.UnprovenMissing}, {absentID, model.ProvenAbsent}} {
		cell := mustCell(t, relation, testCase.id, fixture.leftValue)
		if !cell.Presence().Is(testCase.want) {
			t.Fatalf("presence = %s, want %s", cell.Presence().Kind(), testCase.want)
		}
	}
	if _, ok := mustCell(t, relation, missingID, fixture.leftValue).Value(); ok {
		t.Fatal("unproven missing exposed a value")
	}
}

func TestRowsRejectForeignColumnCoordinates(t *testing.T) {
	fixture := lawFixtureFor(t)
	rowID := lawRowID(t, fixture.left, "foreign-column")
	cell := lawPresent(t, fixture.rightKey, fixture.integer, lawValue(t, fixture, "foreign"))
	if _, ok := NewRow(rowID, fixture.scope, []Cell{cell}); ok {
		t.Fatal("row accepted a column from another logical relation")
	}
}

func TestClosedOperatorsUseOnlyLogicalRows(t *testing.T) {
	fixture := lawFixtureFor(t)
	leftID := lawRowID(t, fixture.left, "left")
	rightID := lawRowID(t, fixture.right, "right")
	leftKeyValue := lawValue(t, fixture, "join-key")
	left := lawRelation(t, fixture.left, lawRow(t, leftID, fixture.scope,
		lawPresent(t, fixture.leftKey, fixture.integer, leftKeyValue),
		lawPresent(t, fixture.leftValue, fixture.integer, lawValue(t, fixture, "left-value"))))
	right := lawRelation(t, fixture.right, lawRow(t, rightID, fixture.scope,
		lawPresent(t, fixture.rightKey, fixture.integer, leftKeyValue),
		lawPresent(t, fixture.rightValue, fixture.integer, lawValue(t, fixture, "right-value"))))
	selected := SelectByScope(left, fixture.scope, ExactScope{})
	if len(selected.Rows()) != 1 {
		t.Fatalf("select returned %d rows", len(selected.Rows()))
	}
	projected := Project(left, NewProjectSpec(fixture.joined, []ColumnMapping{NewColumnMapping(fixture.leftKey, fixture.projectKey, fixture.integer)}))
	if len(projected.Rows()) != 1 {
		t.Fatalf("project returned %d rows", len(projected.Rows()))
	}
	leftMappings := []ColumnMapping{
		NewColumnMapping(fixture.leftKey, fixture.joinedKey, fixture.integer),
		NewColumnMapping(fixture.leftValue, fixture.joinedValue, fixture.integer),
	}
	rightMappings := []ColumnMapping{
		NewColumnMapping(fixture.rightKey, fixture.joinedKey, fixture.integer),
		NewColumnMapping(fixture.rightValue, fixture.joinedValue, fixture.integer),
	}
	joinSpec := NewJoinSpec(fixture.joined, []model.ColumnID{fixture.leftKey}, []model.ColumnID{fixture.rightKey}, leftMappings, rightMappings, fixture.registry, ExactScope{})
	joined := Join(left, right, joinSpec)
	if len(joined.Rows()) != 1 {
		t.Fatalf("join returned %d rows", len(joined.Rows()))
	}
	merged := Merge([]Relation{left, left}, fixture.registry)
	mergedValue, mergedOK := mustCell(t, merged, leftID, fixture.leftValue).Value()
	if !mergedOK || !mergedValue.Equal(lawValue(t, fixture, "left-value")) {
		t.Fatal("identity merge changed one value")
	}
	groups := GroupByRowID(left)
	if len(groups) != 1 || len(groups[0].Rows()) != 1 {
		t.Fatalf("group shape = %d/%d", len(groups), len(groups[0].Rows()))
	}
	entry, ok := NewDenominatorEntry(leftID, fixture.scope)
	if !ok {
		t.Fatal("entry")
	}
	missingID := lawRowID(t, fixture.left, "denominator-missing")
	missingEntry, ok := NewDenominatorEntry(missingID, fixture.otherScope)
	if !ok {
		t.Fatal("missing entry")
	}
	denominator, ok := NewDenominator(fixture.left, []DenominatorEntry{entry, missingEntry})
	if !ok {
		t.Fatal("denominator")
	}
	completed := Complete(left, denominator, []ColumnType{NewColumnType(fixture.leftKey, fixture.integer), NewColumnType(fixture.leftValue, fixture.integer)})
	if !mustCell(t, completed, missingID, fixture.leftValue).Presence().Is(model.ProvenAbsent) {
		t.Fatal("complete did not materialize proven absence")
	}
	produced, _ := outcome.NewResult(outcome.Produced, model.RefusalID{})
	noSelection, _ := outcome.NewResult(outcome.NoSelection, model.RefusalID{})
	applied := Apply(left, fixture.joined, JudgmentFunc(func(row Row) ApplyResult {
		if row.id == leftID {
			return ApplyResult{Outcome: produced, Cells: []Cell{
				lawPresent(t, fixture.joinedKey, fixture.integer, leftKeyValue),
				lawPresent(t, fixture.joinedValue, fixture.integer, lawValue(t, fixture, "left-value")),
			}}
		}
		return ApplyResult{Outcome: noSelection}
	}))
	if !applied.Available() || len(applied.Rows()) != 1 || len(applied.Outcomes()) != 1 {
		t.Fatalf("apply shape invalid")
	}
	published := Publish(left, left, fixture.registry)
	publishedValue, publishedOK := mustCell(t, published, leftID, fixture.leftValue).Value()
	if !publishedOK || !publishedValue.Equal(lawValue(t, fixture, "left-value")) {
		t.Fatal("publish changed value")
	}
}
