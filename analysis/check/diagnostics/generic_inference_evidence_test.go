package diagnostics

import (
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestGenericInferenceTypesConflictAllowsCompatibleLiteralFamilies(t *testing.T) {
	tests := []struct {
		name  string
		left  typ.Type
		right typ.Type
	}{
		{name: "strings", left: typ.LiteralString("a"), right: typ.LiteralString("bb")},
		{name: "integers", left: typ.LiteralInt(1), right: typ.LiteralInt(2)},
		{name: "booleans", left: typ.LiteralBool(true), right: typ.LiteralBool(false)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if genericInferenceTypesConflict(tt.left, tt.right) {
				t.Fatalf("genericInferenceTypesConflict(%s, %s) = true, want false", formatType(tt.left), formatType(tt.right))
			}
		})
	}
}

func TestGenericInferenceTypesConflictKeepsRecordShapeConflicts(t *testing.T) {
	event := typetable.NewRecord().
		Field("kind", typ.LiteralString("event")).
		Field("id", typ.String).
		Build()
	timer := typetable.NewRecord().
		Field("kind", typ.LiteralString("timer")).
		Field("elapsed", typ.Number).
		Build()

	if !genericInferenceTypesConflict(event, timer) {
		t.Fatal("genericInferenceTypesConflict(event, timer) = false, want true")
	}
}
