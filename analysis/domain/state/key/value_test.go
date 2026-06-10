package key

import (
	"strconv"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/ssa"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestValueConstructorsAreStableAndTyped(t *testing.T) {
	if got := SymbolValue(42); got != Value("s42") {
		t.Fatalf("SymbolValue(42) = %q, want s42", got)
	}
	if got := ReturnSlot(3); got != Value("r3") {
		t.Fatalf("ReturnSlot(3) = %q, want r3", got)
	}
	if got := ReturnSlot(-1); got != "" {
		t.Fatalf("ReturnSlot(-1) = %q, want empty", got)
	}
}

func TestValueSlotNormalizesSymbolKeys(t *testing.T) {
	symbolSlot, ok := ValueSlot(SymbolValue(42))
	if !ok {
		t.Fatal("ValueSlot(symbol) reported absent")
	}
	if got, ok := symbolSlot.Symbol(); !ok || got != 42 {
		t.Fatalf("ValueSlot(symbol).Symbol = %d/%v, want 42/true", got, ok)
	}
	if _, ok := symbolSlot.Key(); ok {
		t.Fatal("ValueSlot(symbol).Key reported present")
	}
	if got, ok := symbolSlot.Value(); !ok || got != SymbolValue(42) {
		t.Fatalf("ValueSlot(symbol).Value = %q/%v, want s42/true", got, ok)
	}

	keySlot, ok := ValueSlot(ReturnSlot(1))
	if !ok {
		t.Fatal("ValueSlot(return) reported absent")
	}
	if got, ok := keySlot.Key(); !ok || got != ReturnSlot(1) {
		t.Fatalf("ValueSlot(return).Key = %q/%v, want r1/true", got, ok)
	}
}

func TestParseValueKeysRejectInvalidShapes(t *testing.T) {
	if sym, ok := ParseSymbolValue(ReturnSlot(0)); ok || sym != 0 {
		t.Fatalf("ParseSymbolValue(return) = %d/%v, want false", sym, ok)
	}
	if sym, ok := ParseSymbolValue(Value("s42.field")); ok || sym != 0 {
		t.Fatalf("ParseSymbolValue(s42.field) = %d/%v, want false", sym, ok)
	}
	if idx, ok := ParseReturnSlot(SymbolValue(1)); ok || idx != 0 {
		t.Fatalf("ParseReturnSlot(symbol) = %d/%v, want false", idx, ok)
	}
	if idx, ok := ParseReturnSlot(Value("r1.field")); ok || idx != 0 {
		t.Fatalf("ParseReturnSlot(bad) = %d/%v, want false", idx, ok)
	}
}

func TestParseReturnSlotRejectsOverflow(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	overflow := "r" + strconv.FormatInt(int64(maxInt), 10) + "0"
	if idx, ok := ParseReturnSlot(Value(overflow)); ok || idx != 0 {
		t.Fatalf("ParseReturnSlot(%q) = %d/%v, want false", overflow, idx, ok)
	}
}

type mockVersions struct {
	versions map[cfg.Point]map[symbol.ID]ssa.Version
}

func (m mockVersions) VisibleVersion(point cfg.Point, sym symbol.ID) ssa.Version {
	if byPoint, ok := m.versions[point]; ok {
		return byPoint[sym]
	}
	return ssa.Version{}
}

func TestResolverKeyAtUsesVisibleVersion(t *testing.T) {
	resolver := NewResolver(mockVersions{
		versions: map[cfg.Point]map[symbol.ID]ssa.Version{
			1: {100: {Root: "x", Symbol: 100, ID: 3}},
		},
	})
	path := pathdom.NewPath(100, "x").Field("field")

	if got, want := resolver.KeyAt(1, path), pathdom.PathKey("sym100@3.field"); got != want {
		t.Fatalf("KeyAt(versioned path) = %q, want %q", got, want)
	}
}

func TestResolverRejectsMissingVersionAndUnresolvedRoot(t *testing.T) {
	resolver := NewResolver(mockVersions{})
	if got := resolver.KeyAt(1, pathdom.NewPath(100, "x")); got != "" {
		t.Fatalf("KeyAt without version = %q, want empty", got)
	}
	if got := resolver.KeyAt(1, pathdom.Path{Root: "x"}); got != "" {
		t.Fatalf("KeyAt unresolved root = %q, want empty", got)
	}
	if got := resolver.KeyAt(1, pathdom.Path{}); got != "" {
		t.Fatalf("KeyAt empty = %q, want empty", got)
	}
}

func TestResolverPlaceholderUsesCurrentPathKey(t *testing.T) {
	resolver := NewResolver(nil)
	path := pathdom.NewPlaceholder(0).IndexStr("item")

	if got, want := resolver.KeyAt(1, path), path.Key(); got != want {
		t.Fatalf("KeyAt(placeholder) = %q, want %q", got, want)
	}
}

func TestParsePathKeyAndRootSuffix(t *testing.T) {
	sym, version, suffix, ok := ParsePathKey(`sym42@3.field["k"]`)
	if !ok || sym != 42 || version != 3 || suffix != `.field["k"]` {
		t.Fatalf("ParsePathKey = %d/%d/%q/%v, want 42/3/suffix/true", sym, version, suffix, ok)
	}
	if _, _, _, ok := ParsePathKey("sym1@.field"); ok {
		t.Fatal("ParsePathKey accepted invalid version")
	}
	if _, _, _, ok := ParsePathKey("sym1@1[bad]"); ok {
		t.Fatal("ParsePathKey accepted invalid suffix")
	}
}
