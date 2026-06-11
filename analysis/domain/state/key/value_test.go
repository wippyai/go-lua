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

func TestSymbolVersionPathUsesPureKeySpelling(t *testing.T) {
	segments := []segment.Segment{
		{Kind: segment.SegmentField, Name: "field"},
		{Kind: segment.SegmentIndexString, Name: "k"},
	}
	if got := SymbolVersionPath(42, 3, segments); got != `sym42@3.field["k"]` {
		t.Fatalf("SymbolVersionPath = %q, want verbose versioned path", got)
	}
	if got := SymbolVersionPath(0, 3, segments); got != "" {
		t.Fatalf("SymbolVersionPath with zero symbol = %q, want empty", got)
	}
	if got := SymbolVersionPath(42, 0, segments); got != "" {
		t.Fatalf("SymbolVersionPath with zero version = %q, want empty", got)
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
