package algebra_test

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

func TestLogicalNodesHaveOnlyDeclarativeFields(t *testing.T) {
	cases := []struct {
		typeValue reflect.Type
		fields    []string
	}{
		{reflect.TypeOf(algebra.Input{}), []string{"relation"}},
		{reflect.TypeOf(algebra.Select{}), []string{"child", "contract"}},
		{reflect.TypeOf(algebra.Project{}), []string{"child", "contract"}},
		{reflect.TypeOf(algebra.Join{}), []string{"left", "right", "contract"}},
		{reflect.TypeOf(algebra.Merge{}), []string{"inputs", "contract"}},
		{reflect.TypeOf(algebra.Group{}), []string{"child", "contract"}},
		{reflect.TypeOf(algebra.Complete{}), []string{"child", "denominator"}},
		{reflect.TypeOf(algebra.Apply{}), []string{"inputs", "contract"}},
		{reflect.TypeOf(algebra.Publish{}), []string{"child", "contract"}},
	}
	for _, testCase := range cases {
		if testCase.typeValue.NumField() != len(testCase.fields) {
			t.Fatalf("%s has %d fields, want %d", testCase.typeValue.Name(), testCase.typeValue.NumField(), len(testCase.fields))
		}
		for index, expected := range testCase.fields {
			field := testCase.typeValue.Field(index)
			if field.Name != expected {
				t.Errorf("%s field %d = %q, want %q", testCase.typeValue.Name(), index, field.Name, expected)
			}
			switch field.Type.Kind() {
			case reflect.Func, reflect.Chan, reflect.UnsafePointer:
				t.Errorf("%s.%s carries executable or runtime state: %s", testCase.typeValue.Name(), field.Name, field.Type)
			}
		}
	}
}

func TestContractsCarryOnlyLogicalRoles(t *testing.T) {
	cases := []struct {
		typeValue reflect.Type
		fields    []string
	}{
		{reflect.TypeOf(algebra.SelectContract{}), []string{"mode", "scope"}},
		{reflect.TypeOf(algebra.ProjectContract{}), []string{"target", "mappings", "key"}},
		{reflect.TypeOf(algebra.JoinContract{}), []string{"leftColumns", "rightColumns"}},
		{reflect.TypeOf(algebra.MergeContract{}), []string{"key"}},
		{reflect.TypeOf(algebra.GroupContract{}), []string{"key", "cardinality"}},
		{reflect.TypeOf(algebra.ApplyContract{}), []string{"operation"}},
		{reflect.TypeOf(algebra.PublishContract{}), []string{"destination", "key"}},
	}
	for _, testCase := range cases {
		if testCase.typeValue.NumField() != len(testCase.fields) {
			t.Fatalf("%s has %d fields, want %d", testCase.typeValue.Name(), testCase.typeValue.NumField(), len(testCase.fields))
		}
		for index, expected := range testCase.fields {
			field := testCase.typeValue.Field(index)
			if field.Name != expected {
				t.Errorf("%s field %d = %q, want %q", testCase.typeValue.Name(), index, field.Name, expected)
			}
			switch field.Type.Kind() {
			case reflect.Func, reflect.Chan, reflect.UnsafePointer:
				t.Errorf("%s.%s carries executable or runtime state: %s", testCase.typeValue.Name(), field.Name, field.Type)
			}
		}
	}
	if got := reflect.TypeOf(algebra.GroupContract{}).Field(1).Type; got != reflect.TypeOf(model.Cardinality{}) {
		t.Fatalf("GroupContract.cardinality type = %s, want model.Cardinality", got)
	}
	if got := reflect.TypeOf(algebra.SelectContract{}).Field(1).Type; got != reflect.TypeOf(model.ScopeID{}) {
		t.Fatalf("SelectContract.scope type = %s, want model.ScopeID", got)
	}
	joinColumns := reflect.TypeOf(algebra.JoinContract{}).Field(0).Type
	if joinColumns.Kind() != reflect.Slice || joinColumns.Elem() != reflect.TypeOf(model.ColumnID{}) {
		t.Fatalf("JoinContract.leftColumns type = %s, want []model.ColumnID", joinColumns)
	}
}

func TestMergeAndSelectHaveNoSecondSemanticOperationBoundary(t *testing.T) {
	mergeType := reflect.TypeOf(algebra.MergeContract{})
	for index := 0; index < mergeType.NumField(); index++ {
		if mergeType.Field(index).Type.PkgPath() == "github.com/wippyai/go-lua/analysis/relation/semantic/signature" {
			t.Fatalf("MergeContract retains a semantic operation field: %s", mergeType.Field(index).Name)
		}
	}
	selectType := reflect.TypeOf(algebra.SelectContract{})
	for index := 0; index < selectType.NumField(); index++ {
		if selectType.Field(index).Type.PkgPath() == "github.com/wippyai/go-lua/analysis/relation/semantic/signature" {
			t.Fatalf("SelectContract retains a semantic operation field: %s", selectType.Field(index).Name)
		}
	}
}
