package typecontract

import (
	"context"
	"testing"

	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
)

func TestPrimitiveAndScopedGraphRoundTrip(t *testing.T) {
	ctx := context.Background()
	for _, value := range []typ.Type{typ.Nil, typ.Any, typ.Never} {
		portable, err := Encode(ctx, value, nil)
		if err != nil {
			t.Fatalf("Encode(%v): %v", value, err)
		}
		if _, ok := portable.Primitive(); !ok {
			t.Fatalf("Encode(%v) did not use primitive envelope", value)
		}
		decoded, err := Decode(ctx, portable, nil)
		if err != nil || !typ.TypeEquals(decoded, value) {
			t.Fatalf("Decode(%v) = %v/%v, want %v", value, decoded, err, value)
		}
	}

	formal := typ.NewTypeParam("T", nil)
	portable, err := Encode(ctx, typ.NewArray(formal), []*typ.TypeParam{formal})
	if err != nil {
		t.Fatalf("Encode scoped graph: %v", err)
	}
	receiver := typ.NewTypeParam("U", nil)
	decoded, err := Decode(ctx, portable, []*typ.TypeParam{receiver})
	if err != nil {
		t.Fatalf("Decode scoped graph: %v", err)
	}
	if !typ.TypeEquals(decoded, typ.NewArray(receiver)) {
		t.Fatalf("decoded scoped graph = %v, want array(U)", decoded)
	}
}

func TestNeutralSemanticsRejectsInvalidRelationAndHonorsClosedScope(t *testing.T) {
	semantics := NewSemantics()
	closedPrimitive, err := Encode(context.Background(), typ.Any, nil)
	if err != nil {
		t.Fatalf("Encode closed primitive: %v", err)
	}
	closedGraph, err := EncodeStorage(context.Background(), typ.NewArray(typ.String), nil)
	if err != nil {
		t.Fatalf("Encode closed graph: %v", err)
	}
	formalScope := []schematype.Type{{}}
	for name, value := range map[string]schematype.Type{"primitive": closedPrimitive, "graph": closedGraph} {
		if err := semantics.Validate(value, formalScope); err != nil {
			t.Fatalf("Validate closed %s under outer scope: %v", name, err)
		}
	}

	invalid, ok := schematype.NewEncoded([]byte("not-a-canonical-type"), 0)
	if !ok {
		t.Fatal("invalid test envelope was unavailable")
	}
	if _, err := semantics.Assignable(invalid, closedPrimitive, nil); err == nil {
		t.Fatal("invalid decode was reported as a valid relation")
	}
	stringType, err := Encode(context.Background(), typ.String, nil)
	if err != nil {
		t.Fatalf("Encode string: %v", err)
	}
	integerType, err := Encode(context.Background(), typ.Integer, nil)
	if err != nil {
		t.Fatalf("Encode integer: %v", err)
	}
	rejected, err := semantics.Assignable(stringType, integerType, nil)
	if err != nil {
		t.Fatalf("valid rejected relation returned error: %v", err)
	}
	if rejected {
		t.Fatal("string-to-integer relation was unexpectedly accepted")
	}
}

func TestAuthoringAdmissionAndAssignmentStayDomainOwned(t *testing.T) {
	if err := ValidateAuthoring(context.Background(), typ.NewRef("m", "T"), nil); err == nil {
		t.Fatal("unresolved reference was admitted")
	}
	if !Assignable(typ.Integer, typ.Number) || !Assignable(typ.Never, typ.String) {
		t.Fatal("expected subtype assignment law")
	}
	if Assignable(typ.Any, typ.Never) {
		t.Fatal("Any assigned to Never")
	}

	fn := typ.Func().Returns(typ.String).Build()
	callableRecord := table.NewRecord().
		Metatable(table.NewRecord().Field("__call", fn).Build()).
		Build()
	if !Admits(fn, DirectFunction) || Admits(callableRecord, DirectFunction) {
		t.Fatal("direct-function admission confused ordinary callable records")
	}
	if !Admits(callableRecord, OrdinaryCallable) {
		t.Fatal("ordinary callable record was rejected")
	}
	if !FreshCompatible(table.NewRecord().Build(), FreshTable) || !FreshCompatible(fn, FreshFunction) {
		t.Fatal("fresh runtime shape admission rejected its canonical shape")
	}
}
