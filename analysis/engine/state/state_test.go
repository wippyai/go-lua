package state

import (
	"os/exec"
	"strings"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestBottomReadsProductBottom(t *testing.T) {
	reg := product.DefaultRegistry()
	valueDomain := product.Domain(reg)
	var s State

	if got := s.ReadValue(reg, key.SymbolValue(1)); !valueDomain.Equal(got, valueDomain.Bottom()) {
		t.Fatalf("absent value slot = %s, want product bottom", formatValue(reg, got))
	}
	if got := s.ReadPathKey(reg, pathdom.PathKey("sym1@1.field")); !valueDomain.Equal(got, valueDomain.Bottom()) {
		t.Fatalf("absent path key = %s, want product bottom", formatValue(reg, got))
	}
}

func TestWriteReadValueSlots(t *testing.T) {
	reg := product.DefaultRegistry()
	valueDomain := product.Domain(reg)
	symSlot := key.SymbolValue(symbol.ID(10))
	retSlot := key.ReturnSlot(1)
	symValue := presentValue(reg)
	retValue := absentValue(reg)

	s := State{}.
		WriteValue(reg, symSlot, symValue).
		WriteValue(reg, retSlot, retValue)

	if got := s.ReadValue(reg, symSlot); !valueDomain.Equal(got, symValue) {
		t.Fatalf("symbol slot = %s, want %s", formatValue(reg, got), formatValue(reg, symValue))
	}
	if got := s.ReadValue(reg, retSlot); !valueDomain.Equal(got, retValue) {
		t.Fatalf("return slot = %s, want %s", formatValue(reg, got), formatValue(reg, retValue))
	}
}

func TestWritesAreImmutable(t *testing.T) {
	reg := product.DefaultRegistry()
	valueDomain := product.Domain(reg)
	slot := key.SymbolValue(symbol.ID(11))
	pathKey := pathdom.PathKey("sym11@1.field")
	present := presentValue(reg)
	absent := absentValue(reg)

	s1 := State{}.
		WriteValue(reg, slot, present).
		WritePathKey(reg, pathKey, present)
	s2 := s1.
		WriteValue(reg, slot, absent).
		WritePathKey(reg, pathKey, absent)

	if got := s1.ReadValue(reg, slot); !valueDomain.Equal(got, present) {
		t.Fatalf("original value slot changed to %s", formatValue(reg, got))
	}
	if got := s1.ReadPathKey(reg, pathKey); !valueDomain.Equal(got, present) {
		t.Fatalf("original path key changed to %s", formatValue(reg, got))
	}
	if got := s2.ReadValue(reg, slot); !valueDomain.Equal(got, absent) {
		t.Fatalf("updated value slot = %s, want absent value", formatValue(reg, got))
	}
	if got := s2.ReadPathKey(reg, pathKey); !valueDomain.Equal(got, absent) {
		t.Fatalf("updated path key = %s, want absent value", formatValue(reg, got))
	}
}

func TestUpdateHelpersReadCurrentAndCanonicalizeBottom(t *testing.T) {
	reg := product.DefaultRegistry()
	valueDomain := product.Domain(reg)
	stateDomain := Domain(reg)
	slot := key.SymbolValue(symbol.ID(12))
	retSlot := 0
	pathKey := pathdom.PathKey("sym12@1.field")
	present := presentValue(reg)
	absent := absentValue(reg)
	bottom := valueDomain.Bottom()

	s1 := State{}.
		WriteValue(reg, slot, present).
		WriteReturnSlot(reg, retSlot, present).
		WritePathKey(reg, pathKey, present)
	s2 := s1.
		UpdateValue(reg, slot, func(got product.Value) product.Value {
			if !valueDomain.Equal(got, present) {
				t.Fatalf("UpdateValue read %s, want present", formatValue(reg, got))
			}
			return bottom
		}).
		UpdateReturnSlot(reg, retSlot, func(got product.Value) product.Value {
			if !valueDomain.Equal(got, present) {
				t.Fatalf("UpdateReturnSlot read %s, want present", formatValue(reg, got))
			}
			return absent
		}).
		UpdatePathKey(reg, pathKey, func(got product.Value) product.Value {
			if !valueDomain.Equal(got, present) {
				t.Fatalf("UpdatePathKey read %s, want present", formatValue(reg, got))
			}
			return bottom
		})

	if got := s1.ReadValue(reg, slot); !valueDomain.Equal(got, present) {
		t.Fatalf("original value slot changed to %s", formatValue(reg, got))
	}
	if got := s1.ReadReturnSlot(reg, retSlot); !valueDomain.Equal(got, present) {
		t.Fatalf("original return slot changed to %s", formatValue(reg, got))
	}
	if got := s1.ReadPathKey(reg, pathKey); !valueDomain.Equal(got, present) {
		t.Fatalf("original path key changed to %s", formatValue(reg, got))
	}
	if got := s2.ReadValue(reg, slot); !valueDomain.Equal(got, bottom) {
		t.Fatalf("updated value slot = %s, want bottom", formatValue(reg, got))
	}
	if got := s2.ReadReturnSlot(reg, retSlot); !valueDomain.Equal(got, absent) {
		t.Fatalf("updated return slot = %s, want absent", formatValue(reg, got))
	}
	if got := s2.ReadPathKey(reg, pathKey); !valueDomain.Equal(got, bottom) {
		t.Fatalf("updated path key = %s, want bottom", formatValue(reg, got))
	}
	if _, ok := s2.values[slot]; ok {
		t.Fatalf("UpdateValue to bottom kept finite value entry")
	}
	if _, ok := s2.paths[pathKey]; ok {
		t.Fatalf("UpdatePathKey to bottom kept finite path entry")
	}
	if !stateDomain.Equal(State{}.WriteReturnSlot(reg, retSlot, absent), State{}.WriteValue(reg, key.ReturnSlot(retSlot), absent)) {
		t.Fatalf("return-slot helper does not use key.ReturnSlot spelling")
	}
}

func TestDomainPointwiseOperations(t *testing.T) {
	reg := product.DefaultRegistry()
	valueDomain := product.Domain(reg)
	stateDomain := Domain(reg)
	present := presentValue(reg)
	absent := absentValue(reg)
	valueSlot := key.SymbolValue(symbol.ID(21))
	retSlot := key.ReturnSlot(0)
	pathKey := pathdom.PathKey("sym21@2.field")
	otherPathKey := pathdom.PathKey("$0.item")

	a := State{}.
		WriteValue(reg, valueSlot, present).
		WritePathKey(reg, pathKey, present)
	b := State{}.
		WriteValue(reg, valueSlot, absent).
		WriteValue(reg, retSlot, present).
		WritePathKey(reg, pathKey, absent).
		WritePathKey(reg, otherPathKey, present)

	joined := stateDomain.Join(a, b)
	if got := joined.ReadValue(reg, valueSlot); !valueDomain.Equal(got, product.Top()) {
		t.Fatalf("joined shared value slot = %s, want top", formatValue(reg, got))
	}
	if got := joined.ReadValue(reg, retSlot); !valueDomain.Equal(got, present) {
		t.Fatalf("joined disjoint value slot = %s, want present", formatValue(reg, got))
	}
	if got := joined.ReadPathKey(reg, pathKey); !valueDomain.Equal(got, product.Top()) {
		t.Fatalf("joined shared path key = %s, want top", formatValue(reg, got))
	}
	if got := joined.ReadPathKey(reg, otherPathKey); !valueDomain.Equal(got, present) {
		t.Fatalf("joined disjoint path key = %s, want present", formatValue(reg, got))
	}

	if widened := stateDomain.Widen(a, b); !stateDomain.Equal(widened, joined) {
		t.Fatalf("Widen differs from Join: got %s, want %s", formatState(reg, widened), formatState(reg, joined))
	}
	if !stateDomain.LessOrEq(a, joined) || !stateDomain.LessOrEq(b, joined) {
		t.Fatalf("Join is not an upper bound: a=%s b=%s joined=%s",
			formatState(reg, a), formatState(reg, b), formatState(reg, joined))
	}
	if stateDomain.LessOrEq(joined, a) {
		t.Fatalf("joined state unexpectedly <= left operand")
	}
	if stateDomain.Equal(a, b) {
		t.Fatalf("states with different pointwise lanes compare equal")
	}
	if !stateDomain.Equal(a, a.Clone()) {
		t.Fatalf("Clone should preserve state equality")
	}
}

func TestExplicitBottomEntriesCanonicalizeToAbsence(t *testing.T) {
	reg := product.DefaultRegistry()
	valueDomain := product.Domain(reg)
	stateDomain := Domain(reg)
	bottom := valueDomain.Bottom()
	explicit := State{
		values: map[key.Value]product.Value{
			key.ReturnSlot(0): bottom,
		},
		paths: map[pathdom.PathKey]product.Value{
			pathdom.PathKey("sym1@1.field"): bottom,
		},
	}

	if !stateDomain.Equal(explicit, State{}) {
		t.Fatalf("explicit bottom entries should equal absence")
	}
	joined := stateDomain.Join(explicit, State{})
	if !stateDomain.Equal(joined, State{}) {
		t.Fatalf("Join should canonicalize bottom entries away, got %s", formatState(reg, joined))
	}
	if len(joined.values) != 0 || len(joined.paths) != 0 {
		t.Fatalf("Join kept bottom entries: values=%d paths=%d", len(joined.values), len(joined.paths))
	}

	withValue := State{}.WriteValue(reg, key.ReturnSlot(0), presentValue(reg))
	withoutValue := withValue.WriteValue(reg, key.ReturnSlot(0), bottom)
	if !stateDomain.Equal(withoutValue, State{}) {
		t.Fatalf("writing bottom should delete the value entry, got %s", formatState(reg, withoutValue))
	}
}

func TestInvalidatePathKeySubtreeRemovesStructuredDescendants(t *testing.T) {
	reg := product.DefaultRegistry()
	valueDomain := product.Domain(reg)
	present := presentValue(reg)
	bottom := valueDomain.Bottom()
	root := pathdom.PathKey("sym40@3")
	prefix := pathdom.PathKey("sym40@3.field")
	child := pathdom.PathKey("sym40@3.field.deep")
	siblingPrefixCollision := pathdom.PathKey("sym40@3.fieldish")
	localVersionless := pathdom.PathKey("sym40.field.deep")
	otherVersion := pathdom.PathKey("sym40@4.field.deep")
	otherSymbol := pathdom.PathKey("sym41@3.field.deep")
	placeholderPrefix := pathdom.PathKey("$0.field")
	placeholderChild := pathdom.PathKey("$0.field.deep")
	placeholderSibling := pathdom.PathKey("$0.fieldish")

	s := State{}.
		WritePathKey(reg, root, present).
		WritePathKey(reg, prefix, present).
		WritePathKey(reg, child, present).
		WritePathKey(reg, siblingPrefixCollision, present).
		WritePathKey(reg, localVersionless, present).
		WritePathKey(reg, otherVersion, present).
		WritePathKey(reg, otherSymbol, present).
		WritePathKey(reg, placeholderPrefix, present).
		WritePathKey(reg, placeholderChild, present).
		WritePathKey(reg, placeholderSibling, present)

	invalidPrefix, ok := s.InvalidatePathKeySubtree(pathdom.PathKey(".field"))
	if ok {
		t.Fatal("InvalidatePathKeySubtree accepted invalid path key")
	}
	if !Domain(reg).Equal(invalidPrefix, s) {
		t.Fatal("invalid path-key prefix changed state")
	}

	out, ok := s.InvalidatePathKeySubtree(prefix)
	if !ok {
		t.Fatal("InvalidatePathKeySubtree rejected versioned prefix")
	}
	for _, removed := range []pathdom.PathKey{prefix, child} {
		if got := out.ReadPathKey(reg, removed); !valueDomain.Equal(got, bottom) {
			t.Fatalf("%s = %s, want bottom", removed, formatValue(reg, got))
		}
	}
	for _, kept := range []pathdom.PathKey{root, siblingPrefixCollision, localVersionless, otherVersion, otherSymbol, placeholderPrefix, placeholderChild, placeholderSibling} {
		if got := out.ReadPathKey(reg, kept); !valueDomain.Equal(got, present) {
			t.Fatalf("%s = %s, want present", kept, formatValue(reg, got))
		}
	}
	if got := s.ReadPathKey(reg, child); !valueDomain.Equal(got, present) {
		t.Fatalf("original child changed to %s", formatValue(reg, got))
	}

	out, ok = out.InvalidatePathKeySubtree(placeholderPrefix)
	if !ok {
		t.Fatal("InvalidatePathKeySubtree rejected placeholder prefix")
	}
	for _, removed := range []pathdom.PathKey{placeholderPrefix, placeholderChild} {
		if got := out.ReadPathKey(reg, removed); !valueDomain.Equal(got, bottom) {
			t.Fatalf("%s = %s, want bottom", removed, formatValue(reg, got))
		}
	}
	if got := out.ReadPathKey(reg, placeholderSibling); !valueDomain.Equal(got, present) {
		t.Fatalf("%s = %s, want present", placeholderSibling, formatValue(reg, got))
	}
}

func TestTopLanesReadTopAndRejectFiniteUpdates(t *testing.T) {
	reg := product.DefaultRegistry()
	valueDomain := product.Domain(reg)
	top := Domain(reg).Top()
	slot := key.SymbolValue(symbol.ID(50))
	pathKey := pathdom.PathKey("sym50@1.field")
	present := presentValue(reg)

	if got := top.ReadValue(reg, slot); !valueDomain.Equal(got, product.Top()) {
		t.Fatalf("top value read = %s, want top", formatValue(reg, got))
	}
	if got := top.ReadReturnSlot(reg, 0); !valueDomain.Equal(got, product.Top()) {
		t.Fatalf("top return read = %s, want top", formatValue(reg, got))
	}
	if got := top.ReadPathKey(reg, pathKey); !valueDomain.Equal(got, product.Top()) {
		t.Fatalf("top path read = %s, want top", formatValue(reg, got))
	}

	requirePanic(t, func() {
		top.WriteValue(reg, slot, present)
	})
	requirePanic(t, func() {
		top.UpdateValue(reg, slot, func(v product.Value) product.Value {
			if !valueDomain.Equal(v, product.Top()) {
				t.Fatalf("UpdateValue on top read %s, want top", formatValue(reg, v))
			}
			return present
		})
	})
	requirePanic(t, func() {
		top.WriteReturnSlot(reg, 0, present)
	})
	requirePanic(t, func() {
		top.UpdateReturnSlot(reg, 0, func(v product.Value) product.Value {
			if !valueDomain.Equal(v, product.Top()) {
				t.Fatalf("UpdateReturnSlot on top read %s, want top", formatValue(reg, v))
			}
			return present
		})
	})
	requirePanic(t, func() {
		top.WritePathKey(reg, pathKey, present)
	})
	requirePanic(t, func() {
		top.UpdatePathKey(reg, pathKey, func(v product.Value) product.Value {
			if !valueDomain.Equal(v, product.Top()) {
				t.Fatalf("UpdatePathKey on top read %s, want top", formatValue(reg, v))
			}
			return present
		})
	})
	requirePanic(t, func() {
		top.InvalidatePathKeySubtree(pathKey)
	})
}

func TestStatePackageDoesNotImportLuaPackages(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", ".")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps . failed: %v", err)
	}
	banned := []string{
		"github.com/wippyai/go-lua/__old",
		"github.com/wippyai/go-lua/analysis/engine/visibility",
		"github.com/wippyai/go-lua/analysis/ir/cfg",
		"github.com/wippyai/go-lua/analysis/lua",
		"github.com/wippyai/go-lua/compiler",
		"github.com/wippyai/go-lua/compiler/ast",
		"go/ast",
	}
	for _, dep := range strings.Fields(string(out)) {
		for _, prefix := range banned {
			if dep == prefix || strings.HasPrefix(dep, prefix+"/") {
				t.Fatalf("state package imports forbidden dependency %q", dep)
			}
		}
	}
}

func requirePanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}

func presentValue(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Present())
}

func absentValue(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Absent())
}

func formatValue(reg *axis.Registry, v product.Value) string {
	switch {
	case product.Equal(reg, v, product.Bottom(reg)):
		return "bottom"
	case product.Equal(reg, v, product.Top()):
		return "top"
	case presence.Equal(product.PresenceOf(v), presence.Present()):
		return "present"
	case presence.Equal(product.PresenceOf(v), presence.Absent()):
		return "absent"
	default:
		return product.PresenceOf(v).String()
	}
}

func formatState(reg *axis.Registry, s State) string {
	return "value-slot=" + formatValue(reg, s.ReadValue(reg, key.SymbolValue(21))) +
		" return-slot=" + formatValue(reg, s.ReadValue(reg, key.ReturnSlot(0))) +
		" path=" + formatValue(reg, s.ReadPathKey(reg, pathdom.PathKey("sym21@2.field"))) +
		" other-path=" + formatValue(reg, s.ReadPathKey(reg, pathdom.PathKey("$0.item")))
}
