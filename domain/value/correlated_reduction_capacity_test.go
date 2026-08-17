package value

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/runtimekind"
)

// TestFullRowsCapacityTracksSelection ensures singleton reductions do not
// retain capacity for the complete atom denominator. This guards the hot
// Schema sealing path where atomTop invokes fullRows once per atom.
func TestFullRowsCapacityTracksSelection(t *testing.T) {
	schema := &Schema{
		atoms:    make([]atomRow, 128),
		capWords: 3,
	}
	rows := schema.fullRows(func(id uint32) bool { return id == 17 })
	want := schema.stride()
	if len(rows) != want || cap(rows) != want {
		t.Fatalf("singleton reduction rows len/cap = %d/%d, want %d/%d", len(rows), cap(rows), want, want)
	}
	rows = schema.fullRows(func(id uint32) bool { return id%2 == 0 })
	want = 64 * schema.stride()
	if len(rows) != want || cap(rows) != want {
		t.Fatalf("half reduction rows len/cap = %d/%d, want %d/%d", len(rows), cap(rows), want, want)
	}
}

// TestAtomTopReductionStorageScalesLinearly verifies the sealed singleton
// table's retained backing storage directly.  The row arena must contain one
// exact-stride row per atom, and every published Value must expose only its
// own row (not the complete arena).  Doubling the atom vocabulary therefore
// doubles the retained words instead of retaining an atom-denominator array
// for every singleton.
func TestAtomTopReductionStorageScalesLinearly(t *testing.T) {
	const (
		capabilityCount = 1
		firstAtomCount  = 32
		secondAtomCount = 64
	)
	makeSchema := func(atomCount int) *Schema {
		schema := &Schema{
			atoms:              make([]atomRow, atomCount),
			capabilities:       make([]identity.ContentID, capabilityCount),
			capWords:           1,
			firstStoredUnknown: uint32(atomCount + 1),
			firstStoredExact:   uint32(atomCount + 1),
		}
		for index := range schema.atoms {
			runtime := runtimekind.Number
			if index%2 != 0 {
				runtime = runtimekind.String
			}
			schema.atoms[index] = atomRow{kind: atomPrimitive, runtime: runtime}
		}
		schema.potential = uint64(atomCount) * (uint64(capabilityCount) + 1)
		schema.bottom = Value{schema: schema}
		schema.top = Value{schema: schema, top: true}
		if !schema.finishReductions() {
			t.Fatalf("finish reductions for %d atoms", atomCount)
		}
		return schema
	}

	first := makeSchema(firstAtomCount)
	second := makeSchema(secondAtomCount)
	stride := first.stride()
	if cap(first.atomTopImage) != firstAtomCount*stride || cap(second.atomTopImage) != secondAtomCount*stride {
		t.Fatalf("atomTop arena capacity = %d/%d, want %d/%d", cap(first.atomTopImage), cap(second.atomTopImage), firstAtomCount*stride, secondAtomCount*stride)
	}
	for _, schema := range []*Schema{first, second} {
		for id := 1; id <= len(schema.atoms); id++ {
			row := schema.atomTop[id]
			if !schema.owns(row) || row.top || len(row.image) != stride || cap(row.image) != stride {
				t.Fatalf("atomTop[%d] is not an exact owner row: top=%t len/cap=%d/%d", id, row.top, len(row.image), cap(row.image))
			}
			if uint32(row.image[0]) != uint32(id) {
				t.Fatalf("atomTop[%d] image starts at atom %d", id, row.image[0])
			}
		}
	}
	if cap(second.atomTopImage) != 2*cap(first.atomTopImage) {
		t.Fatalf("atomTop arena did not scale linearly: %d vs %d", cap(second.atomTopImage), cap(first.atomTopImage))
	}
}
