package value

import (
	"testing"

	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

func TestAdmitObservation_RecordDiscriminantPreservesPayloadLiterals(t *testing.T) {
	event := typ.NewRecord().
		Field("kind", typ.LiteralString("tool_call")).
		Field("name", typ.LiteralString("search")).
		Field("items", typ.NewArray(typ.LiteralInt(1))).
		Build()

	got := AdmitObservation(event)
	want := typ.NewRecord().
		Field("kind", typ.LiteralString("tool_call")).
		Field("name", typ.LiteralString("search")).
		Field("items", typ.NewArray(typ.Integer)).
		Build()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("AdmitObservation(discriminated record) = %v, want %v", got, want)
	}
}

func TestAdmitObservation_RecordDataFieldLiteralsWiden(t *testing.T) {
	cfg := typ.NewRecord().
		Field("default_temperature", typ.LiteralNumber(0.8)).
		Build()

	got := AdmitObservation(cfg)
	want := typ.NewRecord().
		Field("default_temperature", typ.Number).
		Build()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("AdmitObservation(config record) = %v, want %v", got, want)
	}
}

func TestAdmitObservation_PreservesFalseStructuralSentinel(t *testing.T) {
	meta := typ.NewRecord().Field("type", typ.String).Build()
	got := AdmitObservation(typ.NewUnion(meta, typ.False))
	want := typ.NewUnion(meta, typ.False)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("AdmitObservation(record|false) = %v, want %v", got, want)
	}
}

func TestAdmitObservation_ReusesSharedStructuralRewrite(t *testing.T) {
	payload := typ.NewRecord().
		Field("value", typ.LiteralInt(1)).
		Build()
	input := typ.NewRecord().
		Field("left", payload).
		Field("right", payload).
		Build()

	got := AdmitObservation(input)
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("expected record, got %T", got)
	}
	left := rec.GetField("left")
	right := rec.GetField("right")
	if left == nil || right == nil {
		t.Fatalf("expected left/right fields, got %v", rec)
	}
	if left.Type != right.Type {
		t.Fatalf("expected shared payload rewrite to reuse the widened node, got %p and %p", left.Type, right.Type)
	}

	wantPayload := typ.NewRecord().
		Field("value", typ.Integer).
		Build()
	if !typ.TypeEquals(left.Type, wantPayload) {
		t.Fatalf("shared payload widened to %v, want %v", left.Type, wantPayload)
	}
}

func TestAdmitObservation_FoldsSelfEmbeddingRecordWithoutDepthCap(t *testing.T) {
	value := typ.NewRecord().
		Field("payload", typ.LiteralString("leaf")).
		Build()
	for i := 0; i < typ.DefaultRecursionDepth+8; i++ {
		value = typ.NewRecord().
			Field("payload", typ.LiteralString("node")).
			Field("next", value).
			Build()
	}

	got := AdmitObservation(value)
	rec, ok := got.(*typ.Recursive)
	if !ok {
		t.Fatalf("AdmitObservation(self-embedding record) = %T, want recursive", got)
	}
	body, ok := rec.Body.(*typ.Record)
	if !ok {
		t.Fatalf("recursive body = %T, want record", rec.Body)
	}
	payload := body.GetField("payload")
	if payload == nil || payload.Type != typ.String {
		t.Fatalf("payload = %v, want widened string", payload)
	}
	next := body.GetField("next")
	if next == nil || !typ.IsRecursiveRef(next.Type, rec) {
		t.Fatalf("next = %v, want recursive self reference", next)
	}
}

func TestJoinObservations_FirstRecursiveUnionObservationFolds(t *testing.T) {
	value := typ.NewRecord().
		Field("payload", typ.LiteralString("leaf")).
		Build()
	for i := 0; i < typ.DefaultRecursionDepth+4; i++ {
		value = typ.NewRecord().
			Field("payload", typ.LiteralString("node")).
			Field("next", value).
			Build()
	}
	observation := typ.NewUnion(value, typ.False)

	got := JoinObservations(nil, observation)
	u, ok := got.(*typ.Union)
	if !ok {
		t.Fatalf("JoinObservations(nil, recursive|false) = %T, want union", got)
	}
	hasFalse := false
	hasRecursive := false
	for _, member := range u.Members {
		if member == typ.False {
			hasFalse = true
		}
		if _, ok := member.(*typ.Recursive); ok {
			hasRecursive = true
		}
	}
	if !hasFalse || !hasRecursive {
		t.Fatalf("JoinObservations(nil, recursive|false) = %v, want false plus recursive member", got)
	}
	if again := JoinObservations(got, observation); !typ.TypeEquals(again, got) {
		t.Fatalf("JoinObservations must be idempotent after folding:\nfirst=%v\nagain=%v", got, again)
	}
}

func TestJoinObservations_RefinesSoftStructuralPlaceholder(t *testing.T) {
	entry := typ.NewRecord().Field("id", typ.String).Build()
	soft := typ.NewMap(typ.String, typ.NewArray(typ.Any))
	precise := typ.NewMap(typ.String, typ.NewArray(entry))

	got := JoinObservations(soft, precise)
	if !typ.TypeEquals(got, precise) {
		t.Fatalf("JoinObservations(soft map, precise map) = %v, want %v", got, precise)
	}
}

func TestFoldSelfEmbedding_UsesShapeBeforeDeepEquality(t *testing.T) {
	anchor := typ.NewRecord().Field("next", panicEqualsType{}).Build()
	nested := typ.NewRecord().Field("next", panicEqualsType{}).Build()
	observation := typ.NewRecord().Field("next", nested).Build()

	if _, ok := FoldSelfEmbedding(anchor, observation); !ok {
		t.Fatal("structural self-embedding anchor should be recognized by shape")
	}
}

type panicEqualsType struct{}

func (panicEqualsType) Kind() kind.Kind { return kind.String }
func (panicEqualsType) String() string  { return "panic-equals" }
func (panicEqualsType) Hash() uint64    { return 0x70616e6963657101 }
func (panicEqualsType) Equals(typ.Type) bool {
	panic("deep equality should not be used for self-embedding shape")
}
