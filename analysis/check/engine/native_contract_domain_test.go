package engine

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

// nativeFamilyValues collects the published contract values of one family.
func nativeFamilyValues(t *testing.T, source, family string) []string {
	t.Helper()
	result, err := Check(source)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if result.Native == nil {
		return nil
	}
	values := make([]string, 0)
	for _, fact := range result.Native.Facts() {
		if strings.HasPrefix(fact.Key, family+"/") {
			values = append(values, fact.Value)
		}
	}
	return values
}

// TestClosureCaptureEndsStraightLineAliasing pins that a closure allocation ends
// straight-line alias reasoning for the same reason a call does: the captured
// cell can be stored through by a later invocation this body does not contain.
// A body that only writes members keeps its disjointness contract.
func TestClosureCaptureEndsStraightLineAliasing(t *testing.T) {
	straight := nativeFamilyValues(t, `local a = {}
local b = a
b.x = 1
a.y = 2
return a, b
`, "alias_disjoint")
	if len(straight) == 0 {
		t.Fatal("a straight-line body published no alias contract; the probe no longer reaches the lane")
	}
	captured := nativeFamilyValues(t, `local a = {}
local b = a
local grab = function() b.x = 1 end
a.y = 2
return a, b, grab
`, "alias_disjoint")
	if len(captured) != 0 {
		t.Errorf("a table captured by a closure kept its straight-line alias contract: %v", captured)
	}
}

// TestSummaryExactnessHoldsForExactCapableResults pins the positive side of the
// exactness contract through the ordinary check path.
func TestSummaryExactnessHoldsForExactCapableResults(t *testing.T) {
	for name, source := range map[string]string{
		"record": `local function f(): { a: string } return { a = "x" } end
return f
`,
		"string": `local function f(): string return "x" end
return f
`,
		"array": `local function f(): string[] return {} end
return f
`,
	} {
		values := nativeFamilyValues(t, source, "interproc_summary")
		if len(values) == 0 || !strings.Contains(strings.Join(values, " "), "exactness=exact") {
			t.Errorf("%s result published %v, want an exact summary", name, values)
		}
	}
	optional := nativeFamilyValues(t, `local function f(): string? return nil end
return f
`, "interproc_summary")
	for _, value := range optional {
		if strings.Contains(value, "exactness=exact") {
			t.Errorf("an optional result published an exact summary: %v", optional)
		}
	}
}

// TestExactResultTypeRefusesOpenShapes pins the fail-closed default on the
// predicate itself: the exact-capable shapes are named, and every shape whose
// layout a call site must still discriminate is refused with the unrecognized
// ones. An interface, an intersection, a recursive unfolding and an unresolved
// reference are each satisfied by more than one layout, so no inline cache can
// be built against them.
func TestExactResultTypeRefusesOpenShapes(t *testing.T) {
	for name, item := range map[string]typ.Type{
		"string":      typ.String,
		"integer":     typ.Integer,
		"boolean":     typ.Boolean,
		"nil":         typ.Nil,
		"literal":     typ.LiteralString("a"),
		"array":       typ.NewArray(typ.String),
		"map":         typ.NewMap(typ.String, typ.String),
		"readonlymap": typ.NewReadonlyMap(typ.String, typ.String),
		"tuple":       typ.NewTuple(typ.String, typ.Integer),
	} {
		if !nativeTopologyExactResultType(item) {
			t.Errorf("%s is exact-capable but was refused", name)
		}
	}
	for name, item := range map[string]typ.Type{
		"optional":     typ.MaterializeOptional(typ.String),
		"union":        typ.MaterializeUnion([]typ.Type{typ.String, typ.Integer}),
		"intersection": typ.MaterializeIntersection([]typ.Type{typ.NewArray(typ.String), typ.NewMap(typ.String, typ.String)}),
		"interface":    typ.NewInterface("Shape", nil),
		"recursive":    typ.NewRecursivePlaceholder("R"),
		"reference":    typ.NewRef("other", "Named"),
		"meta":         typ.NewMeta(typ.String),
	} {
		if nativeTopologyExactResultType(item) {
			t.Errorf("%s describes more than one layout but was reported exact", name)
		}
	}
}

// TestTupleReturnCarriesPerSlotTemplate pins that a tuple's declared slots each
// acquire an allocation template, exactly as a record's declared fields do. A
// tuple states one type per integer slot, so nothing about it is representative
// and no slot may be dropped.
func TestTupleReturnCarriesPerSlotTemplate(t *testing.T) {
	value, published := placementDeclaredReturnTemplate(typ.NewTuple(
		typ.NewMap(typ.String, typ.String),
		typ.String,
	))
	if !published {
		t.Fatal("a tuple return published no allocation template")
	}
	suffixes := make([]string, 0, len(value.Table))
	for _, member := range value.Table {
		suffixes = append(suffixes, member.Suffix)
	}
	if len(suffixes) != 2 || suffixes[0] != "[1]" || suffixes[1] != "[2]" {
		t.Fatalf("tuple template members %v, want one per integer slot", suffixes)
	}
	if len(value.Table[0].Value.Table) == 0 {
		t.Fatal("a keyed container inside a tuple slot acquired no nested allocation")
	}
}
