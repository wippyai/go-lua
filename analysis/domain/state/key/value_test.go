package key

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"

	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
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
	symbolSlot, ok := SymbolSlot(42)
	if !ok {
		t.Fatal("SymbolSlot(42) failed")
	}
	if got, ok := symbolSlot.Symbol(); !ok || got != 42 {
		t.Fatalf("Slot{symbol:42}.Symbol = %d/%v, want 42/true", got, ok)
	}
	if _, ok := symbolSlot.Key(); ok {
		t.Fatal("Slot{symbol:42}.Key reported present")
	}
	if got, ok := symbolSlot.Value(); !ok || got != SymbolValue(42) {
		t.Fatalf("Slot{symbol:42}.Value = %q/%v, want s42/true", got, ok)
	}

	keySlot, ok := KeySlot(ReturnSlot(1))
	if !ok {
		t.Fatal("KeySlot(r1) failed")
	}
	if got, ok := keySlot.Key(); !ok || got != ReturnSlot(1) {
		t.Fatalf("Slot{key:r1}.Key = %q/%v, want r1/true", got, ok)
	}
	if got, ok := keySlot.Value(); !ok || got != ReturnSlot(1) {
		t.Fatalf("Slot{key:r1}.Value = %q/%v, want r1/true", got, ok)
	}
}

func TestSlotConstructorsCanonicalizeValueKeys(t *testing.T) {
	if slot, ok := SymbolSlot(0); ok || slot != (Slot{}) {
		t.Fatalf("SymbolSlot(0) = %#v/%v, want zero false", slot, ok)
	}
	if slot, ok := KeySlot(""); ok || slot != (Slot{}) {
		t.Fatalf("KeySlot(empty) = %#v/%v, want zero false", slot, ok)
	}
	if slot, ok := KeySlot(SymbolValue(42)); ok || slot != (Slot{}) {
		t.Fatalf("KeySlot(symbol value) = %#v/%v, want zero false", slot, ok)
	}

	symbolSlot, ok := SlotOfValue(SymbolValue(42))
	if !ok {
		t.Fatal("SlotOfValue(s42) failed")
	}
	if got, ok := symbolSlot.Symbol(); !ok || got != 42 {
		t.Fatalf("SlotOfValue(s42).Symbol = %d/%v, want 42/true", got, ok)
	}

	returnSlot, ok := SlotOfValue(ReturnSlot(2))
	if !ok {
		t.Fatal("SlotOfValue(r2) failed")
	}
	if got, ok := returnSlot.Key(); !ok || got != ReturnSlot(2) {
		t.Fatalf("SlotOfValue(r2).Key = %q/%v, want r2/true", got, ok)
	}
	if symbolSlot.Equal(returnSlot) {
		t.Fatalf("distinct slot constructors produced equal slots: %#v", symbolSlot)
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
}

func TestAddressResolverGrammarIsOwnedElsewhere(t *testing.T) {
	sym, version, suffix, ok := pathaddr.ParseResolverPath("sym12@3.field")
	if !ok || sym != 12 || version != 3 || suffix != ".field" {
		t.Fatalf("ParseResolverPath = %d/%d/%q/%v, want 12/3/.field/true", sym, version, suffix, ok)
	}
	if got := pathaddr.VersionedRootString(42, 3); got != "sym42@3" {
		t.Fatalf("VersionedRootString = %q, want sym42@3", got)
	}
	if got := pathaddr.VersionedRootString(0, 3); got != "" {
		t.Fatalf("VersionedRootString with zero symbol = %q, want empty", got)
	}
	if got := pathaddr.VersionedRootString(42, 0); got != "" {
		t.Fatalf("VersionedRootString with zero version = %q, want empty", got)
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
