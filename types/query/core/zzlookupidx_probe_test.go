package core

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

// TestZZLookupIdxProbe reproduces fixture realworld/lookup-table-cast root:
// a Record with a literal-integer MapKey indexed by a non-literal number.
// Read-only diagnostic probe.
func TestZZLookupIdxProbe(t *testing.T) {
	strUnion := typ.NewUnion(
		typ.LiteralString("rate_limit"),
		typ.LiteralString("authentication"),
		typ.LiteralString("invalid_request"),
		typ.LiteralString("server_error"),
	)

	// Record with literal-400 map key (matches printed {[400]: ...}).
	recLit := typ.NewRecord().MapComponent(typ.LiteralInt(400), strUnion).Build()
	r1, ok1 := Index(recLit, typ.Number)
	t.Logf("Record{[400]:U} indexed by number -> ok=%v t=%v", ok1, fmtT(r1))

	r1b, ok1b := Index(recLit, typ.Integer)
	t.Logf("Record{[400]:U} indexed by integer -> ok=%v t=%v", ok1b, fmtT(r1b))

	// Record with proper number map key (what annotation StatusCodeMap should be).
	recNum := typ.NewRecord().MapComponent(typ.Number, strUnion).Build()
	r2, ok2 := Index(recNum, typ.Number)
	t.Logf("Record{[number]:U} indexed by number -> ok=%v t=%v", ok2, fmtT(r2))

	// Plain Map with number key.
	m := typ.NewMap(typ.Number, typ.String)
	r3, ok3 := Index(m, typ.Number)
	t.Logf("Map<number,string> indexed by number -> ok=%v t=%v", ok3, fmtT(r3))

	// Union of integer literals as map key (alternate inference shape).
	keyUnion := typ.NewUnion(typ.LiteralInt(400), typ.LiteralInt(401), typ.LiteralInt(429), typ.LiteralInt(500))
	recUnion := typ.NewRecord().MapComponent(keyUnion, strUnion).Build()
	r4, ok4 := Index(recUnion, typ.Number)
	t.Logf("Record{[400|401|429|500]:U} indexed by number -> ok=%v t=%v", ok4, fmtT(r4))
}

func fmtT(t typ.Type) string {
	if t == nil {
		return "<nil>"
	}
	return t.String()
}
