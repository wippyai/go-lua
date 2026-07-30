package typ

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/kind"
)

// hashEncoderCorpus returns structurally diverse types spanning every node kind
// that carries non-trivial structure. Each entry is built through the production
// constructors so its Hash() is the construction-time encoding.
func hashEncoderCorpus() []Type {
	rec := NewRecursive("List", func(self Type) Type {
		return RebuildRecord(RecordParts{Fields: []Field{
			{Name: "head", Type: Number},
			{Name: "tail", Type: MaterializeOptional(self)},
		}})
	})

	tp := NewTypeParam("T", nil)
	gen := NewGeneric("Box", []*TypeParam{tp}, RebuildRecord(RecordParts{Fields: []Field{
		{Name: "value", Type: tp},
	}}))
	recursiveFunction := Func().Param("node", rec).Returns(rec).Build()
	recursiveRecord := RebuildRecord(RecordParts{Fields: []Field{{Name: "node", Type: rec}}})
	genericFunction := Func().Param("box", Instantiate(gen, String)).Returns(Instantiate(gen, Number)).Build()

	iface := NewInterface("Reader", []Method{
		{Name: "read", Type: Func().Param("n", Integer).Returns(String).Build()},
	})

	return []Type{
		Nil, Boolean, Number, Integer, String, Any, Unknown, Never,
		LiteralBool(true),
		LiteralInt(7),
		LiteralNumber(2.5),
		LiteralString("k"),
		NewArray(Number),
		NewArray(NewArray(String)),
		NewMap(String, Number),
		NewReadonlyMap(String, Number),
		NewTuple(Number, String, Boolean),
		MaterializeOptional(Number),
		MaterializeOptional(NewArray(String)),
		MaterializeUnion([]Type{Number, String, Boolean}),
		MaterializeIntersection([]Type{
			iface,
			NewInterface("Closer", []Method{{Name: "close", Type: Func().Build()}}),
		}),
		Func().Param("a", Number).OptParam("b", String).Variadic(Boolean).Returns(Number, String).Build(),
		RebuildRecord(RecordParts{
			Fields:    []Field{{Name: "x", Type: Number}, {Name: "y", Type: MaterializeOptional(String)}},
			MapKey:    String,
			MapValue:  Number,
			Metatable: iface,
		}),
		iface,
		NewMeta(iface),
		NewMeta(Number),
		gen,
		Instantiate(gen, String),
		rec,
		MaterializeOptional(rec),
		recursiveFunction,
		recursiveRecord,
		genericFunction,
	}
}

// TestHashConstructionMatchesTraversal pins that the construction-time encoder
// (t.Hash(), produced by the rebuild_record/function/container constructors) and
// the from-scratch traversal encoder (hashWithVisitedMemo, used by EqualityHash
// for mutable nodes) agree for every type. They are two encoders of one layout;
// if they drift, the equality prefilter EqualityHash(a) != EqualityHash(b) can
// reject structurally equal types, an unsound false-negative in dedup/equality.
func TestHashConstructionMatchesTraversal(t *testing.T) {
	for _, ty := range hashEncoderCorpus() {
		scratch := getRecursiveHashScratch()
		traversal := hashWithVisitedMemo(ty, scratch)
		putRecursiveHashScratch(scratch)
		if got := ty.Hash(); got != traversal {
			t.Fatalf("%s (%s): construction Hash()=%d, traversal hash=%d", ty.String(), ty.Kind(), got, traversal)
		}
		if eq := EqualityHash(ty); eq != traversal {
			t.Fatalf("%s (%s): EqualityHash=%d, traversal hash=%d", ty.String(), ty.Kind(), eq, traversal)
		}
	}
}

// TestEqualityHashStableAcrossIndependentConstruction pins that two independently
// built but structurally identical types share one EqualityHash, so the equality
// prefilter never rejects them. This guards the encoders against order- or
// identity-dependent hashing.
func TestEqualityHashStableAcrossIndependentConstruction(t *testing.T) {
	build := func() []Type {
		iface := NewInterface("Reader", []Method{
			{Name: "read", Type: Func().Param("n", Integer).Returns(String).Build()},
		})
		return []Type{
			NewMap(String, Number),
			NewTuple(Number, String),
			MaterializeUnion([]Type{Number, String, Boolean}),
			Func().Param("a", Number).Returns(String).Build(),
			RebuildRecord(RecordParts{Fields: []Field{{Name: "x", Type: Number}, {Name: "y", Type: String}}}),
			iface,
			NewMeta(iface),
		}
	}
	left, right := build(), build()
	for i := range left {
		if a, b := EqualityHash(left[i]), EqualityHash(right[i]); a != b {
			t.Fatalf("%s (%s): EqualityHash differs across independent construction: %d vs %d",
				left[i].String(), left[i].Kind(), a, b)
		}
		if !typeEquals(left[i], right[i]) {
			t.Fatalf("%s (%s): independently built equals must be equal", left[i].String(), left[i].Kind())
		}
	}
	_ = kind.Record
}
