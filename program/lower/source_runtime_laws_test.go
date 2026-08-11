package lower_test

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

func TestSourceBodiesKeepOrderAndParents(t *testing.T) {
	p := parseBindLower(t, "\ndo\n  local normal = 1\n  do local nested = 2 end\nend\ndo return 3 end\nreturn 4\n")
	entry, ok := p.Source().Index().Entry()
	if !ok {
		t.Fatal("missing Source entry")
	}
	if count, ok := p.Source().Order().BodyLen(entry); !ok || count != 3 {
		t.Fatalf("entry Source order = %d/%v, want 3", count, ok)
	}
	normal := controlSourceAt(t, p, entry, 0)
	terminal := controlSourceAt(t, p, entry, 1)
	for _, body := range []keyspace.Term{normal, terminal} {
		if parent, ok := p.Source().Index().BodyParent(body); !ok || parent != entry {
			t.Fatalf("Body parent = %v/%v, want %v", parent, ok, entry)
		}
	}
	nested := controlSourceAt(t, p, normal, 1)
	if parent, ok := p.Source().Index().BodyParent(nested); !ok || parent != normal {
		t.Fatalf("nested Body parent = %v/%v, want %v", parent, ok, normal)
	}
	if body, offset, _, ok := p.Source().Index().Position(nested); !ok || body != normal || offset != 1 {
		t.Fatalf("nested Source position = body %v offset %d ok %v", body, offset, ok)
	}
}

func TestFlowDirectFunctionsDistinguishDirectAndOrdinaryCalls(t *testing.T) {
	for _, sample := range []struct {
		name  string
		input string
		want  bool
	}{
		{"direct-recursion", "local function f() return f() end", true},
		{"ordinary-initializer", "local f = function() return f() end", false},
		{"preinstallation", "local f\nf()\nf = function() return 1 end", false},
	} {
		t.Run(sample.name, func(t *testing.T) {
			p := parseBindLower(t, sample.input)
			call, ok := p.Flow().Authored().Calls().At(0)
			if !ok {
				t.Fatal("missing Call")
			}
			direct, directOK := p.Flow().DirectFunctions().Call(call)
			if sample.want && (!directOK || direct == 0) {
				t.Fatalf("DirectFunctions.Call = %v/%v, want Function", direct, directOK)
			}
			if !sample.want && (directOK || direct != 0) {
				t.Fatalf("DirectFunctions.Call = %v/%v, want absent", direct, directOK)
			}
		})
	}
}

func TestSourceExactKeysNormalizeWithoutSharingOccurrences(t *testing.T) {
	p := parseBindLower(t, "local t = {}\nt[-0.0] = 1\nt[0] = 2\nreturn t")
	exact := p.Flow().Authored().Access().Exact()
	if exact.Count() != 2 {
		t.Fatalf("exact Lens count = %d, want 2", exact.Count())
	}
	left, _ := exact.At(0)
	right, _ := exact.At(1)
	if left == right {
		t.Fatal("distinct numeric Lens occurrences were shared")
	}
	_, _, leftSource, leftKind, leftOK := exact.Get(left)
	_, _, rightSource, rightKind, rightOK := exact.Get(right)
	if !leftOK || !rightOK || leftKind != kind.FieldExact || rightKind != kind.FieldExact {
		t.Fatalf("exact Lens rows = %v/%v %v/%v", leftKind, leftOK, rightKind, rightOK)
	}
	leftKey, leftKeyOK := exactLiteralKey(t, p, leftSource)
	rightKey, rightKeyOK := exactLiteralKey(t, p, rightSource)
	if !leftKeyOK || !rightKeyOK || leftKey != rightKey {
		t.Fatalf("-0.0/0 exact keys = %v/%v %v/%v", leftKey, leftKeyOK, rightKey, rightKeyOK)
	}
	value, ok := p.Source().Keys().Exact(leftKey)
	if !ok || value != (keyspace.LiteralValue{Kind: keyspace.LiteralInteger}) {
		t.Fatalf("normalized zero key = %#v/%v", value, ok)
	}
}

func TestSourceExactKeyEnumerationIsCanonicalAndAllocationFree(t *testing.T) {
	p := parseBindLower(t, "return {[true] = 1, [false] = 2, [7] = 3, [1.5] = 4, field = 5, [-0.0] = 6, [0] = 7, [7.0] = 8}")
	keys := p.Source().Keys()
	want := []keyspace.LiteralValue{
		{Kind: keyspace.LiteralBool},
		{Kind: keyspace.LiteralBool, Bool: true},
		{Kind: keyspace.LiteralInteger},
		{Kind: keyspace.LiteralInteger, Integer: 7},
		{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(1.5)},
		{Kind: keyspace.LiteralString, String: "field"},
	}
	if keys.ExactCount() != len(want) {
		t.Fatalf("Source exact key count = %d, want %d", keys.ExactCount(), len(want))
	}
	for index, expected := range want {
		key, value, ok := keys.ExactAt(index)
		if !ok || key == 0 || value != expected {
			t.Fatalf("Source ExactAt(%d) = %v/%#v/%v, want nonzero/%#v/true", index, key, value, ok, expected)
		}
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		exactKeySink, _, exactKeyOK = keys.ExactAt(3)
	}); allocations != 0 {
		t.Fatalf("Source ExactAt allocations = %v, want 0", allocations)
	}
}

func TestFlowImplicitReadsKeepFirstGlobalOccurrenceOrder(t *testing.T) {
	p := parseBindLower(t, "return first, second, first, second")
	reads := p.Flow().Authored().Storage().Reads()
	if reads.ImplicitCount() != 2 {
		t.Fatalf("implicit Read count = %d, want 2", reads.ImplicitCount())
	}
	for index, want := range []string{"first", "second"} {
		read, _ := reads.ImplicitAt(index)
		_, cell, _, readOK := reads.Get(read)
		_, _, key, cellOK := p.Flow().Authored().Storage().Cells().Get(cell)
		value, keyOK := p.Source().Keys().Exact(key)
		if !readOK || !cellOK || !keyOK || value.Kind != keyspace.LiteralString || value.String != want {
			t.Fatalf("implicit Read[%d] = cell %v key %#v read/cell/key=%v/%v/%v, want %q", index, cell, value, readOK, cellOK, keyOK, want)
		}
	}
}

func TestStaticFunctionContractsKeepOmittedAndExplicitEmptyReturns(t *testing.T) {
	p := parseBindLower(t, "\nlocal function inferred() end\nlocal function empty(): () end\nreturn inferred, empty\n")
	functions := p.Flow().Authored().Functions()
	for index, wantKnown := range []bool{false, true} {
		function, _ := functions.At(index)
		known, ok := p.Static().Contracts().Functions().Get(function)
		if !ok || known != wantKnown {
			t.Fatalf("Function contract[%d] = %v/%v, want %v/true", index, known, ok, wantKnown)
		}
		if count, ok := p.Static().Contracts().Functions().ReturnCount(function); !ok || count != 0 {
			t.Fatalf("Function return count[%d] = %d/%v, want 0/true", index, count, ok)
		}
	}
}

var (
	exactKeySink keyspace.Key
	exactKeyOK   bool
)

var _ flow.CellKind
