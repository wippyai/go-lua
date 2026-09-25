package typ

import "testing"

func TestUnresolvedSurvivesUnionUntilFinalization(t *testing.T) {
	for _, other := range []Type{Any, Unknown, String} {
		got, ok := NewUnion(Unresolved, other).(*Union)
		if !ok || !got.Contains(Unresolved) || !got.Contains(other) {
			t.Fatalf("unresolved alternative lost alongside %v: %v", other, got)
		}
		if IsFinal(got) {
			t.Fatalf("union with unresolved is final: %v", got)
		}
		if pruned, ok := PruneSoftUnionMembers(got).(*Union); !ok || !pruned.Contains(Unresolved) || !pruned.Contains(other) {
			t.Fatalf("soft pruning lost a runtime alternative alongside unresolved: %v", pruned)
		}
	}
	if got := Finalize(NewUnion(Unresolved, String)); !TypeEquals(got, Unknown) {
		t.Fatalf("pending union path must remain uncertain at final phase: %v", got)
	}
}

func TestResolvePreservesDistinctPendingPath(t *testing.T) {
	current := NewUnion(Unresolved, LiteralInt(1))
	evidence := NewUnion(Unresolved, LiteralInt(2))
	got := Resolve(current, evidence)
	if u, ok := got.(*Union); !ok || !u.Contains(Unresolved) || !u.Contains(LiteralInt(1)) {
		t.Fatalf("resolve closed a distinct pending path: %v", got)
	}
	old := NewMap(Unresolved, NewArray(Unresolved))
	newer := NewMap(String, NewArray(Number))
	if got := Resolve(old, newer); !TypeEquals(got, newer) {
		t.Fatalf("resolve failed to fill corresponding map slots: %v", got)
	}
}

func TestDynamicTypesAreFinal(t *testing.T) {
	for _, tpe := range []Type{Any, Unknown, NewArray(Any), NewMap(Unknown, Any)} {
		if !IsFinal(tpe) {
			t.Fatalf("runtime dynamic type is pending: %v", tpe)
		}
	}
}

func TestFinalizeMissingRecursiveBody(t *testing.T) {
	if got := Finalize(NewRecursivePlaceholder("return")); !TypeEquals(got, Unknown) {
		t.Fatalf("unfinished recursive return escaped finalization: %v", got)
	}
}
