package core

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

// TestZZTableTopGradualProbe asserts the bare builtin `table` top projects the
// compatibility atom (`any`) across field/index/method/write access, while a
// TYPED table ({[K]:V} map, {T} array, named record) and an INFERRED `unknown`
// stay strictly checked. The bare-`table` annotation is still opaque: concrete
// operator use must be proved by a guard/assertion/cast.
func TestZZTableTopGradualProbe(t *testing.T) {
	tableTop := typ.NewInterface("table", nil)

	// Bare table top: field/index/method/write all project gradual any.
	if got, ok := Field(tableTop, "dataflow_id"); !ok || got != typ.Any {
		t.Fatalf("Field(table-top): want (any,true), got (%v,%v)", got, ok)
	}
	if got, ok := Index(tableTop, typ.String); !ok || got != typ.Any {
		t.Fatalf("Index(table-top): want (any,true), got (%v,%v)", got, ok)
	}
	if _, ok := Method(tableTop, "submit"); !ok {
		t.Fatalf("Method(table-top): want ok, got !ok")
	}

	// A table-top field read remains opaque. Concatenation is concrete string
	// use, so it cannot fabricate string from `any`.
	fieldT, _ := Field(tableTop, "dataflow_id")
	if got := BinaryOp(typ.String, "..", fieldT); got != typ.Unknown {
		t.Fatalf("concat(string, table-top.field): want unknown, got %v", got)
	}

	// SOUNDNESS: a TYPED map stays strict. A field read off {[string]:number}
	// does not project any; a string field read yields the optional value type,
	// and an incompatible (non-string) key is rejected.
	typedMap := typ.NewMap(typ.String, typ.Number)
	if got, _ := Field(typedMap, "x"); got == typ.Any {
		t.Fatalf("Field(typed-map): must not project any, got %v", got)
	}
	if got, ok := Index(typedMap, typ.String); !ok || got == typ.Any {
		t.Fatalf("Index(typed-map, string): want non-any value type, got (%v,%v)", got, ok)
	}
	if _, ok := Index(typedMap, typ.Boolean); ok {
		t.Fatalf("Index(typed-map, boolean): want !ok (incompatible key), got ok")
	}

	// SOUNDNESS: a typed array stays strict (a string key does not index it).
	typedArr := typ.NewArray(typ.Number)
	if _, ok := Index(typedArr, typ.String); ok {
		t.Fatalf("Index(typed-array, string): want !ok, got ok")
	}

	// SOUNDNESS: a named record stays strict. A missing field is not found and
	// does not silently project any.
	rec := typ.NewRecord().Field("id", typ.Integer).Build()
	if got, ok := Field(rec, "missing"); ok || got == typ.Any {
		t.Fatalf("Field(record, missing): want (_,false) and not any, got (%v,%v)", got, ok)
	}

	// SOUNDNESS: INFERRED unknown stays strict. A field read off unknown stays
	// unknown (NOT any), and concatenating it keeps propagating unknown so a
	// downstream typed use fails soundly.
	if got, ok := Field(typ.Unknown, "x"); !ok || got != typ.Unknown {
		t.Fatalf("Field(unknown): want (unknown,true), got (%v,%v)", got, ok)
	}
	if got := BinaryOp(typ.String, "..", typ.Unknown); got != typ.Unknown {
		t.Fatalf("concat(string, unknown): want unknown (strict, fails downstream), got %v", got)
	}
}
