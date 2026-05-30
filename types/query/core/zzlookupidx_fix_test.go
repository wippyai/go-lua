package core

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

// TestZZLookupIdxLiteralIntMap asserts that indexing a literal-int-keyed map
// (or record map-component) by a runtime number SUCCEEDS yielding the OPTIONAL
// value-union. This mirrors realworld/lookup-table-cast mapper.lua:39 where the
// map-component key was inferred as Literal(400)/literal-int-union and the read
// M.status_codes[code] (code: number) must read as value | nil (the literal key
// may not match the runtime index). An incompatible key type must still error.
func TestZZLookupIdxLiteralIntMap(t *testing.T) {
	valueUnion := typ.NewUnion(
		typ.LiteralString("rate_limit"),
		typ.LiteralString("authentication"),
		typ.LiteralString("invalid_request"),
		typ.LiteralString("server_error"),
	)

	// The optional value-union is represented either as an *Optional wrapper or,
	// when the value is itself a union, as a nil-bearing Union (NewOptional folds
	// nil into the union). ContainsNil captures both: the read must be value | nil.
	mustOptional := func(label string, container, key typ.Type) {
		got, ok := Index(container, key)
		if !ok {
			t.Fatalf("%s: expected ok, got !ok", label)
		}
		if !ContainsNil(got) {
			t.Fatalf("%s: expected optional value-union (value | nil), got %s", label, fmtT(got))
		}
	}
	mustFail := func(label string, container, key typ.Type) {
		if _, ok := Index(container, key); ok {
			t.Fatalf("%s: expected !ok (incompatible key), got ok", label)
		}
	}

	single := typ.NewMap(typ.LiteralInt(400), valueUnion)
	mustOptional("Map{[400]:U} by number", single, typ.Number)
	mustOptional("Map{[400]:U} by integer", single, typ.Integer)

	multi := typ.NewMap(typ.NewUnion(typ.LiteralInt(400), typ.LiteralInt(429), typ.LiteralInt(500)), valueUnion)
	mustOptional("Map{[400|429|500]:U} by number", multi, typ.Number)

	recSingle := typ.NewRecord().MapComponent(typ.LiteralInt(400), valueUnion).Build()
	mustOptional("Record{[400]:U} by number", recSingle, typ.Number)

	recUnion := typ.NewRecord().MapComponent(typ.NewUnion(typ.LiteralInt(400), typ.LiteralInt(429), typ.LiteralInt(500)), valueUnion).Build()
	mustOptional("Record{[400|429|500]:U} by number", recUnion, typ.Number)

	// SOUNDNESS: a non-numeric key into a literal-int Map must still fail. The new
	// literal-int read path is gated on a numeric read key, so a string/boolean
	// index cannot match a literal-int key domain.
	mustFail("Map{[400]:U} by string", single, typ.String)
	mustFail("Map{[400]:U} by boolean", single, typ.Boolean)

	// SOUNDNESS: the new path must not over-broaden. A number index into a Map
	// whose key is a non-literal numeric base type (number) keeps its existing
	// exact subtype handling; a number index into a literal-STRING-keyed Map must
	// fail (literal-string domain is not a literal-int domain, key not numeric).
	litStrMap := typ.NewMap(typ.NewUnion(typ.LiteralString("a"), typ.LiteralString("b")), valueUnion)
	mustFail("Map{[\"a\"|\"b\"]:U} by number", litStrMap, typ.Number)
}
