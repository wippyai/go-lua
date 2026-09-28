package typ

import (
	"testing"

	"github.com/wippyai/go-lua/types/kind"
)

type countedFinalLeaf struct{ visits int }

func (l *countedFinalLeaf) Kind() kind.Kind        { l.visits++; return kind.String }
func (*countedFinalLeaf) String() string           { return "string" }
func (*countedFinalLeaf) Hash() uint64             { return 1 }
func (l *countedFinalLeaf) Equals(other Type) bool { return l == other }

func TestIsFinalSharedTypeGraphVisitsEachNodeOnce(t *testing.T) {
	leaf := &countedFinalLeaf{}
	var graph Type = leaf
	for range 18 {
		graph = &Tuple{Elements: []Type{graph, graph}}
	}
	if !IsFinal(graph) {
		t.Fatal("shared final graph reported pending")
	}
	if leaf.visits > 2 {
		t.Fatalf("shared leaf visited %d times; want at most one traversal", leaf.visits)
	}
}

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

func TestResolveSupersedesPendingUnionWithWholeEvidence(t *testing.T) {
	current := NewUnion(Unresolved, NewOptional(Number))
	if got := Resolve(current, Number); !TypeEquals(got, Number) {
		t.Fatalf("pending union position kept against final evidence: %v", got)
	}
	record := NewRecord().Field("timeout", current).Build()
	evidence := NewRecord().Field("timeout", Number).Build()
	if got := Resolve(record, evidence); !TypeEquals(got, evidence) {
		t.Fatalf("pending record field kept against final evidence: %v", got)
	}
	if got := Resolve(NewRecord().Field("timeout", String).Field("x", Unresolved).Build(), evidence); !TypeEquals(got.(*Record).GetField("timeout").Type, String) {
		t.Fatalf("final leaf replaced by evidence: %v", got)
	}
}

func TestDropPendingAlternatives(t *testing.T) {
	if got := DropPendingAlternatives(NewUnion(Unresolved, String)); !TypeEquals(got, String) {
		t.Fatalf("pending alternative kept: %v", got)
	}
	nested := NewRecord().Field("k", NewUnion(Unresolved, Number)).Build()
	want := NewRecord().Field("k", Number).Build()
	if got := DropPendingAlternatives(nested); !TypeEquals(got, want) {
		t.Fatalf("nested pending alternative kept: %v", got)
	}
	if got := DropPendingAlternatives(Unresolved); got != nil {
		t.Fatalf("wholly pending type has no evidence to keep: %v", got)
	}
	hole := NewRecord().Field("k", Unresolved).Build()
	if got := DropPendingAlternatives(hole); !TypeEquals(got, hole) {
		t.Fatalf("pending position without alternatives changed: %v", got)
	}
}

func TestPartialViewForgetsComplete(t *testing.T) {
	inner := NewRecord().SetOpen(true).SetComplete(true).Build()
	outer := NewRecord().Field("config", inner).SetOpen(true).SetComplete(true).Build()
	shallow := PartialView(outer).(*Record)
	if shallow.Complete || !shallow.GetField("config").Type.(*Record).Complete {
		t.Fatalf("shallow partial view = %v", shallow)
	}
	deep := PartialViewDeep(outer).(*Record)
	if deep.Complete || deep.GetField("config").Type.(*Record).Complete {
		t.Fatalf("deep partial view kept a complete record: %v", deep)
	}
}
