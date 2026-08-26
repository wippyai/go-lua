package algebra_test

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

func TestInputProjectionIsExplicitOrderedAndImmutable(t *testing.T) {
	owner, ok := model.IssueOwnerID(identity.ContentID{91})
	if !ok {
		t.Fatal("owner construction failed")
	}
	relation, ok := model.IssueRelationID(owner, identity.ContentID{92})
	if !ok {
		t.Fatal("relation construction failed")
	}
	first, ok := model.IssueColumnID(relation, identity.ContentID{93})
	if !ok {
		t.Fatal("first column construction failed")
	}
	second, ok := model.IssueColumnID(relation, identity.ContentID{94})
	if !ok {
		t.Fatal("second column construction failed")
	}

	all := algebra.NewInput(relation)
	if !all.Available() || !all.AllColumns() || all.Projection() != algebra.InputProjectionAllColumns {
		t.Fatalf("NewInput did not seal explicit AllColumns: %+v", all)
	}
	if _, exact := all.ExactColumns(); exact || len(all.Columns()) != 0 {
		t.Fatal("AllColumns exposed an exact vector")
	}

	columns := []model.ColumnID{second, first}
	exact, ok := algebra.NewInputColumns(relation, columns)
	if !ok || !exact.Available() || exact.AllColumns() || exact.Projection() != algebra.InputProjectionExactColumns {
		t.Fatalf("exact Input did not seal: ok=%v input=%+v", ok, exact)
	}
	got, gotOK := exact.ExactColumns()
	if !gotOK || !reflect.DeepEqual(got, columns) {
		t.Fatalf("exact columns = %v/%v, want %v/true", got, gotOK, columns)
	}
	got[0] = first
	if stable, _ := exact.ExactColumns(); !reflect.DeepEqual(stable, columns) {
		t.Fatal("ExactColumns exposed mutable storage")
	}
	columns[0] = first
	if stable, _ := exact.ExactColumns(); stable[0] != second {
		t.Fatal("NewInputColumns retained mutable constructor storage")
	}

	if all.Digest() == exact.Digest() {
		t.Fatal("AllColumns and ExactColumns aliased in the expression digest")
	}
	reordered, ok := algebra.NewInputColumns(relation, []model.ColumnID{first, second})
	if !ok || reordered.Digest() == exact.Digest() {
		t.Fatal("exact projection order was omitted from the expression digest")
	}
}

func TestNewInputColumnsRejectsHostileVectors(t *testing.T) {
	owner, ok := model.IssueOwnerID(identity.ContentID{101})
	if !ok {
		t.Fatal("owner construction failed")
	}
	relation, ok := model.IssueRelationID(owner, identity.ContentID{102})
	if !ok {
		t.Fatal("relation construction failed")
	}
	foreignRelation, ok := model.IssueRelationID(owner, identity.ContentID{103})
	if !ok {
		t.Fatal("foreign relation construction failed")
	}
	column, ok := model.IssueColumnID(relation, identity.ContentID{104})
	if !ok {
		t.Fatal("column construction failed")
	}
	foreignColumn, ok := model.IssueColumnID(foreignRelation, identity.ContentID{105})
	if !ok {
		t.Fatal("foreign column construction failed")
	}
	for name, columns := range map[string][]model.ColumnID{
		"empty":     nil,
		"duplicate": {column, column},
		"foreign":   {foreignColumn},
	} {
		t.Run(name, func(t *testing.T) {
			if input, ok := algebra.NewInputColumns(relation, columns); ok || input.Available() {
				t.Fatalf("hostile %s vector was accepted: ok=%v input=%+v", name, ok, input)
			}
		})
	}
}
