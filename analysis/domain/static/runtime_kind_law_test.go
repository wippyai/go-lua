package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestRuntimeTypeOfExhaustiveKindSetAlgebra(t *testing.T) {
	_, _, authority := sealedStatic(t, "local subject = 1\ntype Snapshot = typeof(subject)\n")
	for raw := 0; raw < 256; raw++ {
		got, ok := authority.RuntimeTypeOf(runtimekind.Set(raw))
		if !ok || !authority.Owns(got) {
			t.Fatalf("RuntimeTypeOf(%#x) unavailable", raw)
		}
		if raw == 0 {
			if !authority.Equal(got, authority.Bottom()) {
				t.Fatal("empty KindSet did not map to Static Bottom")
			}
			continue
		}
		if authority.Equal(got, authority.Top()) || !got.IsClosed() {
			t.Fatalf("nonempty KindSet %#x mapped to non-closed or technical Top", raw)
		}
		closed, decoded := authority.ClosedType(got)
		if !decoded {
			t.Fatalf("KindSet %#x did not retain a closed type", raw)
		}
		if runtimekind.Set(raw)&(runtimekind.Bit(runtimekind.Thread)|runtimekind.Bit(runtimekind.Userdata)) != 0 {
			if !typ.IsUnknown(closed) {
				t.Fatalf("unsupported KindSet %#x = %v, want exact Unknown", raw, closed)
			}
		}
	}
	if _, ok := authority.RuntimeTypeOf(runtimekind.Set(1 << 8)); ok {
		t.Fatal("high KindSet bit entered closed runtime-kind denominator")
	}
	for left := 0; left < 256; left++ {
		leftValue, _ := authority.RuntimeTypeOf(runtimekind.Set(left))
		for right := 0; right < 256; right++ {
			rightValue, _ := authority.RuntimeTypeOf(runtimekind.Set(right))
			if left&^right == 0 && !authority.LessOrEq(leftValue, rightValue) {
				t.Fatalf("KindSet monotonicity failed: %#x <= %#x", left, right)
			}
			unionValue, _ := authority.RuntimeTypeOf(runtimekind.Set(left | right))
			if !authority.Equal(authority.Join(leftValue, rightValue), unionValue) {
				t.Fatalf("KindSet join homomorphism failed: %#x join %#x", left, right)
			}
		}
	}
}

func TestRuntimeTypeOfSingletonsUseCanonicalStaticVocabulary(t *testing.T) {
	_, _, authority := sealedStatic(t, "local subject = 1\ntype Snapshot = typeof(subject)\n")
	for _, law := range []struct {
		mask int
		want typ.Type
	}{
		{int(runtimekind.Bit(runtimekind.Nil)), typ.Nil},
		{int(runtimekind.Bit(runtimekind.Boolean)), typ.Boolean},
		{int(runtimekind.Bit(runtimekind.Number)), typ.Number},
		{int(runtimekind.Bit(runtimekind.String)), typ.String},
		{int(runtimekind.Bit(runtimekind.Table)), typ.BuiltinTableTopMarker()},
		{int(runtimekind.Bit(runtimekind.Function)), mustBuiltinFunction(t)},
	} {
		got, ok := authority.RuntimeTypeOf(runtimekind.Set(law.mask))
		if !ok {
			t.Fatalf("RuntimeTypeOf(%#x) unavailable", law.mask)
		}
		actual, ok := authority.ClosedType(got)
		if !ok || !typ.TypeEquals(actual, law.want) {
			t.Fatalf("RuntimeTypeOf(%#x) = %v/%v, want %v", law.mask, actual, ok, law.want)
		}
	}
	if allocations := testing.AllocsPerRun(1_000, func() {
		if _, ok := authority.RuntimeTypeOf(runtimekind.Bit(runtimekind.Number) | runtimekind.Bit(runtimekind.String)); !ok {
			t.Fatal("RuntimeTypeOf stopped returning a sealed result")
		}
	}); allocations != 0 {
		t.Fatalf("RuntimeTypeOf allocations = %v, want 0", allocations)
	}
}

func TestRuntimeTypeOfIsReplayAndModulePermutationStable(t *testing.T) {
	first, err := lower.Lower(lower.Source{Name: "runtime_kind_first.lua", Text: []byte("local first = 1\ntype First = typeof(first)\n")})
	if err != nil {
		t.Fatal(err)
	}
	second, err := lower.Lower(lower.Source{Name: "runtime_kind_second.lua", Text: []byte("local second = 'x'\ntype Second = typeof(second)\n")})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	seal := func(modules []linkproject.Module) *Authority {
		source, err := link.Seal(&link.Spec{Target: contract, Modules: modules})
		if err != nil {
			t.Fatal(err)
		}
		types, ok := typeauthority.Seal(source)
		if !ok {
			t.Fatal("type authority")
		}
		authority, _, err := Seal(source, types)
		if err != nil {
			t.Fatal(err)
		}
		return authority
	}
	forward := seal([]linkproject.Module{{Name: "first", Program: first}, {Name: "second", Program: second}})
	reversed := seal([]linkproject.Module{{Name: "second", Program: second}, {Name: "first", Program: first}})
	if forward.ContentID() != reversed.ContentID() {
		t.Fatal("RuntimeTypeOf table changed Static identity under module permutation")
	}
	for mask := 0; mask < 256; mask++ {
		left, leftOK := forward.RuntimeTypeOf(runtimekind.Set(mask))
		right, rightOK := reversed.RuntimeTypeOf(runtimekind.Set(mask))
		if !leftOK || !rightOK || forward.Fingerprint(left) != reversed.Fingerprint(right) {
			t.Fatalf("RuntimeTypeOf(%#x) changed replay identity", mask)
		}
		if left.IsClosed() {
			leftType, leftDecoded := forward.ClosedType(left)
			rightType, rightDecoded := reversed.ClosedType(right)
			if !leftDecoded || !rightDecoded || !typ.TypeEquals(leftType, rightType) {
				t.Fatalf("RuntimeTypeOf(%#x) changed replay type", mask)
			}
		}
	}
}

func mustBuiltinFunction(t testing.TB) typ.Type {
	t.Helper()
	value, ok := typ.BuiltinPrimitiveType("function")
	if !ok {
		t.Fatal("builtin function")
	}
	return value
}
