package typ

import "testing"

func TestUnwrapStructuralWrappers(t *testing.T) {
	base := NewArray(String)
	alias := &Alias{Name: "A", Target: &Annotated{Inner: &Alias{Name: "B", Target: base}}}
	wrapped := &Annotated{Inner: alias}

	if got := UnwrapStructuralWrappers(wrapped); got != base {
		t.Fatalf("structural wrapper result = %T %p, want base %p", got, got, base)
	}
	if got := UnwrapStructuralWrappers(base); got != base {
		t.Fatal("unwrapped type changed identity")
	}
	if got := UnwrapStructuralWrappers(nil); got != nil {
		t.Fatalf("nil structural wrapper result = %T, want nil", got)
	}
	var typedNil *Annotated
	if got := UnwrapStructuralWrappers(typedNil); got != typedNil {
		t.Fatalf("typed-nil structural wrapper result = %T, want typed nil", got)
	}
	typedNilAlias := &Alias{Name: "typed-nil", Target: typedNil}
	if got := UnwrapStructuralWrappers(typedNilAlias); got != typedNil {
		t.Fatalf("typed-nil Alias target result = %T, want typed nil", got)
	}
}

func TestUnwrapStructuralWrappersMalformedAndCycles(t *testing.T) {
	nilAlias := &Alias{Name: "nil"}
	if got := UnwrapStructuralWrappers(nilAlias); got != nilAlias {
		t.Fatal("nil-target Alias did not remain its own finite structural view")
	}
	nilAnnotation := &Annotated{}
	if got := UnwrapStructuralWrappers(nilAnnotation); got != nilAnnotation {
		t.Fatal("nil-inner Annotated did not remain its own finite structural view")
	}

	selfAlias := &Alias{Name: "self"}
	selfAlias.Target = selfAlias
	if got := UnwrapStructuralWrappers(selfAlias); got != selfAlias {
		t.Fatal("self Alias did not terminate at its cycle entry")
	}
	selfAnnotation := &Annotated{}
	selfAnnotation.Inner = selfAnnotation
	if got := UnwrapStructuralWrappers(selfAnnotation); got != selfAnnotation {
		t.Fatal("self Annotated did not terminate at its cycle entry")
	}
	leftAnnotation := &Annotated{}
	rightAnnotation := &Annotated{}
	leftAnnotation.Inner = rightAnnotation
	rightAnnotation.Inner = leftAnnotation
	if got := UnwrapStructuralWrappers(leftAnnotation); got != leftAnnotation {
		t.Fatalf("Annotated cycle result = %p, want entry %p", got, leftAnnotation)
	}

	left := &Alias{Name: "left"}
	right := &Alias{Name: "right"}
	left.Target = &Annotated{Inner: right}
	right.Target = &Annotated{Inner: left}
	if got := UnwrapStructuralWrappers(left); got != left {
		t.Fatalf("mixed wrapper cycle result = %p, want entry %p", got, left)
	}
	if got := UnwrapStructuralWrappers(right); got != right {
		t.Fatalf("reverse mixed wrapper cycle result = %p, want entry %p", got, right)
	}
}

func TestUnwrapStructuralWrappersShallowPathDoesNotAllocate(t *testing.T) {
	base := NewMap(String, Number)
	wrapper := &Annotated{Inner: &Alias{Name: "A", Target: &Annotated{Inner: base}}}
	if got := UnwrapStructuralWrappers(wrapper); got != base {
		t.Fatal("warmup structural unwrap changed identity")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		if UnwrapStructuralWrappers(wrapper) != base {
			panic("structural unwrap changed identity")
		}
	}); allocations != 0 {
		t.Fatalf("shallow structural unwrap allocations = %v, want 0", allocations)
	}
}
