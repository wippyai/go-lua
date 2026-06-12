package key

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
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

func TestSlotMethods(t *testing.T) {
	symbolSlot := Slot{symbol: 42}
	if got, ok := symbolSlot.Symbol(); !ok || got != 42 {
		t.Fatalf("Slot{symbol:42}.Symbol = %d/%v, want 42/true", got, ok)
	}
	if _, ok := symbolSlot.Key(); ok {
		t.Fatal("Slot{symbol:42}.Key reported present")
	}
	if got, ok := symbolSlot.Value(); !ok || got != SymbolValue(42) {
		t.Fatalf("Slot{symbol:42}.Value = %q/%v, want s42/true", got, ok)
	}

	keySlot := Slot{key: ReturnSlot(1)}
	if got, ok := keySlot.Key(); !ok || got != ReturnSlot(1) {
		t.Fatalf("Slot{key:r1}.Key = %q/%v, want r1/true", got, ok)
	}
	if got, ok := keySlot.Value(); !ok || got != ReturnSlot(1) {
		t.Fatalf("Slot{key:r1}.Value = %q/%v, want r1/true", got, ok)
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

func TestParseResolverPathAndRootSuffix(t *testing.T) {
	sym, version, suffix, ok := ParseResolverPath(`sym42@3.field["k"]`)
	if !ok || sym != 42 || version != 3 || suffix != `.field["k"]` {
		t.Fatalf("ParseResolverPath = %d/%d/%q/%v, want 42/3/suffix/true", sym, version, suffix, ok)
	}
	if _, _, _, ok := ParseResolverPath("sym1@.field"); ok {
		t.Fatal("ParseResolverPath accepted invalid version")
	}
	if _, _, _, ok := ParseResolverPath("sym1@1[bad]"); ok {
		t.Fatal("ParseResolverPath accepted invalid suffix")
	}
}

func TestStateKeyAndPathKeySpellingsStayDisjoint(t *testing.T) {
	if got := SymbolValue(12); got == Value("sym12") {
		t.Fatalf("SymbolValue(12) collided with path-key spelling: %q", got)
	}
	if got := ReturnSlot(0); got == Value("$0") {
		t.Fatalf("ReturnSlot(0) collided with placeholder spelling: %q", got)
	}
	if _, ok := ParseSymbolValue(Value("sym12")); ok {
		t.Fatal("ParseSymbolValue accepted path-key spelling sym12")
	}
	if _, ok := ParseSymbolValue(Value("sym12@3.field")); ok {
		t.Fatal("ParseSymbolValue accepted versioned path-key spelling sym12@3.field")
	}
	if _, ok := ParseReturnSlot(Value("ret[0]")); ok {
		t.Fatal("ParseReturnSlot accepted return-path spelling ret[0]")
	}
	if _, ok := ParseReturnSlot(Value("$0")); ok {
		t.Fatal("ParseReturnSlot accepted placeholder spelling $0")
	}

	sym, version, suffix, ok := ParseResolverPath("sym12@3.field")
	if !ok || sym != 12 || version != 3 || suffix != ".field" {
		t.Fatalf("ParseResolverPath(versioned) = %d/%d/%q/%v, want 12/3/.field/true", sym, version, suffix, ok)
	}
	if got := SymbolVersionRoot(sym, version); got != "sym12@3" {
		t.Fatalf("SymbolVersionRoot = %q, want sym12@3", got)
	}
}

func TestResolverPathKeyIsVersionedAndDistinctFromStableAddressKey(t *testing.T) {
	segments := []segment.Segment{{Kind: segment.SegmentField, Name: "field"}}
	resolverKey := ResolverPath(SymbolVersionRoot(12, 3) + segment.FormatSegments(segments))
	if got := resolverKey.PathKey(); got != "sym12@3.field" {
		t.Fatalf("resolver key = %q, want versioned verbose path", got)
	}

	sym, version, suffix, ok := ParseResolverPath(resolverKey.PathKey())
	if !ok || sym != 12 || version != 3 || suffix != ".field" {
		t.Fatalf("ParseResolverPath = %d/%d/%q/%v, want 12/3/.field/true", sym, version, suffix, ok)
	}
	if _, _, _, ok := ParseResolverPath("s12.field"); ok {
		t.Fatal("ParseResolverPath accepted compact stable address key")
	}
	if _, _, _, ok := ParseResolverPath("$0.field"); ok {
		t.Fatal("ParseResolverPath accepted placeholder local path key")
	}
}

func TestResolverPathUsesPureKeySpelling(t *testing.T) {
	segments := []segment.Segment{
		{Kind: segment.SegmentField, Name: "field"},
		{Kind: segment.SegmentIndexString, Name: "k"},
	}
	if got := ResolverPath(SymbolVersionRoot(42, 3) + segment.FormatSegments(segments)).PathKey(); got != `sym42@3.field["k"]` {
		t.Fatalf("ResolverPath = %q, want verbose versioned path", got)
	}
	if got := SymbolVersionRoot(0, 3); got != "" {
		t.Fatalf("SymbolVersionRoot with zero symbol = %q, want empty", got)
	}
	if got := SymbolVersionRoot(42, 0); got != "" {
		t.Fatalf("SymbolVersionRoot with zero version = %q, want empty", got)
	}
}

func TestPackageDoesNotImportIRorEngine(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", ".")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps . failed: %v", err)
	}
	banned := []string{
		"github.com/wippyai/go-lua/analysis/ir",
		"github.com/wippyai/go-lua/analysis/engine",
	}
	for _, dep := range strings.Fields(string(out)) {
		for _, prefix := range banned {
			if dep == prefix || strings.HasPrefix(dep, prefix+"/") {
				t.Fatalf("key package imports forbidden dependency %q", dep)
			}
		}
	}
}
