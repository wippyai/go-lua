package access

import (
	"testing"

	"github.com/wippyai/go-lua/domain/type/typ"
)

// TestIndexingAMapWithNoStatedValueTypeReadsUnknown states what indexing a map
// yields when the map states no value type at all.
//
// Unknown is the answer: nothing is known about what the map holds, and every
// later judgment has to keep asking. Nil is a different answer entirely -- it
// is a concrete type, and it licenses narrowing a read of this map to the nil
// branch, so a value the map may well hold is proved absent by a type the map
// never stated.
func TestIndexingAMapWithNoStatedValueTypeReadsUnknown(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		container typ.Type
	}{
		{name: "map", container: &typ.Map{Key: typ.String}},
		{name: "readonly map", container: &typ.ReadonlyMap{Key: typ.String}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			value, ok := WritableIndex(testCase.container, typ.String)
			if !ok {
				t.Fatal("indexing a map by its own key type resolved no value")
			}
			if value == typ.Nil || value.Kind() == typ.Nil.Kind() {
				t.Fatalf("a map that states no value type indexed to %s, which proves every read of it is nil", value)
			}
			if value != typ.Unknown {
				t.Fatalf("indexed value = %s, want unknown", value)
			}
		})
	}
}
