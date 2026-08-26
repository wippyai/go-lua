package algebra_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// TestCompleteCellLayoutClosesExactlyOnce pins the output-coordinate law used
// by lowering, checking, and mount. In particular, Complete may add its
// missing denominator cells once, but a later Join must see those cells in
// the same physical order rather than an unmodelled runtime extension.
func TestCompleteCellLayoutClosesExactlyOnce(t *testing.T) {
	owner, ok := model.IssueOwnerID(identity.ContentID{0x91})
	if !ok {
		t.Fatal("owner")
	}
	leftRelation, ok := model.IssueRelationID(owner, identity.ContentID{0x92})
	if !ok {
		t.Fatal("left relation")
	}
	rightRelation, ok := model.IssueRelationID(owner, identity.ContentID{0x93})
	if !ok {
		t.Fatal("right relation")
	}
	targetRelation, ok := model.IssueRelationID(owner, identity.ContentID{0x94})
	if !ok {
		t.Fatal("target relation")
	}
	leftKey, ok := model.IssueKeyID(leftRelation, identity.ContentID{0x95})
	if !ok {
		t.Fatal("left key")
	}
	leftLink, ok := model.IssueColumnID(leftRelation, identity.ContentID{0x96})
	if !ok {
		t.Fatal("left link")
	}
	leftValue, ok := model.IssueColumnID(leftRelation, identity.ContentID{0x97})
	if !ok {
		t.Fatal("left value")
	}
	rightValue, ok := model.IssueColumnID(rightRelation, identity.ContentID{0x98})
	if !ok {
		t.Fatal("right value")
	}
	targetValue, ok := model.IssueColumnID(targetRelation, identity.ContentID{0x99})
	if !ok {
		t.Fatal("target value")
	}
	denominator, ok := model.NewDenominatorRef(leftRelation, leftKey)
	if !ok {
		t.Fatal("denominator")
	}

	sparse, ok := algebra.InputCellLayout(leftRelation, []model.ColumnID{leftLink})
	if !ok {
		t.Fatal("sparse input")
	}
	completed, ok := algebra.CompleteCellLayout(sparse, denominator, []model.ColumnID{leftLink, leftValue})
	if !ok || completed.Len() != 2 {
		t.Fatal("Complete did not append its one missing denominator cell")
	}
	if cell, ok := completed.CellAt(1); !ok || cell.Column() != leftValue || cell.Source() != 0 {
		t.Fatal("Complete appended the missing cell at the wrong coordinate")
	}
	again, ok := algebra.CompleteCellLayout(completed, denominator, []model.ColumnID{leftLink, leftValue})
	if !ok || !again.Equal(completed) || again.Digest() != completed.Digest() {
		t.Fatal("Complete extension was not idempotent")
	}

	right, ok := algebra.InputCellLayout(rightRelation, []model.ColumnID{rightValue})
	if !ok {
		t.Fatal("right input")
	}
	joined, ok := algebra.JoinCellLayouts(completed, right)
	if !ok || joined.Len() != 3 {
		t.Fatal("Join did not concatenate the completed coordinate")
	}
	if cell, ok := joined.CellAt(2); !ok || cell.Column() != rightValue || cell.Source() != 1 {
		t.Fatal("Join did not shift the right source ordinal")
	}

	projected, ok := algebra.ColumnProjectCellLayout(joined, []algebra.ColumnSlot{
		algebra.NewColumnSlot(leftValue, 1),
		algebra.NewColumnSlot(rightValue, 2),
	})
	if !ok || projected.Len() != 2 {
		t.Fatal("ColumnProject did not retain its exact addressed cells")
	}
	if cell, ok := projected.CellAt(1); !ok || cell.Source() != 1 {
		t.Fatal("ColumnProject lost the selected source occurrence")
	}

	projectedInto, ok := algebra.ProjectCellLayout(completed, targetRelation, []algebra.ColumnMapping{
		algebra.NewColumnMapping(leftValue, targetValue),
	})
	if !ok || projectedInto.SourceLen() != 2 || projectedInto.Len() != 1 {
		t.Fatal("Project did not introduce one explicit target occurrence")
	}
	if cell, ok := projectedInto.CellAt(0); !ok || cell.Column() != targetValue || cell.Source() != 1 {
		t.Fatal("Project target cell did not use the appended target occurrence")
	}
}

func TestCompleteCellLayoutRefusesAmbiguousSparseSelfJoin(t *testing.T) {
	owner, ok := model.IssueOwnerID(identity.ContentID{0xa1})
	if !ok {
		t.Fatal("owner")
	}
	relation, ok := model.IssueRelationID(owner, identity.ContentID{0xa2})
	if !ok {
		t.Fatal("relation")
	}
	key, ok := model.IssueKeyID(relation, identity.ContentID{0xa3})
	if !ok {
		t.Fatal("key")
	}
	link, ok := model.IssueColumnID(relation, identity.ContentID{0xa4})
	if !ok {
		t.Fatal("link")
	}
	value, ok := model.IssueColumnID(relation, identity.ContentID{0xa5})
	if !ok {
		t.Fatal("value")
	}
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("denominator")
	}
	input, ok := algebra.InputCellLayout(relation, []model.ColumnID{link})
	if !ok {
		t.Fatal("input")
	}
	selfJoin, ok := algebra.JoinCellLayouts(input, input)
	if !ok {
		t.Fatal("self join coordinate")
	}
	if _, ok := algebra.CompleteCellLayout(selfJoin, denominator, []model.ColumnID{link, value}); ok {
		t.Fatal("Complete chose one duplicate denominator occurrence for a missing cell")
	}
}
