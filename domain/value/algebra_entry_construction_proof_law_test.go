package value

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/domain/runtimekind"
)

// entryProofSchema seals a wide integer-literal schema. Width is the point:
// the laws below separate work that scales with the image from work that does
// not, so the image must be long enough for a scan to be observable.
func entryProofSchema(t *testing.T, atoms int) *Schema {
	t.Helper()
	schema := &Schema{atomByRow: make(map[atomRow]uint32), exactKeys: make(map[keyspace.LiteralValue]keyspace.LiteralValue), potential: uint64(atoms) * 16}
	for index := 0; index < atoms; index++ {
		row := atomRow{kind: atomLiteral, runtime: runtimekind.Number, key: keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: int64(index)}, hasKey: true}
		schema.exactKeys[row.key] = row.key
		if schema.addAtom(row) == 0 {
			t.Fatalf("test atom unavailable at %d", index)
		}
	}
	schema.bottom = Value{schema: schema}
	schema.top = Value{schema: schema, top: true}
	return schema
}

func entryProofWide(t *testing.T, schema *Schema, atoms int) Value {
	t.Helper()
	ids := make([]Atom, 0, atoms)
	for index := 1; index <= atoms; index++ {
		ids = append(ids, Atom{schema: schema, id: uint32(index)})
	}
	value, ok := schema.Alternatives(ids...)
	if !ok {
		t.Fatal("wide alternatives unavailable")
	}
	return value
}

// A Value proves its canonical form exactly once, at the construction gate.
// canonical is the only place a freshly built image becomes a Value, so it is
// the only place the atom column is proved in range and strictly ascending.
func TestCanonicalGateRefusesNonAscendingImage(t *testing.T) {
	schema := entryProofSchema(t, 8)
	stride := schema.stride()

	descending := make([]uint64, 2*stride)
	descending[0] = 5
	descending[stride] = 3
	if schema.owns(schema.canonical(descending)) {
		t.Fatal("canonical admitted a descending atom column")
	}

	repeated := make([]uint64, 2*stride)
	repeated[0] = 4
	repeated[stride] = 4
	if schema.owns(schema.canonical(repeated)) {
		t.Fatal("canonical admitted a repeated atom")
	}

	unsealed := make([]uint64, stride)
	unsealed[0] = uint64(len(schema.atoms) + 1)
	if schema.owns(schema.canonical(unsealed)) {
		t.Fatal("canonical admitted an unsealed atom id")
	}

	zero := make([]uint64, stride)
	if schema.owns(schema.canonical(zero)) {
		t.Fatal("canonical admitted a zero atom id")
	}

	ascending := make([]uint64, 2*stride)
	ascending[0] = 3
	ascending[stride] = 5
	if !schema.owns(schema.canonical(ascending)) {
		t.Fatal("canonical refused an ascending atom column")
	}
}

// An algebra entry costs O(1) in the width of the relation it is handed. The
// construction gate above already proved canonical form, and a Value image is
// immutable, so no entry re-derives that proof and reflexive comparisons are
// answered from image identity rather than from a walk.
//
// Corrupting an already constructed image is the one observable that separates
// a constant-time answer from a walk: a walking implementation reports the
// corruption, a construction-proved one cannot see it.
func TestAlgebraEntryDoesNotRereadOwnedImage(t *testing.T) {
	const atoms = 64
	schema := entryProofSchema(t, atoms)
	stride := schema.stride()
	wide := entryProofWide(t, schema, atoms)
	if len(wide.image) != atoms*stride {
		t.Fatalf("wide image rows = %d, want %d", len(wide.image)/stride, atoms)
	}

	// Descending, out of range and repeated in one stroke: every clause the
	// construction gate proves is now false in this image.
	wide.image[0] = uint64(atoms)
	wide.image[stride] = uint64(atoms)

	if !schema.owns(wide) {
		t.Fatal("owns re-derived canonical form at the algebra entry")
	}
	if !schema.LessOrEq(wide, wide) {
		t.Fatal("LessOrEq walked the image instead of answering reflexivity by identity")
	}
	if !schema.Equal(wide, wide) {
		t.Fatal("Equal walked the image instead of answering identity")
	}
	joined, joinOK := schema.Join(wide, wide)
	if !joinOK || len(joined.image) != len(wide.image) || &joined.image[0] != &wide.image[0] {
		t.Fatal("Join rebuilt an image for an identity operand pair")
	}
}
