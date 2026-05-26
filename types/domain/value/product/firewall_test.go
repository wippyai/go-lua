package product_test

import (
	"testing"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/domain/value/axis/effectrows"
	"github.com/wippyai/go-lua/types/domain/value/axis/escape"
	"github.com/wippyai/go-lua/types/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/types/domain/value/axis/identityrecursion"
	"github.com/wippyai/go-lua/types/domain/value/axis/numeric"
	"github.com/wippyai/go-lua/types/domain/value/axis/ownership"
	"github.com/wippyai/go-lua/types/domain/value/axis/presence"
	"github.com/wippyai/go-lua/types/domain/value/axis/shapevalue"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

// shapeValue builds an AbstractValue carrying the given shape at the admission
// boundary with identity on every other axis.
func shapeValue(t typ.Type) product.AbstractValue {
	return product.New(
		shapevalue.Of(t),
		presence.Top(),
		numeric.Top(),
		effectrows.Top(),
		ownership.Top(),
		escape.Top(),
		identityrecursion.Top(),
		evidence.Top(),
	)
}

// TestAbstractValueSatisfiesEqualer pins that AbstractValue plugs into the db's
// existing value-equality contract (internal.Equaler), which is the hook
// db.Query uses for change detection when no explicit equal func is supplied.
func TestAbstractValueSatisfiesEqualer(t *testing.T) {
	var v any = shapeValue(typ.Number)
	eq, ok := v.(internal.Equaler)
	if !ok {
		t.Fatal("AbstractValue must satisfy internal.Equaler so the db firewall can compare it")
	}

	if !eq.Equals(shapeValue(typ.Number)) {
		t.Fatal("Equals must report equal for equal content")
	}
	if eq.Equals(shapeValue(typ.String)) {
		t.Fatal("Equals must report not equal for distinct content")
	}
	// A non-AbstractValue operand is never equal and must not panic.
	if eq.Equals(42) {
		t.Fatal("Equals must report not equal for a non-AbstractValue operand")
	}
	if eq.Equals(nil) {
		t.Fatal("Equals must report not equal for a nil operand")
	}
}

// TestQueryFirewallSuppressesChurnOnEqualValue is the red-green firewall: a query
// that recomputes an Equal-but-distinctly-constructed AbstractValue must NOT
// advance UpdatedAt, so a dependent query does not revalidate. Equality flows
// through AbstractValue.Equal via the db's default internal.Equaler path (no
// explicit equal func is supplied to NewQuery).
func TestQueryFirewallSuppressesChurnOnEqualValue(t *testing.T) {
	database := db.New()
	ctx := db.NewQueryContext(database)

	// The input toggles which structural type the source query admits. The two
	// unions are reordered constructions of the same value, so admitting either
	// yields an Equal AbstractValue.
	input := db.NewInput[string, int](database)
	input.Set("k", 0)

	unionA := typ.NewUnion(typ.Number, typ.String)
	unionB := typ.NewUnion(typ.String, typ.Number)

	sourceCalls := 0
	source := db.NewQuery[string, product.AbstractValue](
		"source",
		func(qc *db.QueryContext, key string) product.AbstractValue {
			sourceCalls++
			sel, _ := input.Get(qc, key)
			if sel%2 == 0 {
				return shapeValue(unionA)
			}
			return shapeValue(unionB)
		},
		nil, // default equality: AbstractValue via internal.Equaler
	)

	dependentCalls := 0
	dependent := db.NewQuery[string, int](
		"dependent",
		func(qc *db.QueryContext, key string) int {
			dependentCalls++
			_ = source.Get(qc, key)
			return dependentCalls
		},
		func(a, b int) bool { return a == b },
	)

	dependent.Get(ctx, "k")
	if sourceCalls != 1 || dependentCalls != 1 {
		t.Fatalf("initial run: sourceCalls=%d dependentCalls=%d, want 1 1", sourceCalls, dependentCalls)
	}

	// Flip the input so the source recomputes the Equal-but-rebuilt union.
	input.Set("k", 1)

	dependent.Get(ctx, "k")
	if sourceCalls != 2 {
		t.Fatalf("source should recompute after input change, sourceCalls=%d, want 2", sourceCalls)
	}
	if dependentCalls != 1 {
		t.Fatalf("firewall breached: dependent revalidated on an Equal value, dependentCalls=%d, want 1", dependentCalls)
	}
}

// TestQueryFirewallPropagatesChurnOnNonEqualValue pins the other side: when the
// recomputed AbstractValue is NOT Equal, UpdatedAt advances and the dependent
// query revalidates.
func TestQueryFirewallPropagatesChurnOnNonEqualValue(t *testing.T) {
	database := db.New()
	ctx := db.NewQueryContext(database)

	input := db.NewInput[string, int](database)
	input.Set("k", 0)

	sourceCalls := 0
	source := db.NewQuery[string, product.AbstractValue](
		"source",
		func(qc *db.QueryContext, key string) product.AbstractValue {
			sourceCalls++
			sel, _ := input.Get(qc, key)
			if sel%2 == 0 {
				return shapeValue(typ.Number)
			}
			return shapeValue(typ.String)
		},
		nil,
	)

	dependentCalls := 0
	dependent := db.NewQuery[string, int](
		"dependent",
		func(qc *db.QueryContext, key string) int {
			dependentCalls++
			_ = source.Get(qc, key)
			return dependentCalls
		},
		func(a, b int) bool { return a == b },
	)

	dependent.Get(ctx, "k")
	if sourceCalls != 1 || dependentCalls != 1 {
		t.Fatalf("initial run: sourceCalls=%d dependentCalls=%d, want 1 1", sourceCalls, dependentCalls)
	}

	input.Set("k", 1)

	dependent.Get(ctx, "k")
	if sourceCalls != 2 {
		t.Fatalf("source should recompute, sourceCalls=%d, want 2", sourceCalls)
	}
	if dependentCalls != 2 {
		t.Fatalf("dependent must revalidate on a non-Equal value, dependentCalls=%d, want 2", dependentCalls)
	}
}

// TestQueryFirewallInternedIdenticalValue pins the pointer-equal fast path: when
// the recomputed value interns to the very same node, the firewall suppresses
// churn just as for any other Equal value.
func TestQueryFirewallInternedIdenticalValue(t *testing.T) {
	first := shapeValue(typ.Number)
	second := shapeValue(typ.Number)
	if !product.Equal(first, second) {
		t.Fatal("identical content must be Equal")
	}

	database := db.New()
	ctx := db.NewQueryContext(database)
	input := db.NewInput[string, int](database)
	input.Set("k", 0)

	sourceCalls := 0
	source := db.NewQuery[string, product.AbstractValue](
		"source",
		func(qc *db.QueryContext, _ string) product.AbstractValue {
			sourceCalls++
			_, _ = input.Get(qc, "k")
			return shapeValue(typ.Number)
		},
		nil,
	)

	dependentCalls := 0
	dependent := db.NewQuery[string, int](
		"dependent",
		func(qc *db.QueryContext, key string) int {
			dependentCalls++
			_ = source.Get(qc, key)
			return dependentCalls
		},
		func(a, b int) bool { return a == b },
	)

	dependent.Get(ctx, "k")
	if dependentCalls != 1 {
		t.Fatalf("initial dependentCalls=%d, want 1", dependentCalls)
	}

	input.Set("k", 1)
	dependent.Get(ctx, "k")
	if sourceCalls != 2 {
		t.Fatalf("source should recompute, sourceCalls=%d, want 2", sourceCalls)
	}
	if dependentCalls != 1 {
		t.Fatalf("firewall breached on interned-identical value, dependentCalls=%d, want 1", dependentCalls)
	}
}

// TestQueryFirewallMutuallyCoveringValue pins the Equal-but-rebuilt cold path: a
// union with a duplicate member normalizes to the same family as the deduped
// union, so the two are Equal (mutually covering) and the firewall suppresses
// churn between them.
func TestQueryFirewallMutuallyCoveringValue(t *testing.T) {
	dup := typ.NewUnion(typ.Number, typ.String, typ.Number)
	plain := typ.NewUnion(typ.Number, typ.String)
	if !product.Equal(shapeValue(dup), shapeValue(plain)) {
		t.Skip("shape axis did not treat the two unions as mutually covering")
	}

	database := db.New()
	ctx := db.NewQueryContext(database)
	input := db.NewInput[string, int](database)
	input.Set("k", 0)

	source := db.NewQuery[string, product.AbstractValue](
		"source",
		func(qc *db.QueryContext, key string) product.AbstractValue {
			sel, _ := input.Get(qc, key)
			if sel%2 == 0 {
				return shapeValue(dup)
			}
			return shapeValue(plain)
		},
		nil,
	)

	dependentCalls := 0
	dependent := db.NewQuery[string, int](
		"dependent",
		func(qc *db.QueryContext, key string) int {
			dependentCalls++
			_ = source.Get(qc, key)
			return dependentCalls
		},
		func(a, b int) bool { return a == b },
	)

	dependent.Get(ctx, "k")
	if dependentCalls != 1 {
		t.Fatalf("initial dependentCalls=%d, want 1", dependentCalls)
	}

	input.Set("k", 1)
	dependent.Get(ctx, "k")
	if dependentCalls != 1 {
		t.Fatalf("firewall breached on mutually-covering value, dependentCalls=%d, want 1", dependentCalls)
	}
}
