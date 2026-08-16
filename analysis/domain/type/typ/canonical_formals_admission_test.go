package typ

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestCanonicalFormalsAdmissionUsesRepresentabilityNotSemanticCaps(t *testing.T) {
	// These metadata-only requests cross every former admission threshold. No
	// backing storage is created: the law verifies that admission is now solely
	// a checked representability gate, not a hidden semantic budget.
	for _, test := range []struct {
		name                string
		count, elementBytes int64
	}{
		{"raw bytes", 16<<20 + 1, 1},
		{"definitions", 1<<18 + 1, canonicalFormalsNodeBytes},
		{"edges", 1<<20 + 1, canonicalFormalsIntBytes},
		{"scalar bytes", 8<<20 + 1, 1},
		{"output bytes", 16<<20 + 1, 1},
		{"derived work", 2<<30 + 1, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.count > int64(maxInt()) || test.elementBytes > int64(maxInt()) {
				t.Skip("platform cannot represent this metadata")
			}
			admission, err := newCanonicalFormalsAdmission(context.Background(), int(test.count))
			if err != nil {
				t.Fatalf("representable artifact length rejected: %v", err)
			}
			if err := admission.reserve(int(test.count), int(test.elementBytes)); err != nil {
				t.Fatalf("representable allocation rejected: %v", err)
			}
		})
	}

	admission, err := newCanonicalFormalsAdmission(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := admission.reserve(maxInt(), 2); !errors.Is(err, ErrInvalidCanonicalType) {
		t.Fatalf("overflowing allocation = %v", err)
	}
	if _, ok := canonicalFormalsAllocationBytes(maxInt(), 2); ok {
		t.Fatal("overflowing allocation reported representable")
	}
	if !canonicalFormalsCapacityExceeds(maxInt(), 2, maxInt()) {
		t.Fatal("overflowing retained capacity reported within threshold")
	}
	if canonicalFormalsCapacityExceeds(maxInt()/2, 2, maxInt()) {
		t.Fatal("representable retained capacity reported above threshold")
	}
}

func TestCanonicalFormalsOwnedMaterializersPreserveCanonicalSemantics(t *testing.T) {
	formal := NewTypeParam("T", String)
	generic := NewGeneric("Box", []*TypeParam{formal}, NewArray(formal))
	functionFormal := NewTypeParam("U", String)
	function := Func().TypeParamRef(functionFormal).Param("self", functionFormal).Returns(String, Integer).Build()
	record := RebuildRecord(RecordParts{
		Fields:        []Field{{Name: "value", Type: Instantiate(generic, String)}},
		StaticMembers: []StaticMember{{Kind: StaticMemberStringIndex, Name: "read", Type: function}},
		MapKey:        String,
		MapValue:      Integer,
		AssumeSorted:  true,
	})
	external := NewTypeParam("E", String)
	values := []struct {
		value   Type
		formals []*TypeParam
	}{
		{NewTuple(external, String), []*TypeParam{external}},
		{MaterializeUnion([]Type{String, Integer}), nil},
		{MaterializeIntersection([]Type{String, Integer}), nil},
		{record, nil},
		{function, nil},
		{generic, nil},
		{Instantiate(generic, Number), nil},
		{NewInterface("Reader", []Method{{Name: "read", Type: function}}), nil},
	}
	for _, test := range values {
		value := test.value
		encoded, err := EncodeCanonicalFormals(context.Background(), value, test.formals)
		if err != nil {
			t.Fatalf("EncodeCanonicalFormals(%T): %v", value, err)
		}
		var receiver []*TypeParam
		if len(test.formals) != 0 {
			receiver = []*TypeParam{NewTypeParam("U", String)}
		}
		decoded, err := DecodeCanonicalFormals(context.Background(), encoded, receiver)
		if err != nil {
			t.Fatalf("DecodeCanonicalFormals(%T): %v", value, err)
		}
		roundTrip, err := EncodeCanonicalFormals(context.Background(), decoded, receiver)
		if err != nil {
			t.Fatalf("re-encode(%T): %v", value, err)
		}
		if !bytes.Equal(encoded, roundTrip) {
			t.Fatalf("owned materializer changed %T:\n%x\n%x", value, encoded, roundTrip)
		}
	}
}

func TestCanonicalFormalsOwnedMaterializersObserveCancellationBeforeAllocation(t *testing.T) {
	ctx := &canonicalFormalCancelAfterContext{Context: context.Background(), allowedErrCalls: 1, done: make(chan struct{})}
	admission, err := newCanonicalFormalsAdmission(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	children := make([]Type, 512)
	for index := range children {
		children[index] = String
	}
	var steps uint64
	if _, err := materializeCanonicalFormalsVariableNode(ctx, admission, appendCount([]byte{canonicalTuple}, len(children)), canonicalFormalNodeShape{tag: canonicalTuple, children: uint64(len(children))}, children, &steps); !errors.Is(err, context.Canceled) {
		t.Fatalf("tuple materializer cancellation = %v", err)
	}
}
